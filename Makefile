# CiftPay developer entrypoints. See docs/runbooks/local-dev.md.

SHELL := /bin/bash
COMPOSE ?= docker compose
BACKEND := backend
WEB := web
FILE ?= tools/webhooks/c2b_confirmation.json

# Host-run tools (ciftctl migrate/seed/replay-webhook) read the same .env the
# compose stack uses, so DATABASE_URL, PG_PORT etc. only live in one place.
ifneq (,$(wildcard .env))
include .env
export $(shell sed -n 's/^\([A-Za-z_][A-Za-z0-9_]*\)=.*/\1/p' .env)
endif

.DEFAULT_GOAL := help

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[1m%-18s\033[0m %s\n", $$1, $$2}'

.env:
	cp .env.example .env
	@echo "created .env from .env.example - review it before going further"

.PHONY: up
up: .env ## Build and start postgres, api, worker, web and the SMS sink
	$(COMPOSE) up --build -d
	@echo "api      http://localhost:8080/healthz"
	@echo "web      http://localhost:3000"
	@echo "sms-sink http://localhost:8025"

.PHONY: down
down: ## Stop the stack (keeps the postgres volume)
	$(COMPOSE) down

.PHONY: nuke
nuke: ## Stop the stack and delete the postgres volume
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Tail logs for all services
	$(COMPOSE) logs -f --tail=100

.PHONY: psql
psql: ## Open psql inside the postgres container
	$(COMPOSE) exec postgres psql -U ciftpay -d ciftpay

.PHONY: migrate
migrate: ## Apply database migrations with ciftctl
	cd $(BACKEND) && go run ./cmd/ciftctl migrate

.PHONY: seed
seed: ## Insert the demo org, shortcode, item and user
	cd $(BACKEND) && go run ./cmd/ciftctl seed

.PHONY: replay-webhook
replay-webhook: ## Replay a Daraja payload: make replay-webhook FILE=tools/webhooks/stk_callback.json
	cd $(BACKEND) && go run ./cmd/ciftctl replay-webhook ../$(FILE)

# Daraja sandbox tooling. Defaults target the sandbox test Till and Safaricom's
# test MSISDN (the sandbox only delivers C2B callbacks for its own test
# numbers, see docs/runbooks/local-dev.md); override on the command line:
#   make sandbox-c2b SHORTCODE=600000 MSISDN=254708374149 AMOUNT=250
SHORTCODE ?= 600000
MSISDN ?= 254708374149
AMOUNT ?= 1

.PHONY: daraja-fake
daraja-fake: ## Run the in-process fake Daraja on :18090 (point DARAJA_BASE_URL at it)
	cd $(BACKEND) && go run ./cmd/ciftctl daraja-fake --addr :18090

.PHONY: register-urls
register-urls: ## Daraja RegisterURL for $(SHORTCODE) at WEBHOOK_BASE_URL (sandbox or fake)
	cd $(BACKEND) && go run ./cmd/ciftctl register-urls $(SHORTCODE)

.PHONY: simulate-c2b
simulate-c2b: ## Emit a KES $(AMOUNT) C2B from $(MSISDN) to $(SHORTCODE) (sandbox or fake)
	cd $(BACKEND) && go run ./cmd/ciftctl simulate-c2b --shortcode $(SHORTCODE) --msisdn $(MSISDN) --amount $(AMOUNT) $(if $(REF),--ref $(REF),)

.PHONY: verify-shortcode
verify-shortcode: ## Administrative Gate (ADR-0008): mark SHORTCODE (id or number) verified; REASON=... rejects instead
	cd $(BACKEND) && go run ./cmd/ciftctl verify-shortcode $(SHORTCODE) $(if $(REASON),--reject "$(REASON)",) $(if $(NOTE),--note "$(NOTE)",)

.PHONY: sandbox-c2b
sandbox-c2b: ## Smoke the C2B pipe against Daraja: register URLs for SHORTCODE then simulate a payment to it
	@echo "WEBHOOK_BASE_URL=$(WEBHOOK_BASE_URL) (for the real sandbox this must be a public tunnel, e.g. cloudflared tunnel --url http://localhost:8080)"
	@echo "The payment only reaches a ledger if SHORTCODE is 'verified' in CiftPay: make verify-shortcode SHORTCODE=$(SHORTCODE)"
	$(MAKE) register-urls SHORTCODE=$(SHORTCODE)
	$(MAKE) simulate-c2b SHORTCODE=$(SHORTCODE) MSISDN=$(MSISDN) AMOUNT=$(AMOUNT)

.PHONY: gen
gen: gen-sqlc gen-api ## Regenerate sqlc code and the TypeScript API client

.PHONY: gen-sqlc
gen-sqlc: ## Regenerate sqlc code from db/queries
	@command -v sqlc >/dev/null || (echo "sqlc not installed: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest" && exit 1)
	cd $(BACKEND) && sqlc generate

.PHONY: gen-api
gen-api: ## Regenerate the typed API client from api/openapi.yaml
	cd $(WEB) && npm run gen:api

.PHONY: test
test: test-backend test-web ## Run backend and web unit tests

# DB-backed Go tests create a throwaway database per test through the compose
# postgres superuser; without a reachable server they skip themselves.
TEST_ADMIN_DATABASE_URL ?= postgres://postgres:postgres@localhost:$(or $(PG_PORT),5432)/postgres?sslmode=disable
export TEST_ADMIN_DATABASE_URL

.PHONY: test-backend
test-backend: ## go test ./... (DB tests use TEST_ADMIN_DATABASE_URL, skipped when unreachable)
	cd $(BACKEND) && go test ./...

.PHONY: test-web
test-web: ## vitest run
	cd $(WEB) && npm run test

.PHONY: e2e
e2e: ## Playwright smoke tests against a fresh build wired to the e2e stub API (:18080)
	cd $(WEB) && NEXT_PUBLIC_API_BASE_URL=http://127.0.0.1:18080 API_BASE_URL=http://127.0.0.1:18080 npm run build >/dev/null && npx playwright test

.PHONY: lint
lint: lint-backend lint-web ## Lint backend and web

.PHONY: lint-backend
lint-backend: ## go vet + golangci-lint if installed
	cd $(BACKEND) && go vet ./...
	@command -v golangci-lint >/dev/null && (cd $(BACKEND) && golangci-lint run) || echo "golangci-lint not installed, ran go vet only"

.PHONY: lint-web
lint-web: ## tsc + eslint
	cd $(WEB) && npm run lint && npm run typecheck

.PHONY: fmt
fmt: ## gofmt the backend
	cd $(BACKEND) && gofmt -l -w .
