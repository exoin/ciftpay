# Runbook: local development

How to bring up the whole CiftPay stack on one machine, run the checks, and work on the backend or the web app outside the containers.

## Prerequisites

- Docker Engine 24+ with the `docker compose` plugin
- GNU `make`
- Optional, only for running code on the host instead of in containers:
  - Go 1.23+
  - Node 22+ (npm 10+)
  - `sqlc` (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`) for `make gen-sqlc`
  - `golangci-lint` for `make lint-backend` (falls back to `go vet` if missing)

## First start

```bash
git clone <repo> ciftpay && cd ciftpay
cp .env.example .env        # review; every variable is commented, defaults work for local
make up                     # builds and starts postgres, api, worker, web, sms-sink
make migrate                # applies backend/db/migrations with ciftctl
make seed                   # demo org, shortcode 600123, default item, one user
make gen                    # regenerates sqlc code and the TypeScript API client (should be a no-op)
```

`make up` prints the URLs when it finishes. First build takes a few minutes (Go module download, `npm ci`).

## What is running

| Service | URL / address | Notes |
|---|---|---|
| api | http://localhost:8080 | `GET /healthz` returns `{"status":"ok","db":"ok","queue":"ok"}` |
| web | http://localhost:3000 | Next.js PWA, opens on **Today** |
| sms-sink | http://localhost:8025 | fake Africa's Talking endpoint; browse captured SMS/WhatsApp messages |
| postgres | `localhost:${PG_PORT:-5432}` | user `ciftpay`, password `ciftpay`, database `ciftpay` |
| worker | no port | River workers: `SubmitInvoice`, `SendReceipt`, `ReconcilePayments` |

Useful shortcuts:

```bash
make logs                   # tail all services
make psql                   # psql inside the postgres container
make down                   # stop, keep data
make nuke                   # stop and delete the postgres volume
```

## Daily loop

```bash
make test                   # go test ./... + vitest
make lint                   # go vet/golangci-lint + eslint + tsc
make replay-webhook FILE=tools/webhooks/c2b_confirmation.json   # see webhook-replay.md
make e2e                    # Playwright smoke; expects web on :3000
```

## Running on the host instead of in containers

Keep Postgres and the sms-sink in Docker; run the Go binaries and Next.js locally for fast reloads.

```bash
docker compose up -d postgres sms-sink

# terminal 1: api
cd backend && set -a && source ../.env && set +a
export DATABASE_URL=postgres://ciftpay:ciftpay@localhost:5432/ciftpay?sslmode=disable
export AT_BASE_URL=http://localhost:8025
go run ./cmd/api

# terminal 2: worker (same env)
go run ./cmd/worker

# terminal 3: web
cd web && npm ci && NEXT_PUBLIC_API_BASE_URL=http://localhost:8080 npm run dev
```

Stop the containerised `api`/`worker`/`web` first (`docker compose stop api worker web`) or the host processes will fail to bind 8080/3000.

`DATABASE_URL` inside compose points at the `postgres` hostname; on the host it must be `localhost`. `AT_BASE_URL` follows the same rule.

## Shortcode verification loop (own-Till KES 1)

A merchant proves control of a Till/Paybill by paying **KES 1 from their own phone to that number**; the C2B confirmation settles the challenge opened by `POST /shortcodes/{id}/verify` (plan.md §4.1). Nothing is charged by CiftPay and no `payments` row is created for the KES 1. Three ways to run the loop:

| Mode | Daraja | When |
|---|---|---|
| **unconfigured** | none (`DARAJA_CONSUMER_KEY` empty) | `POST …/verify` returns `200 verified` immediately; what `make up` gives you |
| **fake** | `ciftctl daraja-fake` on `:18090` | tests and offline dev; `simulate` posts the confirmation straight to the URL you registered |
| **sandbox** | `https://sandbox.safaricom.co.ke` through a tunnel | one manual run per change to the C2B path |

### Against the fake

```bash
make daraja-fake                                   # terminal 1: fake Daraja on :18090

# terminal 2: api on the host, pointed at the fake (any key/secret works)
cd backend && set -a && source ../.env && set +a
export DARAJA_BASE_URL=http://localhost:18090 DARAJA_CONSUMER_KEY=fake DARAJA_CONSUMER_SECRET=fake
go run ./cmd/api

# terminal 3: drive it
make register-urls SHORTCODE=600123                # RegisterURL -> WEBHOOK_BASE_URL/webhooks/daraja/c2b/...
#   sign in, create the org and the shortcode, open the challenge (see curl below)
make simulate-c2b SHORTCODE=600123 MSISDN=0140994513
#   GET /shortcodes/{id} -> verified: true, verification.status: "verified"
```

`make sandbox-verify SHORTCODE=… MSISDN=…` runs `register-urls` then `simulate-c2b` in one go.

### Against the real sandbox

1. Put the app's `DARAJA_CONSUMER_KEY` / `DARAJA_CONSUMER_SECRET` in `.env` (`DARAJA_ENV=sandbox`, `DARAJA_BASE_URL=https://sandbox.safaricom.co.ke`). The sandbox only knows its **test shortcodes** (`600000`, `600980`, `174379`…); use one of them as the merchant's Till.
2. Expose the api: `cloudflared tunnel --url http://localhost:8080` and copy the `https://<name>.trycloudflare.com` it prints into `WEBHOOK_BASE_URL` in `.env`, then restart the api (host or `docker compose up -d api`).
3. `make register-urls SHORTCODE=600000` — the sandbox answers `{"ResponseDescription":"Success"}`. RegisterURL only sticks for a while in the sandbox; re-run it if a later simulate produces no webhook.
4. Sign in and open the challenge (`APP_ENV=local` logs the OTP code in the api output):

   ```bash
   API=http://localhost:8080
   curl -s $API/auth/otp/request -H 'content-type: application/json' -d '{"msisdn":"0140994513"}'
   curl -s -c cj $API/auth/otp/verify -H 'content-type: application/json' -d '{"msisdn":"0140994513","code":"<from api log>"}'
   # -> {"csrf_token":"…","orgs":[]}; keep the csrf token
   H=(-b cj -H "X-CSRF-Token: $CSRF" -H 'content-type: application/json')
   curl -s "${H[@]}" $API/orgs -d '{"name":"Sandbox Duka","kra_pin":"P051234568Y","vat_registered":false}'
   # -> {"id":"<org>", "kra_pin_verified_at": …}; add -H "X-Org-Id: <org>" to H
   curl -s "${H[@]}" $API/shortcodes -d '{"kind":"till","shortcode":"600000","label":"Sandbox Till"}'
   curl -s "${H[@]}" $API/shortcodes/<id>/verify -d '{}'
   # -> 202 {"status":"pending","msisdn_masked":"2541•••••513","pay":{"kind":"till","shortcode":"600000","amount_cents":100,"account_ref":"CIFTPAY"},"expires_at":…}
   ```

5. Stand in for the merchant's phone: `make simulate-c2b SHORTCODE=600000 MSISDN=254708374149` (see the MSISDN finding below). The sandbox calls back the tunnel within a few seconds; the api log shows `shortcode verified by own payment` and `c2b ingested … rule=verification`.
6. `curl -s "${H[@]}" $API/shortcodes/<id>` → `verified_at` set and `verification.status: "verified"`. Check `payments` has **no** row for the KES 1 and `audit_log` has `shortcode.verified`.

Sandbox findings (2026-09-08 run): `simulate` **accepts** any Kenyan MSISDN with `ResponseCode 0`, but the confirmation callback only ever arrived for Safaricom's test MSISDN `254708374149` (within 3 s); for `254140994513` nothing came back in 5 min. So pass the test number both in `-d '{"msisdn":"254708374149"}'` on `/verify` (a new challenge replaces the pending one, which becomes `expired`) and in `MSISDN=` on `simulate-c2b`; the match is on hash, so both sides must agree. Daraja also rejects any callback URL containing the word `mpesa` (`400.003.02 Invalid ValidationURL - URL has the word MPESA`), which is why the webhook paths are `/webhooks/daraja/...`. In production there is no simulate endpoint: the merchant really sends KES 1, and RegisterURL for a Till that is not under CiftPay's Daraja app needs Safaricom's go-live/partner arrangement (plan.md §9.2), which is why registration is best-effort and surfaced as `c2b_urls_registered_at`.

## Environment variables

All variables are documented in `.env.example`. The ones you will actually touch locally:

| Variable | Local default | Purpose |
|---|---|---|
| `FISCAL_ADAPTER` | `mock` | `mock`, `vendor` or `oscu`; keep `mock` unless you have integrator sandbox keys |
| `MOCK_FAIL_MODE` | `none` | `none`, `retryable`, `terminal`; used to exercise the failure paths (see fiscal-failures.md) |
| `DARAJA_WEBHOOK_TOKEN` | `dev-webhook-token` | path token on `/webhooks/daraja/*/{token}`; `ciftctl replay-webhook` reads it |
| `SESSION_SECRET`, `HASH_PEPPER` | dev values | change for anything that is not a laptop |
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:8080` | what the browser calls (inlined at build time) |
| `PUBLIC_BASE_URL` | `http://localhost:3000` | web origin used in SMS receipt links (`/r/<code>`) |
| `WEBHOOK_BASE_URL` | `http://localhost:8080` | api origin handed to Daraja (RegisterURL, STK callbacks); a tunnel URL for sandbox runs |
| `DARAJA_BASE_URL` | `https://sandbox.safaricom.co.ke` | set to `http://localhost:18090` to use `make daraja-fake` |
| `PG_PORT` | `5432` | host port for the compose Postgres; change together with `DATABASE_URL` |

Never put real Daraja, integrator or Africa's Talking credentials in `.env.example`. `.env` is git-ignored.

## Common problems

**`bind: address already in use` on 5432/8080/3000/8025.** Another Postgres or dev server is running. For Postgres set `PG_PORT=55432` (or any free port) in `.env` and change the port in `DATABASE_URL` to match; `docker-compose.yml` reads `PG_PORT` and the `Makefile` exports `.env` to the host-run tools (`make migrate`, `make seed`, `make replay-webhook`). For the other ports edit the left side of the mapping in `docker-compose.yml`; container-internal ports must not change.

**`make migrate` fails with "missing migration" or "out of order".** goose requires migrations applied in filename order. If you pulled a branch that added `0003_*.sql` after you already applied a local `0003_*.sql` with a different name, either `make nuke && make up && make migrate` or rename your local migration to the next free number. Never edit an applied migration; add a new one.

**`make gen` produces a diff in CI.** Someone changed `backend/db/queries/*.sql` or `api/openapi.yaml` without regenerating. Run `make gen` locally and commit the generated files (`backend/internal/platform/db/gen/`, `web/src/lib/api/schema.d.ts`).

**`Cannot connect to the Docker daemon`.** Start Docker Desktop / `sudo systemctl start docker`, and make sure your user is in the `docker` group.

**`/healthz` reports `"queue":"error"`.** Migrations have not been applied (River's tables are created by `0001_init.sql`). Run `make migrate` and restart the worker.

**RLS returns zero rows in a query you know has data.** The transaction has no `SET LOCAL app.org_id`. In `psql` run `BEGIN; SET LOCAL app.org_id = '<org uuid>'; SELECT ...; COMMIT;`. In Go, use `db.WithOrg`. See ADR-0007.

**Postgres volume from an older schema.** After large schema changes in Phase 0 the simplest reset is `make nuke && make up && make migrate && make seed`.

**Web build fails on fonts.** Fonts are self-hosted under `web/public/fonts`; a shallow clone with LFS disabled may leave them missing. Re-fetch or run `git lfs pull`.

**Postgres container exits with `00-init.sql: Permission denied`.** `tools/postgres/init.sql` is bind-mounted into the container and read by the `postgres` user (uid 70). If your umask created the file or its parent directories without world-read (`-rw-rw----`, `drwxrwx---`), the entrypoint cannot open it and the container dies before creating the `ciftpay` role. Fix with `chmod o+rx tools tools/postgres && chmod o+r tools/postgres/init.sql`, then `make nuke && make up`. The same applies to `tools/webhooks/*.json` if you mount them.

**RLS lets you see every org's rows.** You are connected as a superuser or a role with `BYPASSRLS`; Postgres skips policies for them even with `FORCE ROW LEVEL SECURITY`. The compose stack avoids this by creating `ciftpay` as `NOSUPERUSER` in `tools/postgres/init.sql` and never connecting as `postgres`. Check with `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user;` — both must be `f`. Ad-hoc containers started with `POSTGRES_USER=ciftpay` make that user a superuser and are **not** a valid way to test tenancy.

## Deviations log

| Date | Deviation | Resolution |
|---|---|---|
| 2026-09-03 | Phase-0 backend verified against a throwaway `postgres:16-alpine` container (port 55432) plus the host-built binaries, not `make up`, because a host Postgres already owned 5432. Same `init.sql`, same non-superuser role. | `make up` end-to-end is re-run in Step 6 once `web/` exists. |
| 2026-09-03 | `tools/postgres/init.sql` was unreadable by the container (`0660`), which silently produced a superuser `ciftpay` and an RLS-bypassing local stack on first attempt. | File modes fixed in-repo; documented above. |
| 2026-09-03 | `POST /sales` requires `etims_class_code` per line when `item_id` is omitted (the mock adapter, like KRA, rejects lines without a classification code, which sent the invoice straight to `NEEDS_REVIEW`). | **Resolved 2026-09-07:** `SaleLineInput` in `api/openapi.yaml` now documents `etims_class_code`, `tax_category`, `description`, `unit_price_cents` as required-without-`item_id`. |
| 2026-09-03 | `notifications` gets an ingest-scope `UPDATE` policy and `stk_requests` an ingest-scope `SELECT` policy (Africa's Talking delivery reports and Daraja STK callbacks carry no org). `sales` gets a receipt-scope `SELECT` policy so the `/r/{code}` join works. | **Resolved 2026-09-07:** documented in `docs/data-model.md` §3 (scoped policies table). |
| 2026-09-07 | Quantities: the Go API serialises `qty` as a decimal **string** (`numeric(12,3)` → `"1.000"`) while `api/openapi.yaml` said `number`. The generated TS client hid it and `/r/<code>` crashed with `toFixed is not a function` on the first real receipt. | Contract fixed (`Quantity` string schema, client regenerated); `formatQty` accepts strings; regression test in `format.test.ts`; e2e stub now returns strings. |
| 2026-09-07 | The api's CORS preflight did not allow `X-Org-Id`, so every tenant request from the browser was blocked and the PWA sat on its loading rows. Not caught by Playwright because the shell tests stub the API same-origin. | `httpx.CORS` allows `X-Org-Id`; unit test `httpx_test.go` pins the preflight. |
| 2026-09-07 | Replay produced **two** SMS (`receipt_pending` and `receipt_acked` in the same second) where the gate expects one; the receipt link pointed at the api (`:8080`) instead of the web app. | `receipt_pending` is enqueued with `ScheduledAt = +5 min` and skipped if the invoice is already `ACKED`/terminal (`jobs.PendingReceiptDelay`); `PUBLIC_BASE_URL` defaults to the web origin (`:3000`). |
| 2026-09-07 | `/billing/entitlement` is served by the api but not in `api/openapi.yaml`; the web read wrong field names (`plan`/`used`/`limit` vs `plan_name`/`invoices_acked`/`invoice_cap`) and showed `—`. | Web fixed to the real shape. **Open:** add `Entitlement` to the contract in Phase 1 (§5.6 billing) and drop `rawGet`. |
| 2026-09-07 | OTP SMS said "expires in 10 minutes"; `OTPTTL` is 5 minutes. | Template fixed (EN/SW). |
| 2026-09-07 | `make e2e` ran Playwright against whatever `.next` build was on disk; shell tests only pass when the build has `NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:18080` inlined (it is a build-time constant). | `make e2e` now builds with the stub URL first. The compose `web` image is still built for `:8080`; do not point Playwright at it. |
| 2026-09-07 | Phase-0 gate (G0) run on the full compose stack (postgres on `PG_PORT=55432`, api, worker, web, sms-sink). One replayed C2B for `254140994513` → 1 `payments`, 1 cash `sales`, invoice `ACKED` (mock), 1 SMS in sink, `/r/<code>` 200 / 15.2 KB / 0 scripts from the real API. | G0 ticked in `plan.md` §3.5. |
| 2026-09-08 | Daraja RegisterURL rejected our callbacks: `400.003.02 Invalid ValidationURL - URL has the word MPESA`. The Phase-0 paths were `/webhooks/mpesa/...`. | Webhook paths renamed to `/webhooks/daraja/{c2b/validation,c2b/confirmation,stk}/{token}` in `mpesa.Webhooks.Mount`, `mpesa.Client`, `ciftctl replay-webhook`, `api/openapi.yaml`, docs. Tests updated. |
| 2026-09-08 | Sandbox `simulate` returned `ResponseCode 0` for the dev phone `254140994513` but no confirmation was ever delivered; with the Safaricom test MSISDN `254708374149` the callback hit the tunnel in 3 s and the challenge settled (`rule=verification`, 0 `payments`, `shortcode.verified` audit). | Runbook documents the test-MSISDN fallback (override `{msisdn}` on `/verify`). Real phones are only exercised in production, where the merchant pays for real. |
| 2026-09-08 | §4.1 live sandbox check run on host binaries + compose postgres (`PG_PORT=55432`) through a `cloudflared` quick tunnel; shortcode `600000`, org `Sandbox Duka`. `GET /shortcodes/{id}` → `verified: true`, `verification.status: verified`, `c2b_urls_registered_at` set; `shortcode_verifications` shows the first challenge `expired` and the second `verified` with `trans_id`. | Procedure recorded above; `make sandbox-verify` wraps `register-urls` + `simulate-c2b`. |
