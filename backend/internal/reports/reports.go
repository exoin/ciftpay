// Package reports computes the merchant's Today summary, the VAT position,
// iTax VAT return CSV exports, and dashboard analytics summaries.
package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/exoin/ciftpay/internal/fiscal"
	"github.com/exoin/ciftpay/internal/platform/crypto"
	"github.com/exoin/ciftpay/internal/platform/db"
	"github.com/exoin/ciftpay/internal/platform/db/gen"
	"github.com/exoin/ciftpay/internal/platform/httpx"
)

// Nairobi is the reporting time zone.
var Nairobi = func() *time.Location {
	loc, err := time.LoadLocation("Africa/Nairobi")
	if err != nil {
		return time.FixedZone("EAT", 3*60*60)
	}
	return loc
}()

// DarajaMerchantFeeCents computes the standard Safaricom Lipa na M-Pesa merchant fee.
// Under KES 200 (20,000 cents): 0 fee.
// Over KES 200: 0.55% (55 bps), capped at KES 200 (20,000 cents).
func DarajaMerchantFeeCents(amountCents int64) int64 {
	if amountCents <= 20000 {
		return 0
	}
	fee := (amountCents * 55) / 10000
	if fee > 20000 {
		return 20000
	}
	return fee
}

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

// AnalyticsSummary is the response for GET /analytics/summary.
type AnalyticsSummary struct {
	Period               string `json:"period"` // "today" or "month"
	GrossSalesCents      int64  `json:"gross_sales_cents"`
	NetSalesCents        int64  `json:"net_sales_cents"`
	DarajaFeesCents      int64  `json:"daraja_fees_cents"`
	VATLiabilityCents    int64  `json:"vat_liability_cents"`
	PaymentsCount        int64  `json:"payments_count"`
	InvoicesAckedCount   int64  `json:"invoices_acked_count"`
	InvoicesPendingCount int64  `json:"invoices_pending_count"`
	AttentionCount       int64  `json:"attention_count"`
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
	DB   *db.DB
	Keys *crypto.Keyring
	Now  func() time.Time
}

// New builds a Service.
func New(d *db.DB, k *crypto.Keyring) *Service {
	return &Service{DB: d, Keys: k, Now: time.Now}
}

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

// AnalyticsSummary aggregates real-time metrics using Postgres.
func (s *Service) AnalyticsSummary(ctx context.Context, orgID uuid.UUID, period string) (AnalyticsSummary, error) {
	now := s.Now().In(Nairobi)
	var from, to time.Time

	if period == "month" {
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, Nairobi)
		to = from.AddDate(0, 1, 0)
	} else {
		period = "today"
		from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, Nairobi)
		to = from.AddDate(0, 0, 1)
	}

	out := AnalyticsSummary{Period: period}

	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		// 1. Payments & Daraja Fees
		pmts, err := tx.AnalyticsPayments(ctx, gen.AnalyticsPaymentsParams{
			OrgID:    orgID,
			PaidAt:   from,
			PaidAt_2: to,
		})
		if err != nil {
			return err
		}

		for _, p := range pmts {
			out.GrossSalesCents += p.AmountCents
			fee := DarajaMerchantFeeCents(p.AmountCents)
			out.DarajaFeesCents += fee
			out.PaymentsCount++
		}
		out.NetSalesCents = out.GrossSalesCents - out.DarajaFeesCents

		// 2. VAT Liability
		vatRow, err := tx.AnalyticsVATLiability(ctx, gen.AnalyticsVATLiabilityParams{
			OrgID:     orgID,
			AckedAt:   &from,
			AckedAt_2: &to,
		})
		if err != nil {
			return err
		}
		out.VATLiabilityCents = vatRow.VatLiabilityCents

		// 3. Invoice state breakdown & attention count
		states, err := tx.CountInvoicesByState(ctx, orgID)
		if err != nil {
			return err
		}
		for _, r := range states {
			switch fiscal.State(r.State) {
			case fiscal.StateAcked:
				out.InvoicesAckedCount += r.N
			case fiscal.StateNeedsReview, fiscal.StateFailedTerminal:
				out.AttentionCount += r.N
			default:
				out.InvoicesPendingCount += r.N
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

// ExportItaxCSV streams a CSV file formatted for KRA iTax VAT returns.
func (s *Service) ExportItaxCSV(ctx context.Context, orgID uuid.UUID, period string, w http.ResponseWriter) error {
	from, to, err := ParsePeriod(period)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"ciftpay-itax-%s.csv\"", period))

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	// Write CSV Header
	if err := csvWriter.Write([]string{
		"Date",
		"Invoice/Receipt No",
		"PIN of Purchaser",
		"Total Amount",
		"Taxable Amount",
		"VAT Amount",
		"eTIMS Class Code",
		"Tax Category",
	}); err != nil {
		return err
	}

	return s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListItaxReportRows(ctx, gen.ListItaxReportRowsParams{
			OrgID:     orgID,
			AckedAt:   &from,
			AckedAt_2: &to,
		})
		if err != nil {
			return err
		}

		for _, r := range rows {
			dateStr := r.TransactionDate.In(Nairobi).Format("2006-01-02")
			buyerPIN := ""
			if len(r.BuyerPinEnc) > 0 && s.Keys != nil {
				if dec, err := s.Keys.DecryptString(r.BuyerPinEnc); err == nil {
					buyerPIN = dec
				}
			}

			sign := 1.0
			if r.Kind == "CREDIT_NOTE" {
				sign = -1.0
			}

			totAmt := fmt.Sprintf("%.2f", sign*float64(r.LineTotalCents)/100.0)
			taxblAmt := fmt.Sprintf("%.2f", sign*float64(r.TaxableCents)/100.0)
			taxAmt := fmt.Sprintf("%.2f", sign*float64(r.LineTaxCents)/100.0)

			taxCat := formatTaxCategory(r.TaxCategory)

			classCode := r.EtimsClassCode

			if err := csvWriter.Write([]string{
				dateStr,
				r.InvoiceNo,
				buyerPIN,
				totAmt,
				taxblAmt,
				taxAmt,
				classCode,
				taxCat,
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func formatTaxCategory(cat string) string {
	switch cat {
	case "B":
		return "16%"
	case "E":
		return "8%"
	case "C":
		return "0%"
	case "A":
		return "Exempt"
	case "D":
		return "Non-VAT"
	default:
		return cat
	}
}

// VATPosition sums acked documents in the period.
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

// Handler serves /reports/* and /analytics/*.
type Handler struct{ S *Service }

// Mount registers routes (behind auth + RequireOrg).
func (h *Handler) Mount(r chi.Router) {
	r.Get("/reports/today", h.today)
	r.Get("/reports/vat", h.vat)
	r.Get("/reports/itax/export", h.exportItax)
	r.Get("/analytics/summary", h.analyticsSummary)
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

func (h *Handler) analyticsSummary(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	period := r.URL.Query().Get("period")
	if period != "month" {
		period = "today"
	}
	out, err := h.S.AnalyticsSummary(r.Context(), p.OrgID, period)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not compute analytics summary")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) exportItax(w http.ResponseWriter, r *http.Request) {
	p, _ := httpx.PrincipalFrom(r.Context())
	month := r.URL.Query().Get("month")
	if month == "" {
		month = h.S.Now().In(Nairobi).Format("2006-01")
	}
	if err := h.S.ExportItaxCSV(r.Context(), p.OrgID, month, w); err != nil {
		if _, _, perr := ParsePeriod(month); perr != nil {
			httpx.Fail(w, http.StatusBadRequest, "bad_request", perr.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "internal", "Could not generate iTax CSV export")
		return
	}
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
