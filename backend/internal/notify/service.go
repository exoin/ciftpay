package notify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/httpx"
	"github.com/exoin/ciftpay/internal/platform/jobs"
	plog "github.com/exoin/ciftpay/internal/platform/log"
)

// ErrNoRecipient is returned when neither the payment nor the customer has a
// phone number to send the receipt to.
var ErrNoRecipient = errors.New("notify: invoice has no recipient")

// Service renders and sends notifications.
type Service struct {
	DB            *db.DB
	Keys          *crypto.Keyring
	Sender        Sender
	PublicBaseURL string // where /r/{code} lives (the web app)
	Log           *slog.Logger
}

// New builds a Service.
func New(d *db.DB, k *crypto.Keyring, s Sender, publicBaseURL string, l *slog.Logger) *Service {
	return &Service{DB: d, Keys: k, Sender: s, PublicBaseURL: strings.TrimRight(publicBaseURL, "/"), Log: l}
}

// ReceiptURL is the public verify link for a receipt code.
func (s *Service) ReceiptURL(code string) string { return s.PublicBaseURL + "/r/" + code }

// stillPending reports whether an invoice is still on its way to KRA, i.e. the
// buyer has not received (and will not shortly receive) the final receipt.
func stillPending(state string) bool {
	switch state {
	case "ACKED", "FAILED_TERMINAL":
		return false
	}
	return true
}

// ErrSkipped is returned when a scheduled receipt template is no longer
// relevant (the "pending" SMS for an invoice KRA has already acked).
var ErrSkipped = errors.New("notify: skipped")

// SendReceipt sends the given template for an invoice and records the
// notification. It is idempotent per (invoice, template): a second call finds
// the earlier row and does nothing. The "pending" template is only sent while
// the invoice is still waiting on KRA; otherwise ErrSkipped is returned.
func (s *Service) SendReceipt(ctx context.Context, orgID, invoiceID uuid.UUID, template string) (gen.Notification, error) {
	var (
		n      gen.Notification
		to     string
		body   string
		exists bool
	)
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		prior, err := tx.ListNotificationsForInvoice(ctx, &invoiceID)
		if err != nil {
			return err
		}
		for _, p := range prior {
			if p.Template == template && p.Status != "failed" {
				n, exists = p, true
				return nil
			}
		}
		inv, err := tx.GetInvoice(ctx, invoiceID)
		if err != nil {
			return err
		}
		if template == TemplateReceiptPending && !stillPending(inv.State) {
			return ErrSkipped
		}
		org, err := tx.GetOrg(ctx, inv.OrgID)
		if err != nil {
			return err
		}
		msisdnEnc, msisdnHash, err := s.recipient(ctx, tx, inv)
		if err != nil {
			return err
		}
		to, err = s.Keys.DecryptString(msisdnEnc)
		if err != nil {
			return fmt.Errorf("notify: decrypt recipient: %w", err)
		}
		kra := ""
		if inv.KraInvoiceNo != nil {
			kra = *inv.KraInvoiceNo
		}
		body, err = Render(template, org.Locale, Vars{
			Merchant: org.Name, AmountKES: FormatKES(abs(inv.TotalCents)), KRAInvoice: kra, ReceiptURL: s.ReceiptURL(inv.ReceiptCode),
		})
		if err != nil {
			return err
		}
		n, err = tx.CreateNotification(ctx, gen.CreateNotificationParams{
			OrgID: orgID, InvoiceID: &invoiceID, Channel: "sms", ToMsisdnEnc: msisdnEnc, ToMsisdnHash: msisdnHash,
			Template: template, Locale: org.Locale, Body: body,
		})
		return err
	})
	if err != nil || exists {
		return n, err
	}

	res, sendErr := s.Sender.SendSMS(ctx, E164(to), body)
	err = s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		if sendErr != nil {
			return tx.MarkNotificationFailed(ctx, n.ID)
		}
		if err := tx.MarkNotificationSent(ctx, gen.MarkNotificationSentParams{ID: n.ID, ProviderMessageID: db.Ptr(res.MessageID), CostCents: res.CostCents}); err != nil {
			return err
		}
		return tx.BumpUsage(ctx, gen.BumpUsageParams{OrgID: orgID, Period: time.Now().Format("2006-01"), SmsSent: 1})
	})
	if err != nil {
		return n, err
	}
	if sendErr != nil {
		s.Log.Warn("receipt send failed", "invoice", invoiceID, "template", template, "err", sendErr, plog.Redact("to", to))
		return n, sendErr
	}
	s.Log.Info("receipt sent", "invoice", invoiceID, "template", template, "message_id", res.MessageID, plog.Redact("to", to))
	return n, nil
}

