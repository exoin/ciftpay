package org

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	plog "github.com/exoin/ciftpay/internal/platform/log"
)

// OTPSender delivers login codes (notify.Service in production, a logger locally).
type OTPSender interface {
	SendOTP(ctx context.Context, msisdn, code, locale string) error
}

// PINChecker answers "does this KRA PIN exist". nil means KRA knows the PIN;
// ErrPINUnknown means it does not (onboarding stops with 422 pin_unknown); any
// other error means the checker could not be asked and the org is created
// with kra_pin_verified_at NULL. FiscalPINChecker asks the fiscal provider,
// FormatOnlyPINChecker never contacts anyone.
type PINChecker interface {
	CheckPIN(ctx context.Context, pin string) error
}

// Service implements auth and organisation use cases.
type Service struct {
	DB            *db.DB
	Keys          *crypto.Keyring
	OTP           OTPSender
	PIN           PINChecker
	SessionSecret []byte
	FiscalAdapter string
	// Fiscal is the active fiscal.Provider, used only by ConfigureEtims
	// (Tax Settings, ADR-0009). It is set directly by the composition root
	// (cmd/api, cmd/worker) rather than threaded through New, since most
	// callers (including every existing test) never need it.
	Fiscal fiscal.Provider
	// DevLogOTP logs the code instead of trusting SMS delivery (APP_ENV=local).
	DevLogOTP bool
	Log       *slog.Logger
	Now       func() time.Time
}

// New builds a Service.
func New(d *db.DB, k *crypto.Keyring, otp OTPSender, pin PINChecker, sessionSecret, fiscalAdapter string, devLogOTP bool, l *slog.Logger) *Service {
	if pin == nil {
		pin = FormatOnlyPINChecker{}
	}
	return &Service{DB: d, Keys: k, OTP: otp, PIN: pin, SessionSecret: []byte(sessionSecret), FiscalAdapter: fiscalAdapter, DevLogOTP: devLogOTP, Log: l, Now: time.Now}
}

// FormatOnlyPINChecker accepts any well-formed PIN without contacting iTax.
type FormatOnlyPINChecker struct{}

// CheckPIN implements PINChecker.
func (FormatOnlyPINChecker) CheckPIN(_ context.Context, pin string) error {
	if !PINRe.MatchString(pin) {
		return ErrBadPIN
	}
	return nil
}

// ------------------------------------------------------------------ OTP

// RequestOTP creates and sends a 6-digit code. It never reveals whether the
// number is known.
func (s *Service) RequestOTP(ctx context.Context, msisdn, locale string) (expiresIn time.Duration, err error) {
	norm, err := crypto.NormaliseMSISDN(msisdn)
	if err != nil {
		return 0, ErrBadMSISDN
	}
	if !validLocale(locale) {
		return 0, ErrInvalidLocale
	}
	hash := s.Keys.Hash(norm)
	code, err := randomDigits(6)
	if err != nil {
		return 0, err
	}
	err = s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := tx.CountRecentOTPs(ctx, hash)
		if err != nil {
			return err
		}
		if n >= OTPMaxPerHour {
			return ErrOTPRateLimit
		}
		_, err = tx.CreateOTP(ctx, gen.CreateOTPParams{MsisdnHash: hash, CodeHash: s.hashCode(hash, code), ExpiresAt: s.Now().Add(OTPTTL)})
		return err
	})
	if err != nil {
		return 0, err
	}
	if s.DevLogOTP {
		s.Log.Info("OTP code (local only)", plog.Redact("msisdn", norm), "code", code)
	}
	if s.OTP != nil {
		if err := s.OTP.SendOTP(ctx, norm, code, locale); err != nil {
			s.Log.Warn("otp send failed", "err", err, plog.Redact("msisdn", norm))
			if !s.DevLogOTP {
				return 0, err
			}
		}
	}
	return OTPTTL, nil
}

