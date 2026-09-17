// Package mock is the fiscal.Provider used in development and tests. It
// produces deterministic acks and can inject failures via FailMode.
package mock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
)

// FailMode selects how the adapter fails. It maps to MOCK_FAIL_MODE.
type FailMode string

// Failure modes.
const (
	FailNone      FailMode = "none"
	FailRetryable FailMode = "retryable"
	FailTerminal  FailMode = "terminal"
)

// ParseFailMode validates a MOCK_FAIL_MODE value.
func ParseFailMode(s string) (FailMode, error) {
	switch FailMode(s) {
	case FailNone, FailRetryable, FailTerminal:
		return FailMode(s), nil
	case "":
		return FailNone, nil
	}
	return "", fmt.Errorf("mock: unknown fail mode %q", s)
}

var pinRe = regexp.MustCompile(`^[AP][0-9]{9}[A-Z]$`)

// Provider is an in-memory fiscal backend.
type Provider struct {
	mu       sync.Mutex
	acks     map[string]fiscal.Ack // by document ID
	failMode FailMode
	// FailTimes limits injected failures: after this many failures for a
	// given document ID the submission succeeds. 0 means always fail.
	FailTimes int
	failures  map[string]int
	now       func() time.Time
	Calls     int
}

// New returns a Provider in the given fail mode.
func New(mode FailMode) *Provider {
	return &Provider{
		acks:     map[string]fiscal.Ack{},
		failures: map[string]int{},
		failMode: mode,
		now:      time.Now,
	}
}

// SetFailMode changes the injected failure mode at runtime (tests).
func (p *Provider) SetFailMode(m FailMode) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failMode = m
}

// SetClock overrides the ack timestamp source (tests).
func (p *Provider) SetClock(now func() time.Time) { p.now = now }

// Name implements fiscal.Provider.
func (p *Provider) Name() string { return "mock" }

// Health implements fiscal.Provider.
func (p *Provider) Health(context.Context) error { return nil }

// RegisterDevice implements fiscal.Provider.
func (p *Provider) RegisterDevice(_ context.Context, org fiscal.OrgFiscalProfile) (fiscal.DeviceRef, error) {
	if !pinRe.MatchString(org.KRAPIN) {
		return fiscal.DeviceRef{}, &fiscal.ValidationError{Code: "seller_pin_invalid", Field: "kra_pin", Message: "KRA PIN must match A/P + 9 digits + letter"}
	}
	branch := org.BranchID
	if branch == "" {
		branch = "00"
	}
	id := "MOCKDEV" + shortHash(org.OrgID)[:8]
	raw, _ := json.Marshal(map[string]string{"deviceId": id, "bhfId": branch})
	return fiscal.DeviceRef{DeviceID: id, BranchID: branch, Raw: raw}, nil
}

// SubmitInvoice implements fiscal.Provider. Idempotent by inv.ID.
func (p *Provider) SubmitInvoice(ctx context.Context, inv fiscal.Invoice) (fiscal.Ack, error) {
	if err := validateInvoice(inv); err != nil {
		return fiscal.Ack{}, err
	}
	return p.submit(ctx, inv.ID, "KRACU0100000001/", inv.TotalCents)
}

// SubmitCreditNote implements fiscal.Provider.
func (p *Provider) SubmitCreditNote(ctx context.Context, cn fiscal.CreditNote) (fiscal.Ack, error) {
	if cn.OriginalKRANo == "" {
		return fiscal.Ack{}, &fiscal.ValidationError{Code: "original_invoice_missing", Field: "original_kra_no", Message: "credit note needs the original KRA invoice number"}
	}
	if cn.TotalCents >= 0 {
		return fiscal.Ack{}, &fiscal.ValidationError{Code: "credit_note_not_negative", Field: "total_cents", Message: "credit note total must be negative"}
	}
	return p.submit(ctx, cn.ID, "KRACU0100000001/CN", cn.TotalCents)
}

