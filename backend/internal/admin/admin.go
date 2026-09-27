// Package admin is the CiftPay back-office: org list, the webhook and fiscal
// dead-letter views, feature flags and the shortcode authorization queue
// (ADR-0008). Routes require the admin role.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/ledger"
	"github.com/exoin/ciftpay/internal/mpesa"
	"github.com/exoin/ciftpay/internal/notify"
	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/httpx"
)

// OrgRow is a back-office org summary.
type OrgRow struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	DarajaStatus   string    `json:"daraja_status"`
	VATRegistered  bool      `json:"vat_registered"`
	KRAInitialized bool      `json:"kra_initialized"`
	Tier           string    `json:"tier"`
	CreatedAt      time.Time `json:"created_at"`
}

// DeadWebhook is a webhook event that never completed.
type DeadWebhook struct {
	ID         uuid.UUID `json:"id"`
	Provider   string    `json:"provider"`
	Kind       string    `json:"kind"`
	ExternalID string    `json:"external_id"`
	Error      string    `json:"error"`
	Attempts   int32     `json:"attempts"`
	CreatedAt  time.Time `json:"created_at"`
}

// FlagRow is a feature flag and its override list.
type FlagRow struct {
	Key          string      `json:"key"`
	Description  string      `json:"description"`
	Enabled      bool        `json:"enabled"`
	OrgAllowlist []uuid.UUID `json:"org_allowlist"`
}

// SafaricomExportRow holds exported merchant details with authorization letter link.
type SafaricomExportRow struct {
	OrgID                  uuid.UUID `json:"org_id"`
	OrgName                string    `json:"org_name"`
	DarajaStatus           string    `json:"daraja_status"`
	ShortcodeID            uuid.UUID `json:"shortcode_id"`
	Shortcode              string    `json:"shortcode"`
	ShortcodeKind          string    `json:"shortcode_kind"`
	AuthorizationLetterURL string    `json:"authorization_letter_url"`
}

// DLQInvoiceRow represents a stuck or failed terminal invoice in the fiscal queue.
type DLQInvoiceRow struct {
	ID              uuid.UUID       `json:"id"`
	OrgID           uuid.UUID       `json:"org_id"`
	OrgName         string          `json:"org_name"`
	State           string          `json:"state"`
	TotalCents      int64           `json:"total_cents"`
	ReceiptCode     string          `json:"receipt_code"`
	LastError       string          `json:"last_error"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Attempt         *int32          `json:"attempt,omitempty"`
	SubmissionError *string         `json:"submission_error,omitempty"`
	RawResponse     json.RawMessage `json:"raw_response,omitempty"`
	Classification  *string         `json:"classification,omitempty"`
}

// WebhookEventRow is a recent webhook event for inspection.
type WebhookEventRow struct {
	ID          uuid.UUID       `json:"id"`
	Provider    string          `json:"provider"`
	Kind        string          `json:"kind"`
	ExternalID  string          `json:"external_id"`
	Payload     json.RawMessage `json:"payload"`
	ReceivedAt  time.Time       `json:"received_at"`
	ProcessedAt *time.Time      `json:"processed_at,omitempty"`
	Error       string          `json:"error,omitempty"`
}

// Service provides back-office queries.
type Service struct {
	DB *db.DB
}

// New constructs the admin service.
func New(d *db.DB) *Service { return &Service{DB: d} }

// Orgs lists recent organisations with daraja_status, tier, and KRA status.
func (s *Service) Orgs(ctx context.Context, limit int32) ([]OrgRow, error) {
	var out []OrgRow
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Tx.Query(ctx,
			`SELECT
			   o.id,
			   o.name,
			   o.status,
			   COALESCE(o.daraja_status, 'pending_upload'),
			   o.vat_registered,
			   o.kra_pin_verified_at IS NOT NULL,
			   COALESCE(sub.plan_code, 'hustler'),
			   o.created_at
			 FROM orgs o
			 LEFT JOIN subscriptions sub ON sub.org_id = o.id
			 ORDER BY o.created_at DESC
			 LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r OrgRow
			if err := rows.Scan(&r.ID, &r.Name, &r.Status, &r.DarajaStatus, &r.VATRegistered, &r.KRAInitialized, &r.Tier, &r.CreatedAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// UpdateOrg updates an org's daraja_status and tier.
func (s *Service) UpdateOrg(ctx context.Context, id uuid.UUID, darajaStatus, tier string) (*OrgRow, error) {
	if darajaStatus != "" {
		switch darajaStatus {
		case "pending_upload", "ready_for_safaricom", "sent_to_safaricom", "live":
		default:
			return nil, fmt.Errorf("invalid daraja_status %q", darajaStatus)
		}
	}
	if tier != "" {
		switch tier {
		case "hustler", "duka", "biashara", "accountant":
		default:
			return nil, fmt.Errorf("invalid tier %q", tier)
		}
	}

	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		if darajaStatus != "" {
			ct, err := tx.Tx.Exec(ctx, "UPDATE orgs SET daraja_status = $2, updated_at = now() WHERE id = $1", id, darajaStatus)
			if err != nil {
				return err
			}
			if ct.RowsAffected() == 0 {
				return errors.New("org not found")
			}
		}
		if tier != "" {
			_, err := tx.Tx.Exec(ctx, `
				INSERT INTO subscriptions (org_id, plan_code, status)
				VALUES ($1, $2, 'active')
				ON CONFLICT (org_id) DO UPDATE SET plan_code = EXCLUDED.plan_code, updated_at = now()`,
				id, tier)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var row OrgRow
	err = s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx,
			`SELECT
			   o.id,
			   o.name,
			   o.status,
			   COALESCE(o.daraja_status, 'pending_upload'),
			   o.vat_registered,
			   o.kra_pin_verified_at IS NOT NULL,
			   COALESCE(sub.plan_code, 'hustler'),
			   o.created_at
			 FROM orgs o
			 LEFT JOIN subscriptions sub ON sub.org_id = o.id
			 WHERE o.id = $1`, id).
			Scan(&row.ID, &row.Name, &row.Status, &row.DarajaStatus, &row.VATRegistered, &row.KRAInitialized, &row.Tier, &row.CreatedAt)
	})
	return &row, err
}

