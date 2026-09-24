package mpesa

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/exoin/ciftpay/internal/ledger"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/jobs"
)

// PullWorker performs daily reconciliation by querying Safaricom Daraja's Pull Transactions API.
type PullWorker struct {
	river.WorkerDefaults[jobs.PullTransactionsArgs]
	DB     *db.DB
	Client *Client
	Ledger *ledger.Service
	Log    *slog.Logger
}

// NewPullWorker builds a PullWorker.
func NewPullWorker(d *db.DB, c *Client, l *ledger.Service, log *slog.Logger) *PullWorker {
	return &PullWorker{
		DB:     d,
		Client: c,
		Ledger: l,
		Log:    log,
	}
}

// Work implements river.Worker for jobs.PullTransactionsArgs.
func (w *PullWorker) Work(ctx context.Context, _ *river.Job[jobs.PullTransactionsArgs]) error {
	if w.Client == nil || !w.Client.Configured() {
		w.Log.Info("pull_transactions: daraja client not configured, skipping pull sweep")
		return nil
	}

	start := time.Now()
	// Default query window: past 24 hours up to now
	endDate := time.Now().In(Nairobi).Format("2006-01-02 15:04:05")
	startDate := time.Now().In(Nairobi).Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

	// Query verified shortcodes across tenants
	var shortcodes []gen.MpesaShortcode
	err := w.DB.WithAdmin(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListShortcodesByStatus(ctx, gen.ListShortcodesByStatusParams{
			Status: "verified",
			Limit:  1000,
		})
		if err != nil {
			return err
		}
		for _, r := range rows {
			shortcodes = append(shortcodes, r.MpesaShortcode)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("pull_transactions: list verified shortcodes: %w", err)
	}

	totalPulled := 0
	totalIngested := 0
	totalSkipped := 0

	for _, sc := range shortcodes {
		txs, err := w.Client.PullTransactions(ctx, sc.Shortcode, startDate, endDate, "0")
		if err != nil {
			w.Log.Warn("pull_transactions: failed to pull transactions for shortcode", "shortcode", sc.Shortcode, "err", err)
			continue
		}

		for _, pt := range txs {
			totalPulled++
			if pt.TransID == "" {
				continue
			}

			// Strict Idempotency: Check if transaction already exists in payments table
			var exists bool
			err := w.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
				_, err := tx.GetPaymentByTransID(ctx, pt.TransID)
				if err == nil {
					exists = true
					return nil
				}
				if errors.Is(err, pgx.ErrNoRows) {
					exists = false
					return nil
				}
				return err
			})
			if err != nil {
				w.Log.Warn("pull_transactions: check payment trans_id error", "trans_id", pt.TransID, "err", err)
				continue
			}

			if exists {
				totalSkipped++
				continue
			}

			// Convert PullTransaction to C2BPayload and ingest via IngestC2B
			payload := C2BPayload{
				TransactionType:   pt.TransactionType,
				TransID:           pt.TransID,
				TransTime:         pt.TransTime,
				TransAmount:       pt.TransAmount,
				BusinessShortCode: pt.BusinessShortCode,
				BillRefNumber:     pt.BillRefNumber,
				InvoiceNumber:     pt.InvoiceNumber,
				OrgAccountBalance: pt.OrgAccountBalance,
				ThirdPartyTransID: pt.ThirdPartyTransID,
				MSISDN:            pt.MSISDN,
				FirstName:         pt.FirstName,
				MiddleName:        pt.MiddleName,
				LastName:          pt.LastName,
			}
			raw, _ := json.Marshal(payload)
			input := ToC2BInput(payload, raw)

			res, err := w.Ledger.IngestC2B(ctx, input)
			if err != nil {
				w.Log.Warn("pull_transactions: ingest missing payment failed", "trans_id", pt.TransID, "err", err)
				continue
			}

			totalIngested++
			w.Log.Info("pull_transactions: ingested missing transaction", "trans_id", pt.TransID, "status", res.Status)
		}
	}

	w.Log.Info("pull_transactions sweep finished",
		"shortcodes", len(shortcodes),
		"pulled", totalPulled,
		"ingested", totalIngested,
		"skipped", totalSkipped,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}
