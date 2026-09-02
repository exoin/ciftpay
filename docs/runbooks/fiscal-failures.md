# Runbook: fiscal submission failures

What to do when invoices are not reaching `ACKED`. Covers reading the state machine, the retry policy, telling retryable from terminal failures, clearing `NEEDS_REVIEW`, and surviving a multi-hour vendor or KRA outage.

## Where the truth lives

- `invoices.state` is the current position in the state machine. `invoices.submitted_at`, `acked_at`, `last_error`, `attempt` and `next_attempt_at` summarise the history.
- `fiscal_submissions` is the append-only attempt trail: one row per call to `Provider.SubmitInvoice`/`SubmitCreditNote` with `attempt`, `adapter` (`mock`, `vendor`, `oscu`), `request` (what we sent, PIN fields redacted), `response` (raw vendor/KRA body) and `error` (our classification and message).
- River's job tables hold the scheduled retry (`scheduled_at`) for invoices in `QUEUED`.
- The merchant sees the same failures in the PWA under **Attention**; admins see all tenants in `/ops`.

State machine (see `docs/architecture.md`):

```
DRAFT -> QUEUED -> SUBMITTED -> ACKED
                   SUBMITTED -> FAILED_RETRYABLE -> QUEUED        (backoff, max 8 attempts)
                   SUBMITTED -> FAILED_TERMINAL  -> NEEDS_REVIEW  (human fixes, then re-queue)
ACKED + reversal -> CREDIT_NOTE (new invoice row, same machine, linked by parent_invoice_id)
```

## Retry policy

- Retryable failure at attempt `n` (1-based) schedules the next attempt after `2^(n-1) * 15 s`: 15 s, 30 s, 1 m, 2 m, 4 m, 8 m, 16 m, 32 m. Total window roughly one hour.
- After 8 attempts the invoice moves to `FAILED_TERMINAL` with `last_error = 'max_attempts_exceeded'` and then `NEEDS_REVIEW`.
- Terminal classification at any attempt skips the remaining retries.
- `SubmitInvoice` is idempotent by invoice ID: the adapter sends the same client reference every time, and the vendor returns the existing ack if the first call actually succeeded but the response was lost. A `duplicate` response on retry is therefore treated as success and the ack is extracted from it.
- The buyer already received a "receipt pending KRA confirmation" SMS at `QUEUED`; on `ACKED` a second message with the KRA invoice number and `/r/<code>` link is sent. Nothing is sent on failure; the merchant is notified through **Attention**.

## Retryable vs terminal

| Class | Examples | Classification |
|---|---|---|
| Network | connect timeout, TLS handshake failure, connection reset, DNS failure | retryable |
| Vendor/KRA 5xx | 500, 502, 503, 504 | retryable |
| Rate limiting | 429, vendor `TOO_MANY_REQUESTS` | retryable, honour `Retry-After` if present |
| Maintenance | vendor `SERVICE_UNAVAILABLE`, KRA scheduled downtime notice | retryable |
| Auth | 401/403, expired API key, device not registered | terminal (`config_error`) - fix credentials, then bulk re-queue |
| Validation | invalid item classification code, unknown tax category, negative/zero line, wrong precision | terminal (`validation_error`) |
| Buyer data | buyer KRA PIN malformed (`^[AP][0-9]{9}[A-Z]$`) or rejected by KRA | terminal (`buyer_pin_invalid`) |
| Duplicate | vendor says invoice number already exists | success if the ack can be recovered, otherwise terminal (`duplicate_unresolved`) |
| Unknown | anything not matched above | retryable, but logged at `ERROR` so the classifier can be extended |

The classifier lives in `internal/fiscal/errors.go`. When the vendor introduces a new error code, add it there with a test, do not special-case it in the worker.

## Triage: an invoice in NEEDS_REVIEW

1. Find it. In `psql` (remember RLS, ADR-0007):

   ```sql
   BEGIN; SET LOCAL app.org_id = '<org uuid>';
   SELECT id, state, attempt, last_error, sale_id, payment_id, created_at
     FROM invoices WHERE state = 'NEEDS_REVIEW' ORDER BY created_at;
   SELECT attempt, adapter, error, response
     FROM fiscal_submissions WHERE invoice_id = '<invoice id>' ORDER BY attempt;
   COMMIT;
   ```

   Or use the admin UI at `/ops` -> "Fiscal dead letters", which shows the same data with the request/response side by side.

2. Read `last_error` and the last `response`:
   - `validation_error` on an item: open the sale's items, fix `etims_class_code` / `tax_category` / `unit` on the item in the catalogue (the fix applies to future invoices too), then retry.
   - `buyer_pin_invalid`: clear or correct the buyer PIN on the customer record; if the buyer insists on the PIN, ask them to confirm it on iTax. Retry without PIN issues a valid invoice to an unregistered buyer.
   - `config_error`: check `FISCAL_VENDOR_BASE_URL`, `FISCAL_VENDOR_API_KEY`, the org's device registration (`RegisterDevice` output in `orgs.fiscal_device_ref`). Fix, then re-queue all affected invoices (below).
   - `max_attempts_exceeded`: the vendor was down for more than an hour; nothing is wrong with the invoice. Re-queue.
   - `duplicate_unresolved`: the vendor has an invoice we do not have an ack for. Look it up in the vendor portal by our client reference (the invoice ID), paste the KRA invoice number, signature and QR into the invoice through the admin "Attach ack" action, which moves it to `ACKED` and writes a `fiscal_submissions` row with `adapter = 'manual'`.

