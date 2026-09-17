// Package oscu is the direct KRA eTIMS OSCU client (ADR-0009): CiftPay's own
// Online Sales Control Unit, talking to KRA's gateway system-to-system
// instead of going through a third-party integrator (contrast
// internal/fiscal/vendor). It implements fiscal.Provider so it is selectable
// with FISCAL_ADAPTER=oscu (internal/boot).
//
// Two credential tiers meet here:
//   - CiftPay's own OSCU developer credentials (KRA_OSCU_CONSUMER_KEY/SECRET)
//     are global and authorise the OAuth client-credentials grant.
//   - Per-merchant device identity (taxpayer PIN, branch id "bhfId", device
//     serial "dvcSrlNo") is merchant data; KRA checks it against its own
//     registry on device initialisation and, if it matches, hands back the
//     cmcKey that signs every subsequent call for that (pin, bhfId, dvcSrlNo)
//     triple. RegisterDevice is the only place that identity is used; every
//     later call reads it back from the opaque profile the caller stores on
//     orgs.fiscal_profile (fiscal.Invoice.DeviceProfile).
//
// The sales-submission and item-classification wire shapes here are a
// best-effort mapping to the published OSCU specification (KRA "Online Sales
// Control Unit Requirements & Communication Protocols") and, like
// internal/fiscal/vendor, are expected to be reconciled field-by-field during
// KRA's own certification pass; RegisterDevice's request/response shape is
// the one part of this file that has been checked against a live sandbox
// call and is not a placeholder.
package oscu

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
)

// Config configures the client and, through New, the fiscal.Provider adapter.
type Config struct {
	// BaseURL is the KRA API gateway origin, e.g. https://sbx.kra.go.ke in
	// the sandbox. OAuth tokens are fetched from {BaseURL}/v1/token/generate;
	// the eTIMS OSCU calls are namespaced under {BaseURL}/etims-api.
	BaseURL string
	// ConsumerKey/ConsumerSecret are CiftPay's own OSCU developer credentials
	// (global, from KRA_OSCU_CONSUMER_KEY/SECRET), used as HTTP Basic
	// authentication on the token endpoint.
	ConsumerKey    string
	ConsumerSecret string
	// DeviceSerial is a fallback dvcSrlNo used only when the caller (an org
	// that has not configured its own kra_device_serial yet, or a bare
	// smoke test) does not supply one. Production traffic should always pass
	// the merchant's own serial through fiscal.OrgFiscalProfile.DeviceSerial.
	DeviceSerial string
	// DNSResolver is the UDP address (host:port) this client resolves
	// hostnames through, bypassing the system/library resolver. Needed
	// because some environments' validating resolvers answer SERVFAIL for
	// sbx.kra.go.ke (a DNSSEC misconfiguration on KRA's side, not ours).
	// Empty uses the normal system resolver; this only ever affects this
	// client's own *http.Transport, never global DNS.
	DNSResolver string
	Timeout     time.Duration
}

// Client is the low-level KRA gateway client: OAuth token plus the raw
// selectInitOsdcInfo call. cmd/ciftctl's `kra-init` uses it directly, ahead
// of any database, to smoke-test sandbox connectivity.
type Client struct {
	cfg  Config
	http *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewClient builds the low-level client.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" || cfg.ConsumerKey == "" || cfg.ConsumerSecret == "" {
		return nil, &fiscal.ConfigError{Code: "oscu_not_configured", Message: "KRA_OSCU_BASE_URL, KRA_OSCU_CONSUMER_KEY and KRA_OSCU_CONSUMER_SECRET are required"}
	}
	if _, err := url.Parse(cfg.BaseURL); err != nil {
		return nil, &fiscal.ConfigError{Code: "oscu_bad_url", Message: err.Error()}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 20 * time.Second
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout, Transport: buildTransport(cfg.DNSResolver)}}, nil
}

