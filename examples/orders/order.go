package main

import (
	"context"
	"errors"
	"time"
)

// Domain errors. Handlers map these to HTTP status codes in writeError.
var (
	// ErrNotFound is returned when a requested product or order does not exist.
	ErrNotFound = errors.New("not found")
	// ErrInsufficientStock is returned when an order requests more units than
	// are currently in stock. Returning it from within TxManager.WithTx rolls
	// the whole transaction back.
	ErrInsufficientStock = errors.New("insufficient stock")
)

// Product is an item that can be ordered. Stock is decremented as orders are placed.
type Product struct {
	CreatedAt time.Time `json:"created_at"`
	Name      string    `json:"name"`
	ID        int64     `json:"id"`
	Price     int64     `json:"price_cents"`
	Stock     int64     `json:"stock"`
}

// Order is a purchase of a quantity of a product.
type Order struct {
	CreatedAt time.Time `json:"created_at"`
	ID        int64     `json:"id"`
	ProductID int64     `json:"product_id"`
	Quantity  int64     `json:"quantity"`
	Total     int64     `json:"total_cents"`
}

// ProductRepository persists and retrieves products. Implementations read the
// active transaction (if any) from the context via TxManager.QuerierFromContext,
// so callers never thread a transaction explicitly.
type ProductRepository interface {
	Create(ctx context.Context, name string, price, stock int64) (Product, error)
	GetByID(ctx context.Context, id int64) (Product, error)
	// GetForUpdate reads a product row locked FOR UPDATE. It must be called
	// inside a transaction so the lock is held until commit.
	GetForUpdate(ctx context.Context, id int64) (Product, error)
	DecrementStock(ctx context.Context, id, quantity int64) error
}

// OrderRepository persists and retrieves orders.
type OrderRepository interface {
	Create(ctx context.Context, productID, quantity, total int64) (Order, error)
	GetByID(ctx context.Context, id int64) (Order, error)
}
