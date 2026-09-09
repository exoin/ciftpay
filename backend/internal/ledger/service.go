package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ciftpay/ciftpay/internal/fiscal"
	"github.com/ciftpay/ciftpay/internal/platform/crypto"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	"github.com/ciftpay/ciftpay/internal/platform/jobs"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
)

// ErrDuplicate is returned when a TransID was already ingested.
var ErrDuplicate = errors.New("ledger: duplicate transaction")

// ErrUnknownShortcode is returned when no org holds the BusinessShortCode as
// a verified shortcode (ADR-0008): pending or rejected rows do not count, so
// money never lands in a ledger Safaricom has not confirmed.
var ErrUnknownShortcode = errors.New("ledger: no verified shortcode")

// Service applies matcher decisions to the database.
type Service struct {
	DB   *db.DB
	Jobs *jobs.Client
	Keys *crypto.Keyring
	Log  *slog.Logger
	Now  func() time.Time
}

// New builds a Service.
func New(d *db.DB, j *jobs.Client, k *crypto.Keyring, l *slog.Logger) *Service {
	return &Service{DB: d, Jobs: j, Keys: k, Log: l, Now: time.Now}
}

// C2BInput is a normalised Daraja confirmation.
type C2BInput struct {
	Kind        string // c2b_confirmation | reversal
	TransID     string
	ShortCode   string
	AmountCents int64
	MSISDN      string
	PayerName   string
	BillRef     string
	PaidAt      time.Time
	OriginalID  string // ThirdPartyTransID for reversals
	Raw         json.RawMessage
}

// C2BResult reports what the ingest created.
type C2BResult struct {
	OrgID     uuid.UUID
	PaymentID uuid.UUID
	SaleID    *uuid.UUID
	InvoiceID *uuid.UUID
	Status    string
	Rule      string
	Duplicate bool
}

