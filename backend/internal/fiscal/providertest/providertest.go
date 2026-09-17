// Package providertest is the contract suite every fiscal.Provider adapter
// must pass. Adapters call Run from their own test package:
//
//	func TestContract(t *testing.T) {
//	    providertest.Run(t, func(t *testing.T) fiscal.Provider { return mock.New(mock.FailNone) })
//	}
package providertest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
)

// Factory returns a fresh, healthy provider for one sub-test.
type Factory func(t *testing.T) fiscal.Provider

// ValidInvoice returns a well-formed invoice adapters must accept.
func ValidInvoice(id string) fiscal.Invoice {
	lineTotal := int64(240000)
	tax := fiscal.LineTax(lineTotal, fiscal.TaxStandard.RateBP())
	inv := fiscal.Invoice{
		ID:            id,
		OrgID:         "11111111-1111-1111-1111-111111111111",
		SellerPIN:     "P051234567A",
		SellerName:    "Wanjiru Groceries",
		BranchID:      "00",
		IssuedAt:      time.Date(2026, 9, 2, 9, 15, 0, 0, time.UTC),
		PaymentMethod: "MOBILE_MONEY",
		Lines: []fiscal.Line{{
			ItemCode:       "99000000",
			Description:    "General sale",
			Qty:            "1",
			Unit:           "PCS",
			UnitPriceCents: lineTotal,
			TaxCategory:    fiscal.TaxStandard,
			LineTotalCents: lineTotal,
			LineTaxCents:   tax,
		}},
	}
	inv.SubtotalCents, inv.TaxCents, inv.TotalCents = fiscal.Totals(inv.Lines)
	return inv
}

// Run executes the contract suite.
func Run(t *testing.T, newProvider Factory) {
	t.Helper()
	ctx := context.Background()

	t.Run("Name", func(t *testing.T) {
		if newProvider(t).Name() == "" {
			t.Fatal("Name must not be empty")
		}
	})

	t.Run("Health", func(t *testing.T) {
		if err := newProvider(t).Health(ctx); err != nil {
			t.Fatalf("healthy provider reported %v", err)
		}
	})

	t.Run("RegisterDevice", func(t *testing.T) {
		p := newProvider(t)
		ref, err := p.RegisterDevice(ctx, fiscal.OrgFiscalProfile{OrgID: "org-1", Name: "Test", KRAPIN: "P051234567A", VATRegistered: true})
		if err != nil {
			t.Fatal(err)
		}
		if ref.DeviceID == "" || ref.BranchID == "" {
			t.Fatalf("device ref incomplete: %+v", ref)
		}
		_, err = p.RegisterDevice(ctx, fiscal.OrgFiscalProfile{OrgID: "org-2", KRAPIN: "NOPE"})
		var ve *fiscal.ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("bad PIN must be a ValidationError, got %v", err)
		}
	})

	t.Run("SubmitInvoiceAckShape", func(t *testing.T) {
		p := newProvider(t)
		ack, err := p.SubmitInvoice(ctx, ValidInvoice("inv-ack-shape"))
		if err != nil {
			t.Fatal(err)
		}
		if ack.KRAInvoiceNo == "" || ack.Signature == "" || ack.QRPayload == "" {
			t.Fatalf("ack incomplete: %+v", ack)
		}
		if ack.ReceivedAt.IsZero() {
			t.Fatal("ack.ReceivedAt must be set")
		}
		if len(ack.Raw) == 0 {
			t.Fatal("ack.Raw must carry the upstream body for audit")
		}
	})

	t.Run("SubmitInvoiceIdempotent", func(t *testing.T) {
		p := newProvider(t)
		inv := ValidInvoice("inv-idem")
		a1, err := p.SubmitInvoice(ctx, inv)
		if err != nil {
			t.Fatal(err)
		}
		a2, err := p.SubmitInvoice(ctx, inv)
		if err != nil {
			t.Fatal(err)
		}
		if a1.KRAInvoiceNo != a2.KRAInvoiceNo || a1.Signature != a2.Signature {
			t.Fatalf("resubmitting the same invoice ID must return the original ack: %+v vs %+v", a1, a2)
		}
		a3, err := p.SubmitInvoice(ctx, ValidInvoice("inv-other"))
		if err != nil {
			t.Fatal(err)
		}
		if a3.KRAInvoiceNo == a1.KRAInvoiceNo {
			t.Fatal("different invoices must get different KRA numbers")
		}
	})

	t.Run("ValidationErrorsAreTerminal", func(t *testing.T) {
		p := newProvider(t)
		bad := ValidInvoice("inv-bad-code")
		bad.Lines[0].ItemCode = ""
		_, err := p.SubmitInvoice(ctx, bad)
		if fiscal.Classify(err) != fiscal.ClassTerminal {
			t.Fatalf("missing item code must be terminal, got %v", err)
		}
		badPIN := ValidInvoice("inv-bad-pin")
		badPIN.BuyerPIN = "12345"
		_, err = p.SubmitInvoice(ctx, badPIN)
		if fiscal.Classify(err) != fiscal.ClassTerminal || fiscal.ErrorCode(err) != "buyer_pin_invalid" {
			t.Fatalf("bad buyer PIN must be terminal buyer_pin_invalid, got %v", err)
		}
		mismatch := ValidInvoice("inv-mismatch")
		mismatch.TotalCents++
		_, err = p.SubmitInvoice(ctx, mismatch)
		if fiscal.Classify(err) != fiscal.ClassTerminal {
			t.Fatalf("total mismatch must be terminal, got %v", err)
		}
	})

	t.Run("CreditNote", func(t *testing.T) {
		p := newProvider(t)
		inv := ValidInvoice("inv-for-cn")
		ack, err := p.SubmitInvoice(ctx, inv)
		if err != nil {
			t.Fatal(err)
		}
		cn := fiscal.CreditNote{
			ID:              "cn-1",
			OrgID:           inv.OrgID,
			OriginalKRANo:   ack.KRAInvoiceNo,
			OriginalInvoice: inv,
			Reason:          "M-Pesa reversal",
			Lines:           inv.Lines,
			TotalCents:      -inv.TotalCents,
		}
		cnAck, err := p.SubmitCreditNote(ctx, cn)
		if err != nil {
			t.Fatal(err)
		}
		if cnAck.KRAInvoiceNo == "" || cnAck.KRAInvoiceNo == ack.KRAInvoiceNo {
			t.Fatalf("credit note needs its own KRA number: %+v", cnAck)
		}
		cn.OriginalKRANo = ""
		if _, err := p.SubmitCreditNote(ctx, cn); fiscal.Classify(err) != fiscal.ClassTerminal {
			t.Fatalf("credit note without original must be terminal, got %v", err)
		}
	})

	t.Run("LookupItemCodes", func(t *testing.T) {
		p := newProvider(t)
		codes, err := p.LookupItemCodes(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(codes) == 0 {
			t.Fatal("empty query must return at least one code")
		}
		for _, c := range codes {
			if c.Code == "" || !c.TaxCategory.Valid() {
				t.Fatalf("bad item code entry %+v", c)
			}
		}
	})

	t.Run("ContextCancelled", func(t *testing.T) {
		p := newProvider(t)
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		_, err := p.SubmitInvoice(cctx, ValidInvoice("inv-cancel"))
		if err == nil {
			t.Fatal("cancelled context must fail")
		}
		if fiscal.Classify(err) != fiscal.ClassRetryable {
			t.Fatalf("cancellation is retryable, got %v", err)
		}
	})
}
