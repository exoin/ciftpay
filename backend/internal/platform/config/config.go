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
	AppEnv        string `env:"APP_ENV" envDefault:"local"`
	HTTPAddr      string `env:"HTTP_ADDR" envDefault:":8080"`
	PublicBaseURL string `env:"PUBLIC_BASE_URL" envDefault:"http://localhost:3000"`
	// WebhookBaseURL is the api's own public URL, the base of the callback URLs
	// handed to Daraja (RegisterURL, STK). Distinct from PublicBaseURL, which is
	// the web app buyers open receipt links on.
	WebhookBaseURL string   `env:"WEBHOOK_BASE_URL" envDefault:"http://localhost:8080"`
	CORSOrigins    []string `env:"CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`

	DatabaseURL      string `env:"DATABASE_URL" envDefault:"postgres://ciftpay:ciftpay@localhost:5432/ciftpay?sslmode=disable"`
	DatabaseMaxConns int32  `env:"DATABASE_MAX_CONNS" envDefault:"10"`

	SessionSecret string `env:"SESSION_SECRET" envDefault:"dev-session-secret-change-me"`
	MasterKeyB64  string `env:"MASTER_KEY_B64" envDefault:"ZGV2LW1hc3Rlci1rZXktMzItYnl0ZXMtZm9yLWRldiE="`
	HashPepper    string `env:"HASH_PEPPER" envDefault:"dev-hash-pepper-change-me"`

	// UploadDir is where merchant uploads (the signed Safaricom authorization
	// letters, ADR-0008) are kept. Never served publicly.
	UploadDir string `env:"UPLOAD_DIR" envDefault:"./var/uploads"`

	Daraja Daraja
	Fiscal Fiscal
	KRA    KRA
	AT     AfricasTalking
}

// Daraja holds the Safaricom API settings. There is deliberately no passkey:
// CiftPay never initiates M-Pesa transactions in Phase 1 (ADR-0003, ADR-0008).
type Daraja struct {
	Env            string `env:"DARAJA_ENV" envDefault:"sandbox"`
	BaseURL        string `env:"DARAJA_BASE_URL" envDefault:"https://sandbox.safaricom.co.ke"`
	ConsumerKey    string `env:"DARAJA_CONSUMER_KEY"`
	ConsumerSecret string `env:"DARAJA_CONSUMER_SECRET"`
	// Shortcode is CiftPay's own C2B shortcode (600000 in the sandbox), used by
	// the sandbox tooling (ciftctl register-urls / simulate-c2b).
	Shortcode    string   `env:"DARAJA_SHORTCODE" envDefault:"600000"`
	WebhookToken string   `env:"DARAJA_WEBHOOK_TOKEN" envDefault:"dev-webhook-token"`
	IPAllowlist  []string `env:"DARAJA_IP_ALLOWLIST" envSeparator:","`
}

// Fiscal selects and configures the fiscal.Provider adapter. `oscu` reads its
// credentials from KRA.
type Fiscal struct {
	Adapter        string `env:"FISCAL_ADAPTER" envDefault:"mock"`
	VendorBaseURL  string `env:"FISCAL_VENDOR_BASE_URL"`
	VendorAPIKey   string `env:"FISCAL_VENDOR_API_KEY"`
	TimeoutSeconds int    `env:"FISCAL_TIMEOUT_SECONDS" envDefault:"20"`
	MockFailMode   string `env:"MOCK_FAIL_MODE" envDefault:"none"`
}