// VerifyOTP checks the code, upserts the user and opens a session.
func (s *Service) VerifyOTP(ctx context.Context, msisdn, code, userAgent, ip string) (Session, string, error) {
	norm, err := crypto.NormaliseMSISDN(msisdn)
	if err != nil {
		return Session{}, "", ErrBadMSISDN
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return Session{}, "", ErrOTPInvalid
	}
	hash := s.Keys.Hash(norm)
	enc, err := s.Keys.EncryptString(norm)
	if err != nil {
		return Session{}, "", err
	}

	token, err := randomToken(32)
	if err != nil {
		return Session{}, "", err
	}
	csrf, err := randomToken(24)
	if err != nil {
		return Session{}, "", err
	}
	var sess Session
	err = s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		otp, err := tx.LatestOTP(ctx, hash)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrOTPInvalid
		}
		if err != nil {
			return err
		}
		if otp.Attempts >= OTPMaxAttempts {
			return ErrOTPInvalid
		}
		if subtle.ConstantTimeCompare(otp.CodeHash, s.hashCode(hash, code)) != 1 {
			_ = tx.BumpOTPAttempts(ctx, otp.ID)
			return ErrOTPInvalid
		}
		if err := tx.ConsumeOTP(ctx, otp.ID); err != nil {
			return err
		}
		user, err := tx.CreateUser(ctx, gen.CreateUserParams{MsisdnEnc: enc, MsisdnHash: hash, Locale: "en"})
		if err != nil {
			return err
		}
		var addr *netip.Addr
		if a, err := netip.ParseAddr(ip); err == nil {
			addr = &a
		}
		expires := s.Now().Add(SessionTTL)
		if _, err := tx.CreateSession(ctx, gen.CreateSessionParams{
			UserID: user.ID, TokenHash: s.hashToken(token), CsrfToken: csrf, ExpiresAt: expires, UserAgent: truncate(userAgent, 200), Ip: addr,
		}); err != nil {
			return err
		}
		ms, err := tx.ListMembershipsForUser(ctx, user.ID)
		if err != nil {
			return err
		}
		sess = Session{UserID: user.ID, CSRFToken: csrf, ExpiresAt: expires, Orgs: toMemberships(ms)}
		return nil
	})
	if err != nil {
		return Session{}, "", err
	}
	s.Log.Info("login", "user", sess.UserID, plog.Redact("msisdn", norm))
	return sess, token, nil
}

// Principal resolves a session cookie (+ optional X-Org-Id) into the caller.
func (s *Service) Principal(ctx context.Context, token string, orgHeader string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthorised
	}
	var p Principal
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := tx.GetSessionByTokenHash(ctx, s.hashToken(token))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnauthorised
		}
		if err != nil {
			return err
		}
		ms, err := tx.ListMembershipsForUser(ctx, row.UserID)
		if err != nil {
			return err
		}
		p = Principal{UserID: row.UserID, SessionID: row.ID, CSRF: row.CsrfToken, Locale: row.UserLocale, Memberships: toMemberships(ms)}
		if orgHeader != "" {
			want, err := uuid.Parse(orgHeader)
			if err != nil {
				return ErrForbidden
			}
			for _, m := range p.Memberships {
				if m.OrgID == want {
					p.OrgID, p.Role = m.OrgID, m.Role
					return nil
				}
			}
			return ErrForbidden
		}
		for _, m := range p.Memberships {
			if m.IsDefault || p.OrgID == uuid.Nil {
				p.OrgID, p.Role = m.OrgID, m.Role
			}
		}
		return nil
	})
	return p, err
}

// Logout revokes the session.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	return s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.RevokeSession(ctx, sessionID)
	})
}

// Principal is the authenticated caller with its resolved active org.
type Principal struct {
	UserID      uuid.UUID
	SessionID   uuid.UUID
	OrgID       uuid.UUID // uuid.Nil when the user has no org yet
	Role        string
	CSRF        string
	Locale      string
	Memberships []Membership
}

// ------------------------------------------------------------------ orgs

