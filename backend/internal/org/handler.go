package org

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/httpx"
)

// Handler serves /auth/*, /orgs* and /org/etims.
type Handler struct {
	S            *Service
	SecureCookie bool // Secure attribute; off for plain-http local dev
	// OnEtimsConfigured runs after ConfigureEtims succeeds (composition root
	// wires it to ledger.Service.ActivateTaxPending); nil skips it. Kept as a
	// callback rather than a dependency so org never imports ledger.
	OnEtimsConfigured func(ctx context.Context, orgID uuid.UUID)
}

// MountPublic registers the unauthenticated auth routes.
func (h *Handler) MountPublic(r chi.Router) {
	otpKey := func(r *http.Request) string { return "otp:" + httpx.ClientIP(r) }
	r.With(httpx.RateLimit(20, time.Hour, otpKey)).Post("/auth/otp/request", h.requestOTP)
	r.With(httpx.RateLimit(30, time.Hour, otpKey)).Post("/auth/otp/verify", h.verifyOTP)
}

// MountPrivate registers routes that need a session (mount behind Authenticate).
func (h *Handler) MountPrivate(r chi.Router) {
	r.Post("/auth/logout", h.logout)
	r.Get("/orgs", h.listOrgs)
	r.Post("/orgs", h.createOrg)
	r.With(RequireOrg).Get("/orgs/current", h.currentOrg)
	r.With(RequireOrg).Get("/org/etims", h.getEtims)
	r.With(RequireOrg, RequireRole(RoleOwner, RoleAdmin)).Post("/org/etims", h.configureEtims)
}

