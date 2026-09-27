package fiscal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/jobs"
	plog "github.com/exoin/ciftpay/internal/platform/log"
)

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return true
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		code := strings.ToLower(ve.Code)
		if code == "not_found" || code == "invoice_not_found" || strings.Contains(strings.ToLower(ve.Message), "not found") {
			return true
		}
	}
	var te *TransientError
	if errors.As(err, &te) {
		code := strings.ToLower(te.Code)
		if code == "not_found" || code == "invoice_not_found" || strings.Contains(strings.ToLower(te.Message), "not found") {
			return true
		}
	}
	return false
}

// Submitter drives invoices through the state machine using a Provider. It is
// the body of the fiscal.submit_invoice River job (cmd/worker).
type Submitter struct {
	DB       *db.DB
	Jobs     *jobs.Client
	Keys     *crypto.Keyring
	Provider Provider
	Log      *slog.Logger
	Now      func() time.Time
}

// NewSubmitter builds a Submitter.
func NewSubmitter(d *db.DB, j *jobs.Client, k *crypto.Keyring, p Provider, l *slog.Logger) *Submitter {
	return &Submitter{DB: d, Jobs: j, Keys: k, Provider: p, Log: l, Now: time.Now}
}

// Result summarises one submission attempt for logs and tests.
type Result struct {
	InvoiceID uuid.UUID
	Attempt   int
	State     State
	RetryIn   time.Duration
	Skipped   bool // invoice was not QUEUED (already acked, under review, ...)
	Err       error
}

// profile is the shape stored in orgs.fiscal_profile by RegisterDevice.
type profile struct {
	DeviceID string `json:"device_id"`
	BranchID string `json:"branch_id"`
}

