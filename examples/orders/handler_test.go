package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mazzama/go-bootkit/cachekit/memcache"
	"github.com/mazzama/go-bootkit/databasekit"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// OrdersSuite drives the example through its real HTTP surface against a real
// Postgres (via testcontainers). There are no mocks: requests flow handler ->
// service -> repository -> Postgres, and assertions inspect actual table state
// ("dbcheck"). Tests are written Given/When/Then for readability.
type OrdersSuite struct {
	suite.Suite

	container *postgres.PostgresContainer
	pool      *pgxpool.Pool
	server    *httptest.Server
	cache     *memcache.MemoryCache
	ctx       context.Context
}

func TestOrdersSuite(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("Docker daemon is not running, skipping integration tests")
	}
	suite.Run(t, new(OrdersSuite))
}

// SetupSuite starts Postgres once, applies the goose migrations, and wires the
// full application stack over an httptest server. This mirrors main.run(), minus
// the framework runner (the test owns the pool lifecycle directly).
func (s *OrdersSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := postgres.Run(s.ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("orders"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").WithStartupTimeout(60*time.Second),
		),
	)
	s.Require().NoError(err, "start postgres container")
	s.container = container

	connStr, err := container.ConnectionString(s.ctx, "sslmode=disable")
	s.Require().NoError(err, "resolve connection string")

	s.runMigrations(connStr)

	pool, err := pgxpool.New(s.ctx, connStr)
	s.Require().NoError(err, "open pgx pool")
	s.pool = pool

	// Wire the domain exactly as main does, over a provider backed by the pool.
	logger := slog.New(slog.NewTextHandler(&discardWriter{}, nil))
	txManager := databasekit.NewTxManager(pool)
	s.cache = memcache.New()
	service := NewOrderService(
		txManager,
		NewProductRepository(txManager),
		NewOrderRepository(txManager),
		s.cache,
		logger,
	)
	handler := NewHandler(service, logger)

	router := chiRouterForTest(handler)
	s.server = httptest.NewServer(router)
}

func (s *OrdersSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.pool != nil {
		s.pool.Close()
	}
	if s.container != nil {
		_ = s.container.Terminate(s.ctx)
	}
}

// SetupTest resets state so each test starts from a clean slate.
func (s *OrdersSuite) SetupTest() {
	_, err := s.pool.Exec(s.ctx, "TRUNCATE orders, products RESTART IDENTITY CASCADE")
	s.Require().NoError(err, "truncate tables")
	s.cache.Reset()
}

func (s *OrdersSuite) runMigrations(connStr string) {
	db, err := sql.Open("pgx", connStr)
	s.Require().NoError(err, "open database/sql for goose")
	defer func() { _ = db.Close() }()

	s.Require().NoError(goose.SetDialect("postgres"))
	s.Require().NoError(goose.Up(db, "migrations"), "apply goose migrations")
}

// --- tests ---

func (s *OrdersSuite) TestCreateProduct() {
	// Given a valid product payload
	// When it is posted
	resp := s.doJSON(http.MethodPost, "/products", createProductRequest{
		Name: "Widget", PriceCents: 1500, Stock: 10,
	})
	defer func() { _ = resp.Body.Close() }()

	// Then the response is 201 with the created product
	s.Equal(http.StatusCreated, resp.StatusCode)
	var product Product
	s.decode(resp, &product)
	s.Positive(product.ID)
	s.Equal("Widget", product.Name)

	// And the row exists in Postgres with the given stock (dbcheck)
	name, stock := s.dbProduct(product.ID)
	s.Equal("Widget", name)
	s.Equal(int64(10), stock)
}

func (s *OrdersSuite) TestCreateProductValidation() {
	// Given a payload missing the required name
	// When it is posted
	resp := s.doJSON(http.MethodPost, "/products", createProductRequest{PriceCents: 100, Stock: 1})
	defer func() { _ = resp.Body.Close() }()

	// Then the response is 400 and nothing is written (dbcheck)
	s.Equal(http.StatusBadRequest, resp.StatusCode)
	s.Equal(int64(0), s.dbCount("products"))
}

func (s *OrdersSuite) TestGetProductCachesOnMiss() {
	// Given an existing product
	id := s.seedProduct("Gadget", 500, 5)

	// When it is fetched (cache miss -> Postgres -> populate)
	resp := s.doJSON(http.MethodGet, fmt.Sprintf("/products/%d", id), nil)
	defer func() { _ = resp.Body.Close() }()

	// Then the response is 200 and the cache now holds the product
	s.Equal(http.StatusOK, resp.StatusCode)
	exists, _ := s.cache.Exists(s.ctx, productCacheKey(id))
	s.True(exists, "expected product to be cached after read")
}