// buildTransport clones the default transport and, when resolver is set,
// forces every DNS lookup this transport performs through that server
// instead of the system resolver. Scoped to this one *http.Transport.
func buildTransport(resolver string) *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	if resolver == "" {
		return base
	}
	dialer := &net.Dialer{
		Resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{Timeout: 5 * time.Second}
				return d.DialContext(ctx, "udp", resolver)
			},
		},
	}
	base.DialContext = dialer.DialContext
	return base
}

const tokenPath = "/v1/token/generate"

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

// Token fetches (and caches until shortly before expiry) an OAuth
// client-credentials token: GET {BaseURL}/v1/token/generate?grant_type=client_credentials
// with HTTP Basic Base64(consumerKey:consumerSecret).
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.cfg.BaseURL, "/")+tokenPath+"?grant_type=client_credentials", nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.cfg.ConsumerKey, c.cfg.ConsumerSecret)
	req.Header.Set("Accept", "application/json")

	raw, status, err := c.doRaw(req)
	if err != nil {
		return "", err
	}
	if err := classifyStatus(status, raw); err != nil {
		return "", err
	}
	var tr tokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil || tr.AccessToken == "" {
		return "", &fiscal.TransientError{Code: "oscu_bad_json", Message: "token response missing access_token"}
	}
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if tr.ExpiresIn <= 0 {
		ttl = 55 * time.Minute
	}
	c.mu.Lock()
	c.token, c.tokenExp = tr.AccessToken, time.Now().Add(ttl-30*time.Second)
	c.mu.Unlock()
	return tr.AccessToken, nil
}

// Info is the taxpayer/device record KRA returns from device initialisation.
type Info struct {
	TIN          string
	TaxpayerName string
	BranchID     string // bhfId
	BranchName   string
	DeviceID     string // dvcId
	SDCID        string
	MRCNo        string
	CmcKey       string // the communication key: a signing credential, never logged unmasked
}

const initPath = "/etims-api/selectInitOsdcInfo"

type initRequest struct {
	TIN      string `json:"tin"`
	BhfID    string `json:"bhfId"`
	DvcSrlNo string `json:"dvcSrlNo"`
}

type resultEnvelope struct {
	ResultCd  string          `json:"resultCd"`
	ResultMsg string          `json:"resultMsg"`
	ResultDt  string          `json:"resultDt"`
	Data      json.RawMessage `json:"data"`
}

type initData struct {
	Info struct {
		TIN     string `json:"tin"`
		TaxprNm string `json:"taxprNm"`
		BhfID   string `json:"bhfId"`
		BhfNm   string `json:"bhfNm"`
		DvcID   string `json:"dvcId"`
		SdcID   string `json:"sdcId"`
		MrcNo   string `json:"mrcNo"`
		CmcKey  string `json:"cmcKey"`
	} `json:"info"`
}

