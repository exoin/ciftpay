package reports

import (
	"testing"
)

func TestDarajaMerchantFeeCents(t *testing.T) {
	tests := []struct {
		amountCents int64
		wantFee     int64
	}{
		{0, 0},
		{5000, 0},         // KES 50 -> 0
		{20000, 0},        // KES 200 -> 0
		{20100, 110},      // KES 201 -> 0.55%
		{100000, 550},     // KES 1,000 -> KES 5.50 (550 cents)
		{1000000, 5500},   // KES 10,000 -> KES 55.00 (5500 cents)
		{5000000, 20000},  // KES 50,000 -> capped at KES 200 (20,000 cents)
	}

	for _, tt := range tests {
		got := DarajaMerchantFeeCents(tt.amountCents)
		if got != tt.wantFee {
			t.Errorf("DarajaMerchantFeeCents(%d) = %d; want %d", tt.amountCents, got, tt.wantFee)
		}
	}
}

func TestFormatTaxCategory(t *testing.T) {
	if got := formatTaxCategory("B"); got != "16%" {
		t.Errorf("formatTaxCategory(B) = %s; want 16%%", got)
	}
	if got := formatTaxCategory("E"); got != "8%" {
		t.Errorf("formatTaxCategory(E) = %s; want 8%%", got)
	}
	if got := formatTaxCategory("C"); got != "0%" {
		t.Errorf("formatTaxCategory(C) = %s; want 0%%", got)
	}
	if got := formatTaxCategory("A"); got != "Exempt" {
		t.Errorf("formatTaxCategory(A) = %s; want Exempt", got)
	}
	if got := formatTaxCategory("D"); got != "Non-VAT" {
		t.Errorf("formatTaxCategory(D) = %s; want Non-VAT", got)
	}
}

func TestParsePeriod(t *testing.T) {
	from, to, err := ParsePeriod("2026-09")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if from.Year() != 2026 || from.Month() != 9 || from.Day() != 1 {
		t.Errorf("unexpected from: %v", from)
	}
	if to.Year() != 2026 || to.Month() != 10 || to.Day() != 1 {
		t.Errorf("unexpected to: %v", to)
	}

	if _, _, err := ParsePeriod("invalid"); err == nil {
		t.Error("expected error on invalid period format")
	}
}
