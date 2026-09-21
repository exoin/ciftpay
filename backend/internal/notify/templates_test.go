package notify

import (
	"strings"
	"testing"
)

func TestFormatKES(t *testing.T) {
	cases := map[int64]string{0: "0", 100: "1", 240000: "2,400", 240050: "2,400.50", 123456789: "1,234,567.89", -500: "-5"}
	for cents, want := range cases {
		if got := FormatKES(cents); got != want {
			t.Errorf("FormatKES(%d) = %q, want %q", cents, got, want)
		}
	}
}

func TestRender_AllTemplatesBothLocalesFitOneSegment(t *testing.T) {
	v := Vars{Merchant: "Mama Njeri Wholesalers", AmountKES: "2,400", KRAInvoice: "KRACU0100000123", ReceiptURL: "https://ciftpay.co.ke/r/ABCD-EFGH", Code: "123456"}
	for _, tpl := range []string{TemplateReceiptPending, TemplateReceiptAcked, TemplateReceiptUpdated, TemplateCreditNote, TemplateOTP} {
		for _, loc := range []string{LocaleEN, LocaleSW, "fr"} {
			body, err := Render(tpl, loc, v)
			if err != nil {
				t.Fatalf("%s/%s: %v", tpl, loc, err)
			}
			if len(body) == 0 || len(body) > 160 {
				t.Errorf("%s/%s: %d chars, want 1–160: %q", tpl, loc, len(body), body)
			}
			if tpl != TemplateOTP && !strings.Contains(body, v.ReceiptURL) {
				t.Errorf("%s/%s: receipt link missing: %q", tpl, loc, body)
			}
		}
	}
	if _, err := Render("nope", LocaleEN, v); err == nil {
		t.Error("unknown template must error")
	}
}

func TestParseCostAndE164(t *testing.T) {
	if got := parseCost("KES 0.8000"); got != 80 {
		t.Errorf("parseCost = %d, want 80", got)
	}
	if got := parseCost("garbage"); got != 0 {
		t.Errorf("parseCost(garbage) = %d", got)
	}
	if E164("254708374149") != "+254708374149" || E164("+254708374149") != "+254708374149" {
		t.Error("E164 normalisation wrong")
	}
}
