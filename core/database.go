package core

import "context"

// Database defines a generic abstraction for relational database operations.
// Note: To fully decouple from pgx (which exposes pgx.Rows, pgx.Row, pgx.Tx), 
// additional wrapper types (e.g. core.Rows, core.Row) would need to be implemented.
// This interface provides a starting point for infrastructure decoupling.
type Database interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (int64, error)
}
