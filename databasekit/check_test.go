package databasekit

import (
	"testing"
)

// The pgx-free Querier/TxProvider seam is satisfied by our adapters, not by the
// raw driver. The pool's native fit was deliberately dropped (see ADR-0005);
// a raw *pgxpool.Pool is wrapped by NewPoolProvider at wiring time.
func TestAdaptersImplementTxProvider(t *testing.T) {
	var _ TxProvider = txProviderAdapter{}
	var _ TxProvider = poolAdapter{}
	var _ Tx = pgxTxAdapter{}
}
