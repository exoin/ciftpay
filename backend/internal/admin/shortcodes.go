package admin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/ledger"
	"github.com/exoin/ciftpay/internal/mpesa"
	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/httpx"
	"github.com/exoin/ciftpay/internal/platform/storage"
)

// Shortcodes is the operator side of the Administrative Gate (ADR-0008): the
// cross-tenant queue of shortcodes waiting for (or already given) a verdict
// on their signed Safaricom authorization letter. A nil *Shortcodes on
// Handler disables the /admin/shortcodes* routes entirely.
type Shortcodes struct {
	DB   *db.DB
	Keys *crypto.Keyring
	// Files reads back the letter uploaded through
	// ledger.Service.SubmitAuthorization.
	Files storage.Store
	// Daraja registers C2B URLs once a shortcode is verified. Nil or
	// unconfigured just skips that best-effort step.
	Daraja         *mpesa.Client
	WebhookBaseURL string
	Log            *slog.Logger
}

// List returns the operator queue, defaulting to pending_authorization.
func (s *Shortcodes) List(ctx context.Context, status string, limit int32) ([]ledger.AdminShortcodeView, error) {
	if status == "" {
		status = ledger.ShortcodePendingAuthorization
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	out := []ledger.AdminShortcodeView{}
	err := s.DB.WithAdmin(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListShortcodesByStatus(ctx, gen.ListShortcodesByStatusParams{Status: status, Limit: limit})
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, ledger.ToAdminShortcode(s.Keys, r.MpesaShortcode, r.OrgName, r.OrgKraPinEnc))
		}
		return nil
	})
	return out, err
}

// Get returns one row for the operator (letter download, detail view).
func (s *Shortcodes) Get(ctx context.Context, id uuid.UUID) (ledger.AdminShortcodeView, error) {
	var out ledger.AdminShortcodeView
	err := s.DB.WithAdmin(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := tx.GetShortcodeAdmin(ctx, id)
		if err != nil {
			return err
		}
		out = ledger.ToAdminShortcode(s.Keys, row.MpesaShortcode, row.OrgName, row.OrgKraPinEnc)
		return nil
	})
	return out, err
}

// FindByNumber looks up every row (across tenants) for a shortcode number;
// ciftctl uses it to resolve a bare number to a row id when it is unambiguous.
func (s *Shortcodes) FindByNumber(ctx context.Context, shortcode string) ([]gen.MpesaShortcode, error) {
	var out []gen.MpesaShortcode
	err := s.DB.WithAdmin(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = tx.FindShortcodesByNumber(ctx, shortcode)
		return err
	})
	return out, err
}

// Verify marks a shortcode verified once Safaricom has mapped it to
// CiftPay's Daraja app, then best-effort registers its C2B URLs. Re-running
// it on an already-verified row is a no-op (idempotent replay).
func (s *Shortcodes) Verify(ctx context.Context, id uuid.UUID, reviewedBy, note string) (ledger.AdminShortcodeView, error) {
	row, err := s.Get(ctx, id)
	if err != nil {
		return ledger.AdminShortcodeView{}, err
	}
	if row.Status == ledger.ShortcodeVerified {
		return row, nil
	}

	var out gen.MpesaShortcode
	err = s.DB.WithOrg(ctx, row.OrgID, func(ctx context.Context, tx db.Tx) error {
		var err error
		out, err = tx.VerifyShortcode(ctx, gen.VerifyShortcodeParams{ID: id, ReviewedBy: &reviewedBy})
		if isUniqueViolation(err) {
			return ledger.ErrShortcodeClaimed
		}
		if err != nil {
			return err
		}
		actor := reviewedBy
		return tx.AppendAudit(ctx, gen.AppendAuditParams{
			OrgID: row.OrgID, ActorType: "admin", ActorID: &actor, Action: "shortcode.verified",
			Entity: "mpesa_shortcode", EntityID: id.String(), After: mustJSON(map[string]string{"note": note}),
		})
	})
	if err != nil {
		return ledger.AdminShortcodeView{}, err
	}

	if s.Daraja != nil && s.Daraja.Configured() && s.WebhookBaseURL != "" {
		if rErr := s.Daraja.RegisterC2BURLs(ctx, out.Shortcode, s.WebhookBaseURL); rErr != nil {
			s.logWarn("daraja registerurl failed", "shortcode", out.Shortcode, "err", rErr)
		} else {
			_ = s.DB.WithOrg(ctx, row.OrgID, func(ctx context.Context, tx db.Tx) error {
				return tx.MarkShortcodeC2BRegistered(ctx, id)
			})
		}
	}
	return s.Get(ctx, id)
}

