// Package config loads the process configuration from environment variables.
// Every variable is documented in .env.example at the repository root.
package config

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/caarlos0/env/v11"
)

// Config is the fully parsed environment for api, worker and ciftctl.
type Config struct {
	AppEnv        string   `env:"APP_ENV" envDefault:"local"`
	HTTPAddr      string   `env:"HTTP_ADDR" envDefault:":8080"`
	PublicBaseURL string   `env:"PUBLIC_BASE_URL" envDefault:"http://localhost:8080"`
	CORSOrigins   []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`

	DatabaseURL      string `env:"DATABASE_URL" envDefault:"postgres://ciftpay:ciftpay@localhost:5432/ciftpay?sslmode=disable"`
	DatabaseMaxConns int32  `env:"DATABASE_MAX_CONNS" envDefault:"10"`

	SessionSecret string `env:"SESSION_SECRET" envDefault:"dev-session-secret-change-me"`
	MasterKeyB64  string `env:"MASTER_KEY_B64" envDefault:"ZGV2LW1hc3Rlci1rZXktMzItYnl0ZXMtZm9yLWRldiE="`
	HashPepper    string `env:"HASH_PEPPER" envDefault:"dev-hash-pepper-change-me"`

	Daraja Daraja
	Fiscal Fiscal
	AT     AfricasTalking
}

// Daraja holds the Safaricom API settings.
type Daraja struct {
	Env            string   `env:"DARAJA_ENV" envDefault:"sandbox"`
	BaseURL        string   `env:"DARAJA_BASE_URL" envDefault:"https://sandbox.safaricom.co.ke"`
	ConsumerKey    string   `env:"DARAJA_CONSUMER_KEY"`
	ConsumerSecret string   `env:"DARAJA_CONSUMER_SECRET"`
	Shortcode      string   `env:"DARAJA_SHORTCODE" envDefault:"174379"`
	Passkey        string   `env:"DARAJA_PASSKEY"`
	WebhookToken   string   `env:"DARAJA_WEBHOOK_TOKEN" envDefault:"dev-webhook-token"`
	IPAllowlist    []string `env:"DARAJA_IP_ALLOWLIST" envSeparator:","`
}

// Fiscal selects and configures the fiscal.Provider adapter.
type Fiscal struct {
	Adapter        string `env:"FISCAL_ADAPTER" envDefault:"mock"`
	VendorBaseURL  string `env:"FISCAL_VENDOR_BASE_URL"`
	VendorAPIKey   string `env:"FISCAL_VENDOR_API_KEY"`
	TimeoutSeconds int    `env:"FISCAL_TIMEOUT_SECONDS" envDefault:"20"`
	MockFailMode   string `env:"MOCK_FAIL_MODE" envDefault:"none"`
}

// AfricasTalking holds the SMS/WhatsApp gateway settings.
type AfricasTalking struct {
	BaseURL  string `env:"AT_BASE_URL" envDefault:"http://localhost:8025"`
	Username string `env:"AT_USERNAME" envDefault:"sandbox"`
	APIKey   string `env:"AT_API_KEY" envDefault:"dev-at-key"`
	SenderID string `env:"AT_SENDER_ID" envDefault:"CIFTPAY"`
}

// Load parses the environment and validates the result.
func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate checks invariants that env parsing cannot express.
func (c Config) Validate() error {
	switch c.AppEnv {
	case "local", "test", "staging", "production":
	default:
		return fmt.Errorf("config: APP_ENV %q is not one of local|test|staging|production", c.AppEnv)
	}
	if _, err := c.MasterKey(); err != nil {
		return err
	}
	switch c.Fiscal.Adapter {
	case "mock", "vendor", "oscu":
	default:
		return fmt.Errorf("config: FISCAL_ADAPTER %q is not one of mock|vendor|oscu", c.Fiscal.Adapter)
	}
	switch c.Fiscal.MockFailMode {
	case "none", "retryable", "terminal":
	default:
		return fmt.Errorf("config: MOCK_FAIL_MODE %q is not one of none|retryable|terminal", c.Fiscal.MockFailMode)
	}
	if c.Fiscal.Adapter == "vendor" && (c.Fiscal.VendorBaseURL == "" || c.Fiscal.VendorAPIKey == "") {
		return fmt.Errorf("config: FISCAL_ADAPTER=vendor requires FISCAL_VENDOR_BASE_URL and FISCAL_VENDOR_API_KEY")
	}
	if c.IsProduction() {
		if strings.HasPrefix(c.SessionSecret, "dev-") || strings.HasPrefix(c.HashPepper, "dev-") {
			return fmt.Errorf("config: dev secrets are not allowed when APP_ENV=production")
		}
		if c.Daraja.WebhookToken == "dev-webhook-token" {
			return fmt.Errorf("config: DARAJA_WEBHOOK_TOKEN must be changed when APP_ENV=production")
		}
	}
	return nil
}

// MasterKey decodes MASTER_KEY_B64 and checks that it is exactly 32 bytes.
func (c Config) MasterKey() ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(c.MasterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("config: MASTER_KEY_B64 is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("config: MASTER_KEY_B64 must decode to 32 bytes, got %d", len(key))
	}
	return key, nil
}

// IsProduction reports whether the process runs with production settings.
func (c Config) IsProduction() bool { return c.AppEnv == "production" }

// IsLocal reports whether dev-only shortcuts (logged OTP codes, no IP allow-list) are enabled.
func (c Config) IsLocal() bool { return c.AppEnv == "local" || c.AppEnv == "test" }