// Submit performs one attempt for the invoice. It never returns a Provider
// error to the caller: outcomes are persisted on the invoice and reported in
// Result. Only infrastructure errors (database) are returned.
func (s *Submitter) Submit(ctx context.Context, orgID, invoiceID uuid.UUID) (Result, error) {
	res := Result{InvoiceID: invoiceID}

	// 1. Claim: QUEUED/FAILED_RETRYABLE → SUBMITTED, attempt+1, build the adapter document.
	var doc Invoice
	var kind string
	var parentKRANo string
	var isRetry bool
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		inv, err := tx.GetInvoice(ctx, invoiceID)
		if err != nil {
			return err
		}
		state := State(inv.State)
		if state != StateQueued && state != StateFailedRetryable {
			res.Skipped, res.State = true, state
			return nil
		}
		if state == StateQueued && inv.NextAttemptAt != nil && inv.NextAttemptAt.After(s.Now()) {
			res.Skipped, res.State, res.RetryIn = true, StateQueued, time.Until(*inv.NextAttemptAt)
			return nil
		}
		if state == StateFailedRetryable {
			errStr := ""
			if inv.LastError != nil {
				errStr = *inv.LastError
			}
			outcome := Resolve(int(inv.Attempt), errors.New(errStr))
			if outcome.Next == StateFailedTerminal {
				if _, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
					ID:            invoiceID,
					State:         string(StateFailedTerminal),
					ExpectedState: string(StateFailedRetryable),
					LastError:     inv.LastError,
				}); err != nil {
					if errors.Is(err, pgx.ErrNoRows) {
						res.Skipped = true
						return nil
					}
					return err
				}
				if _, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
					ID:            invoiceID,
					State:         string(StateNeedsReview),
					ExpectedState: string(StateFailedTerminal),
					LastError:     inv.LastError,
				}); err != nil {
					return err
				}
				res.State = StateNeedsReview
				return nil
			}
		}

		if (state == StateFailedRetryable || state == StateQueued) && inv.Attempt > 0 {
			isRetry = true
		}

		next, err := Transition(state, StateSubmitted)
		if err != nil {
			return err
		}
		attempt := inv.Attempt + 1
		if _, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
			ID:            inv.ID,
			State:         string(next),
			Attempt:       &attempt,
			ExpectedState: inv.State,
		}); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				res.Skipped = true
				return nil
			}
			return err
		}
		res.Attempt, res.State, kind = int(attempt), next, inv.Kind
		doc, err = s.buildDocument(ctx, tx, inv)
		if err != nil {
			return err
		}
		if inv.Kind == "CREDIT_NOTE" && inv.ParentInvoiceID != nil {
			parent, err := tx.GetInvoice(ctx, *inv.ParentInvoiceID)
			if err != nil {
				return err
			}
			if parent.KraInvoiceNo != nil {
				parentKRANo = *parent.KraInvoiceNo
			}
		}
		return nil
	})
	if err != nil || res.Skipped {
		return res, err
	}

	// 2. Call KRA (through the adapter) outside any transaction with raw PINs.
	var ack Ack
	var subErr error
	var recovered bool

	if isRetry {
		lookupAck, lookupErr := s.Provider.LookupInvoice(ctx, doc.ID)
		if lookupErr == nil && lookupAck.KRAInvoiceNo != "" {
			ack = lookupAck
			recovered = true
			if s.Log != nil {
				s.Log.Info("recovered invoice ack via lookup", "invoice_id", doc.ID, "kra_invoice_no", ack.KRAInvoiceNo)
			}
		} else if errors.Is(lookupErr, ErrLookupNotSupported) || isNotFound(lookupErr) {
			if s.Log != nil {
				s.Log.Info("lookup invoice not supported or not found, falling back to submit", "invoice_id", doc.ID, "err", lookupErr)
			}
		} else if lookupErr != nil {
			if s.Log != nil {
				s.Log.Warn("lookup invoice failed with error", "invoice_id", doc.ID, "err", lookupErr)
			}
			subErr = lookupErr
		}
	}

	if !recovered && subErr == nil {
		if kind == "CREDIT_NOTE" {
			total := doc.TotalCents
			if total > 0 {
				total = -total
			}
			ack, subErr = s.Provider.SubmitCreditNote(ctx, CreditNote{
				ID: doc.ID, OrgID: doc.OrgID, OriginalKRANo: parentKRANo, OriginalInvoice: doc,
				Reason: "Cancellation / Return", ReasonCode: "01", Lines: doc.Lines, TotalCents: total, DeviceProfile: doc.DeviceProfile,
			})
		} else {
			ack, subErr = s.Provider.SubmitInvoice(ctx, doc)
		}
	}
	outcome := Resolve(res.Attempt, subErr)
	res.Err, res.RetryIn = subErr, outcome.RetryIn

	// Mask KRA PINs before saving request audit to fiscal_submissions (ADR-0002).
	auditDoc := doc
	if auditDoc.SellerPIN != "" {
		auditDoc.SellerPIN = plog.MaskPIN(auditDoc.SellerPIN)
	}
	if auditDoc.BuyerPIN != "" {
		auditDoc.BuyerPIN = plog.MaskPIN(auditDoc.BuyerPIN)
	}
	reqJSON, _ := json.Marshal(auditDoc)

	// 3. Persist the outcome and schedule what follows.
	err = s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		var errStr *string
		if subErr != nil {
			errStr = db.Ptr(subErr.Error())
		}
		if _, err := tx.CreateFiscalSubmission(ctx, gen.CreateFiscalSubmissionParams{
			OrgID: orgID, InvoiceID: invoiceID, Attempt: int32(res.Attempt), Adapter: s.Provider.Name(),
			Request: reqJSON, Response: ack.Raw, Error: errStr, Classification: string(Classify(subErr)),
		}); err != nil {
			return err
		}

		switch outcome.Next {
		case StateAcked:
			ackedAt := ack.ReceivedAt
			if ackedAt.IsZero() {
				ackedAt = s.Now()
			}
			if _, err := tx.AckInvoice(ctx, gen.AckInvoiceParams{
				ID: invoiceID, KraInvoiceNo: db.Ptr(ack.KRAInvoiceNo), KraSignature: db.Ptr(ack.Signature),
				KraQrPayload: db.Ptr(ack.QRPayload), AckedAt: &ackedAt,
			}); err != nil {
				return err
			}
			res.State = StateAcked
			_ = tx.BumpUsage(ctx, gen.BumpUsageParams{OrgID: orgID, Period: ackedAt.Format("2006-01"), InvoicesAcked: 1})
			template := "receipt_acked"
			if kind == "CREDIT_NOTE" {
				template = "credit_note"
			}
			if err := s.Jobs.EnqueueTx(ctx, tx.Tx, jobs.SendReceiptArgs{OrgID: orgID, InvoiceID: invoiceID, Template: template}, nil); err != nil {
				return err
			}
			return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: orgID, ActorType: "system", Action: "invoice.acked", Entity: "invoice", EntityID: invoiceID.String(), After: ack.Raw})

		case StateQueued:
			// SUBMITTED → FAILED_RETRYABLE → QUEUED with next_attempt_at.
			if _, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
				ID:            invoiceID,
				State:         string(StateFailedRetryable),
				ExpectedState: string(StateSubmitted),
				LastError:     errStr,
			}); err != nil {
				return err
			}
			nextAt := s.Now().Add(outcome.RetryIn)
			if _, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
				ID:            invoiceID,
				State:         string(StateQueued),
				ExpectedState: string(StateFailedRetryable),
				NextAttemptAt: &nextAt,
				LastError:     errStr,
			}); err != nil {
				return err
			}
			res.State = StateQueued
			return nil

		default: // StateFailedTerminal → NEEDS_REVIEW
			if _, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
				ID:            invoiceID,
				State:         string(StateFailedTerminal),
				ExpectedState: string(StateSubmitted),
				LastError:     errStr,
			}); err != nil {
				return err
			}
			if _, err := tx.SetInvoiceState(ctx, gen.SetInvoiceStateParams{
				ID:            invoiceID,
				State:         string(StateNeedsReview),
				ExpectedState: string(StateFailedTerminal),
				LastError:     errStr,
			}); err != nil {
				return err
			}
			res.State = StateNeedsReview
			return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: orgID, ActorType: "system", Action: "invoice.needs_review", Entity: "invoice", EntityID: invoiceID.String(), After: []byte(fmt.Sprintf(`{"error":%q}`, subErr.Error()))})
		}
	})
	return res, err
}