// BatchSafaricomExport fetches all ready_for_safaricom orgs and their authorization letters.
func (s *Service) BatchSafaricomExport(ctx context.Context) ([]SafaricomExportRow, error) {
	var out []SafaricomExportRow
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Tx.Query(ctx,
			`SELECT
			   o.id,
			   o.name,
			   COALESCE(o.daraja_status, 'pending_upload'),
			   s.id,
			   s.shortcode,
			   s.kind
			 FROM orgs o
			 JOIN mpesa_shortcodes s ON s.org_id = o.id
			 WHERE o.daraja_status = 'ready_for_safaricom'
			   AND s.authorization_letter_path IS NOT NULL
			 ORDER BY o.name ASC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r SafaricomExportRow
			if err := rows.Scan(&r.OrgID, &r.OrgName, &r.DarajaStatus, &r.ShortcodeID, &r.Shortcode, &r.ShortcodeKind); err != nil {
				return err
			}
			r.AuthorizationLetterURL = fmt.Sprintf("/admin/shortcodes/%s/authorization", r.ShortcodeID)
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// DLQInvoices lists stuck or failed fiscal invoices.
func (s *Service) DLQInvoices(ctx context.Context, limit int32) ([]DLQInvoiceRow, error) {
	var out []DLQInvoiceRow
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Tx.Query(ctx,
			`SELECT
			   i.id,
			   i.org_id,
			   o.name,
			   i.state,
			   i.total_cents,
			   i.receipt_code,
			   COALESCE(i.last_error, ''),
			   i.created_at,
			   i.updated_at,
			   fs.attempt,
			   fs.error,
			   fs.response,
			   fs.classification
			 FROM invoices i
			 JOIN orgs o ON o.id = i.org_id
			 LEFT JOIN LATERAL (
			   SELECT attempt, error, response, classification
			   FROM fiscal_submissions
			   WHERE invoice_id = i.id
			   ORDER BY attempt DESC
			   LIMIT 1
			 ) fs ON true
			 WHERE i.state IN ('FAILED_TERMINAL', 'NEEDS_REVIEW')
			 ORDER BY i.updated_at DESC
			 LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r DLQInvoiceRow
			if err := rows.Scan(&r.ID, &r.OrgID, &r.OrgName, &r.State, &r.TotalCents, &r.ReceiptCode,
				&r.LastError, &r.CreatedAt, &r.UpdatedAt, &r.Attempt, &r.SubmissionError, &r.RawResponse, &r.Classification); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// RequeueInvoice resets stuck invoices back to QUEUED for immediate reprocessing.
func (s *Service) RequeueInvoice(ctx context.Context, id uuid.UUID) error {
	return s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		res, err := tx.Tx.Exec(ctx,
			`UPDATE invoices
			    SET state = 'QUEUED', next_attempt_at = now(), updated_at = now()
			  WHERE id = $1 AND state IN ('FAILED_TERMINAL', 'NEEDS_REVIEW', 'FAILED_RETRYABLE')`, id)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return errors.New("invoice not found or not in retryable state")
		}
		return nil
	})
}