// CreateOrg registers a business and makes the caller its owner. The first
// org a user creates becomes their default.
func (s *Service) CreateOrg(ctx context.Context, userID uuid.UUID, in CreateOrgInput) (Org, error) {
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) < 2 || len(in.Name) > 80 {
		return Org{}, ErrBadName
	}
	pin := crypto.NormalisePIN(in.KRAPin)
	if !PINRe.MatchString(pin) {
		return Org{}, ErrBadPIN
	}
	if !validLocale(in.Locale) {
		return Org{}, ErrInvalidLocale
	}
	if in.Locale == "" {
		in.Locale = "en"
	}
	verified := true
	switch err := s.PIN.CheckPIN(ctx, pin); {
	case errors.Is(err, ErrPINUnknown), errors.Is(err, ErrBadPIN):
		return Org{}, err
	case err != nil:
		s.Log.Warn("pin check unavailable, continuing unverified", "err", err, "pin", plog.MaskPIN(pin))
		verified = false
	}
	pinEnc, err := s.Keys.EncryptString(pin)
	if err != nil {
		return Org{}, err
	}
	pinHash := s.Keys.Hash(pin)

	var out gen.Org
	err = s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.GetOrgByPINHash(ctx, pinHash); err == nil {
			return ErrPINTaken
		}
		o, err := tx.CreateOrg(ctx, gen.CreateOrgParams{Name: in.Name, KraPinEnc: pinEnc, KraPinHash: pinHash, VatRegistered: in.VATRegistered, Locale: in.Locale})
		if err != nil {
			return err
		}
		if verified {
			if _, err := tx.Tx.Exec(ctx, "UPDATE orgs SET kra_pin_verified_at = now() WHERE id = $1", o.ID); err != nil {
				return err
			}
			now := s.Now()
			o.KraPinVerifiedAt = &now
		}
		existing, err := tx.ListMembershipsForUser(ctx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.CreateMembership(ctx, gen.CreateMembershipParams{OrgID: o.ID, UserID: userID, Role: RoleOwner, IsDefault: len(existing) == 0}); err != nil {
			return err
		}
		out = o
		return nil
	})
	if err != nil {
		return Org{}, err
	}
	// Audit under the new org's scope.
	_ = s.DB.WithOrg(ctx, out.ID, func(ctx context.Context, tx db.Tx) error {
		actor := userID.String()
		return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: out.ID, ActorType: "user", ActorID: &actor, Action: "org.created", Entity: "org", EntityID: out.ID.String()})
	})
	s.Log.Info("org created", "org", out.ID, "user", userID, "pin", plog.MaskPIN(pin))
	return toOrg(out, pin, s.FiscalAdapter), nil
}

// GetOrg returns the API view of an organisation.
func (s *Service) GetOrg(ctx context.Context, id uuid.UUID) (Org, error) {
	var o gen.Org
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		o, err = tx.GetOrg(ctx, id)
		return err
	})
	if err != nil {
		return Org{}, err
	}
	pin, _ := s.Keys.DecryptString(o.KraPinEnc)
	return toOrg(o, pin, s.FiscalAdapter), nil
}

// ListMemberships returns the caller's organisations.
func (s *Service) ListMemberships(ctx context.Context, userID uuid.UUID) ([]Membership, error) {
	var ms []Membership
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListMembershipsForUser(ctx, userID)
		if err != nil {
			return err
		}
		ms = toMemberships(rows)
		return nil
	})
	return ms, err
}

// ------------------------------------------------------------- helpers

func (s *Service) hashCode(msisdnHash []byte, code string) []byte {
	m := hmac.New(sha256.New, s.SessionSecret)
	m.Write(msisdnHash)
	m.Write([]byte(":"))
	m.Write([]byte(code))
	return m.Sum(nil)
}

func (s *Service) hashToken(token string) []byte {
	m := hmac.New(sha256.New, s.SessionSecret)
	m.Write([]byte(token))
	return m.Sum(nil)
}

