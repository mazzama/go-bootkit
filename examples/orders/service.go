package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mazzama/go-bootkit/cachekit"
	"github.com/mazzama/go-bootkit/databasekit"
)

// productCacheTTL is the safety-net expiry on cached products. Invalidation on
// write is explicit (delete-after-commit); the TTL only bounds staleness if an
// invalidation is ever missed.
const productCacheTTL = 60 * time.Second

// OrderService holds the business logic for products and orders. It owns the
// transaction boundary (via TxManager.WithTx) and the cache-aside read path.
type OrderService struct {
	txManager *databasekit.TxManager
	products  ProductRepository
	orders    OrderRepository
	cache     cachekit.Cache
	logger    *slog.Logger
}

// NewOrderService wires the service with its dependencies.
func NewOrderService(
	txManager *databasekit.TxManager,
	products ProductRepository,
	orders OrderRepository,
	cache cachekit.Cache,
	logger *slog.Logger,
) *OrderService {
	if logger == nil {
		logger = slog.Default()
	}
	return &OrderService{
		txManager: txManager,
		products:  products,
		orders:    orders,
		cache:     cache,
		logger:    logger,
	}
}

// CreateProduct inserts a new product. Single write, no transaction needed.
func (s *OrderService) CreateProduct(ctx context.Context, name string, price, stock int64) (Product, error) {
	return s.products.Create(ctx, name, price, stock)
}

// GetProduct reads a product using a cache-aside strategy: check the cache, and
// on a miss fall back to Postgres and populate the cache for next time.
func (s *OrderService) GetProduct(ctx context.Context, id int64) (Product, error) {
	key := productCacheKey(id)

	if cached, err := s.cache.Get(ctx, key); err == nil {
		var p Product
		if unmarshalErr := json.Unmarshal([]byte(cached), &p); unmarshalErr == nil {
			s.logger.DebugContext(ctx, "product cache hit", "product_id", id)
			return p, nil
		}
		// A malformed cache entry is treated as a miss.
		s.logger.WarnContext(ctx, "discarding malformed cache entry", "product_id", id)
	}

	p, err := s.products.GetByID(ctx, id)
	if err != nil {
		return Product{}, err
	}

	if encoded, err := json.Marshal(p); err == nil {
		if setErr := s.cache.Set(ctx, key, encoded, productCacheTTL); setErr != nil {
			// Caching is best-effort; a failure to populate must not fail the read.
			s.logger.WarnContext(ctx, "failed to populate product cache", "product_id", id, "error", setErr)
		}
	}

	return p, nil
}

// GetOrder reads an order back from Postgres. Plain read, no cache.
func (s *OrderService) GetOrder(ctx context.Context, id int64) (Order, error) {
	return s.orders.GetByID(ctx, id)
}

// CreateOrder places an order for a product in a single transaction: it locks
// the product row FOR UPDATE, verifies stock, decrements it, and inserts the
// order. Insufficient stock returns ErrInsufficientStock, which rolls the whole
// transaction back — nothing is written. The product cache is invalidated only
// after a successful commit, so a rollback can never leave a stale entry behind.
func (s *OrderService) CreateOrder(ctx context.Context, productID, quantity int64) (Order, error) {
	var order Order

	err := s.txManager.WithTx(ctx, func(ctx context.Context) error {
		product, err := s.products.GetForUpdate(ctx, productID)
		if err != nil {
			return err
		}

		if product.Stock < quantity {
			return fmt.Errorf("%w: have %d, want %d", ErrInsufficientStock, product.Stock, quantity)
		}

		if err := s.products.DecrementStock(ctx, productID, quantity); err != nil {
			return err
		}

		total := product.Price * quantity
		order, err = s.orders.Create(ctx, productID, quantity, total)
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return Order{}, err
	}

	// Commit succeeded: the product's stock changed, so drop its cached copy.
	// Done outside WithTx so a rolled-back transaction never touches the cache.
	if delErr := s.cache.Delete(ctx, productCacheKey(productID)); delErr != nil {
		s.logger.WarnContext(ctx, "failed to invalidate product cache", "product_id", productID, "error", delErr)
	}

	return order, nil
}

func productCacheKey(id int64) string {
	return fmt.Sprintf("product:%d", id)
}
