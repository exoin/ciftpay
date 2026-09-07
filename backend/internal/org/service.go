package org

import (
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

	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
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
