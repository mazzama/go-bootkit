package databasekit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/core/healthkit"
)

type PostgresDB struct {
	core.Lifecycle

	pool     *pgxpool.Pool
	name     string
	connStr  string
	logger   *slog.Logger
	maxConns int32
	minConns int32
}

type PostgresOption func(*PostgresDB)

func NewPostgresDB(options ...PostgresOption) (*PostgresDB, error) {
	db := &PostgresDB{
		name:    "postgres-db",
		connStr: "postgres://postgres:postgres@localhost:5432/postgres",
	}

	for _, option := range options {
		option(db)
	}

	config, errConfig := db.buildPoolConfig()
	if errConfig != nil {
		return nil, errConfig
	}

	db.Lifecycle = core.NewLifecycle(func(ctx context.Context) (func(context.Context) error, error) {
		pool, err := pgxpool.NewWithConfig(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("failed to create connection pool: %w", err)
		}

		if errPing := pool.Ping(ctx); errPing != nil {
			return nil, fmt.Errorf("failed to connect to database: %w", errPing)
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

type errRow struct{ err error }

func (e errRow) Scan(dest ...any) error {
	return e.err
}

type lazyProvider struct {
	db *PostgresDB
}

func (p lazyProvider) wait(ctx context.Context) error {
	select {
	case <-p.db.Ready():
		if p.db.Pool() == nil {
			return fmt.Errorf("database pool is not initialized")
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("pool not ready: %w", ctx.Err())
	}
}

func (p lazyProvider) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	if err := p.wait(ctx); err != nil {
		return pgconn.CommandTag{}, err
	}
	return p.db.Pool().Exec(ctx, sql, arguments...)
}

func (p lazyProvider) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	return p.db.Pool().Query(ctx, sql, args...)
}

func (p lazyProvider) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	if err := p.wait(ctx); err != nil {
		return errRow{err: err}
	}
	return p.db.Pool().QueryRow(ctx, sql, args...)
}

func (p lazyProvider) Begin(ctx context.Context) (pgx.Tx, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	return p.db.Pool().Begin(ctx)
}

// TxProvider returns a databasekit.TxProvider that waits for the database pool
// to become ready before executing queries. This allows consumers to wire their
// transaction managers and repositories before calling runner.Run().
func (db *PostgresDB) TxProvider() TxProvider {
	return lazyProvider{db: db}
}
