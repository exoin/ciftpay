# Architecture Decision Records

This directory holds the architecture decision records (ADRs) for CiftPay. An ADR captures one significant technical decision, the context in which it was made, and its consequences. ADRs are append-only: when a decision changes, write a new ADR that supersedes the old one and update the old one's status; never rewrite history.

See `plan.md` section 2 ("How to work this plan") for when an ADR is required. In short: any change that alters the architecture, a non-negotiable (N1–N10), a datastore, an external integration boundary, or the regulatory posture needs an ADR before the code lands.

## Index

| ADR | Title | Status |
|---|---|---|
| [0001](0001-modular-monolith.md) | Modular monolith with two binaries (`api`, `worker`) | Accepted |
| [0002](0002-fiscal-port-third-party-first.md) | `fiscal.Provider` port, third-party integrator first, direct OSCU later | Accepted |
| [0003](0003-zero-custody.md) | Zero custody: CiftPay never holds, routes or settles funds | Accepted |
| [0004](0004-river-postgres-queue.md) | Postgres-backed job queue (River) instead of a separate broker | Accepted |
| [0005](0005-sqlc-no-orm.md) | Explicit SQL with sqlc + pgx + goose, no ORM, money as integer cents | Accepted |
| [0006](0006-openapi-first.md) | OpenAPI-first contract in `api/openapi.yaml` | Accepted |
| [0007](0007-rls-multitenancy.md) | Multi-tenancy by `org_id` column and forced Postgres RLS | Accepted |

## Numbering and naming

- Files are named `NNNN-short-kebab-title.md` with a zero-padded four-digit sequence. Take the next free number; never reuse one.
- Statuses: `Proposed`, `Accepted`, `Deprecated`, `Superseded by ADR-NNNN`.
- Reference ADRs from code comments, docs and PR descriptions as `ADR-NNNN`.

## Template

```markdown
# ADR-NNNN: <Title>

## Status

Accepted — YYYY-MM-DD

## Context

What situation, constraint or problem forces a decision? State facts, not opinions.
Link to plan.md sections, docs/ and external references where useful.

## Decision

The decision, stated in the imperative. Include the concrete shape (packages,
interfaces, tables, flags) so a reader can verify the code follows it.

## Consequences

### Positive

- ...

### Negative

- ...

## Alternatives considered

### <Alternative A>

Why it was rejected.

### <Alternative B>

Why it was rejected.
```

## Writing guidance

- Keep an ADR to roughly 40–80 lines. If it needs more, the decision is probably several decisions.
- Record the trade-offs honestly; the "Negative" list is the most useful part for future readers.
- Prefer concrete names (`internal/fiscal`, `app.org_id`, `invoices.state`) over generic descriptions.
- When an ADR is superseded, add `Superseded by ADR-NNNN` to its status and a one-line note; do not delete content.
