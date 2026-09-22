-- name: CreateInvoice :one
INSERT INTO invoices (org_id, sale_id, payment_id, kind, parent_invoice_id, state, buyer_pin_enc, buyer_pin_hash, buyer_name, receipt_code, subtotal_cents, tax_cents, total_cents, issued_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: GetInvoice :one
SELECT * FROM invoices WHERE id = $1;

-- name: GetInvoiceByReceiptCodeExact :one
SELECT * FROM invoices WHERE receipt_code = $1;

-- name: GetInvoiceByReceiptCode :one
-- Runs under app.receipt_code (db.WithReceipt) for the public /r/{code} page.
-- Follows the superseded chain so the customer always sees the latest valid KRA invoice.
WITH RECURSIVE chain AS (
  SELECT i.*, 1 AS depth
  FROM invoices i
  WHERE i.receipt_code = $1
  UNION ALL
  SELECT next_i.*, c.depth + 1
  FROM invoices next_i
  JOIN chain c ON next_i.id = c.superseded_by_id
  WHERE c.superseded_by_id IS NOT NULL AND c.depth < 10
)
SELECT c.id, c.org_id, c.sale_id, c.payment_id, c.kind, c.parent_invoice_id, c.state, c.attempt,
       c.next_attempt_at, c.buyer_pin_enc, c.buyer_pin_hash, c.buyer_name, c.kra_invoice_no,
       c.kra_signature, c.kra_qr_payload, c.receipt_code, c.subtotal_cents, c.tax_cents,
       c.total_cents, c.issued_at, c.submitted_at, c.acked_at, c.last_error, c.created_at,
       c.updated_at, c.superseded_by_id, c.superseded_at,
       o.name AS org_name, o.kra_pin_enc AS org_pin_enc, s.ref AS sale_ref
FROM chain c
JOIN orgs o ON o.id = c.org_id
JOIN sales s ON s.id = c.sale_id
ORDER BY c.depth DESC
LIMIT 1;

-- name: SupersedeInvoice :exec
UPDATE invoices
SET superseded_by_id = $2, superseded_at = now()
WHERE id = $1;

-- name: ListInvoices :many
SELECT invoices.*
FROM invoices
WHERE invoices.org_id = $1
  AND (sqlc.narg('state')::text IS NULL OR invoices.state = sqlc.narg('state'))
  AND (sqlc.narg('kind')::text IS NULL OR invoices.kind = sqlc.narg('kind'))
  AND (
    sqlc.narg('query')::text IS NULL OR sqlc.narg('query') = '' OR (
      invoices.receipt_code ILIKE '%' || sqlc.narg('query') || '%'
      OR invoices.kra_invoice_no ILIKE '%' || sqlc.narg('query') || '%'
      OR invoices.buyer_name ILIKE '%' || sqlc.narg('query') || '%'
      OR (sqlc.narg('phone_hash')::bytea IS NOT NULL AND (
        invoices.buyer_pin_hash = sqlc.narg('phone_hash')
        OR EXISTS (
          SELECT 1 FROM payments p WHERE p.id = invoices.payment_id AND p.msisdn_hash = sqlc.narg('phone_hash')
        )
      ))
      OR EXISTS (
        SELECT 1 FROM sale_items si
        WHERE si.sale_id = invoices.sale_id
          AND si.description ILIKE '%' || sqlc.narg('query') || '%'
      )
      OR EXISTS (
        SELECT 1 FROM payments p
        WHERE p.id = invoices.payment_id
          AND (p.trans_id ILIKE '%' || sqlc.narg('query') || '%' OR p.payer_name ILIKE '%' || sqlc.narg('query') || '%')
      )
      OR EXISTS (
        SELECT 1 FROM sales s
        JOIN customers c ON s.customer_id = c.id
        WHERE s.id = invoices.sale_id
          AND c.name ILIKE '%' || sqlc.narg('query') || '%'
      )
    )
  )
ORDER BY invoices.created_at DESC LIMIT $2 OFFSET $3;

-- name: FindCreditNoteForParent :one
SELECT * FROM invoices
WHERE parent_invoice_id = $1 AND kind = 'CREDIT_NOTE'
LIMIT 1;

-- name: CountInvoicesByState :many
SELECT state, count(*) AS n FROM invoices WHERE org_id = $1 GROUP BY state;

-- name: SetInvoiceState :one
UPDATE invoices
SET state = $2,
    attempt = COALESCE(sqlc.narg('attempt'), attempt),
    next_attempt_at = sqlc.narg('next_attempt_at'),
    last_error = sqlc.narg('last_error'),
    submitted_at = CASE WHEN $2 = 'SUBMITTED' THEN now() ELSE submitted_at END
WHERE id = $1
RETURNING *;

-- name: AckInvoice :one
UPDATE invoices
SET state = 'ACKED', kra_invoice_no = $2, kra_signature = $3, kra_qr_payload = $4,
    acked_at = $5, last_error = NULL, next_attempt_at = NULL
WHERE id = $1
RETURNING *;

-- name: ResetInvoiceForRetry :one
UPDATE invoices
SET state = 'QUEUED', attempt = 0, next_attempt_at = NULL, last_error = NULL
WHERE id = $1 AND state IN ('NEEDS_REVIEW', 'FAILED_TERMINAL')
RETURNING *;

-- name: CreateFiscalSubmission :one
INSERT INTO fiscal_submissions (org_id, invoice_id, attempt, adapter, request, response, error, classification, finished_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
RETURNING *;

-- name: ListFiscalSubmissions :many
SELECT * FROM fiscal_submissions WHERE invoice_id = $1 ORDER BY attempt;

-- name: CreateNotification :one
INSERT INTO notifications (org_id, invoice_id, channel, to_msisdn_enc, to_msisdn_hash, template, locale, body)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: MarkNotificationSent :exec
UPDATE notifications SET status = 'sent', provider_message_id = $2, cost_cents = $3, sent_at = now() WHERE id = $1;

-- name: MarkNotificationFailed :exec
UPDATE notifications SET status = 'failed' WHERE id = $1;

-- name: MarkNotificationDelivered :exec
UPDATE notifications SET status = 'delivered', delivered_at = now() WHERE provider_message_id = $1;

-- name: ListNotificationsForInvoice :many
SELECT * FROM notifications WHERE invoice_id = $1 ORDER BY created_at;

-- name: BumpUsage :exec
INSERT INTO usage_counters (org_id, period, invoices_acked, sms_sent, whatsapp_sent)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (org_id, period) DO UPDATE
SET invoices_acked = usage_counters.invoices_acked + EXCLUDED.invoices_acked,
    sms_sent = usage_counters.sms_sent + EXCLUDED.sms_sent,
    whatsapp_sent = usage_counters.whatsapp_sent + EXCLUDED.whatsapp_sent;

-- name: GetUsage :one
SELECT * FROM usage_counters WHERE org_id = $1 AND period = $2;

-- name: GetSubscription :one
SELECT s.*, p.name AS plan_name, p.invoice_cap, p.overage_cents, p.features
FROM subscriptions s JOIN plans p ON p.code = s.plan_code
WHERE s.org_id = $1;

-- name: UpsertSubscription :one
INSERT INTO subscriptions (org_id, plan_code, status, period_start, period_end)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (org_id) DO UPDATE SET plan_code = EXCLUDED.plan_code, status = EXCLUDED.status,
  period_start = EXCLUDED.period_start, period_end = EXCLUDED.period_end
RETURNING *;

-- name: ListPlans :many
SELECT * FROM plans ORDER BY price_cents_monthly;

-- name: AppendAudit :exec
INSERT INTO audit_log (org_id, actor_type, actor_id, action, entity, entity_id, before, after)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: VATPosition :one
SELECT
  COALESCE(sum(CASE WHEN kind = 'INVOICE' THEN total_cents ELSE -total_cents END), 0)::bigint AS sales_cents,
  COALESCE(sum(CASE WHEN kind = 'INVOICE' THEN tax_cents ELSE -tax_cents END), 0)::bigint AS output_vat_cents,
  count(*) FILTER (WHERE kind = 'INVOICE') AS invoices,
  count(*) FILTER (WHERE kind = 'CREDIT_NOTE') AS credit_notes
FROM invoices
WHERE org_id = $1 AND state = 'ACKED' AND acked_at >= $2 AND acked_at < $3;

-- name: GetAckedInvoiceForSale :one
SELECT * FROM invoices WHERE sale_id = $1 AND kind = 'INVOICE' AND state = 'ACKED' ORDER BY created_at DESC LIMIT 1;

-- name: ActivateTaxPendingInvoices :many
-- Progressive onboarding (ADR-0009): once a merchant configures eTIMS, every
-- invoice that was withheld while etims_status was 'unconfigured' is queued
-- for submission in one batch. Returns the ids so the caller can enqueue one
-- SubmitInvoice job per invoice in the same transaction.
UPDATE invoices SET state = 'QUEUED'
WHERE org_id = $1 AND state = 'TAX_PENDING'
RETURNING id;

-- name: CountTaxPendingInvoices :one
SELECT count(*) FROM invoices WHERE org_id = $1 AND state = 'TAX_PENDING';

-- name: GetInvoiceByPayment :one
SELECT * FROM invoices
WHERE payment_id = $1 AND kind = 'INVOICE'
ORDER BY created_at DESC LIMIT 1;

-- name: GetInvoiceBySale :one
SELECT * FROM invoices
WHERE sale_id = $1 AND kind = 'INVOICE'
ORDER BY created_at DESC LIMIT 1;

-- name: ListItaxReportRows :many
SELECT
  i.id AS invoice_id,
  i.kind,
  COALESCE(i.kra_invoice_no, i.receipt_code) AS invoice_no,
  COALESCE(i.acked_at, i.created_at) AS transaction_date,
  i.buyer_pin_enc,
  i.buyer_name,
  si.description,
  si.etims_class_code,
  si.tax_category,
  si.tax_rate_bp,
  si.line_total_cents,
  si.line_tax_cents,
  (si.line_total_cents - si.line_tax_cents)::bigint AS taxable_cents
FROM invoices i
JOIN sales s ON s.id = i.sale_id
JOIN sale_items si ON si.sale_id = s.id
WHERE i.org_id = $1
  AND i.state IN ('ACKED', 'SUBMITTED')
  AND i.superseded_by_id IS NULL
  AND COALESCE(i.acked_at, i.created_at) >= $2
  AND COALESCE(i.acked_at, i.created_at) < $3
ORDER BY COALESCE(i.acked_at, i.created_at) ASC, i.id, si.position;

-- name: AnalyticsVATLiability :one
SELECT
  COALESCE(sum(CASE WHEN kind = 'INVOICE' THEN tax_cents ELSE -tax_cents END), 0)::bigint AS vat_liability_cents,
  count(*) FILTER (WHERE kind = 'INVOICE') AS invoices_count,
  count(*) FILTER (WHERE kind = 'CREDIT_NOTE') AS credit_notes_count
FROM invoices
WHERE org_id = $1
  AND state IN ('ACKED', 'SUBMITTED', 'QUEUED')
  AND superseded_by_id IS NULL
  AND COALESCE(acked_at, created_at) >= $2
  AND COALESCE(acked_at, created_at) < $3;
