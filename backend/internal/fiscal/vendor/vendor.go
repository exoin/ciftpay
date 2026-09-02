// Package vendor is the fiscal.Provider adapter for a KRA-approved third-party
// eTIMS integrator (ADR-0002). Phase 0 ships the HTTP plumbing, request and
// response shapes and error classification against a generic REST contract;
// the concrete endpoint paths and field names are adjusted once the vendor is
// chosen. The adapter must keep passing providertest.
package vendor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ciftpay/ciftpay/internal/fiscal"
)

// Config configures the adapter.
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// Provider talks to the integrator over HTTPS.
type Provider struct {
	cfg  Config
	http *http.Client
}

// New builds the adapter.
func New(cfg Config) (*Provider, error) {
	if cfg.BaseURL == "" || cfg.APIKey == "" {
		return nil, &fiscal.ConfigError{Code: "vendor_not_configured", Message: "FISCAL_VENDOR_BASE_URL and FISCAL_VENDOR_API_KEY are required"}
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, &fiscal.ConfigError{Code: "vendor_bad_url", Message: err.Error()}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 20 * time.Second
	}
	return &Provider{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}, nil
}

// Name implements fiscal.Provider.
func (p *Provider) Name() string { return "vendor" }

// wire types -----------------------------------------------------------------

type wireLine struct {
	ItemCode    string `json:"itemCode"`
	Description string `json:"description"`
	Qty         string `json:"qty"`
	Unit        string `json:"unit"`
	UnitPrice   int64  `json:"unitPriceCents"`
	TaxType     string `json:"taxType"`
	LineTotal   int64  `json:"lineTotalCents"`
	LineTax     int64  `json:"lineTaxCents"`
}

type wireInvoice struct {
	ClientRef     string     `json:"clientRef"`
	Kind          string     `json:"kind"` // INVOICE | CREDIT_NOTE
	OriginalKRANo string     `json:"originalInvoiceNo,omitempty"`
	SellerPIN     string     `json:"sellerPin"`
	SellerName    string     `json:"sellerName"`
	BranchID      string     `json:"branchId"`
	BuyerPIN      string     `json:"buyerPin,omitempty"`
	BuyerName     string     `json:"buyerName,omitempty"`
	IssuedAt      string     `json:"issuedAt"`
	PaymentMethod string     `json:"paymentMethod"`
	Lines         []wireLine `json:"lines"`
	SubtotalCents int64      `json:"subtotalCents"`
	TaxCents      int64      `json:"taxCents"`
	TotalCents    int64      `json:"totalCents"`
}

type wireAck struct {
	InvoiceNo  string `json:"invoiceNo"`
	Signature  string `json:"signature"`
	QRPayload  string `json:"qrPayload"`
	ReceivedAt string `json:"receivedAt"`
}

