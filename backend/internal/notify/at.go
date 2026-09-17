package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/exoin/ciftpay/internal/platform/config"
)

// Sender is the transport abstraction: Africa's Talking in production, the
// sms-sink locally, a fake in tests.
type Sender interface {
	SendSMS(ctx context.Context, to, body string) (SendResult, error)
}

// SendResult is what the gateway returned for one recipient.
type SendResult struct {
	MessageID string
	Status    string
	CostCents int64
}

// ATClient talks to the Africa's Talking messaging API (or the local sink,
// which implements the same request/response shape).
type ATClient struct {
	cfg  config.AfricasTalking
	http *http.Client
}

// NewATClient builds the client.
func NewATClient(cfg config.AfricasTalking) *ATClient {
	return &ATClient{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}}
}

type atResponse struct {
	SMSMessageData struct {
		Message    string `json:"Message"`
		Recipients []struct {
			Number     string `json:"number"`
			Status     string `json:"status"`
			StatusCode int    `json:"statusCode"`
			MessageID  string `json:"messageId"`
			Cost       string `json:"cost"` // "KES 0.8000"
		} `json:"Recipients"`
	} `json:"SMSMessageData"`
}

// SendSMS implements Sender. `to` must be in +2547... form.
func (c *ATClient) SendSMS(ctx context.Context, to, body string) (SendResult, error) {
	form := url.Values{"username": {c.cfg.Username}, "to": {to}, "message": {body}}
	if c.cfg.SenderID != "" {
		form.Set("from", c.cfg.SenderID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/version1/messaging", strings.NewReader(form.Encode()))
	if err != nil {
		return SendResult{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apiKey", c.cfg.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return SendResult{}, fmt.Errorf("notify: at: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 {
		return SendResult{}, fmt.Errorf("notify: at upstream %d", resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return SendResult{}, &PermanentError{Msg: fmt.Sprintf("at %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))}
	}
	var out atResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return SendResult{}, fmt.Errorf("notify: at response: %w", err)
	}
	if len(out.SMSMessageData.Recipients) == 0 {
		return SendResult{}, &PermanentError{Msg: "at: " + out.SMSMessageData.Message}
	}
	r := out.SMSMessageData.Recipients[0]
	if r.StatusCode >= 400 && r.StatusCode < 500 {
		return SendResult{}, &PermanentError{Msg: "at recipient: " + r.Status}
	}
	return SendResult{MessageID: r.MessageID, Status: r.Status, CostCents: parseCost(r.Cost)}, nil
}

// PermanentError means retrying the same message will not help (bad number,
// blocked sender id, invalid credentials).
type PermanentError struct{ Msg string }

func (e *PermanentError) Error() string { return "notify: " + e.Msg }

// parseCost turns "KES 0.8000" into cents (80).
func parseCost(s string) int64 {
	f := strings.Fields(s)
	if len(f) != 2 {
		return 0
	}
	parts := strings.SplitN(f[1], ".", 2)
	whole, _ := strconv.ParseInt(parts[0], 10, 64)
	var frac int64
	if len(parts) == 2 {
		frac, _ = strconv.ParseInt((parts[1] + "00")[:2], 10, 64)
	}
	return whole*100 + frac
}

// E164 renders a normalised 2547XXXXXXXX MSISDN as +2547XXXXXXXX.
func E164(msisdn string) string {
	if strings.HasPrefix(msisdn, "+") {
		return msisdn
	}
	return "+" + msisdn
}
