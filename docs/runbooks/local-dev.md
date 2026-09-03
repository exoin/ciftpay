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
| postgres | `localhost:5432` | user `ciftpay`, password `ciftpay`, database `ciftpay` |
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

## Environment variables

All variables are documented in `.env.example`. The ones you will actually touch locally:

| Variable | Local default | Purpose |
|---|---|---|
| `FISCAL_ADAPTER` | `mock` | `mock`, `vendor` or `oscu`; keep `mock` unless you have integrator sandbox keys |
| `MOCK_FAIL_MODE` | `none` | `none`, `retryable`, `terminal`; used to exercise the failure paths (see fiscal-failures.md) |
| `DARAJA_WEBHOOK_TOKEN` | `dev-webhook-token` | path token on `/webhooks/mpesa/*/{token}`; `ciftctl replay-webhook` reads it |
| `SESSION_SECRET`, `HASH_PEPPER` | dev values | change for anything that is not a laptop |
| `NEXT_PUBLIC_API_BASE_URL` | `http://localhost:8080` | what the browser calls |

Never put real Daraja, integrator or Africa's Talking credentials in `.env.example`. `.env` is git-ignored.

## Common problems

**`bind: address already in use` on 5432/8080/3000/8025.** Another Postgres or dev server is running. Either stop it or change the host port mapping in `docker-compose.yml` (left side of `"5432:5432"`); container-internal ports must not change.

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
| 2026-09-03 | `POST /sales` requires `etims_class_code` per line when `item_id` is omitted (the mock adapter, like KRA, rejects lines without a classification code, which sent the invoice straight to `NEEDS_REVIEW`). | Validation added; `api/openapi.yaml` `SaleLineInput` should list `etims_class_code` as optional-with-item / required-without-item in Step 6. |
| 2026-09-03 | `notifications` gets an ingest-scope `UPDATE` policy and `stk_requests` an ingest-scope `SELECT` policy (Africa's Talking delivery reports and Daraja STK callbacks carry no org). `sales` gets a receipt-scope `SELECT` policy so the `/r/{code}` join works. | Added to `0001_init.sql` (Phase 0 migrations may still be edited); reflected in `docs/data-model.md` at Step 6. |