// Initialize runs KRA's device-initialisation call (selectInitOsdcInfo): the
// one-time-per-branch handshake that trades (tin, bhfId, dvcSrlNo) for the
// cmcKey that signs every subsequent call. It is idempotent on KRA's side
// (re-running it for the same triple returns the same cmcKey) but
// re-initialising with a *different* dvcSrlNo invalidates the old key, so
// callers must not change a merchant's device serial without re-registering.
func (c *Client) Initialize(ctx context.Context, pin, branch, serial string) (Info, json.RawMessage, error) {
	if branch == "" {
		branch = "00"
	}
	if serial == "" {
		serial = c.cfg.DeviceSerial
	}
	if pin == "" || serial == "" {
		return Info{}, nil, &fiscal.ValidationError{Code: "oscu_init_missing_fields", Message: "pin and device serial are required to initialise an OSCU device"}
	}
	tok, err := c.Token(ctx)
	if err != nil {
		return Info{}, nil, err
	}

	body, err := json.Marshal(initRequest{TIN: pin, BhfID: branch, DvcSrlNo: serial})
	if err != nil {
		return Info{}, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+initPath, bytes.NewReader(body))
	if err != nil {
		return Info{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("tin", pin)
	req.Header.Set("bhfId", branch)

	raw, status, err := c.doRaw(req)
	if err != nil {
		return Info{}, nil, err
	}
	if err := classifyStatus(status, raw); err != nil {
		return Info{}, raw, err
	}
	var env resultEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Info{}, raw, &fiscal.TransientError{Code: "oscu_bad_json", Message: err.Error()}
	}
	if env.ResultCd != "" && env.ResultCd != "000" {
		return Info{}, raw, classifyResultCode(env.ResultCd, env.ResultMsg)
	}
	var d initData
	_ = json.Unmarshal(env.Data, &d)
	info := Info{
		TIN: d.Info.TIN, TaxpayerName: d.Info.TaxprNm, BranchID: d.Info.BhfID, BranchName: d.Info.BhfNm,
		DeviceID: d.Info.DvcID, SDCID: d.Info.SdcID, MRCNo: d.Info.MrcNo, CmcKey: d.Info.CmcKey,
	}
	if info.TIN == "" {
		info.TIN = pin
	}
	if info.BranchID == "" {
		info.BranchID = branch
	}
	if info.CmcKey == "" {
		return info, raw, &fiscal.TransientError{Code: "oscu_bad_ack", Message: "device initialisation response is missing cmcKey"}
	}
	return info, raw, nil
}

func (c *Client) doRaw(req *http.Request) (raw []byte, status int, err error) {
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, 0, err
		}
		return nil, 0, &fiscal.TransientError{Code: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	raw, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, &fiscal.TransientError{Code: "network", Message: err.Error()}
	}
	return raw, resp.StatusCode, nil
}

// classifyStatus maps the HTTP-transport-level outcome onto the fiscal error
// classes, before the response body's own resultCd is even looked at.
func classifyStatus(status int, raw []byte) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &fiscal.ConfigError{Code: "invalid_api_key", Message: fmt.Sprintf("KRA gateway returned %d", status)}
	case status == http.StatusTooManyRequests:
		return &fiscal.TransientError{Code: "rate_limited", Message: "KRA gateway rate limit"}
	case status >= 500:
		return &fiscal.TransientError{Code: "upstream_5xx", Message: fmt.Sprintf("KRA gateway returned %d", status)}
	case status >= 400:
		return &fiscal.ValidationError{Code: "oscu_rejected", Message: fmt.Sprintf("KRA gateway returned %d: %s", status, truncate(string(raw), 300))}
	}
	return nil
}

// classifyResultCode maps a KRA business-level resultCd (distinct from the
// HTTP status: the gateway answers 200 even for a rejected document) onto
// the fiscal error classes. "000" (success) never reaches here.
func classifyResultCode(code, msg string) error {
	switch code {
	case "":
		return nil
	case "001", "999": // gateway/system busy per the OSCU spec's generic system-error codes
		return &fiscal.TransientError{Code: "oscu_system_busy", Message: msg}
	default:
		return &fiscal.ValidationError{Code: "oscu_rejected_" + code, Message: msg}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ------------------------------------------------------------- fiscal.Provider

var pinRe = regexp.MustCompile(`^[AP][0-9]{9}[A-Z]$`)

// Provider adapts Client to fiscal.Provider.
type Provider struct {
	c   *Client
	cfg Config

	// mu/acks give SubmitInvoice/SubmitCreditNote client-side idempotency by
	// document ID (ADR-0002 requires it; KRA's own sales API has no such
	// concept, it tracks its own sequential invcNo). This does not survive a
	// process restart: the durable record of what was already submitted is
	// fiscal_submissions (internal/fiscal/submitter.go), which the worker
	// consults before ever calling Submit again for an ACKED invoice.
	mu   sync.Mutex
	acks map[string]fiscal.Ack
}

// New builds the oscu fiscal.Provider adapter.
func New(cfg Config) (fiscal.Provider, error) {
	c, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Provider{c: c, cfg: cfg, acks: map[string]fiscal.Ack{}}, nil
}

// Name implements fiscal.Provider.
func (p *Provider) Name() string { return "oscu" }

// Health implements fiscal.Provider: a live token is the cheapest proof the
// gateway and our credentials both work.
func (p *Provider) Health(ctx context.Context) error {
	_, err := p.c.Token(ctx)
	return err
}

// deviceProfile is the shape RegisterDevice stores on orgs.fiscal_profile
// (device_id/branch_id are shared with every other adapter's profile struct,
// see internal/fiscal/submitter.go; device_serial/cmc_key/sdc_id/mrc_no are
// oscu-specific and ignored by other adapters).
type deviceProfile struct {
	DeviceID     string `json:"device_id"`
	BranchID     string `json:"branch_id"`
	DeviceSerial string `json:"device_serial"`
	CmcKey       string `json:"cmc_key"`
	SDCID        string `json:"sdc_id"`
	MRCNo        string `json:"mrc_no"`
}

func decodeDeviceProfile(raw json.RawMessage) deviceProfile {
	var p deviceProfile
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p)
	}
	return p
}

