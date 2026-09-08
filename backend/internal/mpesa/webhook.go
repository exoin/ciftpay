package mpesa

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ciftpay/ciftpay/internal/ledger"
	"github.com/ciftpay/ciftpay/internal/platform/httpx"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
)

// Ingester is what the webhook handlers need from the ledger.
type Ingester interface {
	IngestC2B(ctx context.Context, in ledger.C2BInput) (ledger.C2BResult, error)
	IngestSTK(ctx context.Context, in ledger.STKInput) error
}

// Webhooks serves the Daraja callback URLs.
type Webhooks struct {
	Token       string
	IPAllowlist []string
	Ingest      Ingester
	Log         *slog.Logger
}

// Mount registers the routes on r. The {token} segment is a shared secret in
// the URL because Daraja does not sign callbacks.
func (h *Webhooks) Mount(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(httpx.IPAllowlist(h.IPAllowlist))
		r.Use(httpx.RateLimit(600, time.Minute, func(r *http.Request) string { return "webhook:" + httpx.ClientIP(r) }))
		r.Use(h.requireToken)
		r.Post("/daraja/c2b/validation/{token}", h.validation)
		r.Post("/daraja/c2b/confirmation/{token}", h.confirmation)
		r.Post("/daraja/stk/{token}", h.stk)
	})
}

func (h *Webhooks) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := chi.URLParam(r, "token")
		if h.Token == "" || subtle.ConstantTimeCompare([]byte(tok), []byte(h.Token)) != 1 {
			httpx.Fail(w, http.StatusUnauthorized, "unauthorised", "bad webhook token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// validation runs before the customer is charged. CiftPay never rejects a
// payment (rejecting means the buyer's money bounces); it only observes.
func (h *Webhooks) validation(w http.ResponseWriter, r *http.Request) {
	var p C2BPayload
	if err := decodeBody(r, &p); err != nil {
		h.Log.Warn("c2b validation bad body", "err", err)
	}
	httpx.JSON(w, http.StatusOK, Accepted)
}

func (h *Webhooks) confirmation(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, DarajaAck{ResultCode: 1, ResultDesc: "unreadable body"})
		return
	}
	var p C2BPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		httpx.JSON(w, http.StatusBadRequest, DarajaAck{ResultCode: 1, ResultDesc: "invalid JSON"})
		return
	}
	if err := p.Validate(); err != nil {
		h.Log.Warn("c2b confirmation invalid", "err", err, "trans_id", p.TransID)
		httpx.JSON(w, http.StatusBadRequest, DarajaAck{ResultCode: 1, ResultDesc: err.Error()})
		return
	}
	in := ToC2BInput(p, raw)
	res, err := h.Ingest.IngestC2B(r.Context(), in)
	switch {
	case errors.Is(err, ledger.ErrUnknownShortcode):
		h.Log.Warn("c2b for unknown shortcode", "shortcode", p.BusinessShortCode, "trans_id", p.TransID)
	case err != nil:
		h.Log.Error("c2b ingest failed", "err", err, "trans_id", p.TransID)
		// Still 200: Daraja retries on non-200 and the event is stored; the
		// reconcile job picks up unprocessed events.
	default:
		h.Log.Info("c2b ingested", "trans_id", p.TransID, "org", res.OrgID, "status", res.Status, "rule", res.Rule,
			"duplicate", res.Duplicate, "amount_cents", in.AmountCents, plog.Redact("msisdn", p.MSISDN))
	}
	httpx.JSON(w, http.StatusOK, Accepted)
}

func (h *Webhooks) stk(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, DarajaAck{ResultCode: 1, ResultDesc: "unreadable body"})
		return
	}
	var cb STKCallback
	if err := json.Unmarshal(raw, &cb); err != nil {
		httpx.JSON(w, http.StatusBadRequest, DarajaAck{ResultCode: 1, ResultDesc: "invalid JSON"})
		return
	}
	res, err := cb.Flatten()
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, DarajaAck{ResultCode: 1, ResultDesc: err.Error()})
		return
	}
	in := ledger.STKInput{
		CheckoutRequestID: res.CheckoutRequestID, MerchantRequestID: res.MerchantRequestID, ResultCode: res.ResultCode,
		ResultDesc: res.ResultDesc, Success: res.Success, AmountCents: res.AmountCents, ReceiptNo: res.ReceiptNo,
		MSISDN: res.MSISDN, PaidAt: res.PaidAt, Raw: raw,
	}
	if err := h.Ingest.IngestSTK(r.Context(), in); err != nil {
		h.Log.Error("stk ingest failed", "err", err, "checkout", res.CheckoutRequestID)
	}
	httpx.JSON(w, http.StatusOK, Accepted)
}

// ToC2BInput normalises a validated Daraja payload for the ledger.
func ToC2BInput(p C2BPayload, raw json.RawMessage) ledger.C2BInput {
	amount, _ := ParseAmount(p.TransAmount)
	paidAt, _ := ParseTransTime(p.TransTime)
	in := ledger.C2BInput{
		Kind: "c2b_confirmation", TransID: p.TransID, ShortCode: p.BusinessShortCode, AmountCents: amount,
		MSISDN: p.MSISDN, PayerName: p.PayerName(), BillRef: p.BillRefNumber, PaidAt: paidAt, Raw: raw,
	}
	if p.IsReversal() {
		in.Kind, in.OriginalID = "reversal", p.ThirdPartyTransID
	}
	return in
}

func decodeBody(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}
