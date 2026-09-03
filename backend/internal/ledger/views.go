package ledger

import (
	"time"

	"github.com/google/uuid"

	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
)

// API view types matching api/openapi.yaml schemas.

// ShortcodeView is the Shortcode schema.
type ShortcodeView struct {
	ID            uuid.UUID  `json:"id"`
	Kind          string     `json:"kind"`
	Shortcode     string     `json:"shortcode"`
	Label         string     `json:"label"`
	DefaultItemID *uuid.UUID `json:"default_item_id"`
	AutoInvoice   bool       `json:"auto_invoice"`
	Verified      bool       `json:"verified"`
	VerifiedAt    *time.Time `json:"verified_at"`
}

// PaymentView is the Payment schema.
type PaymentView struct {
	ID                uuid.UUID  `json:"id"`
	ShortcodeID       uuid.UUID  `json:"shortcode_id"`
	TransID           string     `json:"trans_id"`
	AmountCents       int64      `json:"amount_cents"`
	PayerMsisdnMasked string     `json:"payer_msisdn_masked"`
	PayerName         string     `json:"payer_name"`
	BillRef           string     `json:"bill_ref"`
	PaidAt            time.Time  `json:"paid_at"`
	Status            string     `json:"status"`
	MatchRule         *string    `json:"match_rule"`
	SaleID            *uuid.UUID `json:"sale_id"`
	InvoiceID         *uuid.UUID `json:"invoice_id"`
}

// ItemView is the Item schema.
type ItemView struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	EtimsClassCode string    `json:"etims_class_code"`
	TaxCategory    string    `json:"tax_category"`
	Unit           string    `json:"unit"`
	PriceCents     int64     `json:"price_cents"`
	IsActive       bool      `json:"is_active"`
}

// SaleLineView is the SaleLine schema.
type SaleLineView struct {
	ItemID         *uuid.UUID `json:"item_id"`
	Description    string     `json:"description"`
	Qty            string     `json:"qty"`
	UnitPriceCents int64      `json:"unit_price_cents"`
	TaxCategory    string     `json:"tax_category"`
	LineTotalCents int64      `json:"line_total_cents"`
	LineTaxCents   int64      `json:"line_tax_cents"`
}

// SaleView is the Sale schema.
type SaleView struct {
	ID            uuid.UUID      `json:"id"`
	Ref           string         `json:"ref"`
	Kind          string         `json:"kind"`
	Status        string         `json:"status"`
	CustomerID    *uuid.UUID     `json:"customer_id"`
	SubtotalCents int64          `json:"subtotal_cents"`
	TaxCents      int64          `json:"tax_cents"`
	TotalCents    int64          `json:"total_cents"`
	ClientRef     *string        `json:"client_ref"`
	PaidAt        *time.Time     `json:"paid_at"`
	CreatedAt     time.Time      `json:"created_at"`
	Lines         []SaleLineView `json:"lines,omitempty"`
}

// InvoiceView is the Invoice schema.
type InvoiceView struct {
	ID                uuid.UUID  `json:"id"`
	Kind              string     `json:"kind"`
	ParentInvoiceID   *uuid.UUID `json:"parent_invoice_id"`
	SaleID            uuid.UUID  `json:"sale_id"`
	PaymentID         *uuid.UUID `json:"payment_id"`
	State             string     `json:"state"`
	Attempt           int32      `json:"attempt"`
	ReceiptCode       string     `json:"receipt_code"`
	ReceiptURL        string     `json:"receipt_url"`
	KRAInvoiceNo      *string    `json:"kra_invoice_no"`
	BuyerPinMasked    string     `json:"buyer_pin_masked,omitempty"`
	BuyerMsisdnMasked string     `json:"buyer_msisdn_masked,omitempty"`
	TotalCents        int64      `json:"total_cents"`
	TaxCents          int64      `json:"tax_cents"`
	LastError         *string    `json:"last_error"`
	CreatedAt         time.Time  `json:"created_at"`
	SubmittedAt       *time.Time `json:"submitted_at"`
	AckedAt           *time.Time `json:"acked_at"`
}

