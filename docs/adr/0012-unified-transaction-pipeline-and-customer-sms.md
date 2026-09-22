# ADR-0012: Unified Transaction Pipeline for Manual Sales and Webhooks with Customer SMS Receipts

## Status

Accepted — 2026-09-21.

## Context

The initial system design prioritized automated Daraja M-Pesa C2B callbacks as the primary source of transactions. Manual sales ("Record Sale" in the merchant UI) were initially treated as an offline cash accounting fallback, without a first-class customer notification mechanism.

However, field audits and merchant workflows highlighted two critical gaps:
1. When a merchant records a cash sale or in-person payment via "Record Sale", the customer frequently needs to receive their KRA eTIMS fiscal receipt immediately via SMS on their mobile phone for business expense claims or personal tax deduction.
2. Inconsistent transaction flows (where webhooks followed one asynchronous queue pipeline and manual sales followed another) introduced drift in tax calculations, receipt code generation, and audit logging.

## Decision

1. **Unified Transaction Pipeline**:
   - Both Daraja M-Pesa C2B webhooks and merchant Manual Sales follow the identical 4-stage pipeline:
     1. **Ingest & Validate**: Parse line items, enforce strict mathematical rounding (`totAmt = taxblAmt + taxAmt`), and validate tax categories.
     2. **Ledger Recording**: Atomically insert into `sales` and `sale_items`, recording audit rows.
     3. **Queue eTIMS Submission**: Enqueue a River background job to file the electronic invoice with KRA OSCU.
     4. **Queue SMS Delivery**: Enqueue a River background job to deliver the public receipt link (`/r/[code]`) to the customer's MSISDN.

2. **Customer Phone Field on Manual Sales**:
   - Added an optional customer mobile phone input (`customer_msisdn`) to the "Record a Sale" sheet.
   - When provided, the phone number is normalized to E.164 (`254XXXXXXXXX`), securely encrypted and hashed (`msisdn_enc`, `msisdn_hash`), and triggers the outbound SMS receipt job upon sale creation.

## Consequences

### Positive
- Identical reliability, queue guarantees, and fiscal compliance across cash, bank, and mobile money transactions.
- Customers receiving cash sales get immediate proof of tax compliance via SMS.

### Negative
- Manual sales require outbound SMS sink or provider credits when a customer phone number is provided.
