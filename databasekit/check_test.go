package databasekit

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
)

func TestPoolImplementsTxProvider(t *testing.T) {
	var _ TxProvider = (*pgxpool.Pool)(nil)
}