// SubmissionView is the FiscalSubmission schema.
type SubmissionView struct {
	Attempt        int32      `json:"attempt"`
	Adapter        string     `json:"adapter"`
	Classification string     `json:"classification"`
	Error          *string    `json:"error"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
}

// InvoiceDetailView is the InvoiceDetail schema.
type InvoiceDetailView struct {
	InvoiceView
	Lines        []SaleLineView   `json:"lines"`
	Seller       SellerView       `json:"seller"`
	KRAQRPayload *string          `json:"kra_qr_payload"`
	Submissions  []SubmissionView `json:"submissions"`
}

// SellerView is the Seller schema.
type SellerView struct {
	Name   string `json:"name"`
	KRAPin string `json:"kra_pin"`
}

// AttentionView is the Attention schema.
type AttentionView struct {
	FailedInvoices       []InvoiceView   `json:"failed_invoices"`
	UnmatchedPayments    []PaymentView   `json:"unmatched_payments"`
	UnverifiedShortcodes []ShortcodeView `json:"unverified_shortcodes"`
	PendingLong          []InvoiceView   `json:"pending_long"`
	ActionableCount      int             `json:"actionable_count"`
}

func toShortcode(s gen.MpesaShortcode) ShortcodeView {
	return ShortcodeView{ID: s.ID, Kind: s.Kind, Shortcode: s.Shortcode, Label: s.Label, DefaultItemID: s.DefaultItemID,
		AutoInvoice: s.AutoInvoice, Verified: s.VerifiedAt != nil, VerifiedAt: s.VerifiedAt}
}

func toPayment(k *crypto.Keyring, p gen.Payment, invoiceID *uuid.UUID) PaymentView {
	masked := ""
	if len(p.MsisdnEnc) > 0 {
		if m, err := k.DecryptString(p.MsisdnEnc); err == nil {
			masked = plog.MaskMSISDN(m)
		}
	}
	return PaymentView{ID: p.ID, ShortcodeID: p.ShortcodeID, TransID: p.TransID, AmountCents: p.AmountCents, PayerMsisdnMasked: masked,
		PayerName: p.PayerName, BillRef: p.BillRef, PaidAt: p.PaidAt, Status: p.Status, MatchRule: p.MatchRule, SaleID: p.SaleID, InvoiceID: invoiceID}
}

func toItem(i gen.Item) ItemView {
	return ItemView{ID: i.ID, Name: i.Name, EtimsClassCode: i.EtimsClassCode, TaxCategory: i.TaxCategory, Unit: i.Unit, PriceCents: i.PriceCents, IsActive: i.IsActive}
}

func toLines(items []gen.SaleItem) []SaleLineView {
	out := make([]SaleLineView, 0, len(items))
	for _, it := range items {
		out = append(out, SaleLineView{ItemID: it.ItemID, Description: it.Description, Qty: it.Qty, UnitPriceCents: it.UnitPriceCents,
			TaxCategory: it.TaxCategory, LineTotalCents: it.LineTotalCents, LineTaxCents: it.LineTaxCents})
	}
	return out
}

func toSale(s gen.Sale, lines []gen.SaleItem) SaleView {
	v := SaleView{ID: s.ID, Ref: s.Ref, Kind: s.Kind, Status: s.Status, CustomerID: s.CustomerID, SubtotalCents: s.SubtotalCents,
		TaxCents: s.TaxCents, TotalCents: s.TotalCents, ClientRef: s.ClientRef, PaidAt: s.PaidAt, CreatedAt: s.CreatedAt}
	if lines != nil {
		v.Lines = toLines(lines)
	}
	return v
}

func toInvoice(k *crypto.Keyring, publicBase string, inv gen.Invoice) InvoiceView {
	v := InvoiceView{ID: inv.ID, Kind: inv.Kind, ParentInvoiceID: inv.ParentInvoiceID, SaleID: inv.SaleID, PaymentID: inv.PaymentID,
		State: inv.State, Attempt: inv.Attempt, ReceiptCode: inv.ReceiptCode, ReceiptURL: publicBase + "/r/" + inv.ReceiptCode,
		KRAInvoiceNo: inv.KraInvoiceNo, TotalCents: inv.TotalCents, TaxCents: inv.TaxCents, LastError: inv.LastError,
		CreatedAt: inv.CreatedAt, SubmittedAt: inv.SubmittedAt, AckedAt: inv.AckedAt}
	if len(inv.BuyerPinEnc) > 0 {
		if p, err := k.DecryptString(inv.BuyerPinEnc); err == nil {
			v.BuyerPinMasked = plog.MaskPIN(p)
		}
	}
	return v
}

func toSubmissions(rows []gen.FiscalSubmission) []SubmissionView {
	out := make([]SubmissionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, SubmissionView{Attempt: r.Attempt, Adapter: r.Adapter, Classification: r.Classification, Error: r.Error, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt})
	}
	return out
}