func (s *OrdersSuite) TestGetProductNotFound() {
	// Given no product with this id
	// When it is fetched
	resp := s.doJSON(http.MethodGet, "/products/999", nil)
	defer func() { _ = resp.Body.Close() }()

	// Then the response is 404
	s.Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *OrdersSuite) TestCreateOrderDecrementsStockAndInvalidatesCache() {
	// Given a product with stock 10, already cached by a prior read
	id := s.seedProduct("Widget", 1500, 10)
	getResp := s.doJSON(http.MethodGet, fmt.Sprintf("/products/%d", id), nil)
	_ = getResp.Body.Close()
	exists, _ := s.cache.Exists(s.ctx, productCacheKey(id))
	s.Require().True(exists)

	// When an order for 3 units is placed
	resp := s.doJSON(http.MethodPost, "/orders", createOrderRequest{ProductID: id, Quantity: 3})
	defer func() { _ = resp.Body.Close() }()

	// Then the response is 201 with the computed total
	s.Equal(http.StatusCreated, resp.StatusCode)
	var order Order
	s.decode(resp, &order)
	s.Equal(int64(3), order.Quantity)
	s.Equal(int64(4500), order.Total)

	// And stock is decremented to 7 (dbcheck)
	_, stock := s.dbProduct(id)
	s.Equal(int64(7), stock)

	// And the cached product was invalidated after commit
	exists, _ = s.cache.Exists(s.ctx, productCacheKey(id))
	s.False(exists, "expected cache to be invalidated after order")
}

func (s *OrdersSuite) TestCreateOrderInsufficientStockRollsBack() {
	// Given a product with only 2 in stock
	id := s.seedProduct("Widget", 1500, 2)

	// When an order for 5 units is placed
	resp := s.doJSON(http.MethodPost, "/orders", createOrderRequest{ProductID: id, Quantity: 5})
	defer func() { _ = resp.Body.Close() }()

	// Then the response is 409 Conflict
	s.Equal(http.StatusConflict, resp.StatusCode)

	// And the transaction rolled back: stock unchanged, no order written (dbcheck)
	_, stock := s.dbProduct(id)
	s.Equal(int64(2), stock)
	s.Equal(int64(0), s.dbCount("orders"))
}

func (s *OrdersSuite) TestGetOrderReturnsPersistedOrder() {
	// Given a product and a placed order
	id := s.seedProduct("Widget", 1000, 10)
	createResp := s.doJSON(http.MethodPost, "/orders", createOrderRequest{ProductID: id, Quantity: 2})
	var created Order
	s.decode(createResp, &created)
	_ = createResp.Body.Close()

	// When the order is fetched by id
	resp := s.doJSON(http.MethodGet, fmt.Sprintf("/orders/%d", created.ID), nil)
	defer func() { _ = resp.Body.Close() }()

	// Then the response is 200 with the same order
	s.Equal(http.StatusOK, resp.StatusCode)
	var fetched Order
	s.decode(resp, &fetched)
	s.Equal(created.ID, fetched.ID)
	s.Equal(int64(2), fetched.Quantity)
}

// --- helpers ---

// chiRouterForTest mounts the example routes onto a bare chi router, mirroring
// how main wires them onto serverkit's default handler (minus health probes and
// the framework runner, which the suite doesn't need).
func chiRouterForTest(h *Handler) chi.Router {
	r := chi.NewRouter()
	h.Routes(r)
	return r
}

func (s *OrdersSuite) doJSON(method, path string, body interface{}) *http.Response {
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		s.Require().NoError(err)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(s.ctx, method, s.server.URL+path, reader)
	s.Require().NoError(err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.server.Client().Do(req)
	s.Require().NoError(err)
	return resp
}

func (s *OrdersSuite) decode(resp *http.Response, dst interface{}) {
	s.Require().NoError(json.NewDecoder(resp.Body).Decode(dst))
}

func (s *OrdersSuite) seedProduct(name string, price, stock int64) int64 {
	var id int64
	err := s.pool.QueryRow(s.ctx,
		`INSERT INTO products (name, price_cents, stock) VALUES ($1, $2, $3) RETURNING id`,
		name, price, stock,
	).Scan(&id)
	s.Require().NoError(err, "seed product")
	return id
}

func (s *OrdersSuite) dbProduct(id int64) (string, int64) {
	var name string
	var stock int64
	err := s.pool.QueryRow(s.ctx,
		`SELECT name, stock FROM products WHERE id = $1`, id,
	).Scan(&name, &stock)
	s.Require().NoError(err, "read product row")
	return name, stock
}

func (s *OrdersSuite) dbCount(table string) int64 {
	var n int64
	// table is a fixed test constant, never user input.
	err := s.pool.QueryRow(s.ctx, "SELECT count(*) FROM "+table).Scan(&n)
	s.Require().NoError(err, "count rows")
	return n
}

// discardWriter silences logger output during tests.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
