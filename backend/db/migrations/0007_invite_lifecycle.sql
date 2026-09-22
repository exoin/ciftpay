-- +goose Up
-- +goose StatementBegin

-- 1. Update status check on org_invites to include 'rejected'
ALTER TABLE org_invites DROP CONSTRAINT IF EXISTS org_invites_status_check;
ALTER TABLE org_invites ADD CONSTRAINT org_invites_status_check
  CHECK (status IN ('pending', 'accepted', 'rejected', 'revoked'));

-- 2. Add optional email fields on users table for invite matching
ALTER TABLE users ADD COLUMN IF NOT EXISTS email text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_hash bytea;
CREATE INDEX IF NOT EXISTS users_email_hash_idx ON users (email_hash) WHERE email_hash IS NOT NULL;

-- 3. Cross-tenant scope for accountants reviewing invitations
CREATE POLICY org_invites_accountant_select ON org_invites
  FOR SELECT USING (current_scope() = 'accountant');

CREATE POLICY org_invites_accountant_update ON org_invites
  FOR UPDATE USING (current_scope() = 'accountant')
  WITH CHECK (current_scope() = 'accountant');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP POLICY IF EXISTS org_invites_accountant_select ON org_invites;
DROP POLICY IF EXISTS org_invites_accountant_update ON org_invites;

ALTER TABLE users DROP COLUMN IF EXISTS email_hash;
ALTER TABLE users DROP COLUMN IF EXISTS email;

ALTER TABLE org_invites DROP CONSTRAINT IF EXISTS org_invites_status_check;
ALTER TABLE org_invites ADD CONSTRAINT org_invites_status_check
  CHECK (status IN ('pending', 'accepted', 'revoked'));

-- +goose StatementEnd
