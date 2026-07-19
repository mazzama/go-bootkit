package main

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lazyPoolProvider adapts a PostgresDB to databasekit.TxProvider. The framework
// runner opens the connection pool during startup, so db.Pool() is nil at the
// time we wire the TxManager. This wrapper resolves the pool on each call
// instead of capturing it up front, so the TxManager can be constructed before
// the pool exists.
type lazyPoolProvider struct {
	pool func() *pgxpool.Pool
}

func (p lazyPoolProvider) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return p.pool().Exec(ctx, sql, args...)
}

func (p lazyPoolProvider) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return p.pool().Query(ctx, sql, args...)
}

func (p lazyPoolProvider) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return p.pool().QueryRow(ctx, sql, args...)
}

func (p lazyPoolProvider) Begin(ctx context.Context) (pgx.Tx, error) {
	return p.pool().Begin(ctx)
}