// RegisterDevice implements fiscal.Provider: KRA's selectInitOsdcInfo.
func (p *Provider) RegisterDevice(ctx context.Context, org fiscal.OrgFiscalProfile) (fiscal.DeviceRef, error) {
	if !pinRe.MatchString(org.KRAPIN) {
		return fiscal.DeviceRef{}, &fiscal.ValidationError{Code: "seller_pin_invalid", Field: "kra_pin", Message: "KRA PIN must match A/P + 9 digits + letter"}
	}
	branch := org.BranchID
	if branch == "" {
		branch = "00"
	}
	serial := org.DeviceSerial
	if serial == "" {
		serial = p.cfg.DeviceSerial
	}
	if serial == "" {
		return fiscal.DeviceRef{}, &fiscal.ValidationError{Code: "device_serial_required", Field: "device_serial", Message: "a KRA device serial (dvcSrlNo) is required to initialise OSCU"}
	}
	info, _, err := p.c.Initialize(ctx, org.KRAPIN, branch, serial)
	if err != nil {
		return fiscal.DeviceRef{}, err
	}
	raw, err := json.Marshal(deviceProfile{
		DeviceID: info.DeviceID, BranchID: info.BranchID, DeviceSerial: serial,
		CmcKey: info.CmcKey, SDCID: info.SDCID, MRCNo: info.MRCNo,
	})
	if err != nil {
		return fiscal.DeviceRef{}, err
	}
	return fiscal.DeviceRef{DeviceID: info.DeviceID, BranchID: info.BranchID, Raw: raw}, nil
}

const salesPath = "/etims-api/insertTrnsSalesReq"

type wireLine struct {
	ItemSeq  int    `json:"itemSeq"`
	ItemCd   string `json:"itemCd"`
	ItemNm   string `json:"itemNm"`
	Qty      string `json:"qty"`
	Prc      int64  `json:"prc"`
	SplyAmt  int64  `json:"splyAmt"`
	TaxTyCd  string `json:"taxTyCd"`
	TaxblAmt int64  `json:"taxblAmt"`
	TaxAmt   int64  `json:"taxAmt"`
	TotAmt   int64  `json:"totAmt"`
}

type wireSale struct {
	TIN        string     `json:"tin"`
	BhfID      string     `json:"bhfId"`
	InvcNo     string     `json:"invcNo"` // idempotency key on our side (see Provider.acks)
	OrgInvcNo  string     `json:"orgInvcNo,omitempty"`
	CustTin    string     `json:"custTin,omitempty"`
	RcptTyCd   string     `json:"rcptTyCd"` // "S" sale, "R" credit note/refund
	PmtTyCd    string     `json:"pmtTyCd"`  // "04" mobile money
	SalesDt    string     `json:"salesDt"`
	TotItemCnt int        `json:"totItemCnt"`
	TotTaxblAmt int64     `json:"totTaxblAmt"`
	TotTaxAmt  int64      `json:"totTaxAmt"`
	TotAmt     int64      `json:"totAmt"`
	ItemList   []wireLine `json:"itemList"`
}