// ForceAcked overrides invoice state to ACKED with operator-provided KRA signature.
func (s *Service) ForceAcked(ctx context.Context, invoiceID uuid.UUID, rcptNo, rcptSign string) error {
	return s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		res, err := tx.Tx.Exec(ctx,
			`UPDATE invoices
			    SET state = 'ACKED',
			        kra_invoice_no = $2,
			        kra_signature = $3,
			        acked_at = COALESCE(acked_at, now()),
			        updated_at = now()
			  WHERE id = $1`, invoiceID, rcptNo, rcptSign)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return errors.New("invoice not found")
		}
		return nil
	})
}

// Webhooks lists recent webhook events with optional TransID filter.
func (s *Service) Webhooks(ctx context.Context, transID string, limit int32) ([]WebhookEventRow, error) {
	var out []WebhookEventRow
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		var (
			rows interface {
				Close()
				Err() error
				Next() bool
				Scan(dest ...any) error
			}
			err error
		)
		if transID != "" {
			rows, err = tx.Tx.Query(ctx,
				`SELECT id, provider, kind, external_id, payload, received_at, processed_at, COALESCE(error, '')
				   FROM webhook_events
				  WHERE external_id = $1 OR payload->>'TransID' = $1
				  ORDER BY received_at DESC
				  LIMIT $2`, transID, limit)
		} else {
			rows, err = tx.Tx.Query(ctx,
				`SELECT id, provider, kind, external_id, payload, received_at, processed_at, COALESCE(error, '')
				   FROM webhook_events
				  ORDER BY received_at DESC
				  LIMIT $1`, limit)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r WebhookEventRow
			if err := rows.Scan(&r.ID, &r.Provider, &r.Kind, &r.ExternalID, &r.Payload, &r.ReceivedAt, &r.ProcessedAt, &r.Error); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// ReplayWebhook re-runs the raw payload through the appropriate ingester.
func (s *Service) ReplayWebhook(ctx context.Context, id uuid.UUID, ingest mpesa.Ingester) (any, error) {
	if ingest == nil {
		return nil, errors.New("ingest service not available")
	}
	var (
		provider string
		kind     string
		payload  []byte
	)
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx,
			"SELECT provider, kind, payload FROM webhook_events WHERE id = $1", id).
			Scan(&provider, &kind, &payload)
	})
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, errors.New("webhook event has no payload")
	}

	var ingestErr error
	var result any

	switch kind {
	case "c2b_confirmation":
		var p mpesa.C2BPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("unmarshal c2b payload: %w", err)
		}
		in := mpesa.ToC2BInput(p, payload)
		res, err := ingest.IngestC2B(ctx, in)
		ingestErr = err
		result = res
	case "stk_callback":
		var cb mpesa.STKCallback
		if err := json.Unmarshal(payload, &cb); err != nil {
			return nil, fmt.Errorf("unmarshal stk payload: %w", err)
		}
		res, err := cb.Flatten()
		if err != nil {
			return nil, fmt.Errorf("flatten stk callback: %w", err)
		}
		in := ledger.STKInput{
			CheckoutRequestID: res.CheckoutRequestID, MerchantRequestID: res.MerchantRequestID,
			ResultCode: res.ResultCode, ResultDesc: res.ResultDesc, Success: res.Success,
			AmountCents: res.AmountCents, ReceiptNo: res.ReceiptNo, MSISDN: res.MSISDN,
			PaidAt: res.PaidAt, Raw: payload,
		}
		ingestErr = ingest.IngestSTK(ctx, in)
		result = map[string]any{"status": "ok"}
	case "reversal":
		var p mpesa.ReversalPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("unmarshal reversal payload: %w", err)
		}
		origID := p.ExtractOriginalTransID()
		if origID == "" {
			return nil, errors.New("missing original transaction id")
		}
		reason := p.Reason
		if reason == "" {
			reason = "M-Pesa Automated Reversal"
		}
		ingestErr = ingest.IngestReversal(ctx, origID, reason)
		result = map[string]any{"status": "ok"}
	default:
		return nil, fmt.Errorf("unsupported replay kind %q", kind)
	}

	_ = s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		var errMsg *string
		if ingestErr != nil {
			s := ingestErr.Error()
			errMsg = &s
		}
		_, err := tx.Tx.Exec(ctx,
			"UPDATE webhook_events SET processed_at = now(), error = $2 WHERE id = $1", id, errMsg)
		return err
	})

	if ingestErr != nil {
		return nil, ingestErr
	}
	return result, nil
}

