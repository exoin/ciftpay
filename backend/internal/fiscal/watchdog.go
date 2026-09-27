package fiscal

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/jobs"
)

// RescueStaleInvoices is a periodic worker that finds invoices stuck in SUBMITTED
// for longer than 15 minutes, transitions them to FAILED_RETRYABLE, increments
// their attempt counter, and enqueues a SubmitInvoiceArgs job to let normal backoff resume.
type RescueStaleInvoices struct {
	river.WorkerDefaults[jobs.RescueStaleInvoicesArgs]
	DB   *db.DB
	Jobs *jobs.Client
	Log  *slog.Logger
}

// Watchdog is an alias for RescueStaleInvoices.
type Watchdog = RescueStaleInvoices

// NewRescueStaleInvoices constructs a RescueStaleInvoices worker.
func NewRescueStaleInvoices(d *db.DB, j *jobs.Client, l *slog.Logger) *RescueStaleInvoices {
	return &RescueStaleInvoices{
		DB:   d,
		Jobs: j,
		Log:  l,
	}
}

// Work executes the stale invoices recovery sweep.
func (w *RescueStaleInvoices) Work(ctx context.Context, job *river.Job[jobs.RescueStaleInvoicesArgs]) error {
	var stale []gen.Invoice

	// 1. Scan across all orgs under admin scope.
	if err := w.DB.WithAdmin(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		stale, err = tx.ListStaleSubmittedInvoices(ctx)
		return err
	}); err != nil {
		w.Log.Error("watchdog failed to list stale invoices", "err", err)
		return err
	}

	if len(stale) == 0 {
		return nil
	}

	w.Log.Info("watchdog found stale submitted invoices", "count", len(stale))
	errReason := "rescued from stale SUBMITTED state by watchdog"

	for _, inv := range stale {
		invID := inv.ID
		orgID := inv.OrgID

		err := w.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
			// Increment attempt counter and transition to FAILED_RETRYABLE via CAS.
			attempt := inv.Attempt + 1
			_, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
				ID:            invID,
				State:         string(StateFailedRetryable),
				Attempt:       &attempt,
				ExpectedState: string(StateSubmitted),
				LastError:     &errReason,
			})
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					// Another worker or ack updated the invoice concurrently; skip.
					return nil
				}
				return err
			}

			// Enqueue standard SubmitInvoiceArgs River job so normal retry backoff takes over
			if err := w.Jobs.EnqueueTx(ctx, tx.Tx, jobs.SubmitInvoiceArgs{
				OrgID:     orgID,
				InvoiceID: invID,
			}, nil); err != nil {
				return err
			}

			return tx.AppendAudit(ctx, gen.AppendAuditParams{
				OrgID:     orgID,
				ActorType: "system",
				Action:    "invoice.rescued",
				Entity:    "invoice",
				EntityID:  invID.String(),
				After:     []byte(fmt.Sprintf(`{"reason":%q,"attempt":%d}`, errReason, attempt)),
			})
		})
		if err != nil {
			w.Log.Error("watchdog failed to rescue invoice", "invoice_id", invID, "org_id", orgID, "err", err)
		}
	}

	return nil
}