// recipient prefers the paying MSISDN, then the customer on file.
func (s *Service) recipient(ctx context.Context, tx db.Tx, inv gen.Invoice) (enc, hash []byte, err error) {
	if inv.PaymentID != nil {
		if p, err := tx.GetPayment(ctx, *inv.PaymentID); err == nil && len(p.MsisdnEnc) > 0 {
			return p.MsisdnEnc, p.MsisdnHash, nil
		}
	}
	sale, err := tx.GetSale(ctx, inv.SaleID)
	if err != nil {
		return nil, nil, err
	}
	if sale.CustomerID != nil {
		if c, err := tx.FindCustomerByID(ctx, *sale.CustomerID); err == nil && len(c.MsisdnEnc) > 0 {
			return c.MsisdnEnc, c.MsisdnHash, nil
		}
	}
	return nil, nil, ErrNoRecipient
}

// SendOTP delivers a login code. It is not stored in notifications (no org).
func (s *Service) SendOTP(ctx context.Context, msisdn, code, locale string) error {
	body, err := Render(TemplateOTP, locale, Vars{Code: code})
	if err != nil {
		return err
	}
	_, err = s.Sender.SendSMS(ctx, E164(msisdn), body)
	return err
}

// Worker adapts SendReceipt to River.
type Worker struct {
	river.WorkerDefaults[jobs.SendReceiptArgs]
	S *Service
}

// Work implements river.Worker. Permanent gateway errors and missing
// recipients cancel the job; anything else is retried by River.
func (w *Worker) Work(ctx context.Context, job *river.Job[jobs.SendReceiptArgs]) error {
	_, err := w.S.SendReceipt(ctx, job.Args.OrgID, job.Args.InvoiceID, job.Args.Template)
	if errors.Is(err, ErrSkipped) {
		return nil
	}
	var perm *PermanentError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNoRecipient), errors.As(err, &perm):
		return river.JobCancel(err)
	}
	return err
}

// DeliveryWebhook receives Africa's Talking delivery reports
// (POST /webhooks/at/delivery, form-encoded: id, status, phoneNumber, ...).
type DeliveryWebhook struct {
	DB  *db.DB
	Log *slog.Logger
}

// Mount registers the route.
func (h *DeliveryWebhook) Mount(r chi.Router) {
	r.With(httpx.RateLimit(600, time.Minute, func(r *http.Request) string { return "at:" + httpx.ClientIP(r) })).
		Post("/at/delivery", h.handle)
}

func (h *DeliveryWebhook) handle(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "form body expected")
		return
	}
	id, status := r.Form.Get("id"), r.Form.Get("status")
	if id == "" {
		httpx.Fail(w, http.StatusBadRequest, "bad_request", "id is required")
		return
	}
	// Delivery reports carry no org, so the update runs under the ingest
	// scope (policy notifications_delivery in 0001_init.sql).
	if strings.EqualFold(status, "Success") || strings.EqualFold(status, "Delivered") {
		if err := h.DB.WithIngest(r.Context(), func(ctx context.Context, tx db.Tx) error {
			return tx.MarkNotificationDelivered(ctx, &id)
		}); err != nil {
			h.Log.Warn("at delivery update failed", "message_id", id, "err", err)
		}
	}
	h.Log.Info("at delivery report", "message_id", id, "status", status)
	w.WriteHeader(http.StatusOK)
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
