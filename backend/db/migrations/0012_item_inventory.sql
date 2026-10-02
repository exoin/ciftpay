-- +goose Up
-- +goose StatementBegin
ALTER TABLE items
  ADD COLUMN IF NOT EXISTS track_stock boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS stock_qty numeric(12, 3) NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE items
  DROP COLUMN IF EXISTS stock_qty,
  DROP COLUMN IF EXISTS track_stock;
-- +goose StatementEnd
