// Package boot wires the shared dependencies of api, worker and ciftctl from
// config so the three mains stay short and cannot drift apart.
package boot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/fiscal/mock"
	"github.com/exoin/ciftpay/internal/fiscal/oscu"
	"github.com/exoin/ciftpay/internal/fiscal/vendor"
	"github.com/exoin/ciftpay/internal/notify"
	"github.com/exoin/ciftpay/internal/platform/config"
	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	plog "github.com/exoin/ciftpay/internal/platform/log"
	"github.com/exoin/ciftpay/internal/platform/storage"
)

// Deps are the process-wide singletons.
type Deps struct {
	Cfg  config.Config
	Log  *slog.Logger
	DB   *db.DB
	Keys *crypto.Keyring
}

// Files opens the upload store (UPLOAD_DIR) for authorization letters.
func (d *Deps) Files() (storage.Store, error) { return storage.NewLocal(d.Cfg.UploadDir) }

// Load reads config, opens the database and builds the keyring.
func Load(ctx context.Context) (*Deps, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	log := plog.New(cfg.AppEnv)
	key, _ := cfg.MasterKey()
	keys, err := crypto.New(key, cfg.HashPepper)
	if err != nil {
		return nil, err
	}
	d, err := db.Open(ctx, cfg.DatabaseURL, cfg.DatabaseMaxConns)
	if err != nil {
		return nil, err
	}
	return &Deps{Cfg: cfg, Log: log, DB: d, Keys: keys}, nil
}

// Close releases resources.
func (d *Deps) Close() { d.DB.Close() }

// FiscalProvider builds the adapter selected by FISCAL_ADAPTER. kra carries
// CiftPay's own OSCU credentials for the `oscu` adapter (ADR-0009).
func FiscalProvider(cfg config.Fiscal, kra config.KRA) (fiscal.Provider, error) {
	switch cfg.Adapter {
	case "mock":
		mode, err := mock.ParseFailMode(cfg.MockFailMode)
		if err != nil {
			return nil, err
		}
		return mock.New(mode), nil
	case "vendor":
		return vendor.New(vendor.Config{BaseURL: cfg.VendorBaseURL, APIKey: cfg.VendorAPIKey, Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second})
	case "oscu":
		return oscu.New(OSCUConfig(kra, cfg))
	default:
		return nil, fmt.Errorf("boot: fiscal adapter %q is not available yet", cfg.Adapter)
	}
}

// OSCUConfig maps the environment onto the direct KRA client's settings.
func OSCUConfig(kra config.KRA, f config.Fiscal) oscu.Config {
	return oscu.Config{
		BaseURL: kra.BaseURL, ConsumerKey: kra.ConsumerKey, ConsumerSecret: kra.ConsumerSecret,
		DeviceSerial: kra.DeviceSerial, DNSResolver: kra.DNSResolver, Timeout: time.Duration(f.TimeoutSeconds) * time.Second,
	}
}

// Notifier builds the notify service on Africa's Talking (or the local sink).
func (d *Deps) Notifier() *notify.Service {
	return notify.New(d.DB, d.Keys, notify.NewATClient(d.Cfg.AT), d.Cfg.PublicBaseURL, d.Log)
}
