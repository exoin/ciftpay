// Package ledger turns normalised M-Pesa payments into sales and invoices.
// matcher.go is pure: it decides, the service applies.
package ledger

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// STKWindow is how far back rule 2 looks for a pending request-to-pay.
const STKWindow = 10 * time.Minute

// Rule names stored in payments.match_rule.
const (
	RuleBillRef     = "bill_ref"
	RuleSTKWindow   = "stk_window"
	RuleAutoInvoice = "auto_invoice"
	RuleManual      = "manual"
)

// Payment statuses stored in payments.status.
const (
	StatusMatched   = "matched"
	StatusCashSale  = "cash_sale"
	StatusUnmatched = "unmatched"
	StatusPartial   = "partial"
	StatusReversed  = "reversed"
)

// Incoming is the normalised payment the matcher reasons about.
type Incoming struct {
	AmountCents int64
	BillRef     string
	MSISDNHash  []byte
	PaidAt      time.Time
}

// OpenSale is what the matcher needs to know about a candidate sale.
type OpenSale struct {
	ID         uuid.UUID
	TotalCents int64
}

// PendingSTK is a request-to-pay awaiting its payment.
type PendingSTK struct {
	ID        uuid.UUID
	SaleID    *uuid.UUID
	CreatedAt time.Time
}

// ShortcodeRule is the per-shortcode configuration that drives rule 3.
type ShortcodeRule struct {
	AutoInvoice   bool
	DefaultItemID *uuid.UUID
}

// Lookups are the read-only questions the matcher may ask. Nil funcs mean
// "no such data".
type Lookups struct {
	OpenSaleByRef func(ref string) (OpenSale, bool)
	PendingSTK    func(msisdnHash []byte, amountCents int64, since time.Time) (PendingSTK, bool)
}

// Decision is the outcome. Exactly one of SaleID / CreateCashSale is set when
// Status is matched or cash_sale; both are empty for unmatched.
type Decision struct {
	Status         string
	Rule           string
	SaleID         *uuid.UUID
	STKRequestID   *uuid.UUID
	CreateCashSale bool
	Reason         string
}

// Match applies the three rules in order.
//
//  1. BillRefNumber equals an open sale ref (case-insensitive) → matched if
//     the amount covers the sale, partial if it does not.
//  2. A pending STK request from the same payer for the same amount within
//     STKWindow → matched to that request's sale (or cash sale if none).
//  3. Shortcode has auto_invoice and a default item → cash sale.
//  4. Otherwise unmatched; the merchant converts it from Needs attention.
func Match(in Incoming, sc ShortcodeRule, lk Lookups) Decision {
	if ref := strings.TrimSpace(in.BillRef); ref != "" && lk.OpenSaleByRef != nil {
		if s, ok := lk.OpenSaleByRef(ref); ok {
			d := Decision{Rule: RuleBillRef, SaleID: ptr(s.ID)}
			switch {
			case in.AmountCents >= s.TotalCents:
				d.Status = StatusMatched
			default:
				d.Status = StatusPartial
				d.Reason = "amount below sale total"
			}
			return d
		}
	}
	if len(in.MSISDNHash) > 0 && lk.PendingSTK != nil {
		if r, ok := lk.PendingSTK(in.MSISDNHash, in.AmountCents, in.PaidAt.Add(-STKWindow)); ok {
			d := Decision{Rule: RuleSTKWindow, STKRequestID: ptr(r.ID)}
			if r.SaleID != nil {
				d.Status, d.SaleID = StatusMatched, r.SaleID
			} else {
				d.Status, d.CreateCashSale = StatusCashSale, true
			}
			return d
		}
	}
	if sc.AutoInvoice && sc.DefaultItemID != nil {
		return Decision{Status: StatusCashSale, Rule: RuleAutoInvoice, CreateCashSale: true}
	}
	reason := "no open sale, no STK request"
	if !sc.AutoInvoice {
		reason = "auto-invoice off for this shortcode"
	} else if sc.DefaultItemID == nil {
		reason = "shortcode has no default item"
	}
	return Decision{Status: StatusUnmatched, Reason: reason}
}

func ptr[T any](v T) *T { return &v }
