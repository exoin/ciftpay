-- +goose Up
-- +goose StatementBegin

-- The Administrative Gate (ADR-0008) replaces the own-Till KES 1 control check
-- of migration 0002. Ownership of a shortcode is proven by the merchant's
-- signed Safaricom authorization letter, which Safaricom checks against the
-- Till's KYC before mapping it to CiftPay's Daraja app; a CiftPay operator
-- then flips the row to 'verified'. Only verified rows are visible to the C2B
-- ingest path, so no payment reaches a tax ledger before that.
DROP TABLE IF EXISTS shortcode_verifications CASCADE;

ALTER TABLE mpesa_shortcodes
  DROP COLUMN verification_checkout_id,
  ADD COLUMN status text NOT NULL DEFAULT 'pending_authorization'
    CHECK (status IN ('pending_authorization','verified','rejected')),
  -- storage key of the uploaded letter (relative to UPLOAD_DIR), never a URL
  ADD COLUMN authorization_letter_path text,
  ADD COLUMN authorization_submitted_at timestamptz,
  -- admin user id or 'ciftctl' that took the last decision
  ADD COLUMN reviewed_by text,
  ADD COLUMN reviewed_at timestamptz,
  ADD COLUMN rejection_reason text;

-- Rows verified by the old KES 1 check keep their status. The table FORCEs
-- RLS on its owner too, so the backfill needs the policy lifted for a moment.
ALTER TABLE mpesa_shortcodes NO FORCE ROW LEVEL SECURITY;
UPDATE mpesa_shortcodes SET status = 'verified' WHERE verified_at IS NOT NULL;
ALTER TABLE mpesa_shortcodes FORCE ROW LEVEL SECURITY;

ALTER TABLE mpesa_shortcodes
  ADD CONSTRAINT mpesa_shortcodes_status_verified_at
    CHECK ((status = 'verified') = (verified_at IS NOT NULL));

-- One verified owner per number; pending duplicates across orgs are allowed
-- until Safaricom's answer settles it.
DROP INDEX IF EXISTS mpesa_shortcodes_verified_unique;
CREATE UNIQUE INDEX mpesa_shortcodes_verified_unique ON mpesa_shortcodes (shortcode) WHERE status = 'verified';
CREATE INDEX mpesa_shortcodes_status_idx ON mpesa_shortcodes (status, authorization_submitted_at);

-- Operators review the queue across tenants. Reads only: every decision is
-- written under the owning org's scope so the audit row lands in that tenant.
CREATE POLICY mpesa_shortcodes_admin ON mpesa_shortcodes
  FOR SELECT USING (current_scope() = 'admin');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP POLICY IF EXISTS mpesa_shortcodes_admin ON mpesa_shortcodes;
DROP INDEX IF EXISTS mpesa_shortcodes_status_idx;
DROP INDEX IF EXISTS mpesa_shortcodes_verified_unique;
CREATE UNIQUE INDEX mpesa_shortcodes_verified_unique ON mpesa_shortcodes (shortcode) WHERE verified_at IS NOT NULL;
ALTER TABLE mpesa_shortcodes
  DROP CONSTRAINT IF EXISTS mpesa_shortcodes_status_verified_at,
  DROP COLUMN IF EXISTS rejection_reason,
  DROP COLUMN IF EXISTS reviewed_at,
  DROP COLUMN IF EXISTS reviewed_by,
  DROP COLUMN IF EXISTS authorization_submitted_at,
  DROP COLUMN IF EXISTS authorization_letter_path,
  DROP COLUMN IF EXISTS status,
  ADD COLUMN verification_checkout_id text;
-- +goose StatementEnd
