# CiftPay — API Guide

The contract lives in [`../api/openapi.yaml`](../api/openapi.yaml) (OpenAPI 3.1). It is the **single source of truth** ([ADR-0006](adr/0006-openapi-first.md)): change the YAML first, then the Go handler, then run `make gen` to refresh the TypeScript client in `web/src/lib/api/schema.d.ts`.

## 1. Conventions

| Topic | Rule |
|---|---|
| Base path | `/v1` |
| Money | Integer cents, `*_cents` suffix, `int64`. Never floats. |
| Time | RFC 3339 UTC in JSON. Daraja's `TransTime` (EAT) is converted on ingest. |
| IDs | UUIDs. Public receipts use a 6–8 char Crockford base32 `receipt_code` instead. |
| Naming | `snake_case` JSON fields; Daraja payloads keep Safaricom's `PascalCase` verbatim. |
| Lists | `{ "data": [...], "next_cursor": "..." | null }`; `limit` default 50, max 200; cursors are opaque. |
| Personal data | MSISDN and KRA PIN are **masked** in every response (`2547•••••345`, `A•••••••••B`) except the seller PIN on receipts, which is public by law. |
| Idempotency | Webhooks by `TransID` / `CheckoutRequestID`; `POST /sales` by `client_ref`; provider submissions by invoice ID. |
| Versioning | Additive changes only within `/v1`; breaking changes → `/v2`. |

## 2. Authentication & tenancy

1. `POST /auth/otp/request` → SMS with a 6-digit code (5-minute TTL, 5 requests/hour/MSISDN).
2. `POST /auth/otp/verify` → sets `ciftpay_session` (HttpOnly, `SameSite=Lax`, `Secure` outside dev, 30-day sliding) and returns `csrf_token` + the caller's org memberships.
3. Every non-GET request must send `X-CSRF-Token: <csrf_token>`.
4. The **active org** is the caller's `is_default` membership unless `X-Org-Id: <uuid>` is sent; the header must match one of the caller's memberships or the API answers `403 forbidden`.
5. The server opens a transaction and runs `SET LOCAL app.org_id = $1` so Postgres RLS scopes every query ([ADR-0007](adr/0007-rls-multitenancy.md)).

Roles: `owner` (everything), `staff` (no settings/plan), `accountant` (read + exports across client orgs), `admin` (CiftPay staff; `/ops`).

## 3. Error envelope

```json
{
  "error": {
    "code": "shortcode_claimed",
    "message": "This till is already on CiftPay under another business.",
    "details": { "shortcode": "512345" },
    "request_id": "01J6…"
  }
}
```

| HTTP | `code` | When |
|---|---|---|
| 400 | `bad_request` | Malformed JSON, unknown query param |
| 401 | `unauthenticated` | No/expired session, bad OTP |
| 403 | `forbidden` | Role or org mismatch, CSRF failure |
| 404 | `not_found` | Resource not in the active org (RLS makes foreign rows invisible, so also 404) |
| 409 | `conflict`, `shortcode_claimed`, `already_converted`, `illegal_state` | Uniqueness or state-machine violations; `shortcode_claimed` = another org already holds that number **verified** (on `POST /shortcodes` and `PATCH /admin/shortcodes/{id}/verify`); `conflict` on `POST /shortcodes/{id}/authorization` = already verified |
| 422 | `validation_failed`, `pin_unknown` | Field-level errors in `details.fields`; `pin_unknown` = the fiscal provider's PIN lookup does not know the KRA PIN (`POST /orgs`) |
| 429 | `rate_limited` | With `Retry-After` |
| 500 | `internal` | Never leaks internals; `request_id` for support |

`message` is always safe to show to a user and localised by `Accept-Language` (`en`, `sw`).

## 4. Endpoint map

