package databasekit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
	"github.com/mazzama/go-bootkit/core/retry"
)

// PostgresDB is a lifecycle-managed PostgreSQL connection pool component.
type PostgresDB struct {
	core.Lifecycle

	pool          *pgxpool.Pool
	name          string
	connStr       string
	logger        *slog.Logger
	maxConns      int32
	minConns      int32
	retryAttempts int
	retryBackoff  time.Duration
}

// PostgresOption configures a PostgresDB.
type PostgresOption func(*PostgresDB)

// txProviderAdapter wraps a PostgresDB to present only the TxProvider interface.
type txProviderAdapter struct {
	db *PostgresDB
}

var _ core.Component = (*PostgresDB)(nil)

// NewPostgresDB creates a PostgresDB from a connection string and options.
func NewPostgresDB(connStr string, options ...PostgresOption) (*PostgresDB, error) {
	if connStr == "" {
		return nil, errors.New("connection string cannot be empty")
	}

	db := &PostgresDB{
		name:    "postgres-db",
		connStr: connStr,
	}

	for _, option := range options {
		option(db)
	}

	config, errConfig := db.buildPoolConfig()
	if errConfig != nil {
		return nil, errConfig
	}

	db.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		var pool *pgxpool.Pool

		attempts := db.retryAttempts
		if attempts <= 0 {
			attempts = 1
		}

		err := retry.Do(ctx, attempts, db.retryBackoff, func(ctx context.Context) error {
			p, createErr := pgxpool.NewWithConfig(ctx, config)
			if createErr != nil {
				return createErr
			}
			pingErr := p.Ping(ctx)
			if pingErr != nil {
				p.Close()
				return pingErr
			}
			pool = p
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", err)
		}

		db.pool = pool

		return func(_ context.Context) error {
			db.pool.Close()
			return nil
		}, nil
	})

	return db, nil
}

// buildPoolConfig parses the connection string and layers the sizing options on
// top of it. Options only override the connstr-parsed values when set (> 0), so
// pool settings carried in the connection string (e.g. pool_max_conns) are
// preserved unless explicitly overridden.
//
// Pool sizing math: size MaxConns so that, across all running replicas, the
// total does not exhaust Postgres's server-side limit:
//
//	replicas × MaxConns < Postgres max_connections − headroom
//
// Leave headroom for superuser connections, migrations, and admin tooling.
func (db *PostgresDB) buildPoolConfig() (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(db.connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	if db.maxConns > 0 {
		config.MaxConns = db.maxConns
	}
	if db.minConns > 0 {
		config.MinConns = db.minConns
	}

	return config, nil
}

// WithDBName sets the component name reported by PostgresDB.Name.
func WithDBName(name string) PostgresOption {
	return func(db *PostgresDB) {
		db.name = name
	}
}

// WithMaxConns sets the maximum number of connections in the pool. Size this
// against Postgres's max_connections across all replicas (see buildPoolConfig).
func WithMaxConns(n int32) PostgresOption {
	return func(db *PostgresDB) {
		if n > 0 {
			db.maxConns = n
		}
	}
}

// WithMinConns sets the minimum number of idle connections the pool maintains.
func WithMinConns(n int32) PostgresOption {
	return func(db *PostgresDB) {
		if n > 0 {
			db.minConns = n
		}
	}
}

// WithLogger sets the logger used by the database component.
func WithLogger(logger *slog.Logger) PostgresOption {
	return func(db *PostgresDB) {
		db.logger = logger
	}
}

// WithConnectRetry sets the retry policy for establishing the initial
// connection: up to maxAttempts tries with the given backoff between them.
func WithConnectRetry(maxAttempts int, backoff time.Duration) PostgresOption {
	return func(db *PostgresDB) {
		if maxAttempts > 0 && backoff > 0 {
			db.retryAttempts = maxAttempts
			db.retryBackoff = backoff
		}
	}
}

// Name returns the component name of the database.
func (db *PostgresDB) Name() string {
	return db.name
}

// HealthChecks returns the liveness/readiness checks for the database pool.
func (db *PostgresDB) HealthChecks() []healthkit.Check {
	return healthkit.StandardChecks(db.name, func(ctx context.Context) error {
		pool := db.pool
		if pool == nil {
			return fmt.Errorf("db is not initialized")
		}
		return pool.Ping(ctx)
	})
}

func (db *PostgresDB) exec(ctx context.Context, sql string, args ...any) (int64, error) {
	if db.pool == nil {
		return 0, errors.New("database pool is not initialized")
	}
	tag, err := db.pool.Exec(ctx, sql, args...)
	return tag.RowsAffected(), err
}

func (db *PostgresDB) query(ctx context.Context, sql string, args ...any) (Rows, error) {
	if db.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}
	rows, err := db.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgxRowsAdapter{rows: rows}, nil
}

func (db *PostgresDB) queryRow(ctx context.Context, sql string, args ...any) Row {
	if db.pool == nil {
		return errRow{err: errors.New("database pool is not initialized")}
	}
	return pgxRowAdapter{row: db.pool.QueryRow(ctx, sql, args...)}
}

func (db *PostgresDB) begin(ctx context.Context) (Tx, error) {
	if db.pool == nil {
		return nil, errors.New("database pool is not initialized")
	}
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return newPgxTxAdapter(tx), nil
}

func (a txProviderAdapter) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return a.db.exec(ctx, sql, args...)
}

func (a txProviderAdapter) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	return a.db.query(ctx, sql, args...)
}

func (a txProviderAdapter) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return a.db.queryRow(ctx, sql, args...)
}

func (a txProviderAdapter) Begin(ctx context.Context) (Tx, error) {
	return a.db.begin(ctx)
}

func (a txProviderAdapter) Ready() <-chan struct{} {
	return a.db.Ready()
}

// TxProvider returns an adapter that satisfies TxProvider by delegating to the underlying pool.
func (db *PostgresDB) TxProvider() TxProvider {
	return txProviderAdapter{db: db}
}
