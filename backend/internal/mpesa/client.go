package mpesa

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ciftpay/ciftpay/internal/platform/config"
)

// Client is the outbound Daraja client: OAuth token caching, C2B RegisterURL
// and the sandbox C2B simulator. CiftPay initiates no M-Pesa transactions in
// Phase 1 (ADR-0003, ADR-0008); STK push returns ErrSTKNotConfigured until
// Phase 2 request-to-pay brings a passkey for CiftPay's own Paybill.
type Client struct {
	cfg  config.Daraja
	http *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewClient builds a client; it does not call Safaricom until first use.
func NewClient(cfg config.Daraja) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: 20 * time.Second}}
}

// Configured reports whether credentials are present.
func (c *Client) Configured() bool { return c.cfg.ConsumerKey != "" && c.cfg.ConsumerSecret != "" }

// Token returns a cached OAuth bearer token.
func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.tokenExp) > 30*time.Second {
		return c.token, nil
	}
	if !c.Configured() {
		return "", fmt.Errorf("mpesa: DARAJA_CONSUMER_KEY/SECRET not configured")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/oauth/v1/generate?grant_type=client_credentials", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.cfg.ConsumerKey+":"+c.cfg.ConsumerSecret)))
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	if err := c.do(req, &out); err != nil {
		return "", fmt.Errorf("mpesa: oauth: %w", err)
	}
	ttl := time.Hour
	if d, err := time.ParseDuration(out.ExpiresIn + "s"); err == nil {
		ttl = d
	}
	c.token, c.tokenExp = out.AccessToken, time.Now().Add(ttl)
	return c.token, nil
}

// RegisterC2BURLs points a shortcode's validation/confirmation at CiftPay.
// webhookBaseURL is the api's public URL (config WEBHOOK_BASE_URL).
func (c *Client) RegisterC2BURLs(ctx context.Context, shortcode, webhookBaseURL string) error {
	body := map[string]string{
		"ShortCode":       shortcode,
		"ResponseType":    "Completed",
		"ConfirmationURL": fmt.Sprintf("%s/webhooks/daraja/c2b/confirmation/%s", webhookBaseURL, c.cfg.WebhookToken),
		"ValidationURL":   fmt.Sprintf("%s/webhooks/daraja/c2b/validation/%s", webhookBaseURL, c.cfg.WebhookToken),
	}
	var out map[string]any
	return c.post(ctx, "/mpesa/c2b/v1/registerurl", body, &out)
}

// STKPushResult is the synchronous response to an STK push request.
type STKPushResult struct {
	MerchantRequestID   string `json:"MerchantRequestID"`
	CheckoutRequestID   string `json:"CheckoutRequestID"`
	ResponseCode        string `json:"ResponseCode"`
	ResponseDescription string `json:"ResponseDescription"`
	CustomerMessage     string `json:"CustomerMessage"`
}

// ErrSTKNotConfigured is returned by STKPush: Lipa na M-Pesa Online needs a
// per-shortcode passkey, which CiftPay does not hold for anyone in Phase 1.
var ErrSTKNotConfigured = errors.New("mpesa: STK push is not available (no DARAJA passkey; request-to-pay is Phase 2)")

// STKPush is the Phase 2 request-to-pay entry point. The callback URL it will
// use is already routed (/webhooks/daraja/stk/{token}); the request itself is
// disabled until CiftPay's own Paybill is live with a passkey.
func (c *Client) STKPush(ctx context.Context, shortcode, msisdn string, amountCents int64, accountRef, desc, webhookBaseURL string) (STKPushResult, error) {
	_, _, _, _, _, _ = shortcode, msisdn, amountCents, accountRef, desc, webhookBaseURL
	return STKPushResult{}, ErrSTKNotConfigured
}

// SimulateC2B asks the Daraja *sandbox* to emit a C2B confirmation for
// shortcode as if msisdn had paid amountCents (the customer-to-business
// simulator; production has no such endpoint). billRef is the account number
// for Paybill numbers. Used by `ciftctl simulate-c2b` and the fake server.
func (c *Client) SimulateC2B(ctx context.Context, shortcode, msisdn string, amountCents int64, billRef string) (map[string]any, error) {
	if c.cfg.Env == "production" {
		return nil, fmt.Errorf("mpesa: C2B simulate is a sandbox-only endpoint")
	}
	body := map[string]any{
		"ShortCode":     shortcode,
		"CommandID":     "CustomerPayBillOnline",
		"Amount":        amountCents / 100,
		"Msisdn":        msisdn,
		"BillRefNumber": billRef,
	}
	var out map[string]any
	if err := c.post(ctx, "/mpesa/c2b/v1/simulate", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	tok, err := c.Token(ctx)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+path, bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mpesa: %s -> %d: %s", req.URL.Path, resp.StatusCode, bytes.TrimSpace(raw))
	}
	if out != nil && len(raw) > 0 {
		return json.Unmarshal(raw, out)
	}
	return nil
}