type saleData struct {
	RcptNo         string `json:"rcptNo"`
	IntrlData      string `json:"intrlData"`
	RcptSign       string `json:"rcptSign"`
	VsdcRcptPbctDt string `json:"vsdcRcptPbctDate"`
	QrCodeURL      string `json:"qrCodeUrl"`
}

// SubmitInvoice implements fiscal.Provider.
func (p *Provider) SubmitInvoice(ctx context.Context, inv fiscal.Invoice) (fiscal.Ack, error) {
	if err := validateInvoice(inv); err != nil {
		return fiscal.Ack{}, err
	}
	prof := decodeDeviceProfile(inv.DeviceProfile)
	if prof.CmcKey == "" {
		return fiscal.Ack{}, &fiscal.ConfigError{Code: "device_not_registered", Message: "this org has no cmcKey yet; configure eTIMS (RegisterDevice) first"}
	}
	w := wireSale{
		TIN: inv.SellerPIN, BhfID: coalesce(prof.BranchID, inv.BranchID, "00"), CustTin: inv.BuyerPIN,
		RcptTyCd: "S", PmtTyCd: "04", SalesDt: inv.IssuedAt.UTC().Format("20060102"),
		TotItemCnt: len(inv.Lines), TotTaxblAmt: inv.SubtotalCents, TotTaxAmt: inv.TaxCents, TotAmt: inv.TotalCents,
		ItemList: toWireLines(inv.Lines),
	}
	return p.submit(ctx, inv.ID, prof, w)
}

// SubmitCreditNote implements fiscal.Provider.
func (p *Provider) SubmitCreditNote(ctx context.Context, cn fiscal.CreditNote) (fiscal.Ack, error) {
	if cn.OriginalKRANo == "" {
		return fiscal.Ack{}, &fiscal.ValidationError{Code: "original_invoice_missing", Field: "original_kra_no", Message: "credit note needs the original KRA invoice number"}
	}
	prof := decodeDeviceProfile(cn.DeviceProfile)
	if prof.CmcKey == "" {
		return fiscal.Ack{}, &fiscal.ConfigError{Code: "device_not_registered", Message: "this org has no cmcKey yet; configure eTIMS (RegisterDevice) first"}
	}
	subtotal, tax, total := fiscal.Totals(cn.Lines)
	w := wireSale{
		TIN: cn.OriginalInvoice.SellerPIN, BhfID: coalesce(prof.BranchID, cn.OriginalInvoice.BranchID, "00"),
		CustTin: cn.OriginalInvoice.BuyerPIN, OrgInvcNo: cn.OriginalKRANo, RcptTyCd: "R", PmtTyCd: "04",
		SalesDt: time.Now().UTC().Format("20060102"), TotItemCnt: len(cn.Lines),
		TotTaxblAmt: -abs64(subtotal), TotTaxAmt: -abs64(tax), TotAmt: -abs64(total),
		ItemList: toWireLines(cn.Lines),
	}
	if w.TotAmt >= 0 {
		return fiscal.Ack{}, &fiscal.ValidationError{Code: "credit_note_not_negative", Field: "total_cents", Message: "credit note total must be negative"}
	}
	return p.submit(ctx, cn.ID, prof, w)
}

