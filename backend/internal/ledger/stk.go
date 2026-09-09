package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	plog "github.com/ciftpay/ciftpay/internal/platform/log"
)

// ErrUnknownSTKRequest is returned when the CheckoutRequestID was not issued
// by CiftPay (or has been purged).
var ErrUnknownSTKRequest = errors.New("ledger: unknown stk request")

// STKInput is a flattened Daraja STK push callback.
type STKInput struct {
	CheckoutRequestID string
	MerchantRequestID string
	ResultCode        int
	ResultDesc        string
	Success           bool
	AmountCents       int64
	ReceiptNo         string // MpesaReceiptNumber, doubles as TransID
	MSISDN            string
	PaidAt            time.Time
	Raw               json.RawMessage
}

// IngestSTK closes the loop on a request-to-pay: it records the result on the
// stk_requests row and, on success, ingests the payment exactly like a C2B
// confirmation would (same idempotency key: the M-Pesa receipt number).
func (s *Service) IngestSTK(ctx context.Context, in STKInput) error {
	var req gen.StkRequest
	var sc gen.ResolveShortcodeRow
	err := s.DB.WithIngest(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		req, err = tx.GetSTKRequestByCheckoutID(ctx, in.CheckoutRequestID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownSTKRequest
		}
		if err != nil {
			return err
		}
		scRow, err := tx.GetShortcode(ctx, req.ShortcodeID)
		if err != nil {
			return err
		}
		// GetShortcode runs under ingest scope too; re-shape to ResolveShortcodeRow.
		sc = gen.ResolveShortcodeRow{ID: scRow.ID, OrgID: scRow.OrgID, Kind: scRow.Kind, Shortcode: scRow.Shortcode,
			DefaultItemID: scRow.DefaultItemID, AutoInvoice: scRow.AutoInvoice, Status: scRow.Status}
		return nil
	})
	if err != nil {
		return err
	}

	status := "failed"
	if in.Success {
		status = "success"
	}
	code := int32(in.ResultCode)
	err = s.DB.WithOrg(ctx, req.OrgID, func(ctx context.Context, tx db.Tx) error {
		return tx.UpdateSTKRequestResult(ctx, gen.UpdateSTKRequestResultParams{
			ID: req.ID, Status: status, ResultCode: &code, ResultDesc: db.Ptr(in.ResultDesc),
		})
	})
	if err != nil {
		return err
	}
	if !in.Success {
		s.Log.Info("stk request failed", "org", req.OrgID, "checkout", in.CheckoutRequestID, "code", in.ResultCode, "desc", in.ResultDesc)
		return nil
	}
	if in.ReceiptNo == "" {
		s.Log.Warn("successful stk callback without receipt number", "checkout", in.CheckoutRequestID)
		return nil
	}

	// Rows with purpose "verify" predate the Administrative Gate (ADR-0008);
	// no payment ever verifies a shortcode any more, so they are recorded and
	// otherwise ignored.
	if req.Purpose == "verify" {
		s.Log.Warn("ignoring legacy verify stk callback", "org", req.OrgID, "checkout", in.CheckoutRequestID)
		return nil
	}

	// A paid request-to-pay is a payment. Use the C2B path so a later C2B
	// confirmation for the same receipt (Daraja sends both) is a duplicate.
	paidAt := in.PaidAt
	if paidAt.IsZero() {
		paidAt = s.Now()
	}
	billRef := ""
	if req.SaleID != nil {
		if sale, err := s.saleRef(ctx, req.OrgID, *req.SaleID); err == nil {
			billRef = sale
		}
	}
	res, err := s.IngestC2B(ctx, C2BInput{
		Kind: "c2b_confirmation", TransID: in.ReceiptNo, ShortCode: sc.Shortcode, AmountCents: in.AmountCents,
		MSISDN: in.MSISDN, BillRef: billRef, PaidAt: paidAt, Raw: in.Raw,
	})
	if err != nil {
		return err
	}
	s.Log.Info("stk payment ingested", "org", req.OrgID, "checkout", in.CheckoutRequestID, "trans_id", in.ReceiptNo,
		"status", res.Status, "duplicate", res.Duplicate, plog.Redact("msisdn", in.MSISDN))
	return nil
}

func (s *Service) saleRef(ctx context.Context, orgID, saleID uuid.UUID) (string, error) {
	var ref string
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		sale, err := tx.GetSale(ctx, saleID)
		if err != nil {
			return err
		}
		ref = sale.Ref
		return nil
	})
	return ref, err
}
