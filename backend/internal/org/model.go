// Package org owns organisations, users, memberships and phone-OTP
// authentication with cookie sessions. See docs/api.md §2.
package org

import (
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/platform/db/gen"
	plog "github.com/exoin/ciftpay/internal/platform/log"
)

// Roles a membership can carry.
const (
	RoleOwner      = "owner"
	RoleStaff      = "staff"
	RoleAccountant = "accountant"
	RoleAdmin      = "admin"
)

// Auth constants from docs/api.md.
const (
	SessionCookie   = "ciftpay_session"
	SessionTTL      = 30 * 24 * time.Hour
	OTPTTL          = 5 * time.Minute
	OTPMaxPerHour   = 5
	OTPMaxAttempts  = 5
	HeaderOrgID     = "X-Org-Id"
	HeaderCSRFToken = "X-CSRF-Token"
)

// Errors returned by the service and mapped to HTTP by the handler.
var (
	ErrBadMSISDN     = errors.New("org: phone number is not a valid Kenyan MSISDN")
	ErrBadPIN        = errors.New("org: KRA PIN must be A or P, nine digits and a letter")
	ErrBadName       = errors.New("org: name must be 2–80 characters")
	ErrOTPRateLimit  = errors.New("org: too many codes requested for this number")
	ErrOTPInvalid    = errors.New("org: code is wrong or expired")
	ErrUnauthorised  = errors.New("org: not authenticated")
	ErrForbidden     = errors.New("org: not a member of that organisation")
	ErrNotFound      = errors.New("org: not found")
	ErrPINTaken      = errors.New("org: an organisation with this KRA PIN already exists")
	ErrPINUnknown    = errors.New("org: KRA does not know this PIN")
	ErrNoMembership  = errors.New("org: user has no organisation yet")
	ErrCSRFMismatch  = errors.New("org: csrf token mismatch")
	ErrInvalidLocale = errors.New("org: locale must be en or sw")
)

// PINRe validates the KRA PIN format: A/P + 9 digits + check letter.
var PINRe = regexp.MustCompile(`^[AP][0-9]{9}[A-Z]$`)

// Membership is the API view of a membership row.
type Membership struct {
	OrgID     uuid.UUID `json:"org_id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	IsDefault bool      `json:"is_default"`
}

// Session is the response of POST /auth/otp/verify.
type Session struct {
	UserID    uuid.UUID    `json:"user_id"`
	CSRFToken string       `json:"csrf_token"`
	ExpiresAt time.Time    `json:"expires_at"`
	Orgs      []Membership `json:"orgs"`
}

// Org is the API view of an organisation.
type Org struct {
	ID               uuid.UUID  `json:"id"`
	Name             string     `json:"name"`
	KRAPin           string     `json:"kra_pin,omitempty"`
	KRAPinMasked     string     `json:"kra_pin_masked"`
	KRAPinVerifiedAt *time.Time `json:"kra_pin_verified_at"`
	VATRegistered    bool       `json:"vat_registered"`
	Locale           string     `json:"locale"`
	FiscalAdapter    string     `json:"fiscal_adapter,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// CreateOrgInput is the body of POST /orgs.
type CreateOrgInput struct {
	Name          string `json:"name"`
	KRAPin        string `json:"kra_pin"`
	VATRegistered bool   `json:"vat_registered"`
	Locale        string `json:"locale"`
}

func toMemberships(rows []gen.ListMembershipsForUserRow) []Membership {
	out := make([]Membership, 0, len(rows))
	for _, r := range rows {
		out = append(out, Membership{OrgID: r.OrgID, Name: r.OrgName, Role: r.Role, IsDefault: r.IsDefault})
	}
	return out
}

func toOrg(o gen.Org, pin, adapter string) Org {
	return Org{
		ID: o.ID, Name: o.Name, KRAPin: pin, KRAPinMasked: plog.MaskPIN(pin), KRAPinVerifiedAt: o.KraPinVerifiedAt,
		VATRegistered: o.VatRegistered, Locale: o.Locale, FiscalAdapter: adapter, CreatedAt: o.CreatedAt,
	}
}

func validLocale(l string) bool { return l == "" || l == "en" || l == "sw" }

// Invite is the API view of an org_invites row.
type Invite struct {
	ID        uuid.UUID `json:"id"`
	OrgID     uuid.UUID `json:"org_id"`
	Role      string    `json:"role"`
	Phone     string    `json:"phone,omitempty"`
	Email     string    `json:"email,omitempty"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// InviteInput is the body of POST /org/invites.
type InviteInput struct {
	Phone string `json:"phone"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Member is the API view of a member in an organisation.
type Member struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Role        string    `json:"role"`
	Name        string    `json:"name"`
	PhoneMasked string    `json:"phone_masked"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
}
