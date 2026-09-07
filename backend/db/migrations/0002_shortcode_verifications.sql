-- +goose Up
-- +goose StatementBegin

-- Shortcode control check (plan.md §4.1, ADR-0003): the merchant proves a
-- Till/Paybill/Pochi is theirs by paying exactly KES 1 to it from their own
-- phone. A row here is the open challenge the C2B confirmation is matched
-- against (shortcode + payer msisdn + amount inside the window). The KES 1 is
-- never recorded in payments; the challenge keeps the TransID instead.
CREATE TABLE shortcode_verifications (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  shortcode_id  uuid NOT NULL REFERENCES mpesa_shortcodes(id) ON DELETE CASCADE,
  msisdn_hash   bytea NOT NULL,
  amount_cents  bigint NOT NULL DEFAULT 100 CHECK (amount_cents > 0),
  status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','verified','expired','failed')),
  trans_id      text,
  paid_at       timestamptz,
  expires_at    timestamptz NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX shortcode_verifications_match_idx ON shortcode_verifications (shortcode_id, msisdn_hash, status);
CREATE INDEX shortcode_verifications_latest_idx ON shortcode_verifications (shortcode_id, created_at DESC);
CREATE TRIGGER shortcode_verifications_updated_at BEFORE UPDATE ON shortcode_verifications FOR EACH ROW EXECUTE FUNCTION set_updated_at();

ALTER TABLE shortcode_verifications ENABLE ROW LEVEL SECURITY;
ALTER TABLE shortcode_verifications FORCE ROW LEVEL SECURITY;
CREATE POLICY shortcode_verifications_tenant ON shortcode_verifications
  USING (org_id = current_org()) WITH CHECK (org_id = current_org());

-- An unverified number may exist in several orgs; the C2B ingest path reads
-- the open challenges for the number to pick the org that is proving it.
CREATE POLICY shortcode_verifications_ingest ON shortcode_verifications
  FOR SELECT USING (current_scope() = 'ingest');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS shortcode_verifications CASCADE;
-- +goose StatementEnd
