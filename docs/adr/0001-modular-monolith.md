# ADR-0001: Modular monolith with two binaries (`api`, `worker`)

## Status

Accepted — 2026-09-02

## Context

CiftPay turns every M-Pesa payment into a KRA eTIMS invoice, a ledger row and a buyer receipt. The domain has a handful of clearly separable areas (`mpesa`, `ledger`, `fiscal`, `notify`, `org`, `billing`, `reports`, `publicapi`, `admin`), a strong need for transactional consistency between the payment ledger and the fiscal queue (see N2 in `plan.md`), and a very small team that must ship an MVP to design partners in weeks, not quarters.

Traffic is bursty but modest: a single Daraja callback per payment, one fiscal submission and one SMS per invoice. Nothing in Phases 0–2 needs independent horizontal scaling per domain. What does need isolation is the *runtime shape*: HTTP request handling must stay fast and stateless, while fiscal submissions and notifications are slow, retried and rate-limited by third parties.

## Decision

Build CiftPay as a single Go 1.23 module (`backend/`) organised as a modular monolith:

- Domain packages live under `internal/<domain>` (`mpesa`, `ledger`, `fiscal`, `notify`, `org`, `billing`, `reports`, `publicapi`, `admin`) and shared infrastructure under `internal/platform/*` (`config`, `db`, `httpx`, `jobs`, `log`, `crypto`).
- Packages communicate through Go interfaces and direct function calls, not through a network or a message broker.
- Two production binaries share these packages:
  - `cmd/api` — chi HTTP server: auth, merchant API, public receipt page, Daraja and Africa's Talking webhooks. It writes to Postgres and enqueues River jobs; it never calls KRA or sends SMS inline.
  - `cmd/worker` — River job workers: fiscal submission, receipt sending, reconciliation, billing counters.
- A third dev/ops binary, `cmd/ciftctl`, provides `migrate`, `seed` and `replay-webhook <file>`.
- Postgres 16 is the only stateful dependency; it holds both business data and the job queue (see ADR-0004).

Package boundaries are enforced by Go visibility (`internal/`) and code review. Cross-domain calls go through the domain's service type, not its repository.

## Consequences

### Positive

- One repository, one build, one deploy unit per binary; `make up` runs the entire stack from a clean checkout.
- A ledger write and its follow-up job can be committed in a single Postgres transaction, which is the simplest possible way to satisfy N2 (idempotency everywhere money is touched).
- Refactoring across domains is a compile-time operation, not a coordinated multi-service release.
- Local debugging, integration tests (testcontainers Postgres) and webhook replay involve no infrastructure beyond Postgres.
- `api` and `worker` can still be scaled and restarted independently because they are separate processes.

### Negative

- Domain boundaries are conventions, not hard walls; without discipline `internal/*` packages can grow tangled imports. Mitigation: review checklist and `golangci-lint` `depguard` rules as the codebase grows.
- Any package's memory or CPU regression affects the whole binary it lives in.
- A future need to hand one domain to another team or language (for example a heavy reporting engine) requires carving it out later rather than starting separate.

## Alternatives considered

### Event-driven services over NATS JetStream

Separate `mpesa`, `fiscal` and `notify` services exchanging events through JetStream. Rejected for now: it introduces a second stateful system, at-least-once delivery semantics that must be reconciled with Postgres uniqueness constraints, and operational overhead disproportionate to a two-person team and a few thousand invoices per day. The internal package boundaries chosen here keep this option open if Phase 4 scale demands it.

### Single binary with in-process goroutines

Run HTTP handlers and background workers in one process. Rejected because slow, retried fiscal calls and SMS sends would compete with webhook latency budgets, a crash in a worker would take the webhook endpoint down (and Daraja retries are finite), and the two workloads have different scaling and deploy cadences. The cost of a second binary sharing the same module is negligible.
