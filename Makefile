-include .env
POSTGRES_PORT ?= 55432
TEST_DATABASE_URL ?= postgres://campaign:campaign@localhost:$(POSTGRES_PORT)/campaign?sslmode=disable
HAS_GO := $(shell command -v go 2>/dev/null)

.PHONY: up down test test-go test-web postgres

up:
	docker compose up --build

down:
	docker compose down

test: test-go test-web

postgres:
	docker compose up -d --wait postgres

# Go tests run on the host when Go is installed, otherwise in a golang container
# on the compose network. Either way they hit the compose Postgres.
test-go: postgres
ifdef HAS_GO
	cd backend && TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -count=1 ./...
else
	docker compose --profile test run --build --rm gotest
endif

web/node_modules: web/package-lock.json
	cd web && npm ci
	touch web/node_modules

test-web: web/node_modules
	cd web && npx vitest run
