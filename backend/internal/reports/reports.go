// Package reports computes the merchant's Today summary and the VAT position.
// Return packs (CSV/XLSX/PDF) and P&L arrive in Phase 2.
package reports

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/ciftpay/ciftpay/internal/fiscal"
	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
	"github.com/ciftpay/ciftpay/internal/platform/httpx"
)

// Nairobi is the reporting time zone.
var Nairobi = func() *time.Location {
	loc, err := time.LoadLocation("Africa/Nairobi")
	if err != nil {
		return time.FixedZone("EAT", 3*60*60)
	}
	return loc
}()

// Today is the TodaySummary schema. RecentPayments is filled by the ledger
// handler, which owns the Payment view type.
type Today struct {
	Date            string `json:"date"`
	ReceivedCents   int64  `json:"received_cents"`
	PaymentsCount   int64  `json:"payments_count"`
	InvoicesAcked   int64  `json:"invoices_acked"`
	InvoicesPending int64  `json:"invoices_pending"`
	AttentionCount  int64  `json:"attention_count"`
	RecentPayments  []any  `json:"recent_payments"`
}

// VATCategoryRow is one row of the VAT report.
type VATCategoryRow struct {
	Category     string `json:"category"`
	TaxableCents int64  `json:"taxable_cents"`
	TaxCents     int64  `json:"tax_cents"`
	Count        int64  `json:"count"`
}

// VAT is the VatReport schema.
type VAT struct {
	Period           string           `json:"period"`
	ByCategory       []VATCategoryRow `json:"by_category"`
	InvoicesCount    int64            `json:"invoices_count"`
	CreditNotesCount int64            `json:"credit_notes_count"`
	OutputVATCents   int64            `json:"output_vat_cents"`
}

// Service runs the queries.
type Service struct {
	DB  *db.DB
	Now func() time.Time
}

// New builds a Service.
func New(d *db.DB) *Service { return &Service{DB: d, Now: time.Now} }

// Today aggregates the day so far (Nairobi time).
func (s *Service) Today(ctx context.Context, orgID uuid.UUID) (Today, error) {
	out := Today{Date: s.Now().In(Nairobi).Format("2006-01-02"), RecentPayments: []any{}}
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		t, err := tx.TodayTotals(ctx, orgID)
		if err != nil {
			return err
		}
		out.ReceivedCents, out.PaymentsCount = t.AmountCents, t.Payments
		states, err := tx.CountInvoicesByState(ctx, orgID)
		if err != nil {
			return err
		}
		for _, r := range states {
			switch fiscal.State(r.State) {
			case fiscal.StateAcked:
				out.InvoicesAcked += r.N
			case fiscal.StateNeedsReview, fiscal.StateFailedTerminal:
				out.AttentionCount += r.N
			default:
				out.InvoicesPending += r.N
			}
		}
		ps, err := tx.CountPaymentsByStatus(ctx, orgID)
		if err != nil {
			return err
		}
		for _, r := range ps {
			if r.Status == "unmatched" || r.Status == "partial" {
				out.AttentionCount += r.N
			}
		}
		return nil
	})
	return out, err
}

// ParsePeriod accepts YYYY-MM and returns [from, to) in Nairobi time.
func ParsePeriod(p string) (from, to time.Time, err error) {
	t, err := time.ParseInLocation("2006-01", p, Nairobi)
	if err != nil {
		return from, to, fmt.Errorf("reports: period must be YYYY-MM")
	}
	return t, t.AddDate(0, 1, 0), nil
}

// VATPosition sums acked documents in the period. Per-category rows are
// derived from sale lines in Phase 2; Phase 0 reports the standard-rate total.
func (s *Service) VATPosition(ctx context.Context, orgID uuid.UUID, period string) (VAT, error) {
	from, to, err := ParsePeriod(period)
	if err != nil {
		return VAT{}, err
	}
	out := VAT{Period: period, ByCategory: []VATCategoryRow{}}
	err = s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		row, err := tx.VATPosition(ctx, gen.VATPositionParams{OrgID: orgID, AckedAt: &from, AckedAt_2: &to})
		if err != nil {
			return err
		}
		out.InvoicesCount, out.CreditNotesCount, out.OutputVATCents = row.Invoices, row.CreditNotes, row.OutputVatCents
		if row.Invoices+row.CreditNotes > 0 {
			out.ByCategory = append(out.ByCategory, VATCategoryRow{
				Category: string(fiscal.TaxStandard), TaxableCents: row.SalesCents - row.OutputVatCents,
				TaxCents: row.OutputVatCents, Count: row.Invoices + row.CreditNotes,
			})
		}
		return nil
	})
	return out, err
}

// Handler serves /reports/*.
type Handler struct{ S *Service }

// Mount registers routes (behind auth + RequireOrg).
func (h *Handler) Mount(r chi.Router) {
	r.Get("/reports/today", h.today)
	r.Get("/reports/vat", h.vat)
}

func (h *Handler) today(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	out, err := h.S.Today(r.Context(), p.OrgID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not compute today's summary")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) vat(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	period := r.URL.Query().Get("period")
	if period == "" {
		period = h.S.Now().In(Nairobi).Format("2006-01")
	}
	out, err := h.S.VATPosition(r.Context(), p.OrgID, period)
	if err != nil {
		if _, _, perr := ParsePeriod(period); perr != nil {
			httpx.Fail(w, http.StatusBadRequest, "bad_request", perr.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not compute the VAT position")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