// NotifyMerchant triggers a notification (SMS/email) to an organization's owner.
func (s *Service) NotifyMerchant(ctx context.Context, orgID uuid.UUID, message, channel string, keys *crypto.Keyring, notifier *notify.Service) error {
	if channel == "" {
		channel = "sms"
	}
	if channel != "sms" && channel != "email" {
		return fmt.Errorf("unsupported channel %q", channel)
	}
	if strings.TrimSpace(message) == "" {
		return errors.New("message cannot be empty")
	}

	var msisdnEnc []byte
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Tx.QueryRow(ctx,
			`SELECT u.msisdn_enc
			   FROM memberships m
			   JOIN users u ON u.id = m.user_id
			  WHERE m.org_id = $1 AND m.role = 'owner'
			  LIMIT 1`, orgID).Scan(&msisdnEnc)
	})
	if err != nil {
		return fmt.Errorf("lookup org owner: %w", err)
	}

	var phone string
	if len(msisdnEnc) > 0 && keys != nil {
		p, err := keys.DecryptString(msisdnEnc)
		if err == nil {
			phone = p
		}
	}

	if channel == "sms" && notifier != nil && notifier.Sender != nil && phone != "" {
		if _, err := notifier.Sender.SendSMS(ctx, notify.E164(phone), message); err != nil {
			return fmt.Errorf("send sms: %w", err)
		}
	}

	_ = s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := tx.Tx.Exec(ctx,
			`INSERT INTO notifications (org_id, channel, template, locale, body, status, sent_at)
			 VALUES ($1, $2, 'admin_broadcast', 'en', $3, 'sent', now())`,
			orgID, channel, message)
		return err
	})

	return nil
}