| Area | Endpoints | Notes |
|---|---|---|
| System | `GET /healthz` | `{status, db, queue, version}`; 503 when degraded |
| Auth | `POST /auth/otp/request`, `POST /auth/otp/verify`, `POST /auth/logout` | |
| Orgs | `GET /orgs`, `POST /orgs`, `GET /orgs/current` | `POST` runs the PIN lookup through `fiscal.PINLookup` (mock: `P000000000Z` is unknown; vendor: `GET {base}/taxpayers/{pin}`); provider unavailable → 201 with `kra_pin_verified_at: null` |
| Shortcodes | `GET/POST /shortcodes`, `GET/PATCH /shortcodes/{id}`, `POST /shortcodes/{id}/authorization` | Created as `status: pending_authorization`; the merchant uploads the signed Safaricom authorization letter, see §4.1 |
| Admin | `GET /admin/shortcodes?status=`, `GET /admin/shortcodes/{id}/authorization`, `PATCH /admin/shortcodes/{id}/verify`, `PATCH /admin/shortcodes/{id}/reject`, `GET /admin/orgs`, `GET /admin/webhooks/dead`, `GET /admin/flags` | `admin` role only; the operator queue of the Administrative Gate |
| Payments | `GET /payments?status=`, `GET /payments/{id}`, `POST /payments/{id}/convert` | Convert turns `unmatched` → sale + queued invoice |
| Items | `GET/POST /items`, `GET /items/codes?q=` | Codes proxy `fiscal.Provider.LookupItemCodes` |
| Sales | `GET/POST /sales` | `kind=open` (request-to-pay, Phase 2) or `cash` |
| Invoices | `GET /invoices?state=`, `GET /invoices/{id}`, `POST /invoices/{id}/retry`, `POST /invoices/{id}/resend` | Retry only from `NEEDS_REVIEW` |
| Attention | `GET /attention` | Grouped, with `actionable_count` for the nav badge |
| Reports | `GET /reports/today`, `GET /reports/vat?period=YYYY-MM` | Africa/Nairobi boundaries |
| Public | `GET /r/{code}` | No auth; edge-cacheable 60 s; the Next.js page renders from it |
| Webhooks | see §5 | No session; token/signature based |

### 4.1 Shortcode authorization — the Administrative Gate ([ADR-0008](adr/0008-administrative-gate.md))

CiftPay never proves Till ownership by a payment. `Shortcode.status` is `pending_authorization | verified | rejected` (`verified` is the boolean shorthand); the transitions are:

| Step | Who | Call | Effect |
|---|---|---|---|
| 1 | merchant | `POST /shortcodes` | `201`, `status: pending_authorization`. `409 shortcode_claimed` only if another org already holds the number **verified**; pending duplicates are allowed. |
| 2 | merchant | `POST /shortcodes/{id}/authorization` (`multipart/form-data`, field `letter`, JPEG/PNG/WebP/PDF ≤ 10 MB, sniffed by content) | `200 Shortcode` with `authorization_letter_uploaded: true`, `authorization_submitted_at`; a `rejected` row returns to `pending_authorization`; `409 conflict` when already verified; `422 validation` for type/size/missing file. Audit `shortcode.authorization_submitted`. |
| 3 | ops | `GET /admin/shortcodes?status=pending_authorization`, `GET /admin/shortcodes/{id}/authorization` | Queue with `org_name`, unmasked `org_kra_pin`; the letter streams with its content type. Ops forward the letter to Safaricom. |
| 4a | ops, after Safaricom confirms the mapping | `PATCH /admin/shortcodes/{id}/verify {note?}` | `status: verified`, `verified_at`, `reviewed_by`; audit `shortcode.verified {mode: admin}`; then Daraja `RegisterURL` (best effort, `c2b_urls_registered_at`). `409 shortcode_claimed` if another org got there first. Same path from the shell: `ciftctl verify-shortcode <id|number>`. |
| 4b | ops | `PATCH /admin/shortcodes/{id}/reject {reason}` | `status: rejected`, `rejection_reason` shown to the merchant; `409 conflict` on a verified row (revoke is not an API operation). |

