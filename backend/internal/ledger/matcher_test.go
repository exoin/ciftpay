package ledger

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMatch(t *testing.T) {
	now := time.Date(2026, 9, 2, 9, 15, 0, 0, time.UTC)
	saleID := uuid.New()
	stkID := uuid.New()
	stkSale := uuid.New()
	item := uuid.New()
	payer := []byte("hash-of-254708374149")

	openSales := map[string]OpenSale{"S-260902-0001": {ID: saleID, TotalCents: 240000}}
	lookups := Lookups{
		OpenSaleByRef: func(ref string) (OpenSale, bool) {
			s, ok := openSales[ref]
			return s, ok
		},
		PendingSTK: func(h []byte, amount int64, since time.Time) (PendingSTK, bool) {
			created := now.Add(-3 * time.Minute)
			if bytes.Equal(h, payer) && amount == 500 && created.After(since) {
				return PendingSTK{ID: stkID, SaleID: &stkSale, CreatedAt: created}, true
			}
			return PendingSTK{}, false
		},
	}
	auto := ShortcodeRule{AutoInvoice: true, DefaultItemID: &item}

	cases := []struct {
		name string
		in   Incoming
		sc   ShortcodeRule
		lk   Lookups
		want Decision
	}{
		{
			name: "bill ref matches open sale with full amount",
			in:   Incoming{AmountCents: 240000, BillRef: "S-260902-0001", PaidAt: now},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusMatched, Rule: RuleBillRef, SaleID: &saleID},
		},
		{
			name: "bill ref matches but underpaid",
			in:   Incoming{AmountCents: 100000, BillRef: " S-260902-0001 ", PaidAt: now},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusPartial, Rule: RuleBillRef, SaleID: &saleID, Reason: "amount below sale total"},
		},
		{
			name: "overpayment still matches",
			in:   Incoming{AmountCents: 250000, BillRef: "S-260902-0001", PaidAt: now},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusMatched, Rule: RuleBillRef, SaleID: &saleID},
		},
		{
			name: "unknown bill ref falls through to cash sale",
			in:   Incoming{AmountCents: 240000, BillRef: "RENT", PaidAt: now},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusCashSale, Rule: RuleAutoInvoice, CreateCashSale: true},
		},
		{
			name: "stk window match",
			in:   Incoming{AmountCents: 500, MSISDNHash: payer, PaidAt: now},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusMatched, Rule: RuleSTKWindow, SaleID: &stkSale, STKRequestID: &stkID},
		},
		{
			name: "stk different amount does not match",
			in:   Incoming{AmountCents: 600, MSISDNHash: payer, PaidAt: now},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusCashSale, Rule: RuleAutoInvoice, CreateCashSale: true},
		},
		{
			name: "stk outside window does not match",
			in:   Incoming{AmountCents: 500, MSISDNHash: payer, PaidAt: now.Add(30 * time.Minute)},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusCashSale, Rule: RuleAutoInvoice, CreateCashSale: true},
		},
		{
			name: "no msisdn hash skips stk rule",
			in:   Incoming{AmountCents: 500, PaidAt: now},
			sc:   auto, lk: lookups,
			want: Decision{Status: StatusCashSale, Rule: RuleAutoInvoice, CreateCashSale: true},
		},
		{
			name: "auto invoice off → unmatched",
			in:   Incoming{AmountCents: 240000, PaidAt: now},
			sc:   ShortcodeRule{AutoInvoice: false, DefaultItemID: &item}, lk: lookups,
			want: Decision{Status: StatusUnmatched, Reason: "auto-invoice off for this shortcode"},
		},
		{
			name: "no default item → unmatched",
			in:   Incoming{AmountCents: 240000, PaidAt: now},
			sc:   ShortcodeRule{AutoInvoice: true}, lk: lookups,
			want: Decision{Status: StatusUnmatched, Reason: "shortcode has no default item"},
		},
		{
			name: "nil lookups are tolerated",
			in:   Incoming{AmountCents: 240000, BillRef: "X", MSISDNHash: payer, PaidAt: now},
			sc:   auto, lk: Lookups{},
			want: Decision{Status: StatusCashSale, Rule: RuleAutoInvoice, CreateCashSale: true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Match(tc.in, tc.sc, tc.lk)
			if got.Status != tc.want.Status || got.Rule != tc.want.Rule || got.CreateCashSale != tc.want.CreateCashSale || got.Reason != tc.want.Reason {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
			if !uuidPtrEq(got.SaleID, tc.want.SaleID) || !uuidPtrEq(got.STKRequestID, tc.want.STKRequestID) {
				t.Fatalf("ids: got sale=%v stk=%v want sale=%v stk=%v", got.SaleID, got.STKRequestID, tc.want.SaleID, tc.want.STKRequestID)
			}
		})
	}
}

func uuidPtrEq(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
