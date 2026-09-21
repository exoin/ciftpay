-- +goose Up
-- +goose StatementBegin

-- Credit Note and Re-issue architecture (superseded state tracking).
-- When an invoice is amended or retroactively claimed with a buyer KRA PIN,
-- KRA immutability rules require:
--   1. An offsetting Credit Note (rcptTyCd = "R") referencing the original KRA sequence.
--   2. A new re-issued Sales Invoice (rcptTyCd = "S") with the new buyer PIN.
-- The original invoice and credit note record superseded_by_id pointing to the
-- active re-issued invoice, allowing the public /r/{code} URL to always resolve
-- to the latest valid KRA invoice and signature.
ALTER TABLE invoices
  ADD COLUMN superseded_by_id uuid REFERENCES invoices(id),
  ADD COLUMN superseded_at timestamptz;

CREATE INDEX invoices_superseded_idx ON invoices (superseded_by_id) WHERE superseded_by_id IS NOT NULL;

-- Security definer helper avoids infinite recursion in RLS evaluation on invoices
CREATE OR REPLACE FUNCTION get_receipt_chain_invoice_ids(rc text)
RETURNS SETOF uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
AS $$
  WITH RECURSIVE chain AS (
    SELECT id, superseded_by_id, 1 AS depth FROM invoices WHERE receipt_code = rc
    UNION ALL
    SELECT i.id, i.superseded_by_id, c.depth + 1 FROM invoices i JOIN chain c ON i.id = c.superseded_by_id WHERE c.superseded_by_id IS NOT NULL AND c.depth < 10
  )
  SELECT id FROM chain;
$$;

-- Public receipt RLS policy: allows reading the invoice matching current_receipt_code()
-- as well as any successor invoice in the recursive superseded chain.
DROP POLICY IF EXISTS invoices_public_receipt ON invoices;
CREATE POLICY invoices_public_receipt ON invoices
  FOR SELECT USING (
    current_receipt_code() IS NOT NULL AND (
      receipt_code = current_receipt_code() OR
      id IN (SELECT get_receipt_chain_invoice_ids(current_receipt_code()))
    )
  );

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP POLICY IF EXISTS invoices_public_receipt ON invoices;
CREATE POLICY invoices_public_receipt ON invoices
  FOR SELECT USING (receipt_code = current_receipt_code());

DROP FUNCTION IF EXISTS get_receipt_chain_invoice_ids(text);

DROP INDEX IF EXISTS invoices_superseded_idx;
ALTER TABLE invoices
  DROP COLUMN IF EXISTS superseded_at,
  DROP COLUMN IF EXISTS superseded_by_id;

-- +goose StatementEnd