func randomDigits(n int) (string, error) {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
	v, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%0*d", n, v), nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// InviteMember invites an accountant or team member by phone or email.
func (s *Service) InviteMember(ctx context.Context, orgID, inviterID uuid.UUID, in InviteInput) (Invite, error) {
	role := in.Role
	if role == "" {
		role = RoleAccountant
	}
	if role != RoleAccountant && role != RoleStaff && role != RoleAdmin {
		return Invite{}, errors.New("org: invalid role")
	}
	if in.Phone == "" && in.Email == "" {
		return Invite{}, errors.New("org: phone or email required")
	}

	var out Invite
	var phoneNorm string
	var phoneHash []byte
	
	if in.Phone != "" {
		p, err := crypto.NormaliseMSISDN(in.Phone)
		if err != nil {
			return Invite{}, ErrBadMSISDN
		}
		phoneNorm = p
		phoneHash = s.Keys.Hash(phoneNorm)

	}

	var emailPtr *string
	if in.Email != "" {
		emailPtr = &in.Email
	}

	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		var phonePtr *string
		status := "pending"

		if phoneNorm != "" {
			phonePtr = &phoneNorm
			status = "pending"

			u, err := tx.GetUserByMSISDNHash(ctx, phoneHash)
			if err == nil {
				if u.ID == inviterID {
					return errors.New("you cannot invite yourself to this business")
				}
				members, err := tx.ListMembersForOrg(ctx, orgID)
				if err == nil {
					for _, m := range members {
						if m.UserID == u.ID {
							return errors.New("user is already a member of this business")
						}
					}
				}
			}
		}

		inv, err := tx.CreateInvite(ctx, gen.CreateInviteParams{
			OrgID:     orgID,
			InvitedBy: inviterID,
			Role:      role,
			Phone:     phonePtr,
			PhoneHash: phoneHash,
			Email:     emailPtr,
			Status:    status,
		})
		if err != nil {
			return err
		}

		out = Invite{
			ID:        inv.ID,
			OrgID:     inv.OrgID,
			Role:      inv.Role,
			Status:    inv.Status,
			CreatedAt: inv.CreatedAt,
		}
		if inv.Phone != nil {
			out.Phone = *inv.Phone
		}
		if inv.Email != nil {
			out.Email = *inv.Email
		}
		return nil
	})

	return out, err
}

// ListInvites returns pending invites for an organisation.
func (s *Service) ListInvites(ctx context.Context, orgID uuid.UUID) ([]Invite, error) {
	out := []Invite{}
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListInvitesForOrg(ctx, orgID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			inv := Invite{
				ID:        r.ID,
				OrgID:     r.OrgID,
				Role:      r.Role,
				Status:    r.Status,
				CreatedAt: r.CreatedAt,
			}
			if r.Phone != nil {
				inv.Phone = *r.Phone
			}
			if r.Email != nil {
				inv.Email = *r.Email
			}
			out = append(out, inv)
		}
		return nil
	})
	return out, err
}

// RevokeInvite cancels a pending invite and removes any provisioned membership.
func (s *Service) RevokeInvite(ctx context.Context, orgID, inviteID uuid.UUID) error {
	return s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		invites, err := tx.ListInvitesForOrg(ctx, orgID)
		if err == nil {
			for _, inv := range invites {
				if inv.ID == inviteID && len(inv.PhoneHash) > 0 {
					if u, err := tx.GetUserByMSISDNHash(ctx, inv.PhoneHash); err == nil {
						_ = tx.DeleteMembership(ctx, gen.DeleteMembershipParams{OrgID: orgID, UserID: u.ID})
					}
				}
			}
		}
		return tx.RevokeInvite(ctx, gen.RevokeInviteParams{ID: inviteID, OrgID: orgID})
	})
}

