-- +goose Up
-- +goose StatementBegin

-- CiftPay initial schema. See docs/data-model.md and docs/adr/0007-rls-multitenancy.md.
-- Money is bigint cents. Time is timestamptz (UTC). Personal data is stored as
-- *_enc (envelope-encrypted) + *_hash (HMAC) pairs, never in clear.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- current_org() is the single place that reads the RLS scope. NULLIF guards
-- against the empty-string value a session keeps after SET LOCAL has expired.
CREATE OR REPLACE FUNCTION current_org() RETURNS uuid
LANGUAGE sql STABLE AS $$
  SELECT NULLIF(current_setting('app.org_id', true), '')::uuid
$$;

CREATE OR REPLACE FUNCTION current_scope() RETURNS text
LANGUAGE sql STABLE AS $$
  SELECT NULLIF(current_setting('app.scope', true), '')
$$;

CREATE OR REPLACE FUNCTION current_receipt_code() RETURNS text
LANGUAGE sql STABLE AS $$
  SELECT NULLIF(current_setting('app.receipt_code', true), '')
$$;

-- ---------------------------------------------------------------- identity

CREATE TABLE orgs (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name                 text NOT NULL,
  kra_pin_enc          bytea,
  kra_pin_hash         bytea UNIQUE,
  kra_pin_verified_at  timestamptz,
  vat_registered       boolean NOT NULL DEFAULT false,
  fiscal_profile       jsonb NOT NULL DEFAULT '{}'::jsonb,
  locale               text NOT NULL DEFAULT 'en' CHECK (locale IN ('en','sw')),
  status               text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER orgs_updated_at BEFORE UPDATE ON orgs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE users (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  msisdn_enc     bytea NOT NULL,
  msisdn_hash    bytea NOT NULL UNIQUE,
  name           text NOT NULL DEFAULT '',
  locale         text NOT NULL DEFAULT 'en' CHECK (locale IN ('en','sw')),
  last_login_at  timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE memberships (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role        text NOT NULL CHECK (role IN ('owner','staff','accountant','admin')),
  is_default  boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);

CREATE TABLE sessions (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  bytea NOT NULL UNIQUE,
  csrf_token  text NOT NULL,
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz,
  user_agent  text NOT NULL DEFAULT '',
  ip          inet,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE otp_codes (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  msisdn_hash  bytea NOT NULL,
  code_hash    bytea NOT NULL,
  expires_at   timestamptz NOT NULL,
  attempts     int NOT NULL DEFAULT 0,
  consumed_at  timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX otp_codes_lookup_idx ON otp_codes (msisdn_hash, expires_at);

-- ------------------------------------------------------------------ m-pesa

CREATE TABLE webhook_events (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider      text NOT NULL CHECK (provider IN ('mpesa','at')),
  kind          text NOT NULL CHECK (kind IN ('c2b_validation','c2b_confirmation','stk_callback','reversal','at_delivery')),
  external_id   text NOT NULL UNIQUE,
  payload       jsonb,
  received_at   timestamptz NOT NULL DEFAULT now(),
  processed_at  timestamptz,
  error         text
);
CREATE INDEX webhook_events_unprocessed_idx ON webhook_events (received_at) WHERE processed_at IS NULL;

CREATE TABLE items (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name              text NOT NULL,
  etims_class_code  text NOT NULL,
  tax_category      char(1) NOT NULL CHECK (tax_category IN ('A','B','C','D','E')),
  unit              text NOT NULL DEFAULT 'PCS',
  price_cents       bigint NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  is_active         boolean NOT NULL DEFAULT true,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX items_org_active_idx ON items (org_id, is_active);
CREATE TRIGGER items_updated_at BEFORE UPDATE ON items FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE mpesa_shortcodes (
  id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id                    uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  kind                      text NOT NULL CHECK (kind IN ('till','paybill','pochi')),
  shortcode                 text NOT NULL,
  label                     text NOT NULL DEFAULT '',
  default_item_id           uuid REFERENCES items(id),
  auto_invoice              boolean NOT NULL DEFAULT true,
  verified_at               timestamptz,
  verification_checkout_id  text,
  c2b_urls_registered_at    timestamptz,
  created_at                timestamptz NOT NULL DEFAULT now(),
  updated_at                timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX mpesa_shortcodes_verified_unique ON mpesa_shortcodes (shortcode) WHERE verified_at IS NOT NULL;
CREATE INDEX mpesa_shortcodes_org_idx ON mpesa_shortcodes (org_id);
CREATE INDEX mpesa_shortcodes_lookup_idx ON mpesa_shortcodes (shortcode);
CREATE TRIGGER mpesa_shortcodes_updated_at BEFORE UPDATE ON mpesa_shortcodes FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE customers (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name          text NOT NULL DEFAULT '',
  msisdn_enc    bytea,
  msisdn_hash   bytea,
  kra_pin_enc   bytea,
  kra_pin_hash  bytea,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX customers_org_pin_unique ON customers (org_id, kra_pin_hash) WHERE kra_pin_hash IS NOT NULL;
CREATE INDEX customers_org_msisdn_idx ON customers (org_id, msisdn_hash);
CREATE TRIGGER customers_updated_at BEFORE UPDATE ON customers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ------------------------------------------------------------------ ledger

CREATE TABLE sales (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  ref             text NOT NULL,
  kind            text NOT NULL CHECK (kind IN ('open','cash')),
  status          text NOT NULL DEFAULT 'open' CHECK (status IN ('open','paid','void')),
  customer_id     uuid REFERENCES customers(id),
  subtotal_cents  bigint NOT NULL DEFAULT 0,
  tax_cents       bigint NOT NULL DEFAULT 0,
  total_cents     bigint NOT NULL DEFAULT 0,
  client_ref      text,
  created_by      uuid REFERENCES users(id),
  paid_at         timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, ref)
);
CREATE UNIQUE INDEX sales_org_client_ref_unique ON sales (org_id, client_ref) WHERE client_ref IS NOT NULL;
CREATE INDEX sales_org_status_idx ON sales (org_id, status, created_at DESC);
CREATE TRIGGER sales_updated_at BEFORE UPDATE ON sales FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sale_items (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  sale_id           uuid NOT NULL REFERENCES sales(id) ON DELETE CASCADE,
  item_id           uuid REFERENCES items(id),
  description       text NOT NULL,
  etims_class_code  text NOT NULL,
  unit              text NOT NULL DEFAULT 'PCS',
  qty               numeric(12,3) NOT NULL DEFAULT 1,
  unit_price_cents  bigint NOT NULL,
  tax_category      char(1) NOT NULL CHECK (tax_category IN ('A','B','C','D','E')),
  tax_rate_bp       int NOT NULL,
  line_total_cents  bigint NOT NULL,
  line_tax_cents    bigint NOT NULL,
  position          int NOT NULL DEFAULT 0
);
CREATE INDEX sale_items_sale_idx ON sale_items (sale_id, position);

CREATE TABLE stk_requests (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id               uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  sale_id              uuid REFERENCES sales(id),
  shortcode_id         uuid NOT NULL REFERENCES mpesa_shortcodes(id),
  msisdn_hash          bytea NOT NULL,
  amount_cents         bigint NOT NULL,
  checkout_request_id  text NOT NULL UNIQUE,
  merchant_request_id  text,
  purpose              text NOT NULL DEFAULT 'sale' CHECK (purpose IN ('sale','verify')),
  status               text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','success','failed','expired')),
  result_code          int,
  result_desc          text,
  expires_at           timestamptz NOT NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX stk_requests_window_idx ON stk_requests (org_id, msisdn_hash, amount_cents, created_at DESC);
CREATE TRIGGER stk_requests_updated_at BEFORE UPDATE ON stk_requests FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payments (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id            uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  shortcode_id      uuid NOT NULL REFERENCES mpesa_shortcodes(id),
  webhook_event_id  uuid REFERENCES webhook_events(id),
  trans_id          text NOT NULL UNIQUE,
  amount_cents      bigint NOT NULL CHECK (amount_cents > 0),
  msisdn_enc        bytea,
  msisdn_hash       bytea,
  payer_name        text NOT NULL DEFAULT '',
  bill_ref          text NOT NULL DEFAULT '',
  paid_at           timestamptz NOT NULL,
  status            text NOT NULL CHECK (status IN ('matched','cash_sale','unmatched','reversed','partial')),
  sale_id           uuid REFERENCES sales(id),
  match_rule        text CHECK (match_rule IN ('bill_ref','stk_window','auto_invoice','manual')),
  reversed_at       timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payments_org_paid_idx ON payments (org_id, paid_at DESC);
CREATE INDEX payments_org_status_idx ON payments (org_id, status);
CREATE INDEX payments_msisdn_idx ON payments (org_id, msisdn_hash, paid_at DESC);
CREATE TRIGGER payments_updated_at BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ------------------------------------------------------------------ fiscal

CREATE TABLE invoices (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id             uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  sale_id            uuid NOT NULL REFERENCES sales(id),
  payment_id         uuid REFERENCES payments(id),
  kind               text NOT NULL DEFAULT 'INVOICE' CHECK (kind IN ('INVOICE','CREDIT_NOTE')),
  parent_invoice_id  uuid REFERENCES invoices(id),
  state              text NOT NULL DEFAULT 'DRAFT'
                     CHECK (state IN ('DRAFT','QUEUED','SUBMITTED','ACKED','FAILED_RETRYABLE','FAILED_TERMINAL','NEEDS_REVIEW')),
  attempt            int NOT NULL DEFAULT 0,
  next_attempt_at    timestamptz,
  buyer_pin_enc      bytea,
  buyer_pin_hash     bytea,
  buyer_name         text NOT NULL DEFAULT '',
  kra_invoice_no     text,
  kra_signature      text,
  kra_qr_payload     text,
  receipt_code       text NOT NULL UNIQUE,
  subtotal_cents     bigint NOT NULL,
  tax_cents          bigint NOT NULL,
  total_cents        bigint NOT NULL,
  issued_at          timestamptz NOT NULL DEFAULT now(),
  submitted_at       timestamptz,
  acked_at           timestamptz,
  last_error         text,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX invoices_org_state_idx ON invoices (org_id, state);
CREATE INDEX invoices_org_acked_idx ON invoices (org_id, acked_at DESC);
CREATE INDEX invoices_worker_idx ON invoices (state, next_attempt_at);
CREATE INDEX invoices_sale_idx ON invoices (sale_id);
CREATE TRIGGER invoices_updated_at BEFORE UPDATE ON invoices FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE fiscal_submissions (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  invoice_id      uuid NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  attempt         int NOT NULL,
  adapter         text NOT NULL,
  request         jsonb,
  response        jsonb,
  error           text,
  classification  text NOT NULL CHECK (classification IN ('ok','retryable','terminal')),
  started_at      timestamptz NOT NULL DEFAULT now(),
  finished_at     timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fiscal_submissions_invoice_idx ON fiscal_submissions (invoice_id, attempt);

-- -------------------------------------------------- notifications & billing

CREATE TABLE notifications (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id               uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  invoice_id           uuid REFERENCES invoices(id) ON DELETE SET NULL,
  channel              text NOT NULL CHECK (channel IN ('sms','whatsapp')),
  to_msisdn_enc        bytea,
  to_msisdn_hash       bytea,
  template             text NOT NULL,
  locale               text NOT NULL DEFAULT 'en',
  body                 text NOT NULL,
  provider_message_id  text,
  status               text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sent','delivered','failed')),
  cost_cents           bigint NOT NULL DEFAULT 0,
  sent_at              timestamptz,
  delivered_at         timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_org_idx ON notifications (org_id, created_at DESC);
CREATE INDEX notifications_provider_idx ON notifications (provider_message_id) WHERE provider_message_id IS NOT NULL;
CREATE TRIGGER notifications_updated_at BEFORE UPDATE ON notifications FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE plans (
  code                 text PRIMARY KEY,
  name                 text NOT NULL,
  price_cents_monthly  bigint NOT NULL,
  invoice_cap          int,
  overage_cents        bigint NOT NULL DEFAULT 0,
  features             jsonb NOT NULL DEFAULT '{}'::jsonb
);
INSERT INTO plans (code, name, price_cents_monthly, invoice_cap, overage_cents, features) VALUES
  ('hustler',    'Hustler',    0,      30,   500, '{"shortcodes":1,"whatsapp":false,"exports":false,"accountant_seat":false,"api":false}'),
  ('duka',       'Duka',       150000, 500,  500, '{"shortcodes":3,"whatsapp":true,"exports":true,"accountant_seat":false,"api":false}'),
  ('biashara',   'Biashara',   450000, NULL, 0,   '{"shortcodes":null,"whatsapp":true,"exports":true,"accountant_seat":true,"api":true}'),
  ('accountant', 'Accountant', 900000, NULL, 0,   '{"client_orgs":25,"whatsapp":true,"exports":true,"accountant_seat":true,"api":true}');

CREATE TABLE subscriptions (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id        uuid NOT NULL UNIQUE REFERENCES orgs(id) ON DELETE CASCADE,
  plan_code     text NOT NULL REFERENCES plans(code),
  status        text NOT NULL DEFAULT 'trial' CHECK (status IN ('trial','active','past_due','cancelled')),
  period_start  timestamptz NOT NULL DEFAULT now(),
  period_end    timestamptz,
  grace_until   timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER subscriptions_updated_at BEFORE UPDATE ON subscriptions FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE usage_counters (
  org_id          uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  period          text NOT NULL, -- YYYY-MM in Africa/Nairobi
  invoices_acked  int NOT NULL DEFAULT 0,
  sms_sent        int NOT NULL DEFAULT 0,
  whatsapp_sent   int NOT NULL DEFAULT 0,
  PRIMARY KEY (org_id, period)
);

CREATE TABLE audit_log (
  id          bigserial PRIMARY KEY,
  org_id      uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  actor_type  text NOT NULL CHECK (actor_type IN ('user','system','admin')),
  actor_id    text,
  action      text NOT NULL,
  entity      text NOT NULL,
  entity_id   text NOT NULL,
  before      jsonb,
  after       jsonb,
  at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_org_idx ON audit_log (org_id, at DESC);
CREATE INDEX audit_log_entity_idx ON audit_log (entity, entity_id);

CREATE TABLE feature_flags (
  key            text PRIMARY KEY,
  enabled        boolean NOT NULL DEFAULT false,
  org_allowlist  uuid[] NOT NULL DEFAULT '{}'
);

-- --------------------------------------------------------------------- RLS
-- Every tenant table: ENABLE + FORCE so the policy binds the owner too (the
-- local role owns the tables). Scope comes from SET LOCAL app.org_id inside a
-- transaction (db.WithOrg). Two narrow extra scopes exist:
--   app.scope = 'ingest'        api webhook path may SELECT mpesa_shortcodes to
--                               resolve a shortcode to its org before scoping.
--   app.receipt_code = <code>   public receipt path may SELECT the one invoice,
--                               its sale lines and nothing else.

DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'items','mpesa_shortcodes','customers','sales','sale_items','stk_requests',
    'payments','invoices','fiscal_submissions','notifications','subscriptions',
    'usage_counters','audit_log'
  ] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY %I ON %I USING (org_id = current_org()) WITH CHECK (org_id = current_org())',
      t || '_tenant', t);
  END LOOP;
END $$;

CREATE POLICY mpesa_shortcodes_ingest ON mpesa_shortcodes
  FOR SELECT USING (current_scope() = 'ingest');

-- STK callbacks carry no shortcode, only the CheckoutRequestID CiftPay issued;
-- the ingest path may read stk_requests to find the org before scoping.
CREATE POLICY stk_requests_ingest ON stk_requests
  FOR SELECT USING (current_scope() = 'ingest');

-- Africa's Talking delivery reports carry only the provider message id; the
-- ingest path may flip the matching notification to 'delivered'.
CREATE POLICY notifications_delivery ON notifications
  FOR UPDATE USING (current_scope() = 'ingest') WITH CHECK (current_scope() = 'ingest');

CREATE POLICY invoices_public_receipt ON invoices
  FOR SELECT USING (receipt_code = current_receipt_code());

-- GetInvoiceByReceiptCode joins sales for the sale ref.
CREATE POLICY sales_public_receipt ON sales
  FOR SELECT USING (
    current_receipt_code() IS NOT NULL AND id IN (
      SELECT sale_id FROM invoices WHERE receipt_code = current_receipt_code()
    )
  );

CREATE POLICY sale_items_public_receipt ON sale_items
  FOR SELECT USING (
    current_receipt_code() IS NOT NULL AND sale_id IN (
      SELECT sale_id FROM invoices WHERE receipt_code = current_receipt_code()
    )
  );

-- audit_log is append-only for everyone, including the owner.
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit_log is append-only';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER audit_log_no_update BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS feature_flags, audit_log, usage_counters, subscriptions, plans,
  notifications, fiscal_submissions, invoices, payments, stk_requests, sale_items,
  sales, customers, mpesa_shortcodes, items, webhook_events, otp_codes, sessions,
  memberships, users, orgs CASCADE;
DROP FUNCTION IF EXISTS audit_log_immutable();
DROP FUNCTION IF EXISTS current_receipt_code();
DROP FUNCTION IF EXISTS current_scope();
DROP FUNCTION IF EXISTS current_org();
DROP FUNCTION IF EXISTS set_updated_at();
-- +goose StatementEnd
