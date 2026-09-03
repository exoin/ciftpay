// Package admin is the CiftPay back-office: org list, the webhook and fiscal
// dead-letter views and feature flags. Routes require the admin role.
package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/httpx"
)

// OrgRow is a back-office org summary.
type OrgRow struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	VATRegistered bool      `json:"vat_registered"`
	CreatedAt     time.Time `json:"created_at"`
}

// DeadWebhook is a webhook event that never completed.
type DeadWebhook struct {
	ID         uuid.UUID `json:"id"`
	Provider   string    `json:"provider"`
	Kind       string    `json:"kind"`
	ExternalID string    `json:"external_id"`
	ReceivedAt time.Time `json:"received_at"`
	Error      *string   `json:"error"`
}

// Flag is a feature flag.
type Flag struct {
	Key          string      `json:"key"`
	Enabled      bool        `json:"enabled"`
	OrgAllowlist []uuid.UUID `json:"org_allowlist"`
}

// Service runs back-office queries on global tables.
type Service struct{ DB *db.DB }

// New builds a Service.
func New(d *db.DB) *Service { return &Service{DB: d} }

// Orgs lists the newest organisations.
func (s *Service) Orgs(ctx context.Context, limit int32) ([]OrgRow, error) {
	out := []OrgRow{}
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListOrgs(ctx, limit)
		if err != nil {
			return err
		}
		for _, o := range rows {
			out = append(out, OrgRow{ID: o.ID, Name: o.Name, Status: o.Status, VATRegistered: o.VatRegistered, CreatedAt: o.CreatedAt})
		}
		return nil
	})
	return out, err
}

// DeadWebhooks lists unprocessed webhook events (the reconcile job's backlog).
func (s *Service) DeadWebhooks(ctx context.Context, limit int32) ([]DeadWebhook, error) {
	out := []DeadWebhook{}
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListUnprocessedWebhookEvents(ctx, limit)
		if err != nil {
			return err
		}
		for _, e := range rows {
			out = append(out, DeadWebhook{ID: e.ID, Provider: e.Provider, Kind: e.Kind, ExternalID: e.ExternalID, ReceivedAt: e.ReceivedAt, Error: e.Error})
		}
		return nil
	})
	return out, err
}

// Flags lists feature flags.
func (s *Service) Flags(ctx context.Context) ([]Flag, error) {
	out := []Flag{}
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Tx.Query(ctx, "SELECT key, enabled, org_allowlist FROM feature_flags ORDER BY key")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var f Flag
			if err := rows.Scan(&f.Key, &f.Enabled, &f.OrgAllowlist); err != nil {
				return err
			}
			out = append(out, f)
		}
		return rows.Err()
	})
	return out, err
}

// Enabled reports whether a flag is on for an org (globally or via allow-list).
func (s *Service) Enabled(ctx context.Context, key string, orgID uuid.UUID) bool {
	var on bool
	_ = s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx,
			"SELECT enabled OR $2 = ANY(org_allowlist) FROM feature_flags WHERE key = $1", key, orgID).Scan(&on)
	})
	return on
}

// Handler serves /admin/* (mount behind RequireRole(admin)).
type Handler struct{ S *Service }

// Mount registers the routes.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/admin/orgs", h.orgs)
	r.Get("/admin/webhooks/dead", h.dead)
	r.Get("/admin/flags", h.flags)
}

func limitParam(r *http.Request) int32 {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return 100
	}
	return int32(n)
}

func (h *Handler) orgs(w http.ResponseWriter, r *http.Request) {
	out, err := h.S.Orgs(r.Context(), limitParam(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list organisations")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) dead(w http.ResponseWriter, r *http.Request) {
	out, err := h.S.DeadWebhooks(r.Context(), limitParam(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list webhook events")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) flags(w http.ResponseWriter, r *http.Request) {
	out, err := h.S.Flags(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list flags")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}
