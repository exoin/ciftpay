// Package publicapi serves the unauthenticated receipt verification endpoint
// GET /r/{code}. It reads under db.WithReceipt, whose RLS policy exposes
// exactly one invoice and its lines.
package publicapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ciftpay/ciftpay/internal/fiscal"
	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	"github.com/ciftpay/ciftpay/internal/platform/httpx"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
)

// ErrNotFound is returned for unknown or malformed codes.
var ErrNotFound = errors.New("publicapi: receipt not found")

// Seller is the merchant block on a receipt.
type Seller struct {
	Name   string `json:"name"`
	KRAPin string `json:"kra_pin"`
}

// Line is one receipt line.
type Line struct {
	ItemID         *uuid.UUID `json:"item_id,omitempty"`
	Description    string     `json:"description"`
	Qty            string     `json:"qty"`
	UnitPriceCents int64      `json:"unit_price_cents"`
	TaxCategory    string     `json:"tax_category"`
	LineTotalCents int64      `json:"line_total_cents"`
	LineTaxCents   int64      `json:"line_tax_cents"`
}

// VATRow is the per-category VAT breakdown.
type VATRow struct {
	Category     string `json:"category"`
	TaxableCents int64  `json:"taxable_cents"`
	TaxCents     int64  `json:"tax_cents"`
}

// Receipt is the PublicReceipt schema.
type Receipt struct {
	ReceiptCode    string     `json:"receipt_code"`
	State          string     `json:"state"` // verified | pending | cancelled
	Kind           string     `json:"kind"`
	Seller         Seller     `json:"seller"`
	KRAInvoiceNo   *string    `json:"kra_invoice_no,omitempty"`
	KRAQRPayload   *string    `json:"kra_qr_payload,omitempty"`
	BuyerPinMasked string     `json:"buyer_pin_masked,omitempty"`
	Lines          []Line     `json:"lines"`
	VATByCategory  []VATRow   `json:"vat_by_category"`
	SubtotalCents  int64      `json:"subtotal_cents"`
	TaxCents       int64      `json:"tax_cents"`
	TotalCents     int64      `json:"total_cents"`
	IssuedAt       time.Time  `json:"issued_at"`
	CreditNoteOf   *uuid.UUID `json:"credit_note_of,omitempty"`
}

// Service loads receipts.
type Service struct {
	DB   *db.DB
	Keys *crypto.Keyring
}

// New builds a Service.
func New(d *db.DB, k *crypto.Keyring) *Service { return &Service{DB: d, Keys: k} }

// Lookup returns the receipt for a public code.
func (s *Service) Lookup(ctx context.Context, rawCode string) (Receipt, error) {
	code, err := fiscal.NormaliseReceiptCode(rawCode)
	if err != nil {
		return Receipt{}, ErrNotFound
	}
	var out Receipt
	err = s.DB.WithReceipt(ctx, code, func(ctx context.Context, tx db.Tx) error {
		row, err := tx.GetInvoiceByReceiptCode(ctx, code)
		if err != nil {
			if db.NotFound(err) {
				return ErrNotFound
			}
			return err
		}
		items, err := tx.ListSaleItems(ctx, row.SaleID)
		if err != nil {
			return err
		}
		out = s.build(row, items)
		return nil
	})
	return out, err
}

func (s *Service) build(row gen.GetInvoiceByReceiptCodeRow, items []gen.SaleItem) Receipt {
	pin, _ := s.Keys.DecryptString(row.OrgPinEnc)
	r := Receipt{
		ReceiptCode: row.ReceiptCode, Kind: row.Kind, Seller: Seller{Name: row.OrgName, KRAPin: pin},
		KRAInvoiceNo: row.KraInvoiceNo, KRAQRPayload: row.KraQrPayload, SubtotalCents: row.SubtotalCents,
		TaxCents: row.TaxCents, TotalCents: row.TotalCents, IssuedAt: row.IssuedAt, CreditNoteOf: row.ParentInvoiceID,
		Lines: make([]Line, 0, len(items)), VATByCategory: []VATRow{},
	}
	switch fiscal.State(row.State) {
	case fiscal.StateAcked:
		r.State = "verified"
	case fiscal.StateNeedsReview, fiscal.StateFailedTerminal:
		r.State = "cancelled"
	default:
		r.State = "pending"
	}
	if len(row.BuyerPinEnc) > 0 {
		if bp, err := s.Keys.DecryptString(row.BuyerPinEnc); err == nil {
			r.BuyerPinMasked = plog.MaskPIN(bp)
		}
	}
	byCat := map[string]*VATRow{}
	order := []string{}
	for _, it := range items {
		r.Lines = append(r.Lines, Line{
			ItemID: it.ItemID, Description: it.Description, Qty: it.Qty, UnitPriceCents: it.UnitPriceCents,
			TaxCategory: it.TaxCategory, LineTotalCents: it.LineTotalCents, LineTaxCents: it.LineTaxCents,
		})
		row, ok := byCat[it.TaxCategory]
		if !ok {
			row = &VATRow{Category: it.TaxCategory}
			byCat[it.TaxCategory] = row
			order = append(order, it.TaxCategory)
		}
		row.TaxableCents += it.LineTotalCents - it.LineTaxCents
		row.TaxCents += it.LineTaxCents
	}
	for _, c := range order {
		r.VATByCategory = append(r.VATByCategory, *byCat[c])
	}
	return r
}

// Handler serves GET /r/{code}.
type Handler struct {
	S   *Service
	Log *slog.Logger
}

// Mount registers the route with a per-IP rate limit (codes are guessable).
func (h *Handler) Mount(r chi.Router) {
	r.With(httpx.RateLimit(120, time.Minute, func(r *http.Request) string { return "receipt:" + httpx.ClientIP(r) })).
		Get("/r/{code}", h.get)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	rc, err := h.S.Lookup(r.Context(), chi.URLParam(r, "code"))
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.Fail(w, http.StatusNotFound, "not_found", "No receipt with that code")
	case err != nil:
		h.Log.Error("receipt lookup failed", "err", err)
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not load the receipt")
	default:
		w.Header().Set("Cache-Control", "public, max-age=60")
		httpx.JSON(w, http.StatusOK, rc)
	}
}
