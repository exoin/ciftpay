# CiftPay — Executable Master Plan

> **C.I.F.T. = Compliant Invoicing & Fund Tracking.**
> Every M-Pesa payment a Kenyan business receives becomes a KRA eTIMS invoice, a reconciled ledger line and a fiscal receipt on the buyer's phone — before the merchant has put their phone down.

This file is the single entry point for any human or coding agent working on CiftPay. It tells you **what** to build, **in which order**, **how you know it is done**, and **where the detail lives** (`docs/`, `api/openapi.yaml`). Read section 2 before touching code.

---

## 0. Table of contents

1. [Mission & non-negotiables](#1-mission--non-negotiables)
2. [How to work this plan](#2-how-to-work-this-plan)
3. [Phase 0 — Foundation](#3-phase-0--foundation-this-repository-scaffold)
4. [Phase 1 — MVP (weeks 2–8) → Gate G1](#4-phase-1--mvp-weeks-28)
5. [Phase 2 — Growth (weeks 8–16) → Gate G2](#5-phase-2--growth-weeks-816)
6. [Phase 3 — Moat (months 5–9) → Gate G3](#6-phase-3--moat-months-59)
7. [Phase 4 — Scale](#7-phase-4--scale)
8. [Cross-cutting concerns](#8-cross-cutting-concerns)
9. [Appendix](#9-appendix)

Deeper documents:

| Topic | Document |
|---|---|
| Product strategy, tournament, pricing, personas | [`docs/product.md`](docs/product.md) |
| Architecture, packages, request flows, retry policy | [`docs/architecture.md`](docs/architecture.md) |
| Data model, RLS, encryption, retention | [`docs/data-model.md`](docs/data-model.md) |
| Regulatory posture (NPS Act, eTIMS, DPA, DCP) | [`docs/compliance.md`](docs/compliance.md) |
| Design system ("the honest receipt") | [`docs/design-system.md`](docs/design-system.md) |
| UX flows & screen inventory | [`docs/ux-flows.md`](docs/ux-flows.md) |
| API contract & conventions | [`docs/api.md`](docs/api.md), [`api/openapi.yaml`](api/openapi.yaml) |
| Architecture decision records | [`docs/adr/`](docs/adr/) |
| Runbooks | [`docs/runbooks/`](docs/runbooks/) |

---

## 1. Mission & non-negotiables

### Mission
Keep Kenya's M-Pesa-first micro and small businesses inside formal B2B supply chains by making eTIMS compliance a side effect of getting paid.

### The forcing function
Finance Act 2023: since **1 January 2024**, a business expense not supported by a KRA eTIMS invoice is **non-deductible**. Buyers now refuse suppliers that cannot issue eTIMS invoices. KRA's own tools (eTIMS Lite, USSD `*222#`) are manual, single-invoice and disconnected from M-Pesa — where the money actually lands.

### Non-negotiables (every phase, every PR)

| # | Rule | Why | Enforced by |
|---|---|---|---|
| N1 | **Zero custody.** CiftPay never holds, routes, or settles funds. It only *observes* Daraja callbacks and *fiscalises*. | Keeps CiftPay outside CBK PSP licensing (National Payment System Act 2011). See [ADR-0003](docs/adr/0003-zero-custody.md). | Code review: no B2C/B2B disbursement endpoints without a signed-off ADR. |
| N2 | **Idempotency everywhere money is touched.** `webhook_events.external_id`, `payments.trans_id`, `invoices.receipt_code` are `UNIQUE`; `fiscal.Provider.SubmitInvoice` is idempotent by `invoice.ID`. | Daraja retries; KRA times out; workers crash. | Migrations + tests in `internal/mpesa`, `internal/fiscal`. |
| N3 | **Personal data encrypted at rest.** MSISDN and KRA PIN stored as envelope-encrypted blobs plus a keyed hash for lookup. Never logged in plaintext. | Data Protection Act 2019, ODPC registration. | `internal/platform/crypto`; lint rule on `slog` fields. |
| N4 | **Every fiscal action is audited.** Submit, ack, fail, retry, credit note, resend → `audit_log`. | KRA disputes, merchant support. | `internal/fiscal` service layer. |
| N5 | **EN + SW from day one.** All user-facing strings in `web/messages/{en,sw}.json` and `internal/notify` templates. | Target users. | No hard-coded UI strings (eslint rule). |
| N6 | **Low bandwidth / low-end device.** Receipt page < 30 KB, PWA shell works offline, fonts self-hosted and subset, no third-party scripts. | 3G Android Go phones. | Lighthouse budget in `web-ci.yml`. |
| N7 | **Explicit SQL, no ORM.** `sqlc` + `pgx` + `goose`. | Auditable money code. See [ADR-0005](docs/adr/0005-sqlc-no-orm.md). | `make gen` produces no diff. |
| N8 | **Contract first.** `api/openapi.yaml` changes before Go handlers or TS client. | One truth for web + backend. See [ADR-0006](docs/adr/0006-openapi-first.md). | `make gen` + CI diff check. |
| N9 | **Multi-tenancy by RLS.** Every tenant table carries `org_id`; Postgres RLS policies are on; the API sets `app.org_id` per request. | Accountant portal spans orgs. See [ADR-0007](docs/adr/0007-rls-multitenancy.md). | Integration test: foreign `org_id` → zero rows. |
| N10 | **No secrets in git.** `.env.example` is documentation; real values come from env / Fly secrets. | Obvious. | `gitleaks` in CI (Phase 1). |

---

## 2. How to work this plan

1. **One phase at a time.** Do not start Phase N+1 epics before Phase N's gate checklist is ticked and reviewed.
2. **Epics are checklists.** Each epic below lists acceptance criteria as `- [ ]` items. Tick them in this file in the same PR that delivers them. Never tick something that is not verified by a test, a script or a documented manual check.
3. **Definition of done (any item):**
   - Code merged to `main` with tests (`make test` green) and lint (`make lint` green).
   - `api/openapi.yaml` and generated clients updated if the API changed.
   - Relevant doc under `docs/` updated (architecture, data model, runbook).
   - Feature works via `make up` on a clean checkout.
4. **Where things live:** business rules → `docs/`; decisions → `docs/adr/NNNN-*.md` (append, never rewrite); operational how-tos → `docs/runbooks/`.
5. **When the plan is wrong**, change the plan first (small PR to `plan.md` + an ADR if architectural), then the code.
6. **Agents:** work from the top-most unticked item in the current phase. Do not skip ahead to "interesting" items. Do not invent features not listed here without adding them to the plan first.

---

## 3. Phase 0 — Foundation (this repository scaffold)

**Goal:** a clean checkout runs the whole stack with one command; a replayed Daraja webhook produces an `ACKED` mock invoice and a receipt. Nothing here talks to a real external system.

### 3.1 Documents
- [x] `plan.md` (this file) and `README.md`
- [ ] `docs/product.md`, `docs/architecture.md`, `docs/data-model.md`, `docs/compliance.md`
- [ ] `docs/design-system.md`, `docs/ux-flows.md`
- [ ] `docs/api.md` + `api/openapi.yaml` (v1 surface, see §9.3)
- [ ] `docs/adr/0001`–`0007`
- [ ] `docs/runbooks/{local-dev,webhook-replay,fiscal-failures}.md`

### 3.2 Repo tooling
- [ ] `Makefile`: `up`, `down`, `migrate`, `gen`, `test`, `lint`, `seed`, `replay-webhook`
- [ ] `docker-compose.yml`: `postgres:16`, `api`, `worker`, `web`, `sms-sink`
- [ ] `.env.example`, `.gitignore`, `.editorconfig`, `.golangci.yml`, `LICENSE`
- [ ] `.github/workflows/go-ci.yml`, `.github/workflows/web-ci.yml`
- [ ] `tools/webhooks/{c2b_confirmation,stk_callback,reversal}.json`
- [ ] `deploy/fly.{api,worker,web}.toml`

### 3.3 Backend (`backend/`, Go module)
- [ ] `go.mod` with chi, pgx/v5, river, goose, env, slog, otel, testcontainers-go
- [ ] `internal/platform/{config,db,httpx,jobs,log,crypto}` compile and are unit-tested where logic exists (crypto round-trip, config defaults)
- [ ] `db/migrations/0001_init.sql` — all core tables, unique constraints, RLS policies; `sqlc.yaml` + `db/queries/*.sql`
- [ ] `internal/mpesa` — Daraja client skeleton, C2B validation/confirmation + STK callback handlers, idempotent raw event storage, `testdata/`
- [ ] `internal/ledger` — payment normalisation + matcher with table-driven tests
- [ ] `internal/fiscal` — `Provider` port, state machine + tests, tax categories, `mock` adapter (failure injection), `vendor` stub, `providertest` contract suite
- [ ] `internal/{org,notify,billing,reports,publicapi,admin}` skeletons with working health + `/r/{code}`
- [ ] `cmd/api`, `cmd/worker`, `cmd/ciftctl` build; `go test ./...` and `golangci-lint run` pass
- [ ] `backend/Dockerfile` multi-stage

### 3.4 Web (`web/`, Next.js 15 PWA)
- [ ] Project init: App Router, TS, Tailwind v4, Serwist, TanStack Query, `openapi-typescript`, `next-intl`
- [ ] `src/styles/tokens.css` + `globals.css` implementing [`docs/design-system.md`](docs/design-system.md)
- [ ] `src/components/ui`: `Button`, `Money`, `ReceiptCard`, `DataTable`, `Sheet`, `Tabs`, `EmptyState`, `StatusChip`
- [ ] `src/components/shell`: `BottomNav`, `TopBar`, `OrgSwitcher`
- [ ] Routes: `(auth)/login`, `(merchant)/{today,payments,invoices,items,attention,settings}`, `(accountant)/clients`, `(admin)/ops`, `r/[code]`
- [ ] `public/manifest.webmanifest` + icons; Vitest (`formatKES`); Playwright smoke; `web/Dockerfile`

### 3.5 Phase-0 gate (G0)
- [ ] `make up && make migrate && make gen` on a clean checkout; `GET /healthz` → `200 {"db":"ok","queue":"ok"}`
- [ ] `make replay-webhook FILE=tools/webhooks/c2b_confirmation.json` → 1 `payments` row, 1 cash `sales` row, invoice reaches `ACKED` via mock adapter, 1 notification in `sms-sink`, `/r/<code>` renders
- [ ] `make test` and `make lint` green for backend and web
- [ ] Deviations recorded in `docs/runbooks/local-dev.md`; Phase-1 external accounts listed in §9.2

---

## 4. Phase 1 — MVP (weeks 2–8)

**Goal:** 10 design-partner merchants issue real eTIMS invoices from real Till payments through a KRA-approved integrator, with SMS receipts to buyers.

**Sequencing:** 4.1 → 4.2 → 4.3 → 4.4 → 4.5 → 4.6 (UI can start in parallel with 4.2 once OpenAPI stubs exist).

### 4.1 Onboarding & shortcode verification (`internal/org`, `internal/mpesa`)
- [ ] `POST /auth/otp/request` sends a 6-digit OTP via Africa's Talking; rate-limited 5/hour/MSISDN; codes hashed, 5-min TTL
- [ ] `POST /auth/otp/verify` issues HttpOnly `SameSite=Lax` session cookie; CSRF token returned in body for mutations
- [ ] `POST /orgs` with business name + KRA PIN: regex `^[AP]\d{9}[A-Z]$`, then iTax PIN-checker lookup; store `kra_pin_enc` + `pin_hash`
- [ ] `POST /shortcodes` registers Till / Paybill / Pochi; `POST /shortcodes/{id}/verify` triggers a KES 1 STK push to the merchant's own MSISDN and marks `verified_at` on successful callback; a shortcode already verified by another org is rejected with `409 shortcode_claimed`
- [ ] Daraja C2B `RegisterURL` called on verification with our `validation`/`confirmation` URLs (Phase-1 uses Daraja sandbox; production requires Safaricom Go-Live)
- [ ] Onboarding completes in ≤ 3 minutes on a mid-range Android (measured with 3 design partners)

### 4.2 C2B ingestion & matching (`internal/mpesa`, `internal/ledger`)
- [ ] `POST /webhooks/mpesa/c2b/confirmation/{token}`: verify path token + Daraja IP allow-list; store raw payload in `webhook_events` (`external_id = provider:TransID`); duplicate → `200` and no side effects
- [ ] Normalise to `payments` (amount in cents, `msisdn_hash`, `msisdn_enc`, `bill_ref`, `paid_at`); `trans_id` unique
- [ ] Matcher order: (1) `BillRefNumber` = open `sales.ref` → mark sale `paid`; (2) amount + `msisdn_hash` matches a pending STK request within 10 min; (3) shortcode `auto_invoice=true` → **cash sale** with `default_item_id`; (4) else `payments.status='unmatched'` → Needs attention
- [ ] `GET /payments?status=unmatched` and `POST /payments/{id}/convert` (choose items → sale → invoice)
- [ ] Reconcile job every 15 min: Daraja `TransactionStatus` for payments seen in validation but not confirmation
- [ ] ≥ 95 % of Till payments auto-matched at design partners (measured)

### 4.3 Cash-sale auto-invoice & fiscal port (`internal/fiscal`)
- [ ] `vendor` adapter implemented against the chosen KRA-approved integrator's sandbox (see §9.2); passes `providertest`
- [ ] `RegisterDevice` during org onboarding stores `DeviceRef` on `orgs.fiscal_profile`
- [ ] Worker `SubmitInvoice`: `QUEUED → SUBMITTED → ACKED`; retryable failures back off `2^n · 15 s` up to 8 attempts; terminal → `NEEDS_REVIEW`
- [ ] Tax categories A/B/C/D/E applied from `items.tax_category`; VAT computed in cents with banker's rounding documented in `docs/data-model.md`
- [ ] `POST /invoices/{id}/retry` re-queues from `NEEDS_REVIEW`
- [ ] p95 payment-received → KRA ack < 60 s on sandbox (measured over 100 replays)

### 4.4 Buyer receipt & verify page (`internal/notify`, `internal/publicapi`, `web/r/[code]`)
- [ ] Worker `SendReceipt` after `ACKED`: SMS (EN/SW template) with `https://ciftpay.co.ke/r/<code>`; a "pending KRA" SMS is sent if ack takes > 5 min and upgraded later
- [ ] `GET /r/{code}` public JSON + server-rendered page: merchant name, KRA PIN, invoice no., items, VAT, QR, status badge; < 30 KB, no JS required
- [ ] Africa's Talking delivery receipts → `notifications.status`
- [ ] Footer CTA: "Issue your own eTIMS receipts — CiftPay" with UTM tracking

### 4.5 Merchant PWA (`web/`)
- [ ] **Today**: live total received today, ACKED / pending / attention counts, last 5 payments, primary "Record a sale"
- [ ] **Payments**: feed with `StatusChip` (Matched · Cash sale · Unmatched), tap → convert sheet
- [ ] **Invoices**: list + detail as receipt facsimile with QR, "Resend to buyer"
- [ ] **Attention**: failed submissions, unmatched payments, unverified shortcodes — one fix action each
- [ ] **Items**: catalog with guided KRA item-code picker (`LookupItemCodes`) and tax category
- [ ] **Settings**: shortcodes, business profile, receipt template, language, plan
- [ ] Offline shell: app loads without network; "Record a sale" queues locally and syncs
- [ ] Lighthouse PWA ≥ 90, performance ≥ 80 on simulated 3G

### 4.6 Ops & safety
- [ ] Structured logs with `request_id`, `org_id`, no PII; OpenTelemetry traces to a local collector
- [ ] Admin `/ops`: search org, view dead-letter invoices, re-queue
- [ ] Rate limits: OTP, webhooks, public receipt page
- [ ] `gitleaks` + dependency audit in CI

### Gate G1 (exit Phase 1)
- [ ] 10 design-partner merchants live on production Daraja with verified shortcodes
- [ ] ≥ 500 real eTIMS invoices ACKED
- [ ] ≥ 95 % auto-match rate on Till payments
- [ ] p95 payment → KRA ack < 60 s
- [ ] ODPC registration filed; privacy notice live on receipt page

---

## 5. Phase 2 — Growth (weeks 8–16)

**Goal:** paying merchants, accountants as a channel, full VAT story.

### 5.1 Request-to-pay
- [ ] `POST /sales` creates an open sale with `ref`; STK push link (`/p/<ref>`) and QR; payment matches by `BillRef`
- [ ] STK callback → `payments` → invoice using the sale's items

### 5.2 Buyer PIN capture
- [ ] Buyer can add their KRA PIN on `/r/<code>` within 72 h → credit note + re-issue with PIN (or amendment where integrator supports it)
- [ ] `customers` table keyed by `pin_hash`; PIN encrypted; retention policy 7 years (KRA) documented

### 5.3 Reversals & credit notes
- [ ] Daraja reversal callback → `payments.status='reversed'` → linked `CREDIT_NOTE` invoice through the same state machine
- [ ] Merchant-initiated void on an `ACKED` invoice → credit note

### 5.4 VAT position & return pack (`internal/reports`)
- [ ] `GET /reports/vat?period=YYYY-MM`: output VAT by category, invoice count, credit notes
- [ ] Return pack export: CSV + XLSX + PDF matching KRA VAT3 layout; month boundary in Africa/Nairobi
- [ ] P&L summary

### 5.5 Accountant portal
- [ ] `accountant` role; `memberships` across orgs; `(accountant)/clients` list with per-client health (unmatched, failed, VAT due)
- [ ] Bulk export across clients

### 5.6 Billing & plans (`internal/billing`)
- [ ] Plans Hustler / Duka / Biashara / Accountant; `usage_counters` per month; overage KES 5/invoice
- [ ] Subscription payment via merchant's own M-Pesa STK to **CiftPay's** Paybill (this is CiftPay's revenue, not custody of merchant funds)
- [ ] Grace period and downgrade rules

### 5.7 Channels & language
- [ ] WhatsApp Business (Africa's Talking) receipts for Duka+ plans, SMS fallback
- [ ] Complete Swahili translation; receipt card copy tested for overflow

### Gate G2
- [ ] 300 paying merchants
- [ ] < 2 % failed submissions (terminal)
- [ ] Monthly logo churn < 4 %

---

## 6. Phase 3 — Moat (months 5–9)

### 6.1 Purchases & input VAT
- [ ] Merchant forwards supplier eTIMS receipts (photo/QR/SMS) → `purchases`; input VAT in the return pack

### 6.2 Cash-flow score
- [ ] Deterministic score from reconciled income (regularity, growth, buyer diversity, dispute rate); explainable factors; merchant opt-in only

### 6.3 Financing marketplace
- [ ] DCP-licensed partner integration; CiftPay shares score + consented data, partner lends, CiftPay earns origination fee (2–4 %). **CiftPay never lends, never collects.** See [`docs/compliance.md`](docs/compliance.md)

### 6.4 Direct OSCU adapter
- [ ] `internal/fiscal/oscu` implementing KRA system-to-system spec; passes `providertest`; KRA integrator certification obtained; per-org switch `fiscal_adapter`

### 6.5 Public API & POS
- [ ] API keys per org; webhooks out (`invoice.acked`); first POS partner integration

### Gate G3
- [ ] CAC payback < 4 months
- [ ] KRA integrator certification obtained
- [ ] First financing origination completed

---

## 7. Phase 4 — Scale

- [ ] Multi-outlet orgs (outlet ↔ shortcode mapping, per-outlet reports)
- [ ] Bank / Pesalink statement feeds for non-M-Pesa income
- [ ] EAC fiscalisation adapters: Uganda EFRIS, Tanzania VFD (new `fiscal.Provider` implementations)
- [ ] Micro-employer payroll module (PAYE/NSSF/SHIF/Housing Levy) — the tournament runner-up
- [ ] Landlord / estate Paybill vertical template

---

## 8. Cross-cutting concerns

### 8.1 Security checklist (review every release)
- Phone OTP + session cookie; CSRF on mutations; sessions revocable
- Daraja: path token + IP allow-list; Africa's Talking: shared secret header
- Envelope encryption (AES-256-GCM data keys wrapped by a master key from env/KMS); key rotation runbook
- RLS enabled and forced on all tenant tables; API role has no `BYPASSRLS`
- Audit log immutable (append-only, no `UPDATE`/`DELETE` grants)
- Dependency audit + `gitleaks` in CI

### 8.2 Observability
- `slog` JSON logs → stdout; fields `request_id`, `org_id`, `job_id`, never MSISDN/PIN
- OpenTelemetry traces: HTTP server, pgx, outbound HTTP (Daraja, integrator, AT)
- Metrics: `payments_ingested_total`, `invoices_by_state`, `fiscal_submit_duration_seconds`, `webhook_duplicates_total`
- Alerts (Phase 1): failed submissions > 2 % over 15 min; queue depth > 500; Daraja token refresh failures

### 8.3 Testing strategy
| Layer | Tooling | What |
|---|---|---|
| Unit (Go) | `go test` | matcher, state machine, tax maths, crypto, mock adapter |
| Contract (Go) | `internal/fiscal/providertest` | every `fiscal.Provider` adapter |
| Integration (Go) | testcontainers Postgres | webhook idempotency, RLS isolation, River job flow |
| Unit (web) | Vitest | `formatKES`, receipt maths, i18n keys parity |
| E2E (web) | Playwright | login shell, receipt page, convert-payment flow |
| Replay | `make replay-webhook` | golden Daraja payloads in `tools/webhooks/` |

Key scenarios (encoded from Phase 0 onwards): duplicate `TransID` → one payment; BillRef match → sale paid; auto-invoice fallback → cash sale with default item (tax B); retryable failure → back-off; terminal → `NEEDS_REVIEW`; foreign `org_id` → zero rows.

Edge cases to cover in later phases: reversal after ack; KRA sandbox timeout; invalid buyer PIN format; shortcode claimed by two orgs; month boundary in VAT report; Swahili overflow in receipt card.

### 8.4 Release process
- Trunk-based; PRs need green CI; `main` deploys to staging (Fly.io) automatically; production by tag `vX.Y.Z`
- Migrations run by `ciftctl migrate` as a release command before new binaries start
- Feature flags in `admin` for risky flows (WhatsApp, financing)

### 8.5 KPI dashboard (Phase 1+)
Merchants active (7d), payments ingested, auto-match %, invoices ACKED, terminal failure %, p95 payment→ack, receipts delivered %, CTA clicks on `/r/<code>`, MRR, churn.

---

## 9. Appendix

### 9.1 Environment variables
Full list with comments in [`.env.example`](.env.example). Summary:

| Variable | Used by | Notes |
|---|---|---|
| `DATABASE_URL` | api, worker, ciftctl | Postgres DSN |
| `APP_ENV` | all | `dev` / `staging` / `prod` |
| `HTTP_ADDR` | api | default `:8080` |
| `PUBLIC_BASE_URL` | api, worker | e.g. `https://ciftpay.co.ke` for receipt links |
| `SESSION_SECRET` | api | cookie signing |
| `MASTER_KEY_B64` | api, worker | 32-byte base64 master key for envelope encryption |
| `DARAJA_ENV`, `DARAJA_CONSUMER_KEY`, `DARAJA_CONSUMER_SECRET`, `DARAJA_PASSKEY`, `DARAJA_WEBHOOK_TOKEN` | api, worker | Safaricom Daraja |
| `FISCAL_ADAPTER` | worker | `mock` / `vendor` / `oscu` |
| `FISCAL_VENDOR_BASE_URL`, `FISCAL_VENDOR_API_KEY` | worker | KRA-approved integrator |
| `AT_USERNAME`, `AT_API_KEY`, `AT_SENDER_ID`, `AT_BASE_URL` | worker | Africa's Talking; `AT_BASE_URL` points at `sms-sink` locally |
| `NEXT_PUBLIC_API_BASE_URL` | web | API origin |

### 9.2 External accounts to open (before Phase 1)
1. **Safaricom Daraja** — sandbox app (C2B, STK Push, Transaction Status, Reversal); later Go-Live with the design partners' shortcodes.
2. **KRA-approved eTIMS integrator sandbox** — pick one from KRA's published list of approved third-party integrators; obtain API credentials and their item-code catalogue endpoint.
3. **Africa's Talking** — SMS sender ID, WhatsApp Business (Phase 2), USSD not needed.
4. **ODPC** — register CiftPay as data controller & processor (Data Protection Act 2019).
5. **Fly.io** (or Render) — staging org; Postgres 16.
6. **Domain** `ciftpay.co.ke` (KeNIC) + TLS.
7. **DCP partner** shortlist (Phase 3) — CBK-licensed Digital Credit Providers.

### 9.3 API surface v1 (see `api/openapi.yaml`)
`POST /auth/otp/request` · `POST /auth/otp/verify` · `GET/POST /orgs` · `POST /shortcodes` · `POST /shortcodes/{id}/verify` · `GET /payments` · `POST /payments/{id}/convert` · `GET/POST /items` · `GET/POST /sales` · `GET /invoices` · `POST /invoices/{id}/retry` · `GET /reports/vat` · `GET /r/{code}` (public) · `POST /webhooks/mpesa/c2b/validation/{token}` · `POST /webhooks/mpesa/c2b/confirmation/{token}` · `POST /webhooks/mpesa/stk/{token}` · `POST /webhooks/at/delivery` · `GET /healthz`

### 9.4 Fiscal port (Go)
```go
type Provider interface {
    RegisterDevice(ctx context.Context, org OrgFiscalProfile) (DeviceRef, error)
    SubmitInvoice(ctx context.Context, inv Invoice) (Ack, error)   // idempotent by inv.ID
    SubmitCreditNote(ctx context.Context, cn CreditNote) (Ack, error)
    LookupItemCodes(ctx context.Context, q string) ([]ItemCode, error)
    Health(ctx context.Context) error
}
// Ack{KRAInvoiceNo, Signature, QRPayload, ReceivedAt, Raw json.RawMessage}
```

### 9.5 Invoice state machine
```
DRAFT → QUEUED → SUBMITTED → ACKED
SUBMITTED → FAILED_RETRYABLE → QUEUED        (backoff 2^n·15s, max 8 attempts)
SUBMITTED → FAILED_TERMINAL → NEEDS_REVIEW   (merchant/admin fixes items or PIN → QUEUED)
ACKED + reversal/void → new CREDIT_NOTE invoice linked by parent_invoice_id (same machine)
```

### 9.6 Glossary
| Term | Meaning |
|---|---|
| **eTIMS** | KRA electronic Tax Invoice Management System |
| **OSCU / VSCU** | Online / Virtual Sales Control Unit — KRA integration modes |
| **Daraja** | Safaricom's M-Pesa API platform |
| **C2B** | Customer-to-Business payment (Till/Paybill) |
| **STK push** | SIM-toolkit prompt asking the customer to authorise a payment |
| **Till / Paybill / Pochi** | Buy-goods shortcode / bill-payment shortcode / Pochi la Biashara |
| **TransID** | Daraja's unique transaction identifier (idempotency key) |
| **BillRef** | Account/reference typed by payer on Paybill; CiftPay's sale reference |
| **PIN** | KRA Personal Identification Number (tax ID), e.g. `A123456789B` |
| **Tax category** | A exempt · B 16 % · C 0 % export · D non-VAT · E 8 % |
| **DCP** | CBK-licensed Digital Credit Provider |
| **ODPC** | Office of the Data Protection Commissioner |
| **RLS** | Postgres Row-Level Security |