func (p *Provider) submit(ctx context.Context, id, prefix string, total int64) (fiscal.Ack, error) {
	if err := ctx.Err(); err != nil {
		return fiscal.Ack{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Calls++
	if ack, ok := p.acks[id]; ok {
		return ack, nil // idempotent replay
	}
	if p.failMode != FailNone && (p.FailTimes == 0 || p.failures[id] < p.FailTimes) {
		p.failures[id]++
		if p.failMode == FailTerminal {
			return fiscal.Ack{}, &fiscal.ValidationError{Code: "invalid_item_code", Field: "lines[0].item_code", Message: "injected terminal failure"}
		}
		return fiscal.Ack{}, &fiscal.TransientError{Code: "upstream_5xx", Message: "injected retryable failure"}
	}
	h := shortHash(id)
	ack := fiscal.Ack{
		KRAInvoiceNo: prefix + strings.ToUpper(h[:10]),
		Signature:    strings.ToUpper(h[10:26]),
		QRPayload:    fmt.Sprintf("https://itax.kra.go.ke/KRA-Portal/invoiceChk.htm?actionCode=loadPage&invoiceNo=%s%s", prefix, strings.ToUpper(h[:10])),
		ReceivedAt:   p.now().UTC(),
	}
	ack.Raw, _ = json.Marshal(map[string]any{
		"resultCd":    "000",
		"resultMsg":   "Successful",
		"curRcptNo":   ack.KRAInvoiceNo,
		"rcptSign":    ack.Signature,
		"totAmt":      total,
		"sdcDateTime": ack.ReceivedAt.Format("20060102150405"),
	})
	p.acks[id] = ack
	return ack, nil
}

// UnknownPIN is the reserved, well-formed PIN the mock reports as unknown to
// KRA so the `pin_unknown` onboarding path can be exercised end to end.
const UnknownPIN = "P000000000Z"

// LookupPIN implements fiscal.PINLookup: every well-formed PIN except
// UnknownPIN is known, with a deterministic taxpayer name. A-PINs (companies)
// are reported VAT registered, P-PINs (individuals) are not.
func (p *Provider) LookupPIN(ctx context.Context, pin string) (fiscal.Taxpayer, error) {
	if err := ctx.Err(); err != nil {
		return fiscal.Taxpayer{}, err
	}
	pin = strings.ToUpper(strings.TrimSpace(pin))
	if !pinRe.MatchString(pin) {
		return fiscal.Taxpayer{}, &fiscal.ValidationError{Code: "pin_invalid", Field: "kra_pin", Message: "KRA PIN must match A/P + 9 digits + letter"}
	}
	if pin == UnknownPIN {
		return fiscal.Taxpayer{}, fiscal.ErrPINUnknown
	}
	return fiscal.Taxpayer{
		PIN:           pin,
		Name:          "TAXPAYER " + strings.ToUpper(shortHash(pin)[:6]),
		VATRegistered: pin[0] == 'A',
	}, nil
}

// LookupItemCodes implements fiscal.Provider with a small curated list.
func (p *Provider) LookupItemCodes(_ context.Context, q string) ([]fiscal.ItemCode, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	var out []fiscal.ItemCode
	for _, c := range Catalog {
		if q == "" || strings.Contains(strings.ToLower(c.Description), q) || strings.HasPrefix(c.Code, q) {
			out = append(out, c)
		}
	}
	return out, nil
}

// Catalog is a handful of real UNSPSC-style codes used by the seed and tests.
var Catalog = []fiscal.ItemCode{
	{Code: "50000000", Description: "Food, beverage and tobacco products", TaxCategory: fiscal.TaxStandard},
	{Code: "50401500", Description: "Fresh vegetables", TaxCategory: fiscal.TaxExempt},
	{Code: "50301500", Description: "Fresh fruits", TaxCategory: fiscal.TaxExempt},
	{Code: "31160000", Description: "Hardware", TaxCategory: fiscal.TaxStandard},
	{Code: "78100000", Description: "Transport services", TaxCategory: fiscal.TaxStandard},
	{Code: "72100000", Description: "Building and facility maintenance services", TaxCategory: fiscal.TaxStandard},
	{Code: "91111500", Description: "Hair and beauty services", TaxCategory: fiscal.TaxStandard},
	{Code: "99000000", Description: "General retail sale", TaxCategory: fiscal.TaxStandard},
}

func validateInvoice(inv fiscal.Invoice) error {
	if inv.ID == "" {
		return &fiscal.ValidationError{Code: "invoice_id_missing", Field: "id", Message: "invoice id is required"}
	}
	if !pinRe.MatchString(inv.SellerPIN) {
		return &fiscal.ValidationError{Code: "seller_pin_invalid", Field: "seller_pin", Message: "seller KRA PIN is malformed"}
	}
	if inv.BuyerPIN != "" && !pinRe.MatchString(inv.BuyerPIN) {
		return &fiscal.ValidationError{Code: "buyer_pin_invalid", Field: "buyer_pin", Message: "buyer KRA PIN is malformed"}
	}
	if len(inv.Lines) == 0 {
		return &fiscal.ValidationError{Code: "no_lines", Field: "lines", Message: "invoice needs at least one line"}
	}
	var total int64
	for i, l := range inv.Lines {
		if l.ItemCode == "" {
			return &fiscal.ValidationError{Code: "invalid_item_code", Field: fmt.Sprintf("lines[%d].item_code", i), Message: "item classification code is required"}
		}
		if !l.TaxCategory.Valid() {
			return &fiscal.ValidationError{Code: "invalid_tax_category", Field: fmt.Sprintf("lines[%d].tax_category", i), Message: "unknown tax category"}
		}
		if l.LineTotalCents <= 0 {
			return &fiscal.ValidationError{Code: "line_total_not_positive", Field: fmt.Sprintf("lines[%d].line_total_cents", i), Message: "line total must be positive"}
		}
		total += l.LineTotalCents
	}
	if total != inv.TotalCents {
		return &fiscal.ValidationError{Code: "total_mismatch", Field: "total_cents", Message: fmt.Sprintf("lines sum to %d, invoice says %d", total, inv.TotalCents)}
	}
	return nil
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
