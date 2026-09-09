// Package boot wires the shared dependencies of api, worker and ciftctl from
// config so the three mains stay short and cannot drift apart.
package boot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ciftpay/ciftpay/internal/fiscal"
	"github.com/ciftpay/ciftpay/internal/fiscal/mock"
	"github.com/ciftpay/ciftpay/internal/fiscal/vendor"
	"github.com/ciftpay/ciftpay/internal/notify"
	"github.com/ciftpay/ciftpay/internal/platform/config"
	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
	"github.com/ciftpay/ciftpay/internal/platform/storage"
)

// Deps are the process-wide singletons.
type Deps struct {
	Cfg  config.Config
	Log  *slog.Logger
	DB   *db.DB
	Keys *crypto.Keyring
}

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

// FiscalProvider builds the adapter selected by FISCAL_ADAPTER.
func FiscalProvider(cfg config.Fiscal) (fiscal.Provider, error) {
	switch cfg.Adapter {
	case "mock":
		mode, err := mock.ParseFailMode(cfg.MockFailMode)
		if err != nil {
			return nil, err
		}
		return mock.New(mode), nil
	case "vendor":
		return vendor.New(vendor.Config{BaseURL: cfg.VendorBaseURL, APIKey: cfg.VendorAPIKey, Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second})
	default:
		return nil, fmt.Errorf("boot: fiscal adapter %q is not available yet", cfg.Adapter)
	}
}

// Notifier builds the notify service on Africa's Talking (or the local sink).
func (d *Deps) Notifier() *notify.Service {
	return notify.New(d.DB, d.Keys, notify.NewATClient(d.Cfg.AT), d.Cfg.PublicBaseURL, d.Log)
}
