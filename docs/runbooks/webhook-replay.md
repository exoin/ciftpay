# Runbook: replaying Daraja webhooks

How to push a recorded Safaricom Daraja payload through the local stack, what should happen, and how to verify it. This is the Phase-0 acceptance test and the fastest way to reproduce a production incident locally.

## The golden payloads

`tools/webhooks/` holds hand-verified JSON bodies in the exact shape Daraja sends:

| File | Simulates | Expected outcome |
|---|---|---|
| `c2b_confirmation.json` | Paybill/Till payment of KES 2 400 from `254708374149` to shortcode `600123`, empty `BillRefNumber` | cash sale on the shortcode's default item, invoice to `ACKED` (mock adapter), SMS receipt |
| `stk_callback.json` | STK push result for a request-to-pay created by CiftPay | matches the open sale by `CheckoutRequestID`, sale marked paid, invoice queued |
| `reversal.json` | Reversal of a previously confirmed `TransID` | payment marked reversed; if its invoice is `ACKED`, a linked `CREDIT_NOTE` is queued |

The seed (`make seed`) creates the org and shortcode `600123` that these payloads target. Without the seed the confirmation is stored in `webhook_events` but no payment is created (unknown shortcode is logged and surfaced in admin `/ops`).

## Replay one payload

```bash
make up && make migrate && make seed      # once
make replay-webhook FILE=tools/webhooks/c2b_confirmation.json
```

`make replay-webhook` runs `ciftctl replay-webhook ../tools/webhooks/c2b_confirmation.json`. The CLI:

1. reads `DARAJA_WEBHOOK_TOKEN` (and `API_BASE_URL`, default `http://localhost:8080`) from the environment / `.env`;
2. picks the route from the payload shape: C2B fields -> `POST /webhooks/mpesa/c2b/confirmation/{token}`, `Body.stkCallback` -> `POST /webhooks/mpesa/stk/{token}`;
3. POSTs the JSON and prints the status and body. Daraja expects `{"ResultCode":0,"ResultDesc":"Accepted"}` and the api returns exactly that, also on duplicates.

Anything other than HTTP 200 means the token is wrong (401), the body failed validation (400) or the api is down.

## Expected state after one replay

The api stores the raw event and the payment in one transaction and enqueues the fiscal job in the same transaction (ADR-0004). The worker then drives the invoice through the state machine. With `FISCAL_ADAPTER=mock` and `MOCK_FAIL_MODE=none` the whole chain finishes in under two seconds.

| Table | Rows | What to look at |
|---|---|---|
| `webhook_events` | 1 | `external_id = 'mpesa:SLJ7X2K91Q'`, `provider = 'mpesa'`, `payload` is the JSON you sent |
| `payments` | 1 | `trans_id = 'SLJ7X2K91Q'`, `amount_cents = 240000`, `status = 'matched'`, `msisdn_hash` set, `msisdn_enc` set |
| `sales` | 1 | `status = 'paid'`, `kind = 'cash'`, `total_cents = 240000` |
| `sale_items` | 1 | the shortcode's default item, `tax_category = 'B'` |
| `invoices` | 1 | `state = 'ACKED'`, `kra_invoice_no`, `signature`, `qr_payload`, `receipt_code` set |
| `fiscal_submissions` | 1 | `attempt = 1`, `adapter = 'mock'`, `response` contains the ack |
| `notifications` | 1 | `channel = 'sms'`, `status = 'sent'`, body contains `/r/<receipt_code>` |

Open http://localhost:8025 and you should see one message to `+254708374149`; follow its `/r/<code>` link to http://localhost:3000/r/<code> and the receipt should render server-side.

## Verify with psql

Tenant tables are behind RLS (ADR-0007), so set the org first. `webhook_events` is global.

```bash
make psql
```

