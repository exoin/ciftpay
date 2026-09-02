# ADR-0005: Explicit SQL with sqlc + pgx + goose, no ORM, money as integer cents

## Status

Accepted — 2026-09-02

## Context

CiftPay's core tables record money and fiscal facts: `payments.amount`, `sale_items`, `invoices`, `fiscal_submissions`. Every row may be inspected by a merchant, an accountant or, in a dispute, by KRA. The queries that touch these rows must be readable by a reviewer without knowing a library's query builder, and must be stable across library upgrades.

The database also carries behaviour that an ORM tends to hide or fight: row-level security driven by `SET LOCAL app.org_id` (ADR-0007), partial unique indexes used for idempotency (`webhook_events.external_id`, `payments.trans_id`, `invoices.receipt_code`), `jsonb` columns for raw webhook and vendor payloads, and River's own tables in the same schema (ADR-0004).

The team is small and the executing agent needs a workflow where a change to a query is a change to a `.sql` file plus a regeneration step, with compile-time failures if the Go code disagrees with the schema.

## Decision

Use explicit SQL end to end. No ORM.

- **Migrations**: goose, plain SQL files in `backend/db/migrations/NNNN_name.sql` with `-- +goose Up` / `-- +goose Down` sections. Applied by `ciftctl migrate` (used by `make migrate`) and at container start. River's schema is included in the same migration path.
- **Queries**: one `.sql` file per aggregate in `backend/db/queries/*.sql` (`payments.sql`, `sales.sql`, `invoices.sql`, `orgs.sql`, ...), annotated for sqlc (`-- name: GetInvoiceByReceiptCode :one`). `sqlc generate` (via `make gen`) emits type-safe Go into `backend/internal/platform/db/gen`. Configuration lives in `backend/sqlc.yaml` with the `pgx/v5` engine.
- **Driver**: `pgx/v5` with `pgxpool`. Services receive a `db.Querier` bound to a transaction so that the RLS `SET LOCAL` and the business writes share one `pgx.Tx`.
- **Money**: stored as integer cents in `bigint` columns (`amount_cents`, `total_cents`, `vat_cents`). Never `float`, never `numeric` parsed into `float64`. The Go side uses `int64`; formatting to `KES 2 400.00` happens only in the presentation layer (`web/src/lib/format.ts`, receipt templates).
- **Timestamps**: `timestamptz` everywhere; the Daraja `TransTime` (`YYYYMMDDHHmmss`, Nairobi local) is parsed once in `internal/mpesa` into UTC.
- Ad-hoc or dynamic queries (admin search, report filters) are written by hand with `pgx` and kept in the same package as the sqlc output, never scattered through handlers.

## Consequences

### Positive

- Every query on a money path is a reviewable SQL statement in version control; auditors and KRA can be shown the exact statement that produced a figure.
- Schema drift is caught at `make gen` time: a renamed column fails compilation instead of failing at runtime.
- RLS, `SET LOCAL`, partial indexes, `jsonb` operators and `ON CONFLICT DO NOTHING` idempotency are used directly, without fighting an abstraction.
- Integer cents make sums exact and make equality checks against Daraja's `TransAmount` trivial after one parse.
- No hidden N+1 queries or lazy loading; the executing agent can reason about the query plan from the file.

### Negative

- More boilerplate than an ORM for CRUD-heavy admin screens; each list/filter variant is a named query.
- Two-step workflow: edit `.sql`, run `make gen`, commit generated code. CI must fail if the generated code is stale.
- sqlc's type inference has limits (dynamic `ORDER BY`, optional filters); those queries fall back to hand-written pgx code, which needs its own tests.
- Developers must know SQL and Postgres specifics; there is no portability layer to another database (none is planned).

## Alternatives considered

### GORM

Familiar and fast for CRUD. Rejected: implicit query generation on money tables, awkward support for RLS session variables and partial indexes, and runtime rather than compile-time failures on schema drift.

### ent

Strong typing and a schema-as-code model. Rejected: it owns the migration story and the schema, which conflicts with goose-managed SQL migrations, River's tables and the hand-tuned RLS policies.

### Hand-rolled pgx scanning without sqlc

Maximum control, no generator. Rejected: the scan/struct boilerplate is where copy-paste bugs live, and sqlc gives the same explicit SQL with compile-time checks for a single `go install`.

### `numeric(12,2)` for money

Exact in Postgres, but every driver boundary risks a `float64` conversion and Daraja already delivers whole shillings. Integer cents in `bigint` keep the value exact in Go, SQL and JSON.