// KRA holds CiftPay's own direct eTIMS OSCU developer credentials (ADR-0009).
// The gateway issues OAuth tokens on GET /v1/token/generate with HTTP Basic
// Base64(key:secret); the token then authorises the eTIMS OSCU calls.
type KRA struct {
	// Env is sandbox | production. Only the sandbox is wired until KRA
	// certification; production refuses to start with sandbox URLs.
	Env string `env:"KRA_OSCU_ENV" envDefault:"sandbox"`
	// BaseURL is the API gateway origin (https://sbx.kra.go.ke in the sandbox).
	BaseURL        string `env:"KRA_OSCU_BASE_URL" envDefault:"https://sbx.kra.go.ke"`
	APIBaseURL     string `env:"KRA_OSCU_API_BASE_URL"`
	ConsumerKey    string `env:"KRA_OSCU_CONSUMER_KEY"`
	ConsumerSecret string `env:"KRA_OSCU_CONSUMER_SECRET"`
	// DeviceSerial is the dvcSrlNo registered on the eTIMS portal for the
	// CiftPay OSCU; device initialisation (selectInitOsdcInfo) returns the
	// cmcKey for it.
	DeviceSerial string `env:"KRA_OSCU_DEVICE_SERIAL"`
	// DNSResolver is the UDP address the KRA client resolves hostnames
	// through. sbx.kra.go.ke has a DNSSEC misconfiguration that makes local
	// validating resolvers (systemd-resolved) answer SERVFAIL, so the client
	// bypasses them. Empty uses the system resolver. This is a narrowly-scoped
	// workaround (it only affects the oscu HTTP client's own transport, and is
	// fully opt-out-able) for a local/sandbox DNS quirk; production
	// infrastructure should still fix its resolver rather than lean on this
	// long-term.
	DNSResolver string `env:"KRA_OSCU_DNS_RESOLVER" envDefault:"8.8.8.8:53"`

	// TestPIN, TestBhfID and TestDeviceSerial are a KRA sandbox developer-portal
	// test identity, used only by the oscu package's own smoke test
	// (TestLiveSandbox, skipped unless all three are set) and by `ciftctl
	// kra-init`'s flag defaults, so the direct OSCU path can be exercised
	// before any merchant has configured orgs.kra_bhf_id / kra_device_serial.
	TestPIN          string `env:"KRA_OSCU_TEST_PIN"`
	TestBhfID        string `env:"KRA_OSCU_TEST_BHF_ID" envDefault:"00"`
	TestDeviceSerial string `env:"KRA_OSCU_TEST_DEVICE_SERIAL"`

	// UseMockGateway enables mock responses for KRA gateway endpoints (/kra/pin, /kra/obligations).
	// STRICT GUARDRAIL: Only permitted when APP_ENV != production. If APP_ENV == production,
	// this flag is rejected in Validate().
	UseMockGateway bool `env:"USE_MOCK_KRA_GATEWAY" envDefault:"false"`
}

// Configured reports whether the OSCU credentials are present.
func (k KRA) Configured() bool { return k.ConsumerKey != "" && k.ConsumerSecret != "" }

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
	switch c.KRA.Env {
	case "sandbox", "production":
	default:
		return fmt.Errorf("config: KRA_OSCU_ENV %q is not one of sandbox|production", c.KRA.Env)
	}
	if c.Fiscal.Adapter == "oscu" {
		if !c.KRA.Configured() {
			return fmt.Errorf("config: FISCAL_ADAPTER=oscu requires KRA_OSCU_CONSUMER_KEY and KRA_OSCU_CONSUMER_SECRET")
		}
		if c.KRA.Env == "production" && strings.Contains(c.KRA.BaseURL, "sbx.") {
			return fmt.Errorf("config: KRA_OSCU_ENV=production cannot use the sandbox KRA_OSCU_BASE_URL")
		}
	}
	if c.IsProduction() && c.KRA.UseMockGateway {
		return fmt.Errorf("config: USE_MOCK_KRA_GATEWAY cannot be true when APP_ENV=production")
	}
	if c.IsProduction() {
		if strings.HasPrefix(c.SessionSecret, "dev-") || strings.HasPrefix(c.HashPepper, "dev-") {
			return fmt.Errorf("config: dev secrets are not allowed when APP_ENV=production")
		}
		if c.Daraja.WebhookToken == "dev-webhook-token" {
			return fmt.Errorf("config: DARAJA_WEBHOOK_TOKEN must be changed when APP_ENV=production")
		}
		if c.Daraja.ConsumerKey == "" || c.Daraja.ConsumerSecret == "" {
			return fmt.Errorf("config: DARAJA_CONSUMER_KEY and DARAJA_CONSUMER_SECRET are required when APP_ENV=production")
		}
	}
	if strings.TrimSpace(c.UploadDir) == "" {
		return fmt.Errorf("config: UPLOAD_DIR must not be empty")
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
