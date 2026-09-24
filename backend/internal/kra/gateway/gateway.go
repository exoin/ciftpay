package gateway

import (
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
)

var (
	PINRe           = regexp.MustCompile(`^[AP][0-9]{9}[A-Z]$`)
	ErrPINInvalid   = errors.New("kra: pin must match format A/P + 9 digits + letter")
	ErrPINNotFound  = errors.New("kra: pin not found in KRA registry")
	ErrNotConfigured = errors.New("kra: gateway credentials not configured")
)

// TokenProvider returns a cached or fresh OAuth2 Bearer token.
type TokenProvider interface {
	Token(ctx context.Context) (string, error)
}

// Config configures the KRA Gateway client.
type Config struct {
	BaseURL        string
	APIBaseURL     string
	ConsumerKey    string
	ConsumerSecret string
	DNSResolver    string
	Timeout        time.Duration
	UseMockGateway bool
	IsProduction   bool
}

// TaxpayerDetails is the payload returned by PIN Checker.
type TaxpayerDetails struct {
	PIN            string `json:"pin"`
	TaxpayerName   string `json:"taxpayer_name"`
	TaxpayerStatus string `json:"taxpayer_status"` // ACTIVE, CANCELLED, etc.
	VATRegistered  bool   `json:"vat_registered"`
	Email          string `json:"email,omitempty"`
}

// TaxObligation represents one registered obligation under KRA (VAT, Income Tax, PAYE, etc.).
type TaxObligation struct {
	ObligationID   string `json:"obligation_id"`
	ObligationName string `json:"obligation_name"`
	EffectiveFrom  string `json:"effective_from"`
	Status         string `json:"status"` // ACTIVE, INACTIVE
}

// Client interacts with KRA developer / gateway endpoints.
type Client struct {
	cfg           Config
	http          *http.Client
	tokenProvider TokenProvider

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewClient builds a KRA Gateway client.
// Reuses token generation from tokenProvider if supplied (e.g. from oscu.Client),
// otherwise uses internal OAuth 2.0 client credentials.
func NewClient(cfg Config, tp TokenProvider) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 15 * time.Second
	}
	transport := buildTransport(cfg.DNSResolver)
	c := &Client{
		cfg:           cfg,
		http:          &http.Client{Timeout: cfg.Timeout, Transport: transport},
		tokenProvider: tp,
	}
	return c
}

// Token implements OAuth 2.0 token generation if no external TokenProvider is provided.
func (c *Client) Token(ctx context.Context) (string, error) {
	if c.tokenProvider != nil {
		return c.tokenProvider.Token(ctx)
	}

	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.tokenExp) {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	if c.cfg.ConsumerKey == "" || c.cfg.ConsumerSecret == "" {
		return "", ErrNotConfigured
	}

	reqURL := strings.TrimRight(c.cfg.BaseURL, "/") + "/v1/token/generate?grant_type=client_credentials"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.cfg.ConsumerKey, c.cfg.ConsumerSecret)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("kra: oauth token request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("kra: read token body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("kra: token returned status %d: %s", resp.StatusCode, string(raw))
	}

	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   any    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil || tr.AccessToken == "" {
		return "", fmt.Errorf("kra: invalid token response: %w", err)
	}

	ttl := 55 * time.Minute
	c.mu.Lock()
	c.token = tr.AccessToken
	c.tokenExp = time.Now().Add(ttl)
	c.mu.Unlock()

	return tr.AccessToken, nil
}

