package databasekit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type PostgresDB struct {
	core.Lifecycle

	pool    *pgxpool.Pool
	name    string
	connStr string
	logger  *slog.Logger
}

type PostgresOption func(*PostgresDB)

func NewPostgresDB(options ...PostgresOption) *PostgresDB {
	db := &PostgresDB{
		name:    "postgres-db",
		connStr: "postgres://postgres:postgres@localhost:5432/postgres",
	}

	for _, option := range options {
		option(db)
	}

	db.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(), error) {
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

		return func() {
			db.pool.Close()
		}, nil
	})

	return db
}

func WithDBName(name string) PostgresOption {
	return func(db *PostgresDB) {
		db.name = name
	}
}

func WithLogger(logger *slog.Logger) PostgresOption {
	return func(db *PostgresDB) {
		db.logger = logger
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

func (db *PostgresDB) Pool() *pgxpool.Pool {
	return db.pool
}

func (db *PostgresDB) HealthChecks() []healthkit.Check {
	return healthkit.StandardChecks(db.name, func(ctx context.Context) error {
		pool := db.Pool()
		if pool == nil {
			return fmt.Errorf("db is not initialized")
		}
		return pool.Ping(ctx)
	})
}

var _ core.Component = (*PostgresDB)(nil)
var _ core.Readyable = (*PostgresDB)(nil)
