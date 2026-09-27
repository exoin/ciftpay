-- +goose Up
-- +goose StatementBegin

-- 1. Safaricom Onboarding State on Orgs
ALTER TABLE orgs
  ADD COLUMN daraja_status VARCHAR(50) NOT NULL DEFAULT 'pending_upload'
    CHECK (daraja_status IN ('pending_upload', 'ready_for_safaricom', 'sent_to_safaricom', 'live'));

CREATE INDEX IF NOT EXISTS orgs_daraja_status_idx ON orgs (daraja_status);

-- 2. Cross-Tenant Admin RLS Policies for Ops-Core
-- Invoices cross-tenant update for manual KRA support resolutions (force-acked)
CREATE POLICY invoices_admin_update ON invoices
  FOR UPDATE USING (current_scope() = 'admin') WITH CHECK (current_scope() = 'admin');

-- Fiscal submissions cross-tenant select for DLQ investigation
CREATE POLICY fiscal_submissions_admin ON fiscal_submissions
  FOR SELECT USING (current_scope() = 'admin');

-- Subscriptions cross-tenant select, insert, update for tier management
CREATE POLICY subscriptions_admin ON subscriptions
  FOR ALL USING (current_scope() = 'admin') WITH CHECK (current_scope() = 'admin');

-- Notifications cross-tenant insert/select for merchant CRM alerts
CREATE POLICY notifications_admin ON notifications
  FOR ALL USING (current_scope() = 'admin') WITH CHECK (current_scope() = 'admin');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP POLICY IF EXISTS notifications_admin ON notifications;
DROP POLICY IF EXISTS subscriptions_admin ON subscriptions;
DROP POLICY IF EXISTS subscriptions_admin_update ON subscriptions;
DROP POLICY IF EXISTS subscriptions_admin_select ON subscriptions;
DROP POLICY IF EXISTS fiscal_submissions_admin ON fiscal_submissions;
DROP POLICY IF EXISTS invoices_admin_update ON invoices;

DROP INDEX IF EXISTS orgs_daraja_status_idx;
ALTER TABLE orgs DROP COLUMN IF EXISTS daraja_status;

-- +goose StatementEnd
