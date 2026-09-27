-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_acked_immutability() RETURNS trigger AS $$
BEGIN
  IF OLD.state = 'ACKED' THEN
    IF NEW.subtotal_cents <> OLD.subtotal_cents
       OR NEW.tax_cents <> OLD.tax_cents
       OR NEW.total_cents <> OLD.total_cents THEN
      RAISE EXCEPTION 'Cannot modify financial amounts of an ACKED invoice';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER invoices_acked_immutable
  BEFORE UPDATE ON invoices
  FOR EACH ROW
  EXECUTE FUNCTION enforce_acked_immutability();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS invoices_acked_immutable ON invoices;
DROP FUNCTION IF EXISTS enforce_acked_immutability();
-- +goose StatementEnd
