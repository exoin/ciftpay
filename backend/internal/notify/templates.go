// Package notify sends buyer receipts over SMS/WhatsApp through Africa's
// Talking and records every message in the notifications table.
package notify

import (
	"fmt"
	"strings"
)

// Template names used in jobs.SendReceiptArgs.Template.
const (
	TemplateReceiptPending = "receipt_pending"
	TemplateReceiptAcked   = "receipt_acked"
	TemplateReceiptUpdated = "receipt_updated"
	TemplateCreditNote     = "credit_note"
	TemplateOTP            = "otp"
)

// Vars are the values a template may reference.
type Vars struct {
	Merchant    string
	AmountKES   string // "2,400"
	KRAInvoice  string
	ReceiptURL  string
	Code        string // OTP
	SaleRef     string
	ExpiryHours int
}

// Locale codes. Anything unknown falls back to English.
const (
	LocaleEN = "en"
	LocaleSW = "sw"
)

// Render returns the message body for template in locale. Bodies stay under
// 160 GSM-7 characters where possible so a receipt is one SMS segment.
func Render(template, locale string, v Vars) (string, error) {
	if locale != LocaleSW {
		locale = LocaleEN
	}
	switch template {
	case TemplateReceiptPending:
		if locale == LocaleSW {
			return fmt.Sprintf("Umelipa KES %s kwa %s. Risiti ya KRA inatayarishwa: %s", v.AmountKES, v.Merchant, v.ReceiptURL), nil
		}
		return fmt.Sprintf("You paid KES %s to %s. Your KRA receipt is being prepared: %s", v.AmountKES, v.Merchant, v.ReceiptURL), nil
	case TemplateReceiptAcked:
		if locale == LocaleSW {
			return fmt.Sprintf("Risiti ya KRA %s kutoka %s, KES %s. Thibitisha na uhifadhi: %s", v.KRAInvoice, v.Merchant, v.AmountKES, v.ReceiptURL), nil
		}
		return fmt.Sprintf("KRA receipt %s from %s, KES %s. Verify and save: %s", v.KRAInvoice, v.Merchant, v.AmountKES, v.ReceiptURL), nil
	case TemplateReceiptUpdated:
		if locale == LocaleSW {
			return fmt.Sprintf("Risiti yako ya CiftPay kutoka %s imesasishwa na KRA PIN yako. Tazama hapa: %s", v.Merchant, v.ReceiptURL), nil
		}
		return fmt.Sprintf("Your CiftPay receipt from %s has been updated with your KRA PIN. View here: %s", v.Merchant, v.ReceiptURL), nil
	case TemplateCreditNote:
		if locale == LocaleSW {
			return fmt.Sprintf("Malipo yako ya KES %s kwa %s yamerudishwa. Credit note ya KRA %s: %s", v.AmountKES, v.Merchant, v.KRAInvoice, v.ReceiptURL), nil
		}
		return fmt.Sprintf("Your KES %s payment to %s was reversed. KRA credit note %s: %s", v.AmountKES, v.Merchant, v.KRAInvoice, v.ReceiptURL), nil
	case TemplateOTP:
		if locale == LocaleSW {
			return fmt.Sprintf("Nambari yako ya CiftPay ni %s. Inaisha baada ya dakika 5. Usimpe mtu yeyote.", v.Code), nil
		}
		return fmt.Sprintf("Your CiftPay code is %s. It expires in 5 minutes. Do not share it.", v.Code), nil
	}
	return "", fmt.Errorf("notify: unknown template %q", template)
}

// FormatKES renders cents as "2,400" or "2,400.50" (no currency symbol; the
// templates add "KES").
func FormatKES(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole, frac := cents/100, cents%100
	s := fmt.Sprintf("%d", whole)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if frac != 0 {
		out += fmt.Sprintf(".%02d", frac)
	}
	if neg {
		out = "-" + out
	}
	return out
}
