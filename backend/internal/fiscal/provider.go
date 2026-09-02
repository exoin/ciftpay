// Package fiscal owns the invoice lifecycle and the KRA eTIMS semantics:
// the Provider port implemented by adapters (mock, vendor, oscu), the invoice
// state machine, tax categories and error classification.
// See docs/architecture.md §4 and docs/adr/0002-fiscal-port-third-party-first.md.
package fiscal

import (
	"context"
	"encoding/json"
	"time"
)

// Provider is the port every eTIMS adapter implements.
type Provider interface {
	// RegisterDevice onboards an organisation with the fiscal backend and
	// returns the reference to store in orgs.fiscal_profile.
	RegisterDevice(ctx context.Context, org OrgFiscalProfile) (DeviceRef, error)
	// SubmitInvoice sends an invoice to KRA. It is idempotent by inv.ID: a
	// second call with the same ID must return the original Ack.
	SubmitInvoice(ctx context.Context, inv Invoice) (Ack, error)
	// SubmitCreditNote sends a credit note that reverses an acked invoice.
	SubmitCreditNote(ctx context.Context, cn CreditNote) (Ack, error)
	// LookupItemCodes searches the KRA item classification list.
	LookupItemCodes(ctx context.Context, q string) ([]ItemCode, error)
	// Health reports whether the backend is reachable.
	Health(ctx context.Context) error
	// Name is the adapter identifier stored in fiscal_submissions.adapter.
	Name() string
}

// OrgFiscalProfile is what an adapter needs to onboard a seller.
type OrgFiscalProfile struct {
	OrgID         string
	Name          string
	KRAPIN        string
	VATRegistered bool
	BranchID      string // KRA branch office id, "00" for head office
}

// DeviceRef is returned by RegisterDevice.
type DeviceRef struct {
	DeviceID string
	BranchID string
	Raw      json.RawMessage
}

// Line is one invoice line as KRA wants it.
type Line struct {
	ItemCode       string // KRA item classification code
	Description    string
	Qty            string // decimal string, e.g. "1" or "2.500"
	Unit           string // PCS, KG, ...
	UnitPriceCents int64
	TaxCategory    TaxCategory
	LineTotalCents int64
	LineTaxCents   int64
}

// Invoice is the adapter-facing view of an invoice.
type Invoice struct {
	ID            string // idempotency key
	OrgID         string
	SellerPIN     string
	SellerName    string
	BranchID      string
	BuyerPIN      string // optional
	BuyerName     string // optional
	IssuedAt      time.Time
	Lines         []Line
	SubtotalCents int64
	TaxCents      int64
	TotalCents    int64
	PaymentMethod string // "MOBILE_MONEY"
}

// CreditNote reverses an acked invoice in full or in part.
type CreditNote struct {
	ID              string
	OrgID           string
	OriginalKRANo   string
	OriginalInvoice Invoice
	Reason          string
	Lines           []Line
	TotalCents      int64
}

// Ack is what KRA (through the adapter) returns for an accepted document.
type Ack struct {
	KRAInvoiceNo string
	Signature    string
	QRPayload    string
	ReceivedAt   time.Time
	Raw          json.RawMessage
}

// ItemCode is one entry from the KRA classification list.
type ItemCode struct {
	Code        string
	Description string
	TaxCategory TaxCategory
}