// DeadWebhooks lists webhook events that failed past their retry limit.
func (s *Service) DeadWebhooks(ctx context.Context, limit int32) ([]DeadWebhook, error) {
	var out []DeadWebhook
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Tx.Query(ctx,
			`SELECT id, provider, kind, external_id, coalesce(last_error,''), attempts, created_at
			   FROM webhook_events
			  WHERE status = 'dead'
			  ORDER BY created_at DESC LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r DeadWebhook
			if err := rows.Scan(&r.ID, &r.Provider, &r.Kind, &r.ExternalID, &r.Error, &r.Attempts, &r.CreatedAt); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// Flags returns all feature flags.
func (s *Service) Flags(ctx context.Context) ([]FlagRow, error) {
	var out []FlagRow
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.Tx.Query(ctx,
			"SELECT key, description, enabled, org_allowlist FROM feature_flags ORDER BY key")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r FlagRow
			if err := rows.Scan(&r.Key, &r.Description, &r.Enabled, &r.OrgAllowlist); err != nil {
				return err
			}
			out = append(out, r)
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
type Handler struct {
	S               *Service
	Shortcodes      *Shortcodes
	DarajaAllowlist *httpx.DynamicAllowlist
	Keys            *crypto.Keyring
	Notifier        *notify.Service
	Ingest          mpesa.Ingester
}

// Mount registers the routes.
func (h *Handler) Mount(r chi.Router) {
	r.Get("/admin/orgs", h.orgs)
	r.Patch("/admin/orgs/{id}", h.updateOrg)
	r.Post("/admin/orgs/batch-safaricom-export", h.batchSafaricomExport)
	r.Post("/admin/orgs/{id}/notify", h.notifyMerchant)

	r.Get("/admin/invoices/dlq", h.dlqInvoices)
	r.Post("/admin/invoices/{id}/requeue", h.requeueInvoice)
	r.Post("/admin/invoices/{id}/force-acked", h.forceAcked)

	r.Get("/admin/webhooks", h.webhooks)
	r.Post("/admin/webhooks/{id}/replay", h.replayWebhook)

	r.Get("/admin/webhooks/dead", h.dead)
	r.Get("/admin/flags", h.flags)
	r.Get("/admin/system/daraja-ips", h.getDarajaIPs)
	r.Post("/admin/system/daraja-ips", h.updateDarajaIPs)
	if h.Shortcodes != nil {
		h.MountShortcodes(r)
	}
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func limitParam(r *http.Request) int32 {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 || n > 500 {
		return 100
	}
	return int32(n)
}

func (h *Handler) orgs(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	out, err := h.S.Orgs(r.Context(), limitParam(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list organisations")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) updateOrg(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid org ID")
		return
	}

	var req struct {
		DarajaStatus string `json:"daraja_status"`
		Tier         string `json:"tier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}

	row, err := h.S.UpdateOrg(r.Context(), id, req.DarajaStatus, req.Tier)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": row})
}

func (h *Handler) batchSafaricomExport(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	out, err := h.S.BatchSafaricomExport(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not export batch")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) notifyMerchant(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid org ID")
		return
	}

	var req struct {
		Message string `json:"message"`
		Channel string `json:"channel"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Message cannot be empty")
		return
	}

	if err := h.S.NotifyMerchant(r.Context(), id, req.Message, req.Channel, h.Keys, h.Notifier); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "sent"}})
}

func (h *Handler) dlqInvoices(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	out, err := h.S.DLQInvoices(r.Context(), limitParam(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list DLQ invoices")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) requeueInvoice(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid invoice ID")
		return
	}
	if err := h.S.RequeueInvoice(r.Context(), id); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "requeued", "id": id}})
}

func (h *Handler) forceAcked(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid invoice ID")
		return
	}

	var req struct {
		KraReceiptNo   string `json:"kra_receipt_no"`
		KraReceiptSign string `json:"kra_receipt_sign"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if strings.TrimSpace(req.KraReceiptNo) == "" || strings.TrimSpace(req.KraReceiptSign) == "" {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Both kra_receipt_no and kra_receipt_sign are required")
		return
	}

	if err := h.S.ForceAcked(r.Context(), id, req.KraReceiptNo, req.KraReceiptSign); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":               id,
			"state":            "ACKED",
			"kra_receipt_no":   req.KraReceiptNo,
			"kra_receipt_sign": req.KraReceiptSign,
		},
	})
}

func (h *Handler) webhooks(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	transID := r.URL.Query().Get("trans_id")
	out, err := h.S.Webhooks(r.Context(), transID, limitParam(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list webhooks")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) replayWebhook(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Invalid webhook event ID")
		return
	}

	res, err := h.S.ReplayWebhook(r.Context(), id, h.Ingest)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": map[string]any{"status": "replayed", "result": res}})
}

func (h *Handler) dead(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	out, err := h.S.DeadWebhooks(r.Context(), limitParam(r))
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list webhook events")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) flags(w http.ResponseWriter, r *http.Request) {
	if h.S == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Admin service not configured")
		return
	}
	out, err := h.S.Flags(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list flags")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"data": out})
}

func (h *Handler) getDarajaIPs(w http.ResponseWriter, r *http.Request) {
	if h.DarajaAllowlist == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Daraja dynamic allowlist is not configured")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"cidrs": h.DarajaAllowlist.Get(),
		},
	})
}

func (h *Handler) updateDarajaIPs(w http.ResponseWriter, r *http.Request) {
	if h.DarajaAllowlist == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "unavailable", "Daraja dynamic allowlist is not configured")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "Failed to read request body")
		return
	}

	var cidrs []string
	if err := json.Unmarshal(body, &cidrs); err != nil {
		var obj struct {
			CIDRs []string `json:"cidrs"`
			IPs   []string `json:"ips"`
		}
		if err2 := json.Unmarshal(body, &obj); err2 != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid_json", "Body must be a JSON array of CIDRs or {\"cidrs\": [...]}")
			return
		}
		if len(obj.CIDRs) > 0 {
			cidrs = obj.CIDRs
		} else {
			cidrs = obj.IPs
		}
	}

	// Validate all CIDR strings
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		cidr := c
		if !strings.Contains(cidr, "/") {
			cidr += "/32"
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			httpx.Fail(w, http.StatusBadRequest, "invalid_cidr", fmt.Sprintf("Invalid CIDR format %q", c))
			return
		}
	}

	if err := h.DarajaAllowlist.Set(cidrs); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid_cidr", err.Error())
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"cidrs": h.DarajaAllowlist.Get(),
		},
	})
}
