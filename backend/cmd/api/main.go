// Command api is the CiftPay HTTP server: merchant/accountant/admin REST
// API, Daraja and Africa's Talking webhooks and the public receipt endpoint.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ciftpay/ciftpay/internal/admin"
	"github.com/ciftpay/ciftpay/internal/billing"
	"github.com/ciftpay/ciftpay/internal/boot"
	"github.com/ciftpay/ciftpay/internal/fiscal"
	"github.com/ciftpay/ciftpay/internal/ledger"
	"github.com/ciftpay/ciftpay/internal/mpesa"
	"github.com/ciftpay/ciftpay/internal/notify"
	"github.com/ciftpay/ciftpay/internal/org"
	"github.com/ciftpay/ciftpay/internal/platform/httpx"
	"github.com/ciftpay/ciftpay/internal/platform/jobs"
	"github.com/ciftpay/ciftpay/internal/publicapi"
	"github.com/ciftpay/ciftpay/internal/reports"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the running server's /healthz and exit")
	flag.Parse()
	if *healthcheck {
		os.Exit(probe())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "api:", err)
		os.Exit(1)
	}
}

// probe backs the docker-compose healthcheck without curl in the image.
func probe() int {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1"+addr+"/healthz", nil)
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d, err := boot.Load(ctx)
	if err != nil {
		return err
	}
	defer d.Close()
	cfg, log := d.Cfg, d.Log

	jc, err := jobs.NewInsertOnly(d.DB.Pool)
	if err != nil {
		return err
	}
	provider, err := boot.FiscalProvider(cfg.Fiscal)
	if err != nil {
		return err
	}

	// Services.
	ledgerSvc := ledger.New(d.DB, jc, d.Keys, log)
	notifier := d.Notifier()
	orgSvc := org.New(d.DB, d.Keys, notifier, nil, cfg.SessionSecret, cfg.Fiscal.Adapter, cfg.IsLocal(), log)
	submitter := fiscal.NewSubmitter(d.DB, jc, d.Keys, provider, log)
	billingSvc := billing.New(d.DB)
	daraja := mpesa.NewClient(cfg.Daraja)

	// Handlers.
	orgH := &org.Handler{S: orgSvc, SecureCookie: !cfg.IsLocal()}
	ledgerH := &ledger.Handler{S: ledgerSvc, Keys: d.Keys, STK: stkAdapter{daraja}, Retrier: submitter, PublicBaseURL: cfg.PublicBaseURL}
	reportsH := &reports.Handler{S: reports.New(d.DB)}
	adminH := &admin.Handler{S: admin.New(d.DB)}
	receiptH := &publicapi.Handler{S: publicapi.New(d.DB, d.Keys), Log: log}
	webhooks := &mpesa.Webhooks{Token: cfg.Daraja.WebhookToken, Ingest: ledgerSvc, Log: log}
	if !cfg.IsLocal() {
		webhooks.IPAllowlist = cfg.Daraja.IPAllowlist
	}
	atH := &notify.DeliveryWebhook{DB: d.DB, Log: log}

	r := chi.NewRouter()
	r.Use(httpx.RequestID, middleware.RealIP, httpx.Recover(log), httpx.Logger(log), httpx.SecurityHeaders, httpx.CORS(cfg.CORSOrigins))
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		status := map[string]string{"db": "ok", "queue": "ok", "fiscal": provider.Name()}
		code := http.StatusOK
		if err := d.DB.Ping(ctx); err != nil {
			status["db"], code = err.Error(), http.StatusServiceUnavailable
		}
		if err := jc.Health(ctx, d.DB.Pool); err != nil {
			status["queue"], code = err.Error(), http.StatusServiceUnavailable
		}
		httpx.JSON(w, code, status)
	})

	r.Route("/webhooks", func(r chi.Router) {
		webhooks.Mount(r)
		atH.Mount(r)
	})
	receiptH.Mount(r)
	orgH.MountPublic(r)
	r.Get("/plans", func(w http.ResponseWriter, r *http.Request) {
		plans, err := billingSvc.Plans(r.Context())
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not list plans")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"data": plans})
	})

	r.Group(func(r chi.Router) {
		r.Use(orgH.Authenticate)
		orgH.MountPrivate(r)
		r.Group(func(r chi.Router) {
			r.Use(org.RequireOrg)
			ledgerH.Mount(r)
			reportsH.Mount(r)
			r.Get("/billing/entitlement", func(w http.ResponseWriter, r *http.Request) {
				p, _ := httpx.PrincipalFrom(r.Context())
				e, err := billingSvc.Entitlement(r.Context(), p.OrgID)
				if err != nil {
					httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not load the plan")
					return
				}
				httpx.JSON(w, http.StatusOK, e)
			})
		})
		r.With(org.RequireRole(org.RoleAdmin)).Group(adminH.Mount)
	})

	srv := &http.Server{
		Addr: cfg.HTTPAddr, Handler: r,
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.AppEnv, "fiscal_adapter", provider.Name(), "daraja_configured", daraja.Configured())
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	log.Info("api shutting down")
	return srv.Shutdown(shutdownCtx)
}

// stkAdapter narrows mpesa.Client to ledger.STKPusher.
type stkAdapter struct{ c *mpesa.Client }

func (a stkAdapter) Configured() bool { return a.c.Configured() }

func (a stkAdapter) STKPush(ctx context.Context, shortcode, msisdn string, amountCents int64, accountRef, desc, publicBaseURL string) (string, string, error) {
	res, err := a.c.STKPush(ctx, shortcode, msisdn, amountCents, accountRef, desc, publicBaseURL)
	return res.CheckoutRequestID, res.MerchantRequestID, err
}
