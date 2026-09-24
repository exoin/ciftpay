// Command worker runs the River job workers: fiscal.submit_invoice,
// notify.send_receipt and the periodic mpesa.reconcile_payments sweep.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/riverqueue/river"

	"github.com/exoin/ciftpay/internal/boot"
	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/ledger"
	"github.com/exoin/ciftpay/internal/mpesa"
	"github.com/exoin/ciftpay/internal/notify"
	"github.com/exoin/ciftpay/internal/platform/jobs"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "worker:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d, err := boot.Load(ctx)
	if err != nil {
		return err
	}
	defer d.Close()

	provider, err := boot.FiscalProvider(d.Cfg.Fiscal, d.Cfg.KRA)
	if err != nil {
		return err
	}

	// The workers enqueue follow-up jobs (receipt after ack) through the same
	// River client they run under, so build the client in two steps.
	workers := river.NewWorkers()
	jc, err := jobs.New(d.DB.Pool, workers)
	if err != nil {
		return err
	}
	ledgerSvc := ledger.New(d.DB, jc, d.Keys, d.Log)
	submitter := fiscal.NewSubmitter(d.DB, jc, d.Keys, provider, d.Log)
	notifier := d.Notifier()
	daraja := mpesa.NewClient(d.Cfg.Daraja)
	reconciler := mpesa.NewReconciler(d.DB, ledgerSvc, d.Log)
	pullWorker := mpesa.NewPullWorker(d.DB, daraja, ledgerSvc, d.Log)

	river.AddWorker(workers, &fiscal.Worker{S: submitter})
	river.AddWorker(workers, &notify.Worker{S: notifier})
	river.AddWorker(workers, &mpesa.Worker{R: reconciler})
	river.AddWorker(workers, pullWorker)

	if err := jc.Start(ctx); err != nil {
		return err
	}
	d.Log.Info("worker started", "env", d.Cfg.AppEnv, "fiscal_adapter", provider.Name(), "mock_fail_mode", d.Cfg.Fiscal.MockFailMode)
	<-ctx.Done()

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d.Log.Info("worker draining")
	return jc.Stop(stopCtx)
}