type wireError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"error"`
}

// Provider methods -----------------------------------------------------------

// Health implements fiscal.Provider.
func (p *Provider) Health(ctx context.Context) error {
	var out map[string]any
	return p.do(ctx, http.MethodGet, "/v1/health", nil, &out)
}

// RegisterDevice implements fiscal.Provider.
func (p *Provider) RegisterDevice(ctx context.Context, org fiscal.OrgFiscalProfile) (fiscal.DeviceRef, error) {
	body := map[string]any{"clientRef": org.OrgID, "pin": org.KRAPIN, "name": org.Name, "vatRegistered": org.VATRegistered, "branchId": org.BranchID}
	var out struct {
		DeviceID string `json:"deviceId"`
		BranchID string `json:"branchId"`
	}
	raw, err := p.doRaw(ctx, http.MethodPost, "/v1/devices", body, &out)
	if err != nil {
		return fiscal.DeviceRef{}, err
	}
	return fiscal.DeviceRef{DeviceID: out.DeviceID, BranchID: out.BranchID, Raw: raw}, nil
}

// SubmitInvoice implements fiscal.Provider.
func (p *Provider) SubmitInvoice(ctx context.Context, inv fiscal.Invoice) (fiscal.Ack, error) {
	w := toWire(inv)
	w.Kind = "INVOICE"
	return p.submit(ctx, w)
}

// SubmitCreditNote implements fiscal.Provider.
func (p *Provider) SubmitCreditNote(ctx context.Context, cn fiscal.CreditNote) (fiscal.Ack, error) {
	w := toWire(cn.OriginalInvoice)
	w.ClientRef = cn.ID
	w.Kind = "CREDIT_NOTE"
	w.OriginalKRANo = cn.OriginalKRANo
	w.Lines = toWireLines(cn.Lines)
	w.TotalCents = cn.TotalCents
	w.SubtotalCents, w.TaxCents, _ = fiscal.Totals(cn.Lines)
	w.SubtotalCents, w.TaxCents = -w.SubtotalCents, -w.TaxCents
	return p.submit(ctx, w)
}

func (p *Provider) submit(ctx context.Context, w wireInvoice) (fiscal.Ack, error) {
	var out wireAck
	raw, err := p.doRaw(ctx, http.MethodPost, "/v1/invoices", w, &out)
	if err != nil {
		return fiscal.Ack{}, err
	}
	received, perr := time.Parse(time.RFC3339, out.ReceivedAt)
	if perr != nil {
		received = time.Now().UTC()
	}
	if out.InvoiceNo == "" {
		return fiscal.Ack{}, &fiscal.TransientError{Code: "vendor_bad_ack", Message: "ack without invoice number"}
	}
	return fiscal.Ack{KRAInvoiceNo: out.InvoiceNo, Signature: out.Signature, QRPayload: out.QRPayload, ReceivedAt: received, Raw: raw}, nil
}

// LookupItemCodes implements fiscal.Provider.
func (p *Provider) LookupItemCodes(ctx context.Context, q string) ([]fiscal.ItemCode, error) {
	var out []struct {
		Code        string `json:"code"`
		Description string `json:"description"`
		TaxType     string `json:"taxType"`
	}
	if err := p.do(ctx, http.MethodGet, "/v1/item-codes?q="+url.QueryEscape(q), nil, &out); err != nil {
		return nil, err
	}
	codes := make([]fiscal.ItemCode, 0, len(out))
	for _, c := range out {
		codes = append(codes, fiscal.ItemCode{Code: c.Code, Description: c.Description, TaxCategory: fiscal.TaxCategory(c.TaxType)})
	}
	return codes, nil
}

// transport ------------------------------------------------------------------

func (p *Provider) do(ctx context.Context, method, path string, in, out any) error {
	_, err := p.doRaw(ctx, method, path, in, out)
	return err
}

func (p *Provider) doRaw(ctx context.Context, method, path string, in, out any) (json.RawMessage, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.cfg.BaseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.http.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, &fiscal.TransientError{Code: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &fiscal.TransientError{Code: "network", Message: err.Error()}
	}
	if err := classifyStatus(resp.StatusCode, raw); err != nil {
		return nil, err
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, &fiscal.TransientError{Code: "vendor_bad_json", Message: err.Error()}
		}
	}
	return raw, nil
}

// classifyStatus maps HTTP outcomes onto the fiscal error classes.
func classifyStatus(status int, raw []byte) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &fiscal.ConfigError{Code: "invalid_api_key", Message: fmt.Sprintf("vendor returned %d", status)}
	case status == http.StatusTooManyRequests:
		return &fiscal.TransientError{Code: "rate_limited", Message: "vendor rate limit"}
	case status >= 500:
		return &fiscal.TransientError{Code: "upstream_5xx", Message: fmt.Sprintf("vendor returned %d", status)}
	}
	var we wireError
	_ = json.Unmarshal(raw, &we)
	code := we.Error.Code
	if code == "" {
		code = "vendor_rejected"
	}
	switch code {
	case "device_not_registered":
		return &fiscal.ConfigError{Code: code, Message: we.Error.Message}
	case "duplicate":
		// Phase 1: fetch the existing ack by clientRef and return it. Until
		// then the worker treats it as terminal for manual attach.
		return &fiscal.ValidationError{Code: "duplicate_unresolved", Message: we.Error.Message}
	}
	return &fiscal.ValidationError{Code: code, Field: we.Error.Field, Message: we.Error.Message}
}

func toWire(inv fiscal.Invoice) wireInvoice {
	return wireInvoice{
		ClientRef:     inv.ID,
		SellerPIN:     inv.SellerPIN,
		SellerName:    inv.SellerName,
		BranchID:      inv.BranchID,
		BuyerPIN:      inv.BuyerPIN,
		BuyerName:     inv.BuyerName,
		IssuedAt:      inv.IssuedAt.UTC().Format(time.RFC3339),
		PaymentMethod: inv.PaymentMethod,
		Lines:         toWireLines(inv.Lines),
		SubtotalCents: inv.SubtotalCents,
		TaxCents:      inv.TaxCents,
		TotalCents:    inv.TotalCents,
	}
}

func toWireLines(lines []fiscal.Line) []wireLine {
	out := make([]wireLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, wireLine{
			ItemCode: l.ItemCode, Description: l.Description, Qty: l.Qty, Unit: l.Unit,
			UnitPrice: l.UnitPriceCents, TaxType: string(l.TaxCategory),
			LineTotal: l.LineTotalCents, LineTax: l.LineTaxCents,
		})
	}
	return out
}
