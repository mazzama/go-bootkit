package databasekit

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPoolImplementsTxProvider(t *testing.T) {
	var _ TxProvider = (*pgxpool.Pool)(nil)
}
