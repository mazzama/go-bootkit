# Orders example — local workflow.
#
# Typical first run:
#   make up        # start Postgres + Redis
#   make migrate   # apply the schema with the goose CLI
#   make run       # start the service on :8080
#
# Integration tests own their own Postgres via testcontainers and need a running
# Docker daemon, but do NOT need `make up`:
#   make test

DB_CONN_STR ?= postgres://orders:orders@localhost:5432/orders?sslmode=disable
REDIS_ADDR  ?= localhost:6379
HTTP_ADDR   ?= :8080

.PHONY: up down migrate migrate-down run test tidy

## up: start backing services (Postgres + Redis)
up:
	docker compose up -d

## down: stop backing services and remove volumes
down:
	docker compose down -v

## migrate: apply migrations with the goose CLI (prerequisite before `run`)
## Install goose: go install github.com/pressly/goose/v3/cmd/goose@latest
migrate:
	goose -dir migrations postgres "$(DB_CONN_STR)" up

## migrate-down: roll back the most recent migration
migrate-down:
	goose -dir migrations postgres "$(DB_CONN_STR)" down

## run: start the service on the host
run:
	DB_CONN_STR="$(DB_CONN_STR)" REDIS_ADDR="$(REDIS_ADDR)" HTTP_ADDR="$(HTTP_ADDR)" go run .

## test: run the integration suite (requires a running Docker daemon)
test:
	go test ./... -v

## tidy: sync go.mod/go.sum
tidy:
	go mod tidy
