-- +goose Up
-- +goose StatementBegin

-- Direct KRA OSCU integration (ADR-0009) and progressive eTIMS onboarding.
-- CiftPay's own OSCU consumer key/secret are global (KRA env vars); the
-- taxpayer PIN a merchant already gave us (orgs.kra_pin_enc) is reused for
-- device initialisation, but KRA also requires a branch id (bhfId) and a
-- device serial (dvcSrlNo) that are specific to the merchant, plus the
-- cmcKey KRA hands back to sign every subsequent call. cmcKey is a signing
-- credential, not personal data, but it gets the same envelope-encryption
-- treatment as kra_pin_enc (never stored in clear; see docs/data-model.md §4).
ALTER TABLE orgs
  ADD COLUMN etims_status text NOT NULL DEFAULT 'unconfigured'
    CHECK (etims_status IN ('unconfigured','initialized','failed')),
  ADD COLUMN kra_bhf_id text,
  ADD COLUMN kra_device_serial text,
  ADD COLUMN kra_cmc_key_enc bytea,
  ADD COLUMN etims_failed_reason text,
  ADD COLUMN etims_initialized_at timestamptz;

-- Progressive onboarding (this migration): a payment can be ingested into the
-- ledger before the merchant has configured eTIMS. Such invoices sit in
-- TAX_PENDING (created, never queued for KRA) until the merchant configures
-- eTIMS, at which point they are moved to QUEUED in one batch. See
-- internal/fiscal/state.go.
ALTER TABLE invoices DROP CONSTRAINT invoices_state_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_state_check
  CHECK (state IN ('DRAFT','TAX_PENDING','QUEUED','SUBMITTED','ACKED','FAILED_RETRYABLE','FAILED_TERMINAL','NEEDS_REVIEW'));

CREATE INDEX invoices_tax_pending_idx ON invoices (org_id) WHERE state = 'TAX_PENDING';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS invoices_tax_pending_idx;
ALTER TABLE invoices DROP CONSTRAINT invoices_state_check;
ALTER TABLE invoices ADD CONSTRAINT invoices_state_check
  CHECK (state IN ('DRAFT','QUEUED','SUBMITTED','ACKED','FAILED_RETRYABLE','FAILED_TERMINAL','NEEDS_REVIEW'));

ALTER TABLE orgs
  DROP COLUMN IF EXISTS etims_initialized_at,
  DROP COLUMN IF EXISTS etims_failed_reason,
  DROP COLUMN IF EXISTS kra_cmc_key_enc,
  DROP COLUMN IF EXISTS kra_device_serial,
  DROP COLUMN IF EXISTS kra_bhf_id,
  DROP COLUMN IF EXISTS etims_status;
-- +goose StatementEnd
