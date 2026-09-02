# ADR-0004: Postgres-backed job queue (River) instead of a separate broker

## Status

Accepted — 2026-09-02

## Context

Most of CiftPay's work happens after the HTTP request has returned: submitting an invoice to KRA through the fiscal adapter, retrying with exponential backoff, sending the SMS/WhatsApp receipt, reconciling payments seen in validation but never confirmed, and updating billing counters. These jobs need:

- durable, at-least-once execution with retries and a dead-letter view for `NEEDS_REVIEW` triage;
- scheduling in the future (`2^n · 15 s` backoff, 15-minute reconcile ticks);
- a guarantee that a job exists if, and only if, the ledger write that caused it was committed. A payment row without a fiscal job is a silent compliance failure; a fiscal job without a payment row is a phantom invoice.

ADR-0001 already fixes Postgres 16 as the single stateful dependency. Introducing Redis or NATS purely for queuing would add a second system to run locally, in CI (testcontainers) and on Fly.io, and would reintroduce the dual-write problem between the database and the broker.

## Decision

Use River (`github.com/riverqueue/river`) with its pgx/v5 driver as the only job queue. River stores jobs in Postgres tables in the same database as business data.

- `cmd/worker` runs the River client with workers for fiscal submission, receipt sending, reconciliation and billing.
- `cmd/api` and `cmd/worker` enqueue jobs with `InsertTx` inside the same `pgx.Tx` that writes `webhook_events`, `payments`, `sales` or `invoices`. If the transaction rolls back, the job is never visible; if it commits, the job is guaranteed to exist.
- Retry policy for fiscal submissions is implemented in `internal/fiscal` on top of River's `snooze`/reschedule: retryable failures move the invoice to `FAILED_RETRYABLE`, then back to `QUEUED` with a `2^n · 15 s` delay, up to 8 attempts, after which the invoice goes to `FAILED_TERMINAL → NEEDS_REVIEW`. Terminal classification short-circuits retries.
- Job uniqueness for `SubmitInvoice` is keyed on the invoice ID so that duplicate enqueues collapse, complementing `SubmitInvoice`'s own idempotency.
- Periodic jobs (reconcile every 15 minutes) use River's periodic job support; no external cron.
- River's tables are created and versioned through the same goose migration path as application tables (`db/migrations/`), so `ciftctl migrate` brings up a complete database.
- `GET /healthz` reports `queue` status by checking River's connectivity alongside `db`.

## Consequences

### Positive

- Transactional enqueue eliminates the dual-write problem; N2 (idempotency everywhere money is touched) is enforced by the database rather than by compensating logic.
- One datastore to back up, restore, migrate and secure; RLS and encryption policies apply uniformly (see ADR-0007).
- Local development, integration tests and webhook replay need only Postgres; `docker-compose.yml` has no broker service.
- Job history, retries and dead letters are queryable with plain SQL, which the admin `/ops` view and the `fiscal-failures` runbook rely on.
- Jobs and business rows can be joined in one query when debugging (`invoices` ↔ `fiscal_submissions` ↔ River job).

### Negative

- Queue traffic adds write load and table bloat to the primary database; River's job tables need routine cleanup and autovacuum tuning as volume grows.
- Postgres is not a purpose-built broker: no pub/sub fan-out, no consumer groups, no streams. Features that need those (Phase 3 outbound webhooks at scale) may warrant a dedicated system later, decided by a new ADR.
- Throughput ceiling is lower than Redis- or NATS-based queues; at Phase 4 volumes the queue may need its own Postgres instance, which weakens the single-transaction guarantee for cross-database enqueues.
- The team is coupled to River's release cadence and its migration scheme.

## Alternatives considered

### Redis with BullMQ or asynq

Mature, fast, familiar. Rejected: it adds a second stateful service, forces an outbox pattern or accepts dual-write risk between Postgres and Redis, and Redis persistence guarantees are weaker than the fiscal record requires.

### NATS JetStream

Durable streams with replay. Rejected for the same dual-write reason and because its strengths (fan-out, streaming) are not needed in Phases 0–2. Kept as a candidate for outbound event delivery in a future ADR if the public API's webhooks-out volume justifies it.

### Hand-rolled `SELECT ... FOR UPDATE SKIP LOCKED` queue

Same transactional properties as River with no dependency. Rejected because River already provides leader election, periodic jobs, retries, snoozing, uniqueness and a maintained schema; rebuilding those would cost more than the dependency saves.
