# ADR-0007: Multi-tenancy by `org_id` column and forced Postgres RLS

## Status

Accepted — 2026-09-02

## Context

Every CiftPay tenant is an organisation (`orgs`): a duka, a transporter, a salon. Its payments, sales, items, customers, invoices and notifications must never be visible to another organisation. At the same time, one accountant user legitimately works across many client organisations (the Accountant plan), and CiftPay staff need a back-office view across all tenants.

The data is sensitive under the Data Protection Act 2019 (MSISDNs, buyer KRA PINs) and under tax law (fiscal invoices). A single forgotten `WHERE org_id = $1` in one of hundreds of queries would be a reportable breach. The team is small and much of the code is written by an agent; the isolation guarantee must therefore live in the database, not in review discipline.

ADR-0001 and ADR-0004 fix a single Postgres 16 database shared by `api` and `worker`, so the mechanism must work for HTTP requests and for background jobs alike.

## Decision

Isolate tenants with an `org_id` column on every tenant table and Postgres row-level security that is enabled and forced.

- **Schema**: every tenant table (`mpesa_shortcodes`, `payments`, `items`, `customers`, `sales`, `sale_items`, `invoices`, `fiscal_submissions`, `notifications`, `subscriptions`, `usage_counters`, `audit_log`) has `org_id uuid NOT NULL REFERENCES orgs(id)` and an index leading with `org_id`. Global tables (`orgs`, `users`, `memberships`, `sessions`, `webhook_events`, River tables) are not under RLS; `webhook_events` is written before the org is known and is only readable by the worker and admin paths.
- **Policies**: in `0001_init.sql`, for each tenant table:

  ```sql
  ALTER TABLE payments ENABLE ROW LEVEL SECURITY;
  ALTER TABLE payments FORCE ROW LEVEL SECURITY;
  CREATE POLICY payments_tenant ON payments
    USING (org_id = current_setting('app.org_id', true)::uuid)
    WITH CHECK (org_id = current_setting('app.org_id', true)::uuid);
  ```

  When `app.org_id` is unset the cast yields `NULL` and the policy matches no rows, so an unscoped query returns nothing rather than everything.
- **Roles**: the application role (`ciftpay` locally, created by `tools/postgres/init.sql`) is `NOSUPERUSER` and does not have `BYPASSRLS`. Because the local role also owns the tables, `FORCE ROW LEVEL SECURITY` is mandatory: without it Postgres would skip policies for the owner. In production the migration owner (`ciftpay_owner`) and the runtime role (`ciftpay_app`) are separate roles, with `FORCE` kept as defence in depth.
- **Per-request scoping**: `internal/platform/db` exposes `WithOrg(ctx, orgID, func(tx) error)` which opens a transaction, runs `SET LOCAL app.org_id = '<uuid>'`, executes the callback and commits. `SET LOCAL` is transaction-scoped, so a pooled connection never leaks a tenant to the next request. Handlers obtain the org from the authenticated membership in the session; the RLS middleware in `internal/platform/httpx` refuses requests whose session has no membership in the requested org.
- **Accountants and multi-org users**: a user has many `memberships(user_id, org_id, role)`. The PWA's `OrgSwitcher` sets the active org; the API scopes each request to exactly one org. Cross-client summaries for accountants are built by iterating memberships, one scoped query per org, never by disabling RLS.
- **Worker jobs**: every River job argument struct carries `OrgID`; the worker wraps its handler in the same `WithOrg`. Jobs that operate on `webhook_events` before an org is resolved run outside `WithOrg` and may only touch non-tenant tables until they have matched a shortcode to an org.
- **Admin back-office**: `internal/admin` searches on global tables (`orgs`, `users`, `webhook_events`) and then drills into one tenant at a time through the same `WithOrg`, writing an `audit_log` row for every read of tenant data. No handler in the `api` binary bypasses RLS; a `BYPASSRLS` role, if ever needed for analytics, requires its own ADR and a separate pool.
- **Tests**: the integration suite asserts that a `payments` query under a different `app.org_id` returns zero rows and that an insert with a mismatched `org_id` is rejected by `WITH CHECK`.

## Consequences

### Positive

- Isolation is enforced by the database for every query, generated or hand-written; a missing `WHERE` clause becomes an empty result, not a leak.
- One schema and one migration path; accountant and admin features do not need per-tenant connection management.
- The approach composes with sqlc (ADR-0005) and River (ADR-0004): scoping is a `SET LOCAL` inside the transaction that already exists.
- DPA and KRA conversations have a simple answer to "how is tenant data separated".

### Negative

- Every tenant query must run inside a transaction with `SET LOCAL`; forgetting `WithOrg` produces confusing empty results rather than an error. The middleware and worker wrapper exist to make this hard to forget, and a debug log line records `app.org_id` per transaction.
- RLS adds a predicate to every query; indexes must lead with `org_id` and query plans need occasional review as volume grows.
- Cross-tenant analytics (Phase 3 cash-flow scoring) need per-org iteration or pre-aggregated global tables, which is deliberate friction.
- Separate owner and runtime roles in production add operational surface: role passwords, grants on new tables in every migration, pool sizing.

## Alternatives considered

### Schema-per-tenant

Strong isolation and simple per-tenant export. Rejected: thousands of small merchants would mean thousands of schemas to migrate, River's queue would need per-schema workers, and accountant cross-org views become expensive.

### Database-per-tenant

Maximum isolation. Rejected for the same operational reasons multiplied, and incompatible with the single-datastore decision in ADR-0004.

### Application-level filtering only

Zero database configuration; each query includes `WHERE org_id = $1`. Rejected: this is exactly the discipline-dependent approach the context rules out. It remains in place as a defence in depth (queries still filter by `org_id` for index use), but it is not the isolation guarantee.