**Ingest gate.** `POST /webhooks/daraja/c2b/confirmation/{token}` resolves the `BusinessShortCode` **only among `verified` rows**. Anything else is stored in `webhook_events`, marked processed with `error = no verified shortcode <n>` and acknowledged with 200 (Daraja must not retry) — no payment, sale or invoice. `RegisterURL` uses `WEBHOOK_BASE_URL` as the callback origin (the api's public URL, a tunnel in sandbox runs); `PUBLIC_BASE_URL` is the web app's. Runbook: `docs/runbooks/local-dev.md` §"Administrative Gate".

## 5. Webhooks (inbound)

### Daraja
| Path | Purpose | Response |
|---|---|---|
| `POST /webhooks/daraja/c2b/validation/{token}` | Pre-payment validation. CiftPay **always accepts** (`ResultCode 0`) — rejecting would block a customer's payment, which is not our role. Payload is stored as `webhook_events.kind='c2b_validation'`. | `{"ResultCode":0,"ResultDesc":"Accepted"}` |
| `POST /webhooks/daraja/c2b/confirmation/{token}` | Money moved. Stored with `external_id = mpesa:<TransID>`; duplicates return 200 without side effects; the shortcode must be `verified` (§4.1) or the event is parked with `no verified shortcode`; otherwise the matcher runs and `SubmitInvoice` is enqueued in the same transaction. | same |
| `POST /webhooks/daraja/stk/{token}` | STK result. `external_id = mpesa:stk:<CheckoutRequestID>`. Request-to-pay (Phase 2); `STKPush` itself returns `ErrSTKNotConfigured` until then (no passkey). Legacy `verify`-purpose callbacks are logged and ignored. | same |

The path segment is `daraja`, not `mpesa`: Safaricom rejects any callback URL containing the word "MPESA" (`400.003.02`).

Security: `{token}` must equal `DARAJA_WEBHOOK_TOKEN`; requests from outside the Safaricom IP allow-list are rejected with 404 when `DARAJA_IP_ALLOWLIST` is set (empty in dev). Bodies over 64 KB are rejected. Handlers must respond within 5 s; no external calls happen inside them.

Daraja C2B fields (verbatim): `TransactionType`, `TransID`, `TransTime` (`YYYYMMDDHHmmss` EAT), `TransAmount` (string decimal), `BusinessShortCode`, `BillRefNumber`, `InvoiceNumber`, `OrgAccountBalance`, `ThirdPartyTransID`, `MSISDN`, `FirstName`, `MiddleName`, `LastName`. Sample payloads: [`../tools/webhooks/`](../tools/webhooks/).

### Africa's Talking
`POST /webhooks/at/delivery` (form-encoded `id`, `status`, `phoneNumber`, `networkCode`, `failureReason`) with `X-AT-Signature` shared secret → updates `notifications.status`.

## 6. Public receipt contract

`GET /r/{code}` returns `PublicReceipt`: seller name + PIN, KRA invoice number, QR payload, lines, VAT by category, totals, `state` ∈ `verified | pending | cancelled`. The Next.js route `web/src/app/r/[code]/page.tsx` server-renders it with no client JS. Responses carry `Cache-Control: public, max-age=60` while `pending`, `max-age=86400` once `verified`.

## 7. Editing the contract

1. Edit `api/openapi.yaml`. Keep `operationId`s stable — they become TS function names.
2. `make gen` → regenerates `web/src/lib/api/schema.d.ts` (via `openapi-typescript`) and runs `sqlc generate`. CI fails on a dirty diff.
3. Implement or adjust the Go handler in the owning package (`internal/<area>/handler.go`) and its route in `cmd/api/main.go`.
4. Add/extend tests: handler test with `httptest`, plus a contract check that the JSON matches the schema (Phase 1: `kin-openapi` request/response validation middleware in dev mode).
5. Update this document's endpoint map if an area changed.

Lint: `make lint` runs `redocly lint api/openapi.yaml` when the CLI is present (optional in Phase 0).
