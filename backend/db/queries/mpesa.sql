-- name: InsertWebhookEvent :one
-- Idempotent intake. Returns zero rows when external_id was already seen.
INSERT INTO webhook_events (provider, kind, external_id, payload)
VALUES ($1, $2, $3, $4)
ON CONFLICT (external_id) DO NOTHING
RETURNING id;

-- name: MarkWebhookProcessed :exec
UPDATE webhook_events SET processed_at = now(), error = $2 WHERE id = $1;

-- name: ListUnprocessedWebhookEvents :many
SELECT * FROM webhook_events
WHERE processed_at IS NULL AND received_at < now() - interval '2 minutes'
ORDER BY received_at LIMIT $1;

-- name: ResolveShortcode :one
-- Runs under app.scope = 'ingest' (db.WithIngest): the only cross-tenant read
-- of the payment path. The Administrative Gate (ADR-0008): only a shortcode an
-- operator marked verified resolves, so money never reaches a ledger Safaricom
-- has not confirmed. The partial unique index guarantees at most one row.
SELECT s.id, s.org_id, s.kind, s.shortcode, s.default_item_id, s.auto_invoice, s.status,
       o.name AS org_name, o.vat_registered, o.locale AS org_locale
FROM mpesa_shortcodes s JOIN orgs o ON o.id = s.org_id
WHERE s.shortcode = $1 AND s.status = 'verified';

-- name: CreateShortcode :one
INSERT INTO mpesa_shortcodes (org_id, kind, shortcode, label, default_item_id, auto_invoice)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetShortcode :one
SELECT * FROM mpesa_shortcodes WHERE id = $1;

-- name: ListShortcodes :many
SELECT * FROM mpesa_shortcodes WHERE org_id = $1 ORDER BY created_at;

-- name: VerifyShortcode :one
-- The operator's decision (admin endpoint or ciftctl). Runs under the owning
-- org's scope; the partial unique index rejects a second verified owner.
UPDATE mpesa_shortcodes
SET status = 'verified', verified_at = now(), reviewed_by = $2, reviewed_at = now(), rejection_reason = NULL
WHERE id = $1
RETURNING *;

-- name: RejectShortcode :one
UPDATE mpesa_shortcodes
SET status = 'rejected', verified_at = NULL, reviewed_by = $2, reviewed_at = now(), rejection_reason = $3
WHERE id = $1
RETURNING *;

-- name: SetShortcodeAuthorizationLetter :one
-- The merchant uploaded (or replaced) the signed letter. A rejected row goes
-- back to the queue; a verified row is left alone by the handler.
UPDATE mpesa_shortcodes
SET authorization_letter_path = $2, authorization_submitted_at = now(),
    status = CASE WHEN status = 'verified' THEN status ELSE 'pending_authorization' END,
    rejection_reason = CASE WHEN status = 'verified' THEN rejection_reason ELSE NULL END
WHERE id = $1
RETURNING *;

-- name: ListShortcodesByStatus :many
-- Runs under app.scope = 'admin' (db.WithAdmin): the operator queue.
SELECT sqlc.embed(s), o.name AS org_name, o.kra_pin_enc AS org_kra_pin_enc
FROM mpesa_shortcodes s JOIN orgs o ON o.id = s.org_id
WHERE s.status = $1
ORDER BY s.authorization_submitted_at NULLS LAST, s.created_at
LIMIT $2;

-- name: GetShortcodeAdmin :one
-- Runs under app.scope = 'admin' (db.WithAdmin).
SELECT sqlc.embed(s), o.name AS org_name, o.kra_pin_enc AS org_kra_pin_enc
FROM mpesa_shortcodes s JOIN orgs o ON o.id = s.org_id
WHERE s.id = $1;

-- name: FindShortcodesByNumber :many
-- Runs under app.scope = 'admin' (db.WithAdmin): ciftctl accepts the number.
SELECT * FROM mpesa_shortcodes WHERE shortcode = $1 ORDER BY created_at;

-- name: UpdateShortcode :one
UPDATE mpesa_shortcodes
SET label = COALESCE(sqlc.narg('label'), label),
    default_item_id = COALESCE(sqlc.narg('default_item_id'), default_item_id),
    auto_invoice = COALESCE(sqlc.narg('auto_invoice'), auto_invoice)
WHERE id = $1
RETURNING *;

-- name: CreateSTKRequest :one
INSERT INTO stk_requests (org_id, sale_id, shortcode_id, msisdn_hash, amount_cents, checkout_request_id, merchant_request_id, purpose, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: FindPendingSTKRequest :one
-- Rule 2 of the matcher: same payer, same amount, created within the window.
SELECT * FROM stk_requests
WHERE org_id = $1 AND msisdn_hash = $2 AND amount_cents = $3
  AND status = 'pending' AND created_at > $4
ORDER BY created_at DESC LIMIT 1;

-- name: GetSTKRequestByCheckoutID :one
SELECT * FROM stk_requests WHERE checkout_request_id = $1;

-- name: UpdateSTKRequestResult :exec
UPDATE stk_requests SET status = $2, result_code = $3, result_desc = $4 WHERE id = $1;

-- name: MarkShortcodeC2BRegistered :exec
UPDATE mpesa_shortcodes SET c2b_urls_registered_at = now() WHERE id = $1;

-- name: CountVerifiedShortcodeElsewhere :one
-- Runs under app.scope = 'ingest' or 'admin': the partial unique index is the
-- arbiter, this is only the friendly pre-check behind shortcode_claimed.
SELECT count(*) FROM mpesa_shortcodes
WHERE shortcode = $1 AND org_id <> $2 AND status = 'verified';
