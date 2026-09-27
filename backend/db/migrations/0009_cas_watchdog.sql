-- +goose Up
-- +goose StatementBegin

-- Task 1 & 2: Compare-and-Swap (CAS) & Watchdog index and cross-tenant admin policy.
-- The stale SUBMITTED watchdog reads across tenants under app.scope = 'admin' (db.WithAdmin)
-- to locate jobs stuck in SUBMITTED state for longer than 15 minutes.
CREATE POLICY invoices_admin ON invoices
  FOR SELECT USING (current_scope() = 'admin');

-- Partial index to make the stale submitted query fast without overhead on settled invoices.
CREATE INDEX IF NOT EXISTS invoices_stale_submitted_idx
  ON invoices (state, updated_at)
  WHERE state = 'SUBMITTED';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS invoices_stale_submitted_idx;
DROP POLICY IF EXISTS invoices_admin ON invoices;

-- +goose StatementEnd
