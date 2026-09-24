package org

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
)

// EtimsStatus values (orgs.etims_status, ADR-0009). Progressive onboarding:
// an org starts "unconfigured" and stays that way until an owner submits
// their KRA branch id and device serial through ConfigureEtims; C2B
// payments are ledgered throughout, just held out of KRA submission
// (internal/ledger.Service.CreateInvoiceForSale/ActivateTaxPending).
const (
	EtimsUnconfigured = "unconfigured"
	EtimsInitialized  = "initialized"
	EtimsFailed       = "failed"
)

var bhfIDRe = regexp.MustCompile(`^[0-9]{2}$`)

// Errors from ConfigureEtims's own validation (distinct from the fiscal
// provider's, which pass through unwrapped so the handler can classify them
// with fiscal.Classify).
var (
	ErrEtimsBadBranch      = errors.New("org: kra_bhf_id must be two digits, e.g. 00")
	ErrEtimsSerialRequired = errors.New("org: kra_device_serial is required")
	ErrNoFiscalProvider    = errors.New("org: no fiscal provider configured for this deployment")
)

// EtimsSettings is the merchant-facing view of an org's direct KRA OSCU
// configuration (Tax Settings screen).
type EtimsSettings struct {
	Status        string     `json:"status"`
	BhfID         *string    `json:"kra_bhf_id"`
	DeviceSerial  *string    `json:"kra_device_serial"`
	FailedReason  *string    `json:"failed_reason"`
	InitializedAt *time.Time `json:"initialized_at"`
}

func toEtimsSettings(o gen.Org) EtimsSettings {
	return EtimsSettings{Status: o.EtimsStatus, BhfID: o.KraBhfID, DeviceSerial: o.KraDeviceSerial, FailedReason: o.EtimsFailedReason, InitializedAt: o.EtimsInitializedAt}
}

// GetEtimsSettings returns the org's current eTIMS configuration.
func (s *Service) GetEtimsSettings(ctx context.Context, orgID uuid.UUID) (EtimsSettings, error) {
	var o gen.Org
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		var err error
		o, err = tx.GetOrg(ctx, orgID)
		return err
	})
	if err != nil {
		return EtimsSettings{}, err
	}
	return toEtimsSettings(o), nil
}

// ConfigureEtims runs KRA's direct-OSCU device initialisation
// (fiscal.Provider.RegisterDevice) for the org's own taxpayer PIN, branch id
// and device serial, and persists the result. On success etims_status
// becomes "initialized"; the caller is responsible for then activating any
// TAX_PENDING invoices (internal/ledger.Service.ActivateTaxPending) — kept
// out of this method so org has no dependency on ledger.
func (s *Service) ConfigureEtims(ctx context.Context, orgID uuid.UUID, bhfID, deviceSerial string) (EtimsSettings, error) {
	if s.Fiscal == nil {
		return EtimsSettings{}, ErrNoFiscalProvider
	}
	bhfID = strings.TrimSpace(bhfID)
	if bhfID == "" {
		bhfID = "00"
	}
	if !bhfIDRe.MatchString(bhfID) {
		return EtimsSettings{}, ErrEtimsBadBranch
	}
	deviceSerial = strings.TrimSpace(deviceSerial)
	if deviceSerial == "" {
		return EtimsSettings{}, ErrEtimsSerialRequired
	}

	var o gen.Org
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		var err error
		o, err = tx.GetOrg(ctx, orgID)
		return err
	})
	if err != nil {
		return EtimsSettings{}, err
	}
	pin, err := s.Keys.DecryptString(o.KraPinEnc)
	if err != nil {
		return EtimsSettings{}, fmt.Errorf("org: could not read the org's KRA PIN: %w", err)
	}

	ref, rErr := s.Fiscal.RegisterDevice(ctx, fiscal.OrgFiscalProfile{
		OrgID: orgID.String(), Name: o.Name, KRAPIN: pin, VATRegistered: o.VatRegistered,
		BranchID: bhfID, DeviceSerial: deviceSerial,
	})
	if rErr != nil {
		reason := rErr.Error()
		var ve *fiscal.ValidationError
		if errors.As(rErr, &ve) && ve.Message != "" {
			reason = ve.Message
		}
		reason = truncateReason(reason)
		_ = s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
			var err error
			o, err = tx.SetEtimsFailed(ctx, gen.SetEtimsFailedParams{ID: orgID, EtimsFailedReason: &reason})
			return err
		})
		s.Log.Warn("etims configuration failed", "org", orgID, "err", rErr)
		return toEtimsSettings(o), rErr
	}

	var cmcEnc []byte
	if cmcKey := decodeCmcKey(ref.Raw); cmcKey != "" {
		if cmcEnc, err = s.Keys.EncryptString(cmcKey); err != nil {
			return EtimsSettings{}, err
		}
	}
	legalName := decodeTaxpayerName(ref.Raw)

	err = s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		var err error
		o, err = tx.SetEtimsConfigured(ctx, gen.SetEtimsConfiguredParams{
			ID: orgID, KraBhfID: &bhfID, KraDeviceSerial: &deviceSerial, KraCmcKeyEnc: cmcEnc, FiscalProfile: ref.Raw,
		})
		if err != nil {
			return err
		}
		// If KRA returned the registered legal name, overwrite the organization's official name
		if legalName != "" && legalName != o.Name {
			if _, err := tx.Tx.Exec(ctx, "UPDATE orgs SET name = $1 WHERE id = $2", legalName, orgID); err == nil {
				o.Name = legalName
			}
		}
		actor := orgID.String()
		return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: orgID, ActorType: "user", ActorID: &actor, Action: "org.etims_configured", Entity: "org", EntityID: orgID.String()})
	})
	if err != nil {
		return EtimsSettings{}, err
	}
	s.Log.Info("etims configured", "org", orgID, "adapter", s.Fiscal.Name())
	return toEtimsSettings(o), nil
}

func decodeCmcKey(raw json.RawMessage) string {
	var v struct {
		CmcKey string `json:"cmc_key"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v.CmcKey
}

func truncateReason(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func decodeTaxpayerName(raw json.RawMessage) string {
	var v struct {
		TaxpayerName string `json:"taxpayer_name"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return strings.TrimSpace(v.TaxpayerName)
}
