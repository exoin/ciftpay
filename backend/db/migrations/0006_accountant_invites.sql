-- +goose Up
-- +goose StatementBegin

CREATE TABLE org_invites (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id       uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  invited_by   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role         text NOT NULL CHECK (role IN ('accountant','staff','admin')),
  phone        text,
  phone_hash   bytea,
  email        text,
  status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked')),
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX org_invites_org_idx ON org_invites (org_id);
CREATE INDEX org_invites_phone_hash_idx ON org_invites (phone_hash) WHERE phone_hash IS NOT NULL;
CREATE INDEX org_invites_email_idx ON org_invites (email) WHERE email IS NOT NULL;

-- Tenant policy on org_invites
ALTER TABLE org_invites ENABLE ROW LEVEL SECURITY;
ALTER TABLE org_invites FORCE ROW LEVEL SECURITY;

CREATE POLICY org_invites_tenant ON org_invites
  USING (org_id = current_org())
  WITH CHECK (org_id = current_org());

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS org_invites;

-- +goose StatementEnd