// Authenticate resolves the session cookie into an httpx.Principal on the
// context and enforces the CSRF header on mutating requests.
func (h *Handler) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(SessionCookie); err == nil {
			token = c.Value
		}
		p, err := h.S.Principal(r.Context(), token, r.Header.Get(HeaderOrgID))
		switch {
		case errors.Is(err, ErrUnauthorised):
			httpx.Fail(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue")
			return
		case errors.Is(err, ErrForbidden):
			httpx.Fail(w, http.StatusForbidden, "forbidden", "You are not a member of that organisation")
			return
		case err != nil:
			httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not load your session")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get(HeaderCSRFToken)), []byte(p.CSRF)) != 1 {
				httpx.Fail(w, http.StatusForbidden, "forbidden", "Missing or invalid CSRF token")
				return
			}
		}
		ctx := httpx.WithPrincipal(r.Context(), httpx.Principal{
			UserID: p.UserID, OrgID: p.OrgID, Role: p.Role, SessionID: p.SessionID, CSRF: p.CSRF, Locale: p.Locale,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireOrg rejects callers who have not created or joined an organisation.
func RequireOrg(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := httpx.PrincipalFrom(r.Context())
		if !ok || p.OrgID == uuid.Nil {
			httpx.Fail(w, http.StatusForbidden, "no_org", "Create your business first")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireRole allows only the listed roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := httpx.PrincipalFrom(r.Context())
			if !ok || !allowed[p.Role] {
				httpx.Fail(w, http.StatusForbidden, "forbidden", "Your role cannot do that")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (h *Handler) requestOTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MSISDN string `json:"msisdn"`
		Locale string `json:"locale"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {msisdn, locale?}")
		return
	}
	ttl, err := h.S.RequestOTP(r.Context(), in.MSISDN, in.Locale)
	switch {
	case errors.Is(err, ErrBadMSISDN), errors.Is(err, ErrInvalidLocale):
		httpx.Fail(w, http.StatusUnprocessableEntity, "validation", err.Error())
	case errors.Is(err, ErrOTPRateLimit):
		httpx.Fail(w, http.StatusTooManyRequests, "rate_limited", "Too many codes requested; try again in an hour")
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not send the code")
	default:
		httpx.JSON(w, http.StatusAccepted, map[string]int{"expires_in_seconds": int(ttl.Seconds())})
	}
}

func (h *Handler) verifyOTP(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MSISDN string `json:"msisdn"`
		Code   string `json:"code"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {msisdn, code}")
		return
	}
	sess, token, err := h.S.VerifyOTP(r.Context(), in.MSISDN, in.Code, r.UserAgent(), httpx.ClientIP(r))
	switch {
	case errors.Is(err, ErrBadMSISDN):
		httpx.Fail(w, http.StatusUnprocessableEntity, "validation", err.Error())
	case errors.Is(err, ErrOTPInvalid):
		httpx.Fail(w, http.StatusUnauthorized, "unauthenticated", "That code is wrong or has expired")
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not sign you in")
	default:
		http.SetCookie(w, &http.Cookie{
			Name: SessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: h.SecureCookie,
			SameSite: http.SameSiteLaxMode, Expires: sess.ExpiresAt,
		})
		httpx.JSON(w, http.StatusOK, sess)
	}
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	if err := h.S.Logout(r.Context(), p.SessionID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not log out")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: h.SecureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listOrgs(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	ms, err := h.S.ListMemberships(r.Context(), p.UserID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list organisations")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": ms})
}

func (h *Handler) createOrg(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	var in CreateOrgInput
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {name, kra_pin, vat_registered?, locale?}")
		return
	}
	o, err := h.S.CreateOrg(r.Context(), p.UserID, in)
	switch {
	case errors.Is(err, ErrBadPIN), errors.Is(err, ErrBadName), errors.Is(err, ErrInvalidLocale):
		httpx.Fail(w, http.StatusUnprocessableEntity, "validation", err.Error())
	case errors.Is(err, ErrPINUnknown):
		httpx.Fail(w, http.StatusUnprocessableEntity, "pin_unknown", "KRA does not recognise this PIN. Check it on iTax and try again")
	case errors.Is(err, ErrPINTaken):
		httpx.Fail(w, http.StatusConflict, "conflict", "A business with this KRA PIN is already registered")
	case err != nil:
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not create the organisation")
	default:
		httpx.JSON(w, http.StatusCreated, o)
	}
}

func (h *Handler) currentOrg(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	o, err := h.S.GetOrg(r.Context(), p.OrgID)
	if db.NotFound(err) {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Organisation not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not load the organisation")
		return
	}
	httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) getEtims(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	out, err := h.S.GetEtimsSettings(r.Context(), p.OrgID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not load the tax settings")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

// configureEtims is Tax Settings' "Connect to KRA" action: it runs direct
// OSCU device initialisation for the org's own PIN and the branch id/device
// serial given here, and requeues any invoices that were held back while
// eTIMS was unconfigured (progressive onboarding, ADR-0009).
func (h *Handler) configureEtims(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	var in struct {
		KRABhfID       string `json:"kra_bhf_id"`
		KRADeviceSerial string `json:"kra_device_serial"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Body must be {kra_bhf_id?, kra_device_serial}")
		return
	}
	out, err := h.S.ConfigureEtims(r.Context(), p.OrgID, in.KRABhfID, in.KRADeviceSerial)
	switch {
	case errors.Is(err, ErrEtimsBadBranch), errors.Is(err, ErrEtimsSerialRequired):
		httpx.Fail(w, http.StatusUnprocessableEntity, "validation", err.Error())
	case errors.Is(err, ErrNoFiscalProvider):
		httpx.Fail(w, http.StatusNotImplemented, "not_implemented", "This deployment has no direct KRA connection configured")
	case err != nil && fiscal.Classify(err) == fiscal.ClassRetryable:
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "KRA could not be reached; try again shortly")
	case err != nil:
		httpx.JSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error": map[string]string{"code": "etims_rejected", "message": err.Error()},
			"data":  out,
		})
	default:
		if h.OnEtimsConfigured != nil {
			h.OnEtimsConfigured(r.Context(), p.OrgID)
		}
		httpx.JSON(w, http.StatusOK, out)
	}
}