3. Retry. Merchant or accountant: **Attention** -> "Try again", which calls `POST /v1/invoices/{id}/retry`. Admin: same endpoint or the "Re-queue" button in `/ops`. The endpoint resets `attempts`, moves the invoice to `QUEUED` and enqueues `SubmitInvoice`. It refuses invoices not in `NEEDS_REVIEW` or `FAILED_TERMINAL` with `409 invalid_state`.

4. Confirm `ACKED` and that the second receipt SMS appears in `notifications` (locally in the sms-sink UI).

## Reproducing failures locally

```bash
# .env
MOCK_FAIL_MODE=retryable    # every submission fails with a retryable error; watch backoff in worker logs
MOCK_FAIL_MODE=terminal     # every submission fails validation -> NEEDS_REVIEW immediately
MOCK_FAIL_MODE=none         # back to normal
docker compose up -d --force-recreate worker
make replay-webhook FILE=tools/webhooks/c2b_confirmation.json    # change TransID first
```

Switch back to `none`, then `POST /v1/invoices/{id}/retry` to see the invoice recover. `internal/fiscal/state_test.go` and `internal/fiscal/mock/mock_test.go` encode the same scenarios.

## Vendor or KRA down for hours

Symptoms: `FAILED_RETRYABLE` count rising across all orgs, `fiscal_submissions.error` all in the network/5xx class, `/healthz` still `ok` (the api does not depend on the vendor).

1. Confirm it is not us: `curl -sS "$FISCAL_VENDOR_BASE_URL/health"` from the worker host, check the vendor status page, check KRA's eTIMS notices.
2. Do nothing for the first hour. Backoff is doing the work; buyers already hold a "pending KRA" receipt and the `/r/<code>` page shows "Awaiting KRA confirmation".
3. If the outage passes an hour, invoices start landing in `NEEDS_REVIEW` with `max_attempts_exceeded`. Do not let merchants retry one by one. When the vendor is back, bulk re-queue from the admin CLI (`ciftctl requeue` is a Phase 2 deliverable listed in `plan.md`; until it exists, use the `/ops` "Re-queue all" action or call `POST /v1/invoices/{id}/retry` in a loop with a delay):

   ```bash
   cd backend && go run ./cmd/ciftctl requeue --reason max_attempts_exceeded --since "2026-09-02T08:00:00Z" --rate 5/s
   ```

   The command iterates orgs (RLS-scoped per org), resets attempts and enqueues `SubmitInvoice` at the given rate so the vendor is not hit with the whole backlog at once. Output is one line per invoice; keep it for the incident record.
4. Watch `SELECT state, count(*) FROM invoices GROUP BY state` (per org, or via the `/ops` counters) until `NEEDS_REVIEW` with that reason is empty.
5. Post an incident note in the merchant-facing status area and log the outage in the compliance register (`docs/compliance.md`, "Incident log"): KRA expects fiscalisation within the timelines set by the integrator agreement, and the trail in `fiscal_submissions` is the evidence that CiftPay kept trying.

## Common error classes and actions

| Symptom in `last_error` / `response` | Likely cause | Action |
|---|---|---|
| `context deadline exceeded` | vendor slow, `FISCAL_TIMEOUT_SECONDS` too low | retryable; if persistent raise timeout to 30 s |
| `429` / `TOO_MANY_REQUESTS` | burst after backlog re-queue | lower `--rate`; retryable |
| `401 invalid api key` | key rotated on vendor side | update `FISCAL_VENDOR_API_KEY`, redeploy worker, bulk re-queue with `--reason config_error` |
| `device not registered` / `bhfId unknown` | org onboarding did not finish `RegisterDevice` | run registration from org settings, retry |
| `invalid item classification code` | wrong `etims_class_code` on item | fix item in catalogue, retry |
| `tax type mismatch` | item tax category vs code disagree (e.g. category `A` with a 16 % code) | fix item, retry |
| `buyer PIN not found` | typo or unregistered buyer | correct or clear PIN, retry |
| `invoice number already exists` | first attempt succeeded, response lost | adapter recovers ack automatically; if not, "Attach ack" manually |
| `max_attempts_exceeded` | long outage | bulk re-queue |
| `credit note exceeds original` | reversal amount larger than acked invoice | check `payments` reversal amount vs invoice total; if Daraja partial reversal, issue partial credit note through admin |

## Escalation

- Vendor: support channel and SLA are recorded in `docs/compliance.md` under "Vendor list".
- KRA: eTIMS support line and the integrator's KRA contact are in the same section; CiftPay does not contact KRA directly for individual invoices while on the vendor adapter.
- Anything that required an "Attach ack" or a manual credit note gets an `audit_log` row automatically; reference its id in the incident note.