```sql
SELECT external_id, received_at FROM webhook_events ORDER BY received_at DESC LIMIT 5;

SELECT id FROM orgs;                                  -- copy the seeded org id
BEGIN;
SET LOCAL app.org_id = '<org uuid>';

SELECT trans_id, amount_cents, status, paid_at FROM payments ORDER BY paid_at DESC LIMIT 5;
SELECT id, kind, status, total_cents FROM sales ORDER BY created_at DESC LIMIT 5;
SELECT id, state, kra_invoice_no, receipt_code, acked_at FROM invoices ORDER BY created_at DESC LIMIT 5;
SELECT invoice_id, attempt, adapter, error FROM fiscal_submissions ORDER BY created_at DESC LIMIT 10;
SELECT channel, status, to_msisdn_hash, created_at FROM notifications ORDER BY created_at DESC LIMIT 5;
COMMIT;
```

If `invoices.state` is stuck at `QUEUED`, the worker is not running or cannot reach Postgres: `docker compose logs worker`. If it is `FAILED_RETRYABLE` or `NEEDS_REVIEW`, follow `fiscal-failures.md`.

## Idempotency test

Daraja retries confirmations and occasionally sends the same `TransID` twice. Replay the same file again:

```bash
make replay-webhook FILE=tools/webhooks/c2b_confirmation.json
make replay-webhook FILE=tools/webhooks/c2b_confirmation.json
```

Both calls return HTTP 200 with `ResultCode 0`. Then:

```sql
SELECT count(*) FROM webhook_events WHERE external_id = 'mpesa:SLJ7X2K91Q';   -- 1
-- inside BEGIN / SET LOCAL app.org_id
SELECT count(*) FROM payments WHERE trans_id = 'SLJ7X2K91Q';                  -- 1
SELECT count(*) FROM invoices;                                                -- still 1
SELECT count(*) FROM notifications;                                           -- still 1
```

The second insert hits `webhook_events.external_id UNIQUE` with `ON CONFLICT DO NOTHING`; the handler sees zero rows affected and returns the acceptance body without enqueueing anything. This is covered by `internal/mpesa/webhook_test.go`.

## Crafting a new payload

Copy `c2b_confirmation.json`, change `TransID` (any new 10-character alphanumeric string) and whatever you are testing. Field reference for a Daraja C2B confirmation:

| Field | Type / format | Notes |
|---|---|---|
| `TransactionType` | string | `Pay Bill` or `Buy Goods` |
| `TransID` | string, 10 chars | M-Pesa receipt number; the idempotency key |
| `TransTime` | `YYYYMMDDHHmmss` | Nairobi local time (EAT, UTC+3); parsed to UTC on ingest |
| `TransAmount` | decimal string | whole shillings in practice; stored as cents |
| `BusinessShortCode` | string | Till or Paybill number; must match a seeded `mpesa_shortcodes.shortcode` |
| `BillRefNumber` | string | Paybill account reference; if it equals an open `sales.ref` the sale is matched, if empty a cash sale is created |
| `InvoiceNumber` | string | usually empty |
| `OrgAccountBalance` | decimal string | merchant balance after the payment; stored in the raw payload only |
| `ThirdPartyTransID` | string | usually empty |
| `MSISDN` | string | payer phone, `2547XXXXXXXX`; hashed and encrypted, never stored in clear. Recent Daraja versions may send a masked/hashed value instead; the matcher then falls back to amount+time |
| `FirstName`, `MiddleName`, `LastName` | string | payer name; used only for the receipt greeting |

To test the open-sale path: create a sale through `POST /v1/sales` (or the PWA "Record a sale"), put its `ref` in `BillRefNumber` and set `TransAmount` to the sale total.

For STK callbacks use `stk_callback.json` as the template; the important fields are `Body.stkCallback.CheckoutRequestID`, `ResultCode` (`0` success, `1032` cancelled by user, `1037` timeout) and `CallbackMetadata.Item[]` with `Amount`, `MpesaReceiptNumber`, `PhoneNumber`.

## Replaying a production incident

1. Export the raw payload from `webhook_events.payload` in production (it is stored verbatim).
2. Save it as `tools/webhooks/incident-<date>.json` (the Makefile resolves `FILE` relative to the repo root); change nothing except, if needed, `BusinessShortCode` to a locally seeded shortcode. Do not commit it unless sanitised.
3. `make replay-webhook FILE=tools/webhooks/incident-<date>.json` and follow the tables above.
4. If the payload exposes a new edge case, add a sanitised copy to `tools/webhooks/` and a case to `internal/ledger/matcher_test.go`.