// buildDocument turns database rows into the adapter-facing Invoice.
func (s *Submitter) buildDocument(ctx context.Context, tx db.Tx, inv gen.Invoice) (Invoice, error) {
	org, err := tx.GetOrg(ctx, inv.OrgID)
	if err != nil {
		return Invoice{}, fmt.Errorf("fiscal: org: %w", err)
	}
	sellerPIN, err := s.Keys.DecryptString(org.KraPinEnc)
	if err != nil {
		return Invoice{}, fmt.Errorf("fiscal: seller pin: %w", err)
	}
	var prof profile
	if len(org.FiscalProfile) > 0 {
		_ = json.Unmarshal(org.FiscalProfile, &prof)
	}
	if prof.BranchID == "" {
		prof.BranchID = "00"
	}
	buyerPIN := ""
	if len(inv.BuyerPinEnc) > 0 {
		buyerPIN, _ = s.Keys.DecryptString(inv.BuyerPinEnc)
	}
	items, err := tx.ListSaleItems(ctx, inv.SaleID)
	if err != nil {
		return Invoice{}, err
	}
	lines := make([]Line, 0, len(items))
	for _, it := range items {
		lines = append(lines, Line{
			ItemCode: it.EtimsClassCode, Description: it.Description, Qty: it.Qty, Unit: it.Unit,
			UnitPriceCents: it.UnitPriceCents, TaxCategory: TaxCategory(it.TaxCategory),
			LineTotalCents: it.LineTotalCents, LineTaxCents: it.LineTaxCents,
		})
	}
	return Invoice{
		ID: inv.ID.String(), OrgID: inv.OrgID.String(), SellerPIN: sellerPIN, SellerName: org.Name, BranchID: prof.BranchID,
		BuyerPIN: buyerPIN, BuyerName: inv.BuyerName, IssuedAt: inv.IssuedAt, Lines: lines,
		SubtotalCents: inv.SubtotalCents, TaxCents: inv.TaxCents, TotalCents: inv.TotalCents, PaymentMethod: "MOBILE_MONEY",
		DeviceProfile: org.FiscalProfile,
	}, nil
}

// Retry re-queues a NEEDS_REVIEW invoice after the merchant fixed its inputs.
func (s *Submitter) Retry(ctx context.Context, orgID, invoiceID uuid.UUID, actor string) error {
	return s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		inv, err := tx.ResetInvoiceForRetry(ctx, invoiceID)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: invoice is not awaiting review", ErrIllegalTransition)
		}
		if err != nil {
			return err
		}
		if err := s.Jobs.EnqueueTx(ctx, tx.Tx, jobs.SubmitInvoiceArgs{OrgID: orgID, InvoiceID: inv.ID}, nil); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: orgID, ActorType: "user", ActorID: &actor, Action: "invoice.retry", Entity: "invoice", EntityID: inv.ID.String()})
	})
}

// Worker adapts Submitter to River. Retries are scheduled by snoozing the
// job for the state machine's backoff, so River's own attempt counter stays
// at 1 (see jobs.SubmitInvoiceArgs.InsertOpts).
type Worker struct {
	river.WorkerDefaults[jobs.SubmitInvoiceArgs]
	S *Submitter
}

// Work implements river.Worker.
func (w *Worker) Work(ctx context.Context, job *river.Job[jobs.SubmitInvoiceArgs]) error {
	res, err := w.S.Submit(ctx, job.Args.OrgID, job.Args.InvoiceID)
	if err != nil {
		w.S.Log.Error("fiscal submit infrastructure error", "err", err, "invoice", job.Args.InvoiceID)
		return err
	}
	switch {
	case res.Skipped && res.RetryIn > 0:
		return river.JobSnooze(res.RetryIn)
	case res.Skipped:
		return nil
	case res.State == StateQueued:
		w.S.Log.Warn("fiscal submit failed, will retry", "invoice", res.InvoiceID, "attempt", res.Attempt, "retry_in", res.RetryIn, "err", res.Err)
		return river.JobSnooze(res.RetryIn)
	case res.State == StateNeedsReview:
		w.S.Log.Error("fiscal submit needs review", "invoice", res.InvoiceID, "attempt", res.Attempt, "err", res.Err)
		return river.JobSnooze(0)
	}
	w.S.Log.Info("fiscal submit acked", "invoice", res.InvoiceID, "attempt", res.Attempt, "adapter", w.S.Provider.Name())
	return nil
}

// Timeout implements river.Worker: a KRA call must never hold a worker slot
// longer than this.
func (w *Worker) Timeout(*river.Job[jobs.SubmitInvoiceArgs]) time.Duration { return 90 * time.Second }