// IngestC2B is the core loop step 2: idempotent store, resolve org, match,
// create sale + invoice, enqueue SubmitInvoice. All in one transaction per org.
func (s *Service) IngestC2B(ctx context.Context, in C2BInput) (C2BResult, error) {
	var res C2BResult

	// 1. Cross-tenant: store the raw event and resolve the shortcode.
	var eventID *uuid.UUID
	var sc gen.ResolveShortcodeRow
	var unknown bool
	err := s.DB.WithIngest(ctx, func(ctx context.Context, tx db.Tx) error {
		id, err := tx.InsertWebhookEvent(ctx, gen.InsertWebhookEventParams{
			Provider: "mpesa", Kind: in.Kind, ExternalID: "mpesa:" + in.TransID, Payload: in.Raw,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return ErrDuplicate
		case err != nil:
			return err
		}
		eventID = &id
		sc, err = s.ResolveForC2B(ctx, tx, in)
		if errors.Is(err, pgx.ErrNoRows) {
			// Commit the event with its reason (returning an error here would
			// roll the insert back and lose the evidence); the caller still
			// answers 200 so Daraja does not retry.
			unknown = true
			return tx.MarkWebhookProcessed(ctx, gen.MarkWebhookProcessedParams{ID: id, Error: db.Ptr("no verified shortcode " + in.ShortCode)})
		}
		return err
	})
	if errors.Is(err, ErrDuplicate) {
		res.Duplicate = true
		return res, nil
	}
	if err != nil {
		return res, err
	}
	if unknown {
		return res, ErrUnknownShortcode
	}
	return s.ProcessStoredC2B(ctx, *eventID, sc, in)
}

// ResolveForC2B finds the org a C2B confirmation belongs to. Must run under
// db.WithIngest. Only a verified shortcode resolves (the Administrative Gate,
// ADR-0008); anything else is pgx.ErrNoRows.
func (s *Service) ResolveForC2B(ctx context.Context, tx db.Tx, in C2BInput) (gen.ResolveShortcodeRow, error) {
	return tx.ResolveShortcode(ctx, in.ShortCode)
}

// ProcessStoredC2B runs the tenant-scoped half of IngestC2B for a webhook
// event that is already stored: payment, sale, invoice and job in one
// transaction, then the event is marked processed. The reconcile job uses it
// to finish events whose first attempt died mid-way.
func (s *Service) ProcessStoredC2B(ctx context.Context, eventID uuid.UUID, sc gen.ResolveShortcodeRow, in C2BInput) (C2BResult, error) {
	res := C2BResult{OrgID: sc.OrgID}
	err := s.DB.WithOrg(ctx, sc.OrgID, func(ctx context.Context, tx db.Tx) error {
		if in.Kind == "reversal" {
			return s.applyReversal(ctx, tx, sc, in, &eventID, &res)
		}
		return s.applyPayment(ctx, tx, sc, in, &eventID, &res)
	})
	if errors.Is(err, ErrDuplicate) {
		// The payment row already exists (previous attempt committed the
		// tenant transaction but not the processed mark).
		res.Duplicate, err = true, nil
	}
	_ = s.DB.WithIngest(ctx, func(ctx context.Context, tx db.Tx) error {
		if err != nil {
			// Keep processed_at NULL so ReconcilePayments retries it; record why.
			_, e := tx.Tx.Exec(ctx, "UPDATE webhook_events SET error = $2 WHERE id = $1", eventID, err.Error())
			return e
		}
		return tx.MarkWebhookProcessed(ctx, gen.MarkWebhookProcessedParams{ID: eventID})
	})
	return res, err
}

func (s *Service) applyPayment(ctx context.Context, tx db.Tx, sc gen.ResolveShortcodeRow, in C2BInput, eventID *uuid.UUID, res *C2BResult) error {
	var msisdnHash, msisdnEnc []byte
	if norm, err := crypto.NormaliseMSISDN(in.MSISDN); err == nil {
		msisdnHash = s.Keys.Hash(norm)
		msisdnEnc, _ = s.Keys.EncryptString(norm)
	}

	decision := Match(
		Incoming{AmountCents: in.AmountCents, BillRef: in.BillRef, MSISDNHash: msisdnHash, PaidAt: in.PaidAt},
		ShortcodeRule{AutoInvoice: sc.AutoInvoice, DefaultItemID: sc.DefaultItemID},
		Lookups{
			OpenSaleByRef: func(ref string) (OpenSale, bool) {
				sale, err := tx.GetOpenSaleByRef(ctx, gen.GetOpenSaleByRefParams{OrgID: sc.OrgID, Upper: ref})
				if err != nil {
					return OpenSale{}, false
				}
				return OpenSale{ID: sale.ID, TotalCents: sale.TotalCents}, true
			},
			PendingSTK: func(h []byte, amount int64, since time.Time) (PendingSTK, bool) {
				r, err := tx.FindPendingSTKRequest(ctx, gen.FindPendingSTKRequestParams{OrgID: sc.OrgID, MsisdnHash: h, AmountCents: amount, CreatedAt: since})
				if err != nil {
					return PendingSTK{}, false
				}
				return PendingSTK{ID: r.ID, SaleID: r.SaleID, CreatedAt: r.CreatedAt}, true
			},
		},
	)
	res.Status, res.Rule = decision.Status, decision.Rule

	var saleID *uuid.UUID
	if decision.CreateCashSale {
		sale, err := s.createCashSale(ctx, tx, sc, in, msisdnHash, msisdnEnc)
		if err != nil {
			return err
		}
		saleID = &sale.ID
	} else if decision.SaleID != nil {
		saleID = decision.SaleID
		if decision.Status == StatusMatched {
			if err := tx.MarkSalePaid(ctx, gen.MarkSalePaidParams{ID: *saleID, PaidAt: &in.PaidAt}); err != nil {
				return err
			}
		}
	}
	res.SaleID = saleID

	var rule *string
	if decision.Rule != "" {
		rule = &decision.Rule
	}
	p, err := tx.CreatePayment(ctx, gen.CreatePaymentParams{
		OrgID: sc.OrgID, ShortcodeID: sc.ID, WebhookEventID: eventID, TransID: in.TransID,
		AmountCents: in.AmountCents, MsisdnEnc: msisdnEnc, MsisdnHash: msisdnHash, PayerName: in.PayerName,
		BillRef: in.BillRef, PaidAt: in.PaidAt, Status: decision.Status, SaleID: saleID, MatchRule: rule,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDuplicate
	}
	if err != nil {
		return err
	}
	res.PaymentID = p.ID
	if decision.STKRequestID != nil {
		_ = tx.UpdateSTKRequestResult(ctx, gen.UpdateSTKRequestResultParams{ID: *decision.STKRequestID, Status: "success", ResultDesc: db.Ptr("matched to " + in.TransID)})
	}

	if saleID == nil || decision.Status == StatusPartial {
		s.Log.Info("payment needs attention", "org", sc.OrgID, "trans_id", in.TransID, "status", decision.Status, "reason", decision.Reason, plog.Redact("msisdn", in.MSISDN))
		return nil
	}

	inv, err := s.CreateInvoiceForSale(ctx, tx, sc.OrgID, *saleID, &p.ID, in.PaidAt)
	if err != nil {
		return err
	}
	res.InvoiceID = &inv.ID
	return nil
}

func (s *Service) createCashSale(ctx context.Context, tx db.Tx, sc gen.ResolveShortcodeRow, in C2BInput, msisdnHash, msisdnEnc []byte) (gen.Sale, error) {
	item, err := tx.GetItem(ctx, *sc.DefaultItemID)
	if err != nil {
		return gen.Sale{}, fmt.Errorf("ledger: default item: %w", err)
	}
	cat, err := fiscal.ParseTaxCategory(item.TaxCategory)
	if err != nil {
		return gen.Sale{}, err
	}
	tax := fiscal.LineTax(in.AmountCents, cat.RateBP())

	var customerID *uuid.UUID
	if msisdnHash != nil {
		if c, err := tx.FindCustomerByMSISDNHash(ctx, gen.FindCustomerByMSISDNHashParams{OrgID: sc.OrgID, MsisdnHash: msisdnHash}); err == nil {
			customerID = &c.ID
		} else if c, err := tx.UpsertCustomerByMSISDN(ctx, gen.UpsertCustomerByMSISDNParams{OrgID: sc.OrgID, Name: in.PayerName, MsisdnEnc: msisdnEnc, MsisdnHash: msisdnHash}); err == nil {
			customerID = &c.ID
		}
	}

	ref, err := tx.NextSaleRef(ctx, sc.OrgID)
	if err != nil {
		return gen.Sale{}, err
	}
	sale, err := tx.CreateSale(ctx, gen.CreateSaleParams{
		OrgID: sc.OrgID, Ref: ref, Kind: "cash", Status: "paid", CustomerID: customerID,
		SubtotalCents: in.AmountCents - tax, TaxCents: tax, TotalCents: in.AmountCents, PaidAt: &in.PaidAt,
	})
	if err != nil {
		return gen.Sale{}, err
	}
	_, err = tx.CreateSaleItem(ctx, gen.CreateSaleItemParams{
		OrgID: sc.OrgID, SaleID: sale.ID, ItemID: &item.ID, Description: item.Name, EtimsClassCode: item.EtimsClassCode,
		Unit: item.Unit, Qty: "1", UnitPriceCents: in.AmountCents, TaxCategory: string(cat), TaxRateBp: int32(cat.RateBP()),
		LineTotalCents: in.AmountCents, LineTaxCents: tax, Position: 0,
	})
	return sale, err
}

// CreateInvoiceForSale creates a QUEUED invoice for a paid sale and enqueues
// the fiscal job in the same transaction. Exported for the manual convert path.
func (s *Service) CreateInvoiceForSale(ctx context.Context, tx db.Tx, orgID, saleID uuid.UUID, paymentID *uuid.UUID, issuedAt time.Time) (gen.Invoice, error) {
	sale, err := tx.GetSale(ctx, saleID)
	if err != nil {
		return gen.Invoice{}, err
	}
	var buyerName string
	var buyerPinEnc, buyerPinHash []byte
	if sale.CustomerID != nil {
		if c, err := tx.Queries.FindCustomerByID(ctx, *sale.CustomerID); err == nil {
			buyerName, buyerPinEnc, buyerPinHash = c.Name, c.KraPinEnc, c.KraPinHash
		}
	}
	var inv gen.Invoice
	for attempt := 0; attempt < 5; attempt++ {
		code, err := fiscal.NewReceiptCode()
		if err != nil {
			return gen.Invoice{}, err
		}
		inv, err = tx.CreateInvoice(ctx, gen.CreateInvoiceParams{
			OrgID: orgID, SaleID: saleID, PaymentID: paymentID, Kind: "INVOICE", State: string(fiscal.StateQueued),
			BuyerPinEnc: buyerPinEnc, BuyerPinHash: buyerPinHash, BuyerName: buyerName, ReceiptCode: code,
			SubtotalCents: sale.SubtotalCents, TaxCents: sale.TaxCents, TotalCents: sale.TotalCents, IssuedAt: issuedAt,
		})
		if err == nil {
			break
		}
		if !isUniqueViolation(err) || attempt == 4 {
			return gen.Invoice{}, err
		}
	}
	if err := s.Jobs.EnqueueTx(ctx, tx.Tx, jobs.SubmitInvoiceArgs{OrgID: orgID, InvoiceID: inv.ID}, nil); err != nil {
		return gen.Invoice{}, err
	}
	// "Your receipt is being prepared" only if KRA has not acked by then; the
	// notify worker skips it for an ACKED invoice (plan.md §4.4).
	if err := s.Jobs.EnqueueAfterTx(ctx, tx.Tx, jobs.SendReceiptArgs{OrgID: orgID, InvoiceID: inv.ID, Template: "receipt_pending"}, jobs.PendingReceiptDelay); err != nil {
		return gen.Invoice{}, err
	}
	return inv, tx.AppendAudit(ctx, gen.AppendAuditParams{
		OrgID: orgID, ActorType: "system", Action: "invoice.created", Entity: "invoice", EntityID: inv.ID.String(),
	})
}

func (s *Service) applyReversal(ctx context.Context, tx db.Tx, sc gen.ResolveShortcodeRow, in C2BInput, eventID *uuid.UUID, res *C2BResult) error {
	orig, err := tx.GetPaymentByTransID(ctx, in.OriginalID)
	if errors.Is(err, pgx.ErrNoRows) {
		s.Log.Warn("reversal for unknown payment", "trans_id", in.TransID, "original", in.OriginalID)
		return nil
	}
	if err != nil {
		return err
	}
	if err := tx.MarkPaymentReversed(ctx, orig.ID); err != nil {
		return err
	}
	res.PaymentID, res.Status, res.SaleID = orig.ID, StatusReversed, orig.SaleID
	if orig.SaleID == nil {
		return nil
	}
	// Credit note for the ACKED invoice of that sale, if any.
	inv, err := tx.GetAckedInvoiceForSale(ctx, *orig.SaleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	code, err := fiscal.NewReceiptCode()
	if err != nil {
		return err
	}
	cn, err := tx.CreateInvoice(ctx, gen.CreateInvoiceParams{
		OrgID: sc.OrgID, SaleID: inv.SaleID, PaymentID: &orig.ID, Kind: "CREDIT_NOTE", ParentInvoiceID: &inv.ID,
		State: string(fiscal.StateQueued), BuyerName: inv.BuyerName, BuyerPinEnc: inv.BuyerPinEnc, BuyerPinHash: inv.BuyerPinHash,
		ReceiptCode: code, SubtotalCents: -inv.SubtotalCents, TaxCents: -inv.TaxCents, TotalCents: -inv.TotalCents, IssuedAt: in.PaidAt,
	})
	if err != nil {
		return err
	}
	res.InvoiceID = &cn.ID
	if err := s.Jobs.EnqueueTx(ctx, tx.Tx, jobs.SubmitInvoiceArgs{OrgID: sc.OrgID, InvoiceID: cn.ID}, nil); err != nil {
		return err
	}
	return tx.AppendAudit(ctx, gen.AppendAuditParams{OrgID: sc.OrgID, ActorType: "system", Action: "credit_note.created", Entity: "invoice", EntityID: cn.ID.String()})
}

func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
