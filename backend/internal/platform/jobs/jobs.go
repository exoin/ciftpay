// Package jobs defines the River job argument types shared by api (enqueue)
// and worker (execute) and a thin client wrapper. Enqueue always happens with
// InsertTx inside the same transaction as the ledger write (ADR-0004).
package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

// Queue names. Fiscal calls are slow and rate-limited by the vendor, so they
// get their own queue with low concurrency.
const (
	QueueFiscal  = "fiscal"
	QueueNotify  = "notify"
	QueueDefault = river.QueueDefault
)

// SubmitInvoiceArgs drives one invoice through the fiscal state machine.
type SubmitInvoiceArgs struct {
	OrgID     uuid.UUID `json:"org_id"`
	InvoiceID uuid.UUID `json:"invoice_id"`
}

// Kind implements river.JobArgs.
func (SubmitInvoiceArgs) Kind() string { return "fiscal.submit_invoice" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (SubmitInvoiceArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       QueueFiscal,
		MaxAttempts: 1, // retries are scheduled by the state machine, not by River
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStateScheduled, rivertype.JobStateRunning, rivertype.JobStatePending, rivertype.JobStateRetryable}},
	}
}

// SendReceiptArgs sends the buyer SMS/WhatsApp for an invoice.
type SendReceiptArgs struct {
	OrgID     uuid.UUID `json:"org_id"`
	InvoiceID uuid.UUID `json:"invoice_id"`
	Template  string    `json:"template"` // receipt_pending | receipt_acked | credit_note
}

// Kind implements river.JobArgs.
func (SendReceiptArgs) Kind() string { return "notify.send_receipt" }

// InsertOpts implements river.JobArgsWithInsertOpts.
func (SendReceiptArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueNotify, MaxAttempts: 5}
}

// ReconcilePaymentsArgs is the periodic sweep for unprocessed webhook events.
type ReconcilePaymentsArgs struct{}

// Kind implements river.JobArgs.
func (ReconcilePaymentsArgs) Kind() string { return "mpesa.reconcile_payments" }

// Client wraps the River client for the api (insert-only) and worker.
type Client struct {
	River *river.Client[pgx.Tx]
}

// NewInsertOnly returns a client that can enqueue but does not run workers.
func NewInsertOnly(pool *pgxpool.Pool) (*Client, error) {
	c, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return nil, err
	}
	return &Client{River: c}, nil
}

// New returns a client that runs the given workers.
func New(pool *pgxpool.Pool, workers *river.Workers) (*Client, error) {
	c, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			QueueDefault: {MaxWorkers: 10},
			QueueFiscal:  {MaxWorkers: 4},
			QueueNotify:  {MaxWorkers: 10},
		},
		Workers:              workers,
		FetchCooldown:        200 * time.Millisecond,
		FetchPollInterval:    time.Second,
		JobTimeout:           2 * time.Minute,
		RescueStuckJobsAfter: 10 * time.Minute,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(
				river.PeriodicInterval(5*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return ReconcilePaymentsArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true},
			),
		},
	})
	if err != nil {
		return nil, err
	}
	return &Client{River: c}, nil
}

// EnqueueTx inserts a job inside tx.
func (c *Client) EnqueueTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) error {
	_, err := c.River.InsertTx(ctx, tx, args, opts)
	return err
}

// Start runs the workers until ctx is cancelled.
func (c *Client) Start(ctx context.Context) error { return c.River.Start(ctx) }

// Stop drains the workers.
func (c *Client) Stop(ctx context.Context) error { return c.River.Stop(ctx) }

// Health checks that River's tables exist and are reachable.
func (c *Client) Health(ctx context.Context, pool *pgxpool.Pool) error {
	var n int
	return pool.QueryRow(ctx, "SELECT count(*) FROM river_job WHERE state = 'available'").Scan(&n)
}