func (p *Provider) submit(ctx context.Context, id string, prof deviceProfile, w wireSale) (fiscal.Ack, error) {
	if err := ctx.Err(); err != nil {
		return fiscal.Ack{}, err
	}
	p.mu.Lock()
	if ack, ok := p.acks[id]; ok {
		p.mu.Unlock()
		return ack, nil // idempotent replay, see Provider.acks
	}
	p.mu.Unlock()

	w.InvcNo = id
	tok, err := p.c.Token(ctx)
	if err != nil {
		return fiscal.Ack{}, err
	}
	body, err := json.Marshal(w)
	if err != nil {
		return fiscal.Ack{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+salesPath, bytes.NewReader(body))
	if err != nil {
		return fiscal.Ack{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("tin", w.TIN)
	req.Header.Set("bhfId", w.BhfID)
	req.Header.Set("cmcKey", prof.CmcKey)

	raw, status, err := p.c.doRaw(req)
	if err != nil {
		return fiscal.Ack{}, err
	}
	if err := classifyStatus(status, raw); err != nil {
		return fiscal.Ack{}, err
	}
	var env resultEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fiscal.Ack{}, &fiscal.TransientError{Code: "oscu_bad_json", Message: err.Error()}
	}
	if env.ResultCd != "" && env.ResultCd != "000" {
		return fiscal.Ack{}, classifyResultCode(env.ResultCd, env.ResultMsg)
	}
	var d saleData
	_ = json.Unmarshal(env.Data, &d)
	kraNo := d.RcptNo
	if kraNo == "" {
		kraNo = d.IntrlData
	}
	if kraNo == "" || d.RcptSign == "" {
		return fiscal.Ack{}, &fiscal.TransientError{Code: "oscu_bad_ack", Message: "sales response missing rcptNo/rcptSign"}
	}
	receivedAt := time.Now().UTC()
	ack := fiscal.Ack{KRAInvoiceNo: kraNo, Signature: d.RcptSign, QRPayload: d.QrCodeURL, ReceivedAt: receivedAt, Raw: raw}
	p.mu.Lock()
	p.acks[id] = ack
	p.mu.Unlock()
	return ack, nil
}

const itemCodePath = "/etims-api/selectItemClsCodeList"

// LookupItemCodes implements fiscal.Provider.
func (p *Provider) LookupItemCodes(ctx context.Context, q string) ([]fiscal.ItemCode, error) {
	tok, err := p.c.Token(ctx)
	if err != nil {
		return nil, err
	}
	u := strings.TrimRight(p.cfg.BaseURL, "/") + itemCodePath
	if q != "" {
		u += "?cls=" + url.QueryEscape(q)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	raw, status, err := p.c.doRaw(req)
	if err != nil {
		return nil, err
	}
	if err := classifyStatus(status, raw); err != nil {
		return nil, err
	}
	var env resultEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, &fiscal.TransientError{Code: "oscu_bad_json", Message: err.Error()}
	}
	if env.ResultCd != "" && env.ResultCd != "000" {
		return nil, classifyResultCode(env.ResultCd, env.ResultMsg)
	}
	var d struct {
		ItemClsList []struct {
			ItemClsCd string `json:"itemClsCd"`
			ItemClsNm string `json:"itemClsNm"`
			TaxTyCd   string `json:"taxTyCd"`
		} `json:"itemClsList"`
	}
	_ = json.Unmarshal(env.Data, &d)
	out := make([]fiscal.ItemCode, 0, len(d.ItemClsList))
	for _, c := range d.ItemClsList {
		cat := fiscal.TaxCategory(c.TaxTyCd)
		if !cat.Valid() {
			continue
		}
		out = append(out, fiscal.ItemCode{Code: c.ItemClsCd, Description: c.ItemClsNm, TaxCategory: cat})
	}
	return out, nil
}

func toWireLines(lines []fiscal.Line) []wireLine {
	out := make([]wireLine, 0, len(lines))
	for i, l := range lines {
		out = append(out, wireLine{
			ItemSeq: i + 1, ItemCd: l.ItemCode, ItemNm: l.Description, Qty: l.Qty, Prc: l.UnitPriceCents,
			SplyAmt: l.LineTotalCents - l.LineTaxCents, TaxTyCd: string(l.TaxCategory),
			TaxblAmt: l.LineTotalCents - l.LineTaxCents, TaxAmt: l.LineTaxCents, TotAmt: l.LineTotalCents,
		})
	}
	return out
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

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// shortHash is used only by tests in this package that need a deterministic
// filler value; kept here (rather than in _test.go) so both the oscu and
// oscu_test packages can share it without an import cycle.
func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
