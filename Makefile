# CiftPay developer entrypoints. See docs/runbooks/local-dev.md.

SHELL := /bin/bash
COMPOSE ?= docker compose
BACKEND := backend
WEB := web
FILE ?= tools/webhooks/c2b_confirmation.json

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

.PHONY: test-backend
test-backend: ## go test ./...
	cd $(BACKEND) && go test ./...

.PHONY: test-web
test-web: ## vitest run
	cd $(WEB) && npm run test

.PHONY: e2e
e2e: ## Playwright smoke tests (expects web on :3000)
	cd $(WEB) && npx playwright test

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
