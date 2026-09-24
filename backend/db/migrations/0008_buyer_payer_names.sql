-- +goose Up
-- +goose StatementBegin

-- Ensure sales table tracks buyer_name alongside invoices for official KRA name resolution.
ALTER TABLE sales
  ADD COLUMN IF NOT EXISTS buyer_name text NOT NULL DEFAULT '';

-- Security definer helper returns all payment_ids associated with a receipt or its superseded chain.
CREATE OR REPLACE FUNCTION get_receipt_chain_payment_ids(rc text)
RETURNS SETOF uuid
LANGUAGE sql
STABLE
SECURITY DEFINER
AS $$
  SELECT payment_id FROM invoices
  WHERE (receipt_code = rc OR id IN (SELECT get_receipt_chain_invoice_ids(rc)))
    AND payment_id IS NOT NULL;
$$;

-- Public receipt RLS policy: allows reading payments linked to the receipt code or its superseded chain.
DROP POLICY IF EXISTS payments_public_receipt ON payments;
CREATE POLICY payments_public_receipt ON payments
  FOR SELECT USING (
    current_receipt_code() IS NOT NULL AND (
      id IN (SELECT get_receipt_chain_payment_ids(current_receipt_code()))
    )
  );

GRANT ALL ON sales, invoices, payments TO ciftpay;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP POLICY IF EXISTS payments_public_receipt ON payments;
DROP FUNCTION IF EXISTS get_receipt_chain_payment_ids(text);
ALTER TABLE sales DROP COLUMN IF EXISTS buyer_name;

-- +goose StatementEnd
