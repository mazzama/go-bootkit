package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/mazzama/go-bootkit/databasekit"
)

// pgProductRepository is a Postgres-backed ProductRepository. It resolves its
// Querier from the context on every call, so the same methods work both inside
// and outside a TxManager transaction.
type pgProductRepository struct {
	resolver databasekit.QuerierResolver
}

// NewProductRepository builds a ProductRepository over the given QuerierResolver.
func NewProductRepository(resolver databasekit.QuerierResolver) ProductRepository {
	return &pgProductRepository{resolver: resolver}
}

func (r *pgProductRepository) Create(ctx context.Context, name string, price, stock int64) (Product, error) {
	q := r.resolver.QuerierFromContext(ctx)

	var p Product
	err := q.QueryRow(ctx,
		`INSERT INTO products (name, price_cents, stock)
		 VALUES ($1, $2, $3)
		 RETURNING id, name, price_cents, stock, created_at`,
		name, price, stock,
	).Scan(&p.ID, &p.Name, &p.Price, &p.Stock, &p.CreatedAt)
	if err != nil {
		return Product{}, fmt.Errorf("insert product: %w", err)
	}
	return p, nil
}

func (r *pgProductRepository) GetByID(ctx context.Context, id int64) (Product, error) {
	q := r.resolver.QuerierFromContext(ctx)
	return scanProduct(ctx, q, id, false)
}

func (r *pgProductRepository) GetForUpdate(ctx context.Context, id int64) (Product, error) {
	q := r.resolver.QuerierFromContext(ctx)
	return scanProduct(ctx, q, id, true)
}

func scanProduct(ctx context.Context, q databasekit.Querier, id int64, forUpdate bool) (Product, error) {
	sql := `SELECT id, name, price_cents, stock, created_at FROM products WHERE id = $1`
	if forUpdate {
		sql += " FOR UPDATE"
	}

	var p Product
	err := q.QueryRow(ctx, sql, id).Scan(&p.ID, &p.Name, &p.Price, &p.Stock, &p.CreatedAt)
	if errors.Is(err, databasekit.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("select product: %w", err)
	}
	return p, nil
}

func (r *pgProductRepository) DecrementStock(ctx context.Context, id, quantity int64) error {
	q := r.resolver.QuerierFromContext(ctx)
	_, err := q.Exec(ctx,
		`UPDATE products SET stock = stock - $1 WHERE id = $2`,
		quantity, id,
	)
	if err != nil {
		return fmt.Errorf("decrement stock: %w", err)
	}
	return nil
}

// pgOrderRepository is a Postgres-backed OrderRepository.
type pgOrderRepository struct {
	resolver databasekit.QuerierResolver
}

// NewOrderRepository builds an OrderRepository over the given QuerierResolver.
func NewOrderRepository(resolver databasekit.QuerierResolver) OrderRepository {
	return &pgOrderRepository{resolver: resolver}
}

func (r *pgOrderRepository) Create(ctx context.Context, productID, quantity, total int64) (Order, error) {
	q := r.resolver.QuerierFromContext(ctx)

	var o Order
	err := q.QueryRow(ctx,
		`INSERT INTO orders (product_id, quantity, total_cents)
		 VALUES ($1, $2, $3)
		 RETURNING id, product_id, quantity, total_cents, created_at`,
		productID, quantity, total,
	).Scan(&o.ID, &o.ProductID, &o.Quantity, &o.Total, &o.CreatedAt)
	if err != nil {
		return Order{}, fmt.Errorf("insert order: %w", err)
	}
	return o, nil
}

func (r *pgOrderRepository) GetByID(ctx context.Context, id int64) (Order, error) {
	q := r.resolver.QuerierFromContext(ctx)

	var o Order
	err := q.QueryRow(ctx,
		`SELECT id, product_id, quantity, total_cents, created_at FROM orders WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.ProductID, &o.Quantity, &o.Total, &o.CreatedAt)
	if errors.Is(err, databasekit.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("select order: %w", err)
	}
	return o, nil
}
