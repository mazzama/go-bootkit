package databasekit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type PostgresDB struct {
	name      string
	connStr   string
	pool      *pgxpool.Pool
	mu        sync.RWMutex
	readyChan chan struct{}
}

type PostgresOption func(*PostgresDB)

func NewPostgresDB(ctx context.Context, options ...PostgresOption) (*PostgresDB, error) {
	db := &PostgresDB{
		name:      "postgres-db",
		connStr:   "postgres://postgres:postgres@localhost:5432/postgres",
		readyChan: make(chan struct{}),
	}

	for _, option := range options {
		option(db)
	}

	config, errParse := pgxpool.ParseConfig(db.connStr)
	if errParse != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", errParse)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if errPing := pool.Ping(ctx); errPing != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", errPing)
	}

	db.pool = pool
	return db, nil
}

func WithDBName(name string) PostgresOption {
	return func(db *PostgresDB) {
		db.name = name
	}
}

func WithConnectionString(connStr string) PostgresOption {
	return func(db *PostgresDB) {
		db.connStr = connStr
	}
}

func (db *PostgresDB) Name() string {
	return db.name
}

func (db *PostgresDB) Start(ctx context.Context) error {
	close(db.readyChan)
	<-ctx.Done()
	return nil
}

func (db *PostgresDB) Stop(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.pool != nil {
		db.pool.Close()
	}
	return nil
}

func (db *PostgresDB) Ready() <-chan struct{} {
	return db.readyChan
}

func (db *PostgresDB) Pool() *pgxpool.Pool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.pool
}

func (db *PostgresDB) Exec(ctx context.Context, sql string, args ...interface{}) (int64, error) {
	result, err := db.Pool().Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (db *PostgresDB) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.Pool().Query(ctx, sql, args...)
}

func (db *PostgresDB) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.Pool().QueryRow(ctx, sql, args...)
}

func (db *PostgresDB) Begin(ctx context.Context) (pgx.Tx, error) {
	return db.Pool().Begin(ctx)
}

func (db *PostgresDB) HealthChecks() []healthkit.Check {
	return []healthkit.Check{
		{
			Name:    db.name + "-liveness",
			Kind:    healthkit.Liveness,
			Timeout: 2 * time.Second,
			Fn: func(ctx context.Context) error {
				pool := db.Pool()
				if pool == nil {
					return fmt.Errorf("db is not initialized")
				}
				return pool.Ping(ctx)
			},
		},
		{
			Name:    db.name + "-readiness",
			Kind:    healthkit.Readiness,
			Timeout: 2 * time.Second,
			Fn: func(ctx context.Context) error {
				if db.Pool() == nil {
					return fmt.Errorf("db is not initialized")
				}
				select {
				case <-db.Ready():
					return nil
				case <-ctx.Done():
					return ctx.Err()
				default:
					return fmt.Errorf("db is not ready")
				}
			},
		},
	}
}

var _ core.Component = (*PostgresDB)(nil)
var _ core.Readyable = (*PostgresDB)(nil)
