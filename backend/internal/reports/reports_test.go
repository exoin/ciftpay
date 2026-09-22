package reports

import (
	"testing"
	"time"
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

func TestParseSummaryWindow(t *testing.T) {
	now := time.Date(2026, 9, 22, 14, 30, 0, 0, Nairobi)

	// Today
	f1, t1 := ParseSummaryWindow(now, "today")
	if f1 != time.Date(2026, 9, 22, 0, 0, 0, 0, Nairobi) || t1 != time.Date(2026, 9, 23, 0, 0, 0, 0, Nairobi) {
		t.Errorf("today mismatch: %v -> %v", f1, t1)
	}

	// Month
	f2, t2 := ParseSummaryWindow(now, "month")
	if f2 != time.Date(2026, 9, 1, 0, 0, 0, 0, Nairobi) || t2 != time.Date(2026, 10, 1, 0, 0, 0, 0, Nairobi) {
		t.Errorf("month mismatch: %v -> %v", f2, t2)
	}

	// Quarter (Sep 2026 is Q3: Jul 1 -> Oct 1)
	f3, t3 := ParseSummaryWindow(now, "quarter")
	if f3 != time.Date(2026, 7, 1, 0, 0, 0, 0, Nairobi) || t3 != time.Date(2026, 10, 1, 0, 0, 0, 0, Nairobi) {
		t.Errorf("quarter mismatch: %v -> %v", f3, t3)
	}

	// Year
	f4, t4 := ParseSummaryWindow(now, "year")
	if f4 != time.Date(2026, 1, 1, 0, 0, 0, 0, Nairobi) || t4 != time.Date(2027, 1, 1, 0, 0, 0, 0, Nairobi) {
		t.Errorf("year mismatch: %v -> %v", f4, t4)
	}

	// Historical Quarter: "2026-Q1"
	f5, t5 := ParseSummaryWindow(now, "2026-Q1")
	if f5 != time.Date(2026, 1, 1, 0, 0, 0, 0, Nairobi) || t5 != time.Date(2026, 4, 1, 0, 0, 0, 0, Nairobi) {
		t.Errorf("historical quarter mismatch: %v -> %v", f5, t5)
	}

	// Historical Year: "2025"
	f6, t6 := ParseSummaryWindow(now, "2025")
	if f6 != time.Date(2025, 1, 1, 0, 0, 0, 0, Nairobi) || t6 != time.Date(2026, 1, 1, 0, 0, 0, 0, Nairobi) {
		t.Errorf("historical year mismatch: %v -> %v", f6, t6)
	}
}
