package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
)

// Shortcode control check (plan.md §4.1): the merchant proves a Till/Paybill/
// Pochi is theirs by paying exactly VerificationAmountCents to it from their
// own phone inside VerificationWindow. The open challenge lives in
// shortcode_verifications; the matching C2B confirmation is consumed here and
// never becomes a payment, sale or invoice.
const (
	VerificationAmountCents int64 = 100
	VerificationWindow            = 10 * time.Minute
	// VerificationAccountRef is what a Paybill payer types as account number.
	VerificationAccountRef = "CIFTPAY"
	// RuleVerification is reported in C2BResult.Rule for a consumed KES 1.
	RuleVerification = "verification"
	// StatusVerification is reported in C2BResult.Status for a consumed KES 1.
	StatusVerification = "verification"
)

// ErrShortcodeClaimed means another organisation already verified this number.
var ErrShortcodeClaimed = errors.New("ledger: shortcode already verified by another organisation")

// Verification statuses.
const (
	VerificationPending  = "pending"
	VerificationVerified = "verified"
	VerificationExpired  = "expired"
	VerificationFailed   = "failed"
)

// ResolveForC2B finds the org a C2B confirmation belongs to. Must run under
// db.WithIngest. When several orgs hold the same unverified number, the row
// with an open challenge from this payer for this amount wins, so the KES 1
// reaches the org that is proving control.
func (s *Service) ResolveForC2B(ctx context.Context, tx db.Tx, in C2BInput) (gen.ResolveShortcodeRow, error) {
	var prefer *uuid.UUID
	if norm, err := crypto.NormaliseMSISDN(in.MSISDN); err == nil {
		v, err := tx.FindOpenShortcodeVerification(ctx, gen.FindOpenShortcodeVerificationParams{
			Shortcode: in.ShortCode, MsisdnHash: s.Keys.Hash(norm), AmountCents: in.AmountCents,
		})
		if err == nil {
			prefer = &v.ShortcodeID
		}
	}
	return tx.ResolveShortcode(ctx, gen.ResolveShortcodeParams{Shortcode: in.ShortCode, PreferID: prefer})
}

// tryVerification consumes a C2B confirmation that settles an open challenge
// on this shortcode. It reports consumed=true when the event was the KES 1
// control payment (verified, or failed because another org verified the
// number first); the caller then skips payment creation. Runs in org scope.
func (s *Service) tryVerification(ctx context.Context, tx db.Tx, sc gen.ResolveShortcodeRow, in C2BInput, msisdnHash []byte, res *C2BResult) (bool, error) {
	if sc.VerifiedAt != nil || msisdnHash == nil {
		return false, nil
	}
	v, err := tx.FindOpenShortcodeVerification(ctx, gen.FindOpenShortcodeVerificationParams{
		Shortcode: sc.Shortcode, MsisdnHash: msisdnHash, AmountCents: in.AmountCents,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if v.ShortcodeID != sc.ID {
		// Challenge belongs to another org's row for the same number; the
		// ingest resolver should have preferred it. Treat as a normal payment.
		return false, nil
	}
	res.Status, res.Rule = StatusVerification, RuleVerification

	// The partial unique index on verified shortcodes is the arbiter of the
	// race between two orgs; a violation aborts the statement only, thanks to
	// the savepoint, and the challenge is settled as failed.
	sp, err := tx.Tx.Begin(ctx)
	if err != nil {
		return true, err
	}
	err = gen.New(sp).MarkShortcodeVerified(ctx, gen.MarkShortcodeVerifiedParams{ID: sc.ID, VerificationCheckoutID: db.Ptr("c2b:" + in.TransID)})
	switch {
	case isUniqueViolation(err):
		_ = sp.Rollback(ctx)
		s.Log.Warn("shortcode verification lost the race", "org", sc.OrgID, "shortcode", sc.Shortcode, "trans_id", in.TransID)
		if err := tx.SettleShortcodeVerification(ctx, gen.SettleShortcodeVerificationParams{ID: v.ID, Status: VerificationFailed, TransID: &in.TransID, PaidAt: &in.PaidAt}); err != nil {
			return true, err
		}
		return true, tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: sc.OrgID, ActorType: "system", Action: "shortcode.verification_failed", Entity: "mpesa_shortcode", EntityID: sc.ID.String(), After: mustJSON(map[string]string{"reason": "claimed", "trans_id": in.TransID})})
	case err != nil:
		_ = sp.Rollback(ctx)
		return true, err
	}
	if err := sp.Commit(ctx); err != nil {
		return true, err
	}
	if err := tx.SettleShortcodeVerification(ctx, gen.SettleShortcodeVerificationParams{ID: v.ID, Status: VerificationVerified, TransID: &in.TransID, PaidAt: &in.PaidAt}); err != nil {
		return true, err
	}
	s.Log.Info("shortcode verified by own payment", "org", sc.OrgID, "shortcode", sc.Shortcode, "trans_id", in.TransID, plog.Redact("msisdn", in.MSISDN))
	return true, tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: sc.OrgID, ActorType: "system", Action: "shortcode.verified", Entity: "mpesa_shortcode", EntityID: sc.ID.String(), After: mustJSON(map[string]string{"mode": "c2b", "trans_id": in.TransID})})
}

// OpenVerification opens (or refreshes) the KES 1 challenge for a shortcode.
// Any earlier pending challenge on the shortcode is expired by the query.
func (s *Service) OpenVerification(ctx context.Context, tx db.Tx, orgID, shortcodeID uuid.UUID, msisdnHash []byte) (gen.ShortcodeVerification, error) {
	return tx.CreateShortcodeVerification(ctx, gen.CreateShortcodeVerificationParams{
		OrgID: orgID, ShortcodeID: shortcodeID, MsisdnHash: msisdnHash,
		AmountCents: VerificationAmountCents, ExpiresAt: s.Now().Add(VerificationWindow),
	})
}

// VerificationState is the `verification` object embedded in Shortcode.
type VerificationState struct {
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// verificationState derives the API view of a challenge row, reporting a
// pending challenge whose window has lapsed as expired even before the
// housekeeping update runs.
func (s *Service) verificationState(v gen.ShortcodeVerification) *VerificationState {
	st := v.Status
	if st == VerificationPending && !v.ExpiresAt.After(s.Now()) {
		st = VerificationExpired
	}
	return &VerificationState{Status: st, ExpiresAt: v.ExpiresAt}
}

// latestVerification returns the API view of the newest challenge, or nil.
func (s *Service) latestVerification(ctx context.Context, tx db.Tx, shortcodeID uuid.UUID) *VerificationState {
	v, err := tx.LatestShortcodeVerification(ctx, shortcodeID)
	if err != nil {
		return nil
	}
	return s.verificationState(v)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