// Reject refuses the authorization (Safaricom refused it, or the letter
// itself is unusable). The merchant sees rejection_reason and can upload
// again, which returns the row to pending_authorization.
func (s *Shortcodes) Reject(ctx context.Context, id uuid.UUID, reviewedBy, reason string) (ledger.AdminShortcodeView, error) {
	row, err := s.Get(ctx, id)
	if err != nil {
		return ledger.AdminShortcodeView{}, err
	}
	if row.Status == ledger.ShortcodeVerified {
		return ledger.AdminShortcodeView{}, ledger.ErrAlreadyVerified
	}
	err = s.DB.WithOrg(ctx, row.OrgID, func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.RejectShortcode(ctx, gen.RejectShortcodeParams{ID: id, ReviewedBy: &reviewedBy, RejectionReason: &reason}); err != nil {
			return err
		}
		actor := reviewedBy
		return tx.AppendAudit(ctx, gen.AppendAuditParams{
			OrgID: row.OrgID, ActorType: "admin", ActorID: &actor, Action: "shortcode.rejected",
			Entity: "mpesa_shortcode", EntityID: id.String(), After: mustJSON(map[string]string{"reason": reason}),
		})
	})
	if err != nil {
		return ledger.AdminShortcodeView{}, err
	}
	return s.Get(ctx, id)
}

func (s *Shortcodes) logWarn(msg string, args ...any) {
	if s.Log != nil {
		s.Log.Warn(msg, args...)
	}
}

// ---------------------------------------------------------------- HTTP

// MountShortcodes registers /admin/shortcodes* (mount behind RequireRole(admin)).
func (h *Handler) MountShortcodes(r chi.Router) {
	r.Get("/admin/shortcodes", h.adminListShortcodes)
	r.Get("/admin/shortcodes/{id}/authorization", h.adminGetLetter)
	r.Patch("/admin/shortcodes/{id}/verify", h.adminVerifyShortcode)
	r.Patch("/admin/shortcodes/{id}/reject", h.adminRejectShortcode)
}

func shortcodeIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Unknown id")
		return uuid.Nil, false
	}
	return id, true
}

func failShortcode(w http.ResponseWriter, err error, msg string) {
	switch {
	case db.NotFound(err):
		httpx.Fail(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, ledger.ErrShortcodeClaimed):
		httpx.Fail(w, http.StatusConflict, "shortcode_claimed", "Another organisation already holds this number verified")
	case errors.Is(err, ledger.ErrAlreadyVerified):
		httpx.Fail(w, http.StatusConflict, "conflict", "This shortcode is already verified; revoke is not supported through the API")
	default:
		httpx.Fail(w, http.StatusInternalServerError, "internal", msg)
	}
}

func (h *Handler) adminListShortcodes(w http.ResponseWriter, r *http.Request) {
	out, err := h.Shortcodes.List(r.Context(), r.URL.Query().Get("status"), limitParam(r))
	if err != nil {
		failShortcode(w, err, "Could not list shortcodes")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) adminGetLetter(w http.ResponseWriter, r *http.Request) {
	id, ok := shortcodeIDParam(w, r)
	if !ok {
		return
	}
	row, err := h.Shortcodes.Get(r.Context(), id)
	if err != nil {
		failShortcode(w, err, "Could not load the shortcode")
		return
	}
	if !row.AuthorizationLetterUploaded {
		httpx.Fail(w, http.StatusNotFound, "not_found", "No letter has been uploaded for this shortcode")
		return
	}
	// The letter's storage key is not exposed on the API view; fetch the raw
	// row (admin scope) to get it.
	var path string
	err = h.Shortcodes.DB.WithAdmin(r.Context(), func(ctx context.Context, tx db.Tx) error {
		sc, err := tx.GetShortcodeAdmin(ctx, id)
		if err != nil {
			return err
		}
		if sc.MpesaShortcode.AuthorizationLetterPath == nil {
			return errors.New("admin: no letter path")
		}
		path = *sc.MpesaShortcode.AuthorizationLetterPath
		return nil
	})
	if err != nil {
		failShortcode(w, err, "Could not load the letter")
		return
	}
	f, err := h.Shortcodes.Files.Open(r.Context(), path)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "not_found", "Letter file is missing")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", contentTypeForExt(filepath.Ext(path)))
	_, _ = io.Copy(w, f)
}

func contentTypeForExt(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func (h *Handler) adminVerifyShortcode(w http.ResponseWriter, r *http.Request) {
	id, ok := shortcodeIDParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	_ = httpx.Decode(r, &in) // body is optional
	actor := adminActor(r)
	out, err := h.Shortcodes.Verify(r.Context(), id, actor, in.Note)
	if err != nil {
		failShortcode(w, err, "Could not verify the shortcode")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) adminRejectShortcode(w http.ResponseWriter, r *http.Request) {
	id, ok := shortcodeIDParam(w, r)
	if !ok {
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if err := httpx.Decode(r, &in); err != nil || len(in.Reason) < 3 || len(in.Reason) > 500 {
		httpx.Fail(w, http.StatusUnprocessableEntity, "validation", "reason must be 3-500 characters")
		return
	}
	actor := adminActor(r)
	out, err := h.Shortcodes.Reject(r.Context(), id, actor, in.Reason)
	if err != nil {
		failShortcode(w, err, "Could not reject the shortcode")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func adminActor(r *http.Request) string {
	if p, ok := httpx.PrincipalFrom(r.Context()); ok {
		return p.UserID.String()
	}
	return "admin"
}
