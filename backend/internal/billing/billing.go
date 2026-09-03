// Package billing knows plans, the org's subscription and monthly usage.
// Payment collection for subscriptions (M-Pesa STK on the plan price) is a
// Phase 2 epic; Phase 0 only tracks entitlement and usage.
package billing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ciftpay/ciftpay/internal/platform/db"
	"github.com/ciftpay/ciftpay/internal/platform/db/gen"
)

// Plan codes seeded by 0001_init.sql.
const (
	PlanHustler    = "hustler"
	PlanDuka       = "duka"
	PlanBiashara   = "biashara"
	PlanAccountant = "accountant"
)

// Plan is the API view of a plan.
type Plan struct {
	Code              string          `json:"code"`
	Name              string          `json:"name"`
	PriceCentsMonthly int64           `json:"price_cents_monthly"`
	InvoiceCap        *int32          `json:"invoice_cap"`
	OverageCents      int64           `json:"overage_cents"`
	Features          json.RawMessage `json:"features"`
}

// Entitlement is what the org may do this period.
type Entitlement struct {
	PlanCode      string `json:"plan_code"`
	PlanName      string `json:"plan_name"`
	Status        string `json:"status"`
	Period        string `json:"period"`
	InvoicesAcked int32  `json:"invoices_acked"`
	InvoiceCap    *int32 `json:"invoice_cap"`
	OverInvoices  int32  `json:"over_invoices"`
	OverageCents  int64  `json:"overage_cents"`
	SMSSent       int32  `json:"sms_sent"`
}

// Service reads plans and usage.
type Service struct {
	DB  *db.DB
	Now func() time.Time
}

// New builds a Service.
func New(d *db.DB) *Service { return &Service{DB: d, Now: time.Now} }

// Plans lists the catalogue.
func (s *Service) Plans(ctx context.Context) ([]Plan, error) {
	var out []Plan
	err := s.DB.Unscoped(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := tx.ListPlans(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, Plan{Code: r.Code, Name: r.Name, PriceCentsMonthly: r.PriceCentsMonthly, InvoiceCap: r.InvoiceCap, OverageCents: r.OverageCents, Features: r.Features})
		}
		return nil
	})
	return out, err
}

// EnsureSubscription puts a new org on the free plan if it has none.
func (s *Service) EnsureSubscription(ctx context.Context, orgID uuid.UUID) error {
	return s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		_, err := tx.GetSubscription(ctx, orgID)
		if err == nil {
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		_, err = tx.UpsertSubscription(ctx, gen.UpsertSubscriptionParams{OrgID: orgID, PlanCode: PlanHustler, Status: "active", PeriodStart: s.Now()})
		return err
	})
}

// Entitlement returns plan + usage for the current month.
func (s *Service) Entitlement(ctx context.Context, orgID uuid.UUID) (Entitlement, error) {
	period := s.Now().Format("2006-01")
	out := Entitlement{Period: period, PlanCode: PlanHustler, PlanName: "Hustler", Status: "active"}
	err := s.DB.WithOrg(ctx, orgID, func(ctx context.Context, tx db.Tx) error {
		sub, err := tx.GetSubscription(ctx, orgID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			out.PlanCode, out.PlanName, out.Status, out.InvoiceCap = sub.PlanCode, sub.PlanName, sub.Status, sub.InvoiceCap
			if out.InvoiceCap != nil {
				out.OverageCents = sub.OverageCents
			}
		}
		u, err := tx.GetUsage(ctx, gen.GetUsageParams{OrgID: orgID, Period: period})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		out.InvoicesAcked, out.SMSSent = u.InvoicesAcked, u.SmsSent
		if out.InvoiceCap != nil && out.InvoicesAcked > *out.InvoiceCap {
			out.OverInvoices = out.InvoicesAcked - *out.InvoiceCap
			out.OverageCents *= int64(out.OverInvoices)
		} else {
			out.OverageCents = 0
		}
		return nil
	})
	return out, err
}
