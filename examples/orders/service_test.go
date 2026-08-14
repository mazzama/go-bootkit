package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/cachekit/memcache"
	"github.com/mazzama/go-bootkit/databasekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockProductRepo struct {
	mock.Mock
}

func (m *mockProductRepo) Create(ctx context.Context, name string, price, stock int64) (Product, error) {
	args := m.Called(ctx, name, price, stock)
	return args.Get(0).(Product), args.Error(1)
}

func (m *mockProductRepo) GetByID(ctx context.Context, id int64) (Product, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(Product), args.Error(1)
}

func (m *mockProductRepo) GetForUpdate(ctx context.Context, id int64) (Product, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(Product), args.Error(1)
}

func (m *mockProductRepo) DecrementStock(ctx context.Context, id, quantity int64) error {
	return m.Called(ctx, id, quantity).Error(0)
}

type mockOrderRepo struct {
	mock.Mock
}

func (m *mockOrderRepo) Create(ctx context.Context, productID, quantity, total int64) (Order, error) {
	args := m.Called(ctx, productID, quantity, total)
	return args.Get(0).(Order), args.Error(1)
}

func (m *mockOrderRepo) GetByID(ctx context.Context, id int64) (Order, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(Order), args.Error(1)
}

type dummyTxProvider struct{}

func (d dummyTxProvider) Begin(ctx context.Context) (databasekit.Tx, error) {
	return &dummyTx{}, nil
}
func (d dummyTxProvider) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return 0, nil
}
func (d dummyTxProvider) Query(ctx context.Context, sql string, args ...any) (databasekit.Rows, error) {
	return nil, nil
}
func (d dummyTxProvider) QueryRow(ctx context.Context, sql string, args ...any) databasekit.Row {
	return nil
}

type dummyTx struct{}

func (d *dummyTx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return 0, nil
}
func (d *dummyTx) Query(ctx context.Context, sql string, args ...any) (databasekit.Rows, error) {
	return nil, nil
}
func (d *dummyTx) QueryRow(ctx context.Context, sql string, args ...any) databasekit.Row {
	return nil
}
func (d *dummyTx) Begin(ctx context.Context) (databasekit.Tx, error) { return d, nil }
func (d *dummyTx) Commit(ctx context.Context) error                  { return nil }
func (d *dummyTx) Rollback(ctx context.Context) error                { return nil }

func TestOrderService_CreateProduct(t *testing.T) {
	ctx := context.Background()
	products := new(mockProductRepo)
	orders := new(mockOrderRepo)
	cache := memcache.New()
	logger := slog.Default()
	txManager := databasekit.NewTxManager(dummyTxProvider{})

	svc := NewOrderService(txManager, products, orders, cache, logger)

	expected := Product{ID: 1, Name: "Test", Price: 100, Stock: 10}
	products.On("Create", ctx, "Test", int64(100), int64(10)).Return(expected, nil)

	p, err := svc.CreateProduct(ctx, "Test", 100, 10)
	assert.NoError(t, err)
	assert.Equal(t, expected, p)
}

func TestOrderService_GetProduct_CacheMiss(t *testing.T) {
	ctx := context.Background()
	products := new(mockProductRepo)
	cache := memcache.New()
	txManager := databasekit.NewTxManager(dummyTxProvider{})

	svc := NewOrderService(txManager, products, nil, cache, nil)

	expected := Product{ID: 1, Name: "Test"}
	products.On("GetByID", ctx, int64(1)).Return(expected, nil)

	p, err := svc.GetProduct(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, expected, p)

	// Verify it was cached
	exists, _ := cache.Exists(ctx, "product:1")
	assert.True(t, exists)
}

func TestOrderService_CreateOrder(t *testing.T) {
	ctx := context.Background()
	products := new(mockProductRepo)
	orders := new(mockOrderRepo)
	cache := memcache.New()
	txManager := databasekit.NewTxManager(dummyTxProvider{})

	svc := NewOrderService(txManager, products, orders, cache, nil)

	// Pre-seed cache to ensure it gets invalidated
	require.NoError(t, cache.Set(ctx, "product:1", Product{ID: 1}, time.Minute))

	p := Product{ID: 1, Price: 100, Stock: 5}
	products.On("GetForUpdate", mock.Anything, int64(1)).Return(p, nil)
	products.On("DecrementStock", mock.Anything, int64(1), int64(2)).Return(nil)

	expectedOrder := Order{ID: 1, ProductID: 1, Quantity: 2, Total: 200}
	orders.On("Create", mock.Anything, int64(1), int64(2), int64(200)).Return(expectedOrder, nil)

	o, err := svc.CreateOrder(ctx, 1, 2)
	assert.NoError(t, err)
	assert.Equal(t, expectedOrder, o)

	// Ensure cache is invalidated
	exists, _ := cache.Exists(ctx, "product:1")
	assert.False(t, exists)
}

func TestOrderService_CreateOrder_InsufficientStock(t *testing.T) {
	ctx := context.Background()
	products := new(mockProductRepo)
	cache := memcache.New()
	txManager := databasekit.NewTxManager(dummyTxProvider{})

	svc := NewOrderService(txManager, products, nil, cache, nil)

	p := Product{ID: 1, Price: 100, Stock: 1} // Only 1 in stock
	products.On("GetForUpdate", mock.Anything, int64(1)).Return(p, nil)

	_, err := svc.CreateOrder(ctx, 1, 2) // Attempting to order 2
	assert.ErrorIs(t, err, ErrInsufficientStock)
}

func TestOrderService_GetOrder(t *testing.T) {
	ctx := context.Background()
	orders := new(mockOrderRepo)
	txManager := databasekit.NewTxManager(dummyTxProvider{})

	svc := NewOrderService(txManager, nil, orders, nil, nil)

	expected := Order{ID: 1, ProductID: 1}
	orders.On("GetByID", ctx, int64(1)).Return(expected, nil)

	o, err := svc.GetOrder(ctx, 1)
	assert.NoError(t, err)
	assert.Equal(t, expected, o)
}