// ListMembers returns all members of an organisation with masked contact info.
func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID) ([]Member, error) {
	out := []Member{}
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListMembersForOrg(ctx, orgID)
		if err != nil {
			return err
		}
		for _, r := range rows {
			phoneMasked := ""
			if len(r.MsisdnEnc) > 0 {
				if clear, err := s.Keys.DecryptString(r.MsisdnEnc); err == nil {
					phoneMasked = plog.MaskMSISDN(clear)
				}
			}
			out = append(out, Member{
				ID:          r.ID,
				UserID:      r.UserID,
				Role:        r.Role,
				Name:        r.UserName,
				PhoneMasked: phoneMasked,
				IsDefault:   r.IsDefault,
				CreatedAt:   r.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}

// RemoveMember deletes a user membership in an organisation.
func (s *Service) RemoveMember(ctx context.Context, orgID, targetUserID uuid.UUID) error {
	return s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.DeleteMembership(ctx, gen.DeleteMembershipParams{OrgID: orgID, UserID: targetUserID})
	})
}

// AccountantInvite represents an invitation presented to an accountant.
type AccountantInvite struct {
	ID        uuid.UUID `json:"id"`
	OrgID     uuid.UUID `json:"org_id"`
	OrgName   string    `json:"org_name"`
	KRAPin    string    `json:"kra_pin"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

// AccountantClient represents a client organisation for an accountant overview.
type AccountantClient struct {
	OrgID             uuid.UUID `json:"org_id"`
	Name              string    `json:"name"`
	KRAPin            string    `json:"kra_pin"`
	CurrentMonthGross int64     `json:"current_month_gross_cents"`
	EstimatedVAT      int64     `json:"estimated_vat_cents"`
	EtimsSyncHealth   string    `json:"etims_sync_health"`
	Role              string    `json:"role"`
}

// ListPendingInvitesForUser returns all pending invites matching the user phone or email.
func (s *Service) ListPendingInvitesForUser(ctx context.Context, userID uuid.UUID) ([]AccountantInvite, error) {
	out := []AccountantInvite{}
	err := s.DB.WithAccountant(ctx, func(ctx context.Context, tx db.Tx) error {
		u, err := tx.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		email := ""
		if u.Email != nil {
			email = *u.Email
		}
		rows, err := tx.ListPendingInvitesForPhoneOrEmail(ctx, gen.ListPendingInvitesForPhoneOrEmailParams{
			PhoneHash: u.MsisdnHash,
			Column2:   email,
		})
		if err != nil {
			return err
		}
		for _, r := range rows {
			kraPin := ""
			if len(r.KraPinEnc) > 0 {
				if dec, err := s.Keys.DecryptString(r.KraPinEnc); err == nil {
					kraPin = dec
				}
			}
			out = append(out, AccountantInvite{
				ID:        r.ID,
				OrgID:     r.OrgID,
				OrgName:   r.OrgName,
				KRAPin:    kraPin,
				Role:      r.Role,
				CreatedAt: r.CreatedAt,
			})
		}
		return nil
	})
	return out, err
}

// AcceptInvite accepts an invitation and provisions active membership.
func (s *Service) AcceptInvite(ctx context.Context, userID, inviteID uuid.UUID) error {
	return s.DB.WithAccountant(ctx, func(ctx context.Context, tx db.Tx) error {
		u, err := tx.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		inv, err := tx.GetInviteByID(ctx, inviteID)
		if err != nil {
			return ErrNotFound
		}
		if inv.Status != "pending" {
			return errors.New("invitation is no longer pending")
		}

		// Verify recipient match
		isPhoneMatch := len(inv.PhoneHash) > 0 && bytes.Equal(inv.PhoneHash, u.MsisdnHash)
		isEmailMatch := inv.Email != nil && u.Email != nil && strings.EqualFold(*inv.Email, *u.Email)
		if !isPhoneMatch && !isEmailMatch {
			return ErrForbidden
		}

		// Transition invite status to accepted
		if _, err := tx.AcceptInvite(ctx, inviteID); err != nil {
			return err
		}

		// Provision active membership
		ms, _ := tx.ListMembershipsForUser(ctx, userID)
		isDefault := len(ms) == 0
		_, err = tx.CreateMembership(ctx, gen.CreateMembershipParams{
			OrgID:     inv.OrgID,
			UserID:    userID,
			Role:      inv.Role,
			IsDefault: isDefault,
		})
		return err
	})
}

// RejectInvite transitions a pending invitation to rejected.
func (s *Service) RejectInvite(ctx context.Context, userID, inviteID uuid.UUID) error {
	return s.DB.WithAccountant(ctx, func(ctx context.Context, tx db.Tx) error {
		u, err := tx.GetUser(ctx, userID)
		if err != nil {
			return err
		}
		inv, err := tx.GetInviteByID(ctx, inviteID)
		if err != nil {
			return ErrNotFound
		}
		if inv.Status != "pending" {
			return errors.New("invitation is no longer pending")
		}

		isPhoneMatch := len(inv.PhoneHash) > 0 && bytes.Equal(inv.PhoneHash, u.MsisdnHash)
		isEmailMatch := inv.Email != nil && u.Email != nil && strings.EqualFold(*inv.Email, *u.Email)
		if !isPhoneMatch && !isEmailMatch {
			return ErrForbidden
		}

		_, err = tx.RejectInvite(ctx, inviteID)
		return err
	})
}

// ListAccountantClients returns client organizations with real-time financial metrics.
func (s *Service) ListAccountantClients(ctx context.Context, userID uuid.UUID) ([]AccountantClient, error) {
	out := []AccountantClient{}
	var memberships []Membership
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		ms, err := tx.ListMembershipsForUser(ctx, userID)
		if err != nil {
			return err
		}
		memberships = toMemberships(ms)
		return nil
	})
	if err != nil {
		return nil, err
	}

	nairobi, _ := time.LoadLocation("Africa/Nairobi")
	if nairobi == nil {
		nairobi = time.UTC
	}
	now := s.Now().In(nairobi)
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, nairobi)
	to := from.AddDate(0, 1, 0)

	for _, m := range memberships {
		var client AccountantClient
		client.OrgID = m.OrgID
		client.Name = m.Name
		client.Role = m.Role

		// Query under WithOrg for strict tenant isolation
		err := s.DB.WithOrg(ctx, m.OrgID, func(ctx context.Context, tx db.Tx) error {
			org, err := tx.GetOrg(ctx, m.OrgID)
			if err != nil {
				return err
			}
			if len(org.KraPinEnc) > 0 {
				if pin, err := s.Keys.DecryptString(org.KraPinEnc); err == nil {
					client.KRAPin = pin
				}
			}

			// Payments in current month
			pmts, err := tx.AnalyticsPayments(ctx, gen.AnalyticsPaymentsParams{
				OrgID:    m.OrgID,
				PaidAt:   from,
				PaidAt_2: to,
			})
			if err == nil {
				for _, p := range pmts {
					client.CurrentMonthGross += p.AmountCents
				}
			}

			// VAT liability in current month
			vatRow, err := tx.AnalyticsVATLiability(ctx, gen.AnalyticsVATLiabilityParams{
				OrgID:     m.OrgID,
				AckedAt:   &from,
				AckedAt_2: &to,
			})
			if err == nil {
				client.EstimatedVAT = vatRow.VatLiabilityCents
			}

			// eTIMS Sync Health
			if org.EtimsStatus != "initialized" {
				client.EtimsSyncHealth = "unconfigured"
			} else {
				states, err := tx.CountInvoicesByState(ctx, m.OrgID)
				attentionCount := int64(0)
				pendingCount := int64(0)
				if err == nil {
					for _, r := range states {
						switch fiscal.State(r.State) {
						case fiscal.StateNeedsReview, fiscal.StateFailedTerminal:
							attentionCount += r.N
						case fiscal.StateDraft, fiscal.StateTaxPending, fiscal.StateQueued, fiscal.StateSubmitted, fiscal.StateFailedRetryable:
							pendingCount += r.N
						}
					}
				}
				if attentionCount > 0 {
					client.EtimsSyncHealth = "action_required"
				} else if pendingCount > 0 {
					client.EtimsSyncHealth = "pending_sync"
				} else {
					client.EtimsSyncHealth = "healthy"
				}
			}

			return nil
		})
		if err != nil {
			return nil, err
		}
		out = append(out, client)
	}

	return out, nil
}
