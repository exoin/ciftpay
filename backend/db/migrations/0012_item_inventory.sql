-- Migration 0012: Item stock inventory tracking for eTIMS compliance.

ALTER TABLE items
  ADD COLUMN track_stock boolean NOT NULL DEFAULT false,
  ADD COLUMN stock_qty numeric(12, 3) NOT NULL DEFAULT 0;
