package mpesa

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/exoin/ciftpay/internal/ledger"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/jobs"
)

// Reconciler finishes webhook events whose first pass did not complete (api
// crashed between storing the event and writing the payment, database
// hiccup, ...). Phase 1 adds the Daraja TransactionStatus query for payments
// Daraja never called back about.
type Reconciler struct {
	DB     *db.DB
	Ledger *ledger.Service
	Log    *slog.Logger
	Batch  int32
}

// NewReconciler builds a Reconciler.
func NewReconciler(d *db.DB, l *ledger.Service, log *slog.Logger) *Reconciler {
	return &Reconciler{DB: d, Ledger: l, Log: log, Batch: 200}
}

// Stats summarises one sweep.
type Stats struct {
	Seen, Processed, Skipped, Failed int
}

// Run performs one sweep over unprocessed events older than two minutes.
func (r *Reconciler) Run(ctx context.Context) (Stats, error) {
	var st Stats
	var events []gen.WebhookEvent
	err := r.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		events, err = tx.ListUnprocessedWebhookEvents(ctx, r.Batch)
		return err
	})
	if err != nil {
		return st, err
	}
	for _, ev := range events {
		st.Seen++
		if ev.Provider != "mpesa" || (ev.Kind != "c2b_confirmation" && ev.Kind != "reversal") {
			st.Skipped++
			continue
		}
		var p C2BPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.Validate() != nil {
			r.markError(ctx, ev, "payload no longer parses")
			st.Failed++
			continue
		}
		in := ToC2BInput(p, ev.Payload)
		var sc gen.ResolveShortcodeRow
		err := r.DB.WithIngest(ctx, func(ctx context.Context, tx db.Tx) error {
			var err error
			sc, err = r.Ledger.ResolveForC2B(ctx, tx, in)
			return err
		})
		if errors.Is(err, pgx.ErrNoRows) {
			r.markError(ctx, ev, "no verified shortcode "+p.BusinessShortCode)
			st.Failed++
			continue
		}
		if err != nil {
			return st, err
		}
		res, err := r.Ledger.ProcessStoredC2B(ctx, ev.ID, sc, in)
		if err != nil {
			r.Log.Warn("reconcile: event still failing", "event", ev.ID, "trans_id", p.TransID, "err", err)
			st.Failed++
			continue
		}
		st.Processed++
		r.Log.Info("reconcile: event processed", "event", ev.ID, "trans_id", p.TransID, "status", res.Status, "duplicate", res.Duplicate)
	}
	return st, nil
}

func (r *Reconciler) markError(ctx context.Context, ev gen.WebhookEvent, msg string) {
	_ = r.DB.WithIngest(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.MarkWebhookProcessed(ctx, gen.MarkWebhookProcessedParams{ID: ev.ID, Error: db.Ptr(msg)})
	})
}

// Worker adapts Reconciler to River's periodic job.
type Worker struct {
	river.WorkerDefaults[jobs.ReconcilePaymentsArgs]
	R *Reconciler
}

// Work implements river.Worker.
func (w *Worker) Work(ctx context.Context, _ *river.Job[jobs.ReconcilePaymentsArgs]) error {
	start := time.Now()
	st, err := w.R.Run(ctx)
	if err != nil {
		return err
	}
	if st.Seen > 0 {
		w.R.Log.Info("reconcile sweep", "seen", st.Seen, "processed", st.Processed, "skipped", st.Skipped, "failed", st.Failed, "ms", time.Since(start).Milliseconds())
	}
	return nil
}
