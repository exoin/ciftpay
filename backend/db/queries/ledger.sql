-- name: CreateItem :one
INSERT INTO items (org_id, name, etims_class_code, tax_category, unit, price_cents)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetItem :one
SELECT * FROM items WHERE id = $1;

-- name: ListItems :many
SELECT * FROM items WHERE org_id = $1 AND is_active ORDER BY name;

-- name: UpdateItem :one
UPDATE items
SET name = COALESCE(sqlc.narg('name'), name),
    etims_class_code = COALESCE(sqlc.narg('etims_class_code'), etims_class_code),
    tax_category = COALESCE(sqlc.narg('tax_category'), tax_category),
    unit = COALESCE(sqlc.narg('unit'), unit),
    price_cents = COALESCE(sqlc.narg('price_cents'), price_cents),
    is_active = COALESCE(sqlc.narg('is_active'), is_active)
WHERE id = $1
RETURNING *;

-- name: UpsertCustomerByMSISDN :one
INSERT INTO customers (org_id, name, msisdn_enc, msisdn_hash)
VALUES ($1, $2, $3, $4)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: FindCustomerByMSISDNHash :one
SELECT * FROM customers WHERE org_id = $1 AND msisdn_hash = $2 ORDER BY created_at LIMIT 1;

-- name: CreateSale :one
INSERT INTO sales (org_id, ref, kind, status, customer_id, subtotal_cents, tax_cents, total_cents, client_ref, created_by, paid_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: GetSale :one
SELECT * FROM sales WHERE id = $1;

-- name: GetOpenSaleByRef :one
SELECT * FROM sales WHERE org_id = $1 AND status = 'open' AND upper(ref) = upper($2);

-- name: ListSales :many
SELECT * FROM sales
WHERE org_id = $1 AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY created_at DESC LIMIT $2 OFFSET $3;

-- name: MarkSalePaid :exec
UPDATE sales SET status = 'paid', paid_at = $2 WHERE id = $1 AND status = 'open';

-- name: NextSaleRef :one
SELECT ('S-' || to_char(now() AT TIME ZONE 'Africa/Nairobi', 'YYMMDD') || '-' ||
       lpad((count(*) + 1)::text, 4, '0'))::text AS ref
FROM sales
WHERE org_id = $1 AND created_at >= date_trunc('day', now() AT TIME ZONE 'Africa/Nairobi') AT TIME ZONE 'Africa/Nairobi';

-- name: CreateSaleItem :one
INSERT INTO sale_items (org_id, sale_id, item_id, description, etims_class_code, unit, qty, unit_price_cents, tax_category, tax_rate_bp, line_total_cents, line_tax_cents, position)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
RETURNING *;

-- name: ListSaleItems :many
SELECT * FROM sale_items WHERE sale_id = $1 ORDER BY position;

-- name: CreatePayment :one
INSERT INTO payments (org_id, shortcode_id, webhook_event_id, trans_id, amount_cents, msisdn_enc, msisdn_hash, payer_name, bill_ref, paid_at, status, sale_id, match_rule)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
ON CONFLICT (trans_id) DO NOTHING
RETURNING *;

-- name: GetPayment :one
SELECT * FROM payments WHERE id = $1;

-- name: GetPaymentByTransID :one
SELECT * FROM payments WHERE trans_id = $1;

-- name: ListPayments :many
SELECT * FROM payments
WHERE org_id = $1 AND (sqlc.narg('status')::text IS NULL OR status = sqlc.narg('status'))
ORDER BY paid_at DESC LIMIT $2 OFFSET $3;

-- name: AttachPaymentToSale :exec
UPDATE payments SET sale_id = $2, status = $3, match_rule = $4 WHERE id = $1;

-- name: MarkPaymentReversed :exec
UPDATE payments SET status = 'reversed', reversed_at = now() WHERE id = $1;

-- name: TodayTotals :one
SELECT count(*) AS payments, COALESCE(sum(amount_cents), 0)::bigint AS amount_cents
FROM payments
WHERE org_id = $1 AND status <> 'reversed'
  AND paid_at >= date_trunc('day', now() AT TIME ZONE 'Africa/Nairobi') AT TIME ZONE 'Africa/Nairobi';

-- name: CountPaymentsByStatus :many
SELECT status, count(*) AS n FROM payments WHERE org_id = $1 GROUP BY status;

-- name: FindCustomerByID :one
SELECT * FROM customers WHERE id = $1;