// CheckPIN validates a KRA PIN and returns taxpayer legal details.
func (c *Client) CheckPIN(ctx context.Context, rawPIN string) (TaxpayerDetails, error) {
	pin := strings.ToUpper(strings.TrimSpace(rawPIN))
	if !PINRe.MatchString(pin) {
		return TaxpayerDetails{}, ErrPINInvalid
	}

	// STRICT GUARDRAIL: Only allow mock deterministic data if explicitly enabled via USE_MOCK_KRA_GATEWAY
	// AND NOT in production.
	if c.cfg.UseMockGateway && !c.cfg.IsProduction {
		return mockTaxpayer(pin), nil
	}

	// Real HTTP call to developer.go.ke / KRA gateway
	tok, err := c.Token(ctx)
	if err != nil {
		return TaxpayerDetails{}, err
	}

	apiBase := c.apiBaseURL()
	reqURL := fmt.Sprintf("%s/kra/pin/%s", apiBase, url.PathEscape(pin))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return TaxpayerDetails{}, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return TaxpayerDetails{}, fmt.Errorf("kra: pin checker network error: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TaxpayerDetails{}, fmt.Errorf("kra: read pin response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return TaxpayerDetails{}, ErrPINNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TaxpayerDetails{}, fmt.Errorf("kra: gateway returned status %d: %s", resp.StatusCode, string(raw))
	}

	var res struct {
		PIN          string `json:"pin"`
		TaxpayerName string `json:"taxpayerName"`
		TaxprNm      string `json:"taxprNm"`
		Name         string `json:"name"`
		Status       string `json:"status"`
		TaxpayerSts  string `json:"taxpayerSts"`
		VATReg       bool   `json:"vatRegistered"`
		Email        string `json:"email"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return TaxpayerDetails{}, fmt.Errorf("kra: invalid pin checker json response: %w", err)
	}

	name := res.TaxpayerName
	if name == "" {
		name = res.TaxprNm
	}
	if name == "" {
		name = res.Name
	}
	status := res.Status
	if status == "" {
		status = res.TaxpayerSts
	}
	if status == "" {
		status = "ACTIVE"
	}

	return TaxpayerDetails{
		PIN:            pin,
		TaxpayerName:   strings.TrimSpace(name),
		TaxpayerStatus: strings.TrimSpace(status),
		VATRegistered:  res.VATReg || pin[0] == 'A',
		Email:          res.Email,
	}, nil
}

// FetchObligations queries registered tax obligations for a given PIN.
func (c *Client) FetchObligations(ctx context.Context, rawPIN string) ([]TaxObligation, error) {
	pin := strings.ToUpper(strings.TrimSpace(rawPIN))
	if !PINRe.MatchString(pin) {
		return nil, ErrPINInvalid
	}

	// STRICT GUARDRAIL: Only allow mock deterministic data if explicitly enabled via USE_MOCK_KRA_GATEWAY
	// AND NOT in production.
	if c.cfg.UseMockGateway && !c.cfg.IsProduction {
		return mockObligations(pin), nil
	}

	tok, err := c.Token(ctx)
	if err != nil {
		return nil, err
	}

	apiBase := c.apiBaseURL()
	reqURL := fmt.Sprintf("%s/kra/obligations/%s", apiBase, url.PathEscape(pin))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kra: obligations network error: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("kra: read obligations response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPINNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kra: gateway obligations status %d: %s", resp.StatusCode, string(raw))
	}

	var res struct {
		Obligations []struct {
			ID            string `json:"obligationId"`
			ObligationID  string `json:"obligation_id"`
			Name          string `json:"obligationName"`
			ObligationNm  string `json:"obligation_name"`
			EffectiveFrom string `json:"effectiveFrom"`
			Status        string `json:"status"`
		} `json:"obligations"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("kra: parse obligations json: %w", err)
	}

	out := make([]TaxObligation, 0, len(res.Obligations))
	for _, o := range res.Obligations {
		id := o.ObligationID
		if id == "" {
			id = o.ID
		}
		name := o.ObligationNm
		if name == "" {
			name = o.Name
		}
		st := o.Status
		if st == "" {
			st = "ACTIVE"
		}
		out = append(out, TaxObligation{
			ObligationID:   id,
			ObligationName: name,
			EffectiveFrom:  o.EffectiveFrom,
			Status:         st,
		})
	}
	return out, nil
}

func (c *Client) apiBaseURL() string {
	if c.cfg.APIBaseURL != "" {
		return strings.TrimRight(c.cfg.APIBaseURL, "/")
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if base == "https://sbx.kra.go.ke" {
		return "https://etims-api-sbx.kra.go.ke"
	}
	if base == "https://api.kra.go.ke" {
		return "https://etims-api.kra.go.ke"
	}
	return base
}

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

func mockTaxpayer(pin string) TaxpayerDetails {
	sum := sha256.Sum256([]byte(pin))
	hexSuffix := strings.ToUpper(hex.EncodeToString(sum[:3]))
	isCompany := pin[0] == 'A'
	name := "TAXPAYER ENTERPRISE " + hexSuffix
	if isCompany {
		name = "SAFARICOM COMMERCIAL PARTNER " + hexSuffix + " LTD"
	}
	return TaxpayerDetails{
		PIN:            pin,
		TaxpayerName:   name,
		TaxpayerStatus: "ACTIVE",
		VATRegistered:  isCompany,
		Email:          "taxpayer." + strings.ToLower(hexSuffix) + "@example.com",
	}
}

func mockObligations(pin string) []TaxObligation {
	isCompany := pin[0] == 'A'
	obs := []TaxObligation{
		{
			ObligationID:   "IT01",
			ObligationName: "Income Tax - Resident Individual",
			EffectiveFrom:  "2020-01-01",
			Status:         "ACTIVE",
		},
	}
	if isCompany {
		obs[0].ObligationName = "Income Tax - Company"
		obs = append(obs, TaxObligation{
			ObligationID:   "VAT01",
			ObligationName: "Value Added Tax (VAT)",
			EffectiveFrom:  "2021-06-01",
			Status:         "ACTIVE",
		})
	}
	return obs
}
