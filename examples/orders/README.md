# Orders Example

A small orders/inventory service built on **go-bootkit**. It exists to show the
framework's moving parts working together in a realistic flow:

- **`core`** — `ApplicationRunner` orchestrates startup, graceful shutdown, and
  auto-wires health checks; `NewLogger` gives trace-correlated JSON logging.
- **`databasekit`** — `TxManager` runs the order-placement flow in a single
  transaction, propagated through `context.Context` (no explicit `*pgx.Tx`
  threading).
- **`cachekit`** — Redis powers a cache-aside read path for products.
- **`serverkit`** — chi-based HTTP server with health probes, request logging,
  and panic recovery already wired.

The code is deliberately **flat** — every file lives in one `package main`
directory so you can read the whole service top to bottom without hopping across
a package tree.

## What it does

| Endpoint              | Description                                                                 |
|-----------------------|-----------------------------------------------------------------------------|
| `POST /products`      | Create a product with an initial stock level.                               |
| `GET /products/{id}`  | Read a product. **Cache-aside**: Redis first, miss falls back to Postgres.  |
| `POST /orders`        | Place an order. **One transaction**: lock the product row `FOR UPDATE`, check stock, decrement it, insert the order. Insufficient stock rolls the whole thing back and returns `409`. On commit, the product's cache entry is invalidated. |
| `GET /orders/{id}`    | Read a placed order back.                                                   |

The interesting bit is `POST /orders`: it's the one place where `TxManager.WithTx`
wraps two writes (decrement stock + insert order) so they commit or roll back as
a unit, and where cache invalidation is done **after commit** so a rollback can
never leave a stale entry behind.

## Prerequisites

- Go 1.26.5+
- Docker (for Postgres + Redis, and for the integration tests)
- The [`goose`](https://github.com/pressly/goose) CLI for migrations:
  ```sh
  go install github.com/pressly/goose/v3/cmd/goose@latest
  ```

## Running locally

```sh
# 1. Start Postgres + Redis
make up

# 2. Apply the schema (goose is a prerequisite step, not run by the app)
make migrate

# 3. Start the service on :8080
make run
```

Then exercise it:

```sh
# Create a product
curl -s localhost:8080/products \
  -d '{"name":"Widget","price_cents":1500,"stock":10}'
# => {"id":1,"name":"Widget","price_cents":1500,"stock":10,"created_at":"..."}

# Read it (populates the Redis cache)
curl -s localhost:8080/products/1

# Place an order for 3 (decrements stock to 7 in one transaction, invalidates cache)
curl -s localhost:8080/orders \
  -d '{"product_id":1,"quantity":3}'
# => {"id":1,"product_id":1,"quantity":3,"total_cents":4500,"created_at":"..."}

# Over-order to see the transactional rollback + 409
curl -s -i localhost:8080/orders \
  -d '{"product_id":1,"quantity":999}'
# => HTTP/1.1 409 Conflict
#    {"error":{"code":"insufficient_stock","message":"insufficient stock: have 7, want 999"}}
```

Health probes are served by `serverkit`:

```sh
curl -s localhost:8080/health/readiness   # 200 "ok" once Postgres + Redis are reachable
```

## Configuration

All configuration is via environment variables (see `config.go`):

| Variable         | Default            | Required | Description                        |
|------------------|--------------------|----------|------------------------------------|
| `DB_CONN_STR`    | —                  | **yes**  | Postgres connection string.        |
| `REDIS_ADDR`     | `localhost:6379`   | no       | Redis address.                     |
| `REDIS_PASSWORD` | *(empty)*          | no       | Redis password.                    |
| `HTTP_ADDR`      | `:8080`            | no       | HTTP listen address.               |

The service fails fast at startup if `DB_CONN_STR` is unset.

## Observability

`main.go` configures an OpenTelemetry tracer that exports spans to **stdout**
(no collector needed). Combined with `core.NewLogger`'s `TraceHandler`, every
request produces a span and the log lines emitted while handling it carry the
same `trace_id` / `span_id`. Watch the process output while hitting an endpoint
to see the correlation.

## Tests

The suite (`handler_test.go`) is an **integration** test: it drives the service
through its real HTTP surface against a real Postgres spun up via
[testcontainers](https://golang.testcontainers.org/), applies the goose
migrations, and asserts on actual table state after each operation. It needs a
running Docker daemon but **not** `make up` (it manages its own container).

```sh
make test
```

Tests are written Given/When/Then with `testify/suite`. The suite starts one
Postgres container in `SetupSuite`, truncates tables between tests, and uses an
in-memory `Cache` so the whole thing runs against a single container while still
exercising the cache-aside and invalidation paths.
