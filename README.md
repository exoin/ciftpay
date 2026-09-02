# CiftPay

**Compliant Invoicing & Fund Tracking for M-Pesa businesses in Kenya.**

CiftPay listens to the M-Pesa payments a business already receives on its Till, Paybill or Pochi, turns each one into a KRA **eTIMS**-compliant invoice, sends the fiscal receipt to the buyer's phone, and keeps the books reconciled in real time. It never holds money: Safaricom still settles to the merchant exactly as before. CiftPay is the compliance and intelligence layer on top of the rail.

Why it matters: since 1 January 2024 (Finance Act 2023) a business expense without an eTIMS invoice is non-deductible, so buyers are dropping suppliers who cannot issue one. Most of those suppliers get paid on M-Pesa and have no practical way to fiscalise 300 payments a day by hand.

## Quickstart

Prerequisites: Docker + Docker Compose, `make`. Go 1.23+ and Node 22+ only if you want to run things outside containers.

```sh
cp .env.example .env          # defaults work for local dev
make up                       # postgres, api, worker, web, sms-sink
make migrate                  # apply db/migrations
make replay-webhook FILE=tools/webhooks/c2b_confirmation.json
```

Then:

- API health: <http://localhost:8080/healthz>
- Merchant PWA: <http://localhost:3000>
- Fake SMS sink (receipts land here locally): <http://localhost:8025>
- The replayed payment becomes a cash sale → invoice → `ACKED` by the mock fiscal adapter → receipt SMS in the sink, with a link to `http://localhost:3000/r/<code>`.

Other targets: `make test`, `make lint`, `make gen` (sqlc + OpenAPI TS client), `make seed`, `make down`.

## Repo map

| Path | What |
|---|---|
| [`plan.md`](plan.md) | **Start here.** Executable master plan: phases, gates, checklists. |
| [`docs/`](docs/) | Product, architecture, data model, compliance, design system, UX flows, API, ADRs, runbooks. |
| [`api/openapi.yaml`](api/openapi.yaml) | v1 REST contract; source of truth for Go handlers and the TS client. |
| [`backend/`](backend/) | Go modular monolith: `cmd/api`, `cmd/worker`, `cmd/ciftctl`, `internal/*`, `db/`. |
| [`web/`](web/) | Next.js 15 PWA: merchant app, accountant portal, admin, public receipt page. |
| [`tools/webhooks/`](tools/webhooks/) | Golden Daraja payloads for replay and tests. |
| [`deploy/`](deploy/) | Fly.io configs for `api`, `worker`, `web`. |
| `.github/workflows/` | CI for Go and web. |

## Architecture in one paragraph

Two Go binaries share one module and one Postgres 16 database. `api` terminates HTTP (merchant REST API, public receipt lookups, Daraja and Africa's Talking webhooks) and writes payments and jobs in the same transaction. `worker` drains a Postgres-backed River queue to submit invoices to KRA through a `fiscal.Provider` adapter (mock locally, a KRA-approved integrator in production, direct OSCU later) and to send receipts. The Next.js PWA talks to `api` through a client generated from `api/openapi.yaml`. Details: [`docs/architecture.md`](docs/architecture.md).

## Non-negotiables

Zero custody · idempotent webhooks and submissions · encrypted MSISDN/PIN · audited fiscal actions · English and Swahili · works on a 3G Android Go phone. The full list with enforcement is in [`plan.md §1`](plan.md#1-mission--non-negotiables).

## Licence

See [`LICENSE`](LICENSE).
