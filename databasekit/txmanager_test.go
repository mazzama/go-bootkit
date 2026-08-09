package databasekit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mazzama/go-bootkit/databasekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockProvider struct {
	mock.Mock
}
type MockReadyProvider struct {
	MockProvider
	readyChan chan struct{}
}

func (m *MockReadyProvider) Ready() <-chan struct{} {
	return m.readyChan
}

func (m *MockProvider) Begin(ctx context.Context) (pgx.Tx, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pgx.Tx), args.Error(1)
}

func (m *MockProvider) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	callArgs := append([]interface{}{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	return args.Get(0).(pgconn.CommandTag), args.Error(1)
}

func (m *MockProvider) Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error) {
	callArgs := append([]interface{}{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pgx.Rows), args.Error(1)
}

func (m *MockProvider) QueryRow(ctx context.Context, sql string, arguments ...interface{}) pgx.Row {
	callArgs := append([]interface{}{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(pgx.Row)
}

type MockTx struct {
	mock.Mock
}

func (m *MockTx) Begin(ctx context.Context) (pgx.Tx, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pgx.Tx), args.Error(1)
}

func (m *MockTx) Commit(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockTx) Rollback(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	args := m.Called(ctx, tableName, columnNames, rowSrc)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	args := m.Called(ctx, b)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(pgx.BatchResults)
}

func (m *MockTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (m *MockTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	args := m.Called(ctx, name, sql)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pgconn.StatementDescription), args.Error(1)
}

func (m *MockTx) Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error) {
	callArgs := append([]interface{}{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	return args.Get(0).(pgconn.CommandTag), args.Error(1)
}

func (m *MockTx) Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error) {
	callArgs := append([]interface{}{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(pgx.Rows), args.Error(1)
}

func (m *MockTx) QueryRow(ctx context.Context, sql string, arguments ...interface{}) pgx.Row {
	callArgs := append([]interface{}{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(pgx.Row)
}

func (m *MockTx) Conn() *pgx.Conn {
	return nil
}

func TestTxManager(t *testing.T) {
	ctx := context.Background()

	t.Run("QuerierFromContext without tx", func(t *testing.T) {
		provider := new(MockProvider)
		tm := databasekit.NewTxManager(provider)

		querier := tm.QuerierFromContext(ctx)
		assert.Equal(t, provider, querier)
	})

	t.Run("Basic commit", func(t *testing.T) {
		provider := new(MockProvider)
		tm := databasekit.NewTxManager(provider)

		mockTx := new(MockTx)
		provider.On("Begin", mock.Anything).Return(mockTx, nil)
		mockTx.On("Commit", mock.Anything).Return(nil)
		// Deferred rollback when successful is either no-op or returns ErrTxClosed
		mockTx.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed)

		// Assert query logic inside the transaction works
		mockTx.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item1')").Return(pgconn.CommandTag{}, nil)

		err := tm.WithTx(ctx, func(ctxTx context.Context) error {
			querier := tm.QuerierFromContext(ctxTx)
			assert.Equal(t, mockTx, querier)
			_, err := querier.Exec(ctxTx, "INSERT INTO items (name) VALUES ('item1')")
			return err
		})

		assert.NoError(t, err)
		provider.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("Rollback on error", func(t *testing.T) {
		provider := new(MockProvider)
		tm := databasekit.NewTxManager(provider)

		mockTx := new(MockTx)
		provider.On("Begin", mock.Anything).Return(mockTx, nil)
		mockTx.On("Rollback", mock.Anything).Return(nil)

		mockTx.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item2')").Return(pgconn.CommandTag{}, nil)

		expectedErr := fmt.Errorf("intentional rollback")
		err := tm.WithTx(ctx, func(ctxTx context.Context) error {
			querier := tm.QuerierFromContext(ctxTx)
			_, err := querier.Exec(ctxTx, "INSERT INTO items (name) VALUES ('item2')")
			assert.NoError(t, err)
			return expectedErr
		})

		assert.EqualError(t, err, expectedErr.Error())
		provider.AssertExpectations(t)
		mockTx.AssertExpectations(t)
	})

	t.Run("Nested savepoint commit", func(t *testing.T) {
		provider := new(MockProvider)
		tm := databasekit.NewTxManager(provider)

		mockTxOuter := new(MockTx)
		mockTxInner := new(MockTx)

		provider.On("Begin", mock.Anything).Return(mockTxOuter, nil)
		mockTxOuter.On("Commit", mock.Anything).Return(nil)
		mockTxOuter.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed)

		mockTxOuter.On("Begin", mock.Anything).Return(mockTxInner, nil)
		mockTxInner.On("Commit", mock.Anything).Return(nil)
		mockTxInner.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed)

		mockTxOuter.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item3')").Return(pgconn.CommandTag{}, nil)
		mockTxInner.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item4')").Return(pgconn.CommandTag{}, nil)

		err := tm.WithTx(ctx, func(ctxTx1 context.Context) error {
			q1 := tm.QuerierFromContext(ctxTx1)
			assert.Equal(t, mockTxOuter, q1)
			_, err := q1.Exec(ctxTx1, "INSERT INTO items (name) VALUES ('item3')")
			assert.NoError(t, err)

			return tm.WithTx(ctxTx1, func(ctxTx2 context.Context) error {
				q2 := tm.QuerierFromContext(ctxTx2)
				assert.Equal(t, mockTxInner, q2)
				_, err := q2.Exec(ctxTx2, "INSERT INTO items (name) VALUES ('item4')")
				return err
			})
		})

		assert.NoError(t, err)
		provider.AssertExpectations(t)
		mockTxOuter.AssertExpectations(t)
		mockTxInner.AssertExpectations(t)
	})

	t.Run("Nested savepoint rollback", func(t *testing.T) {
		provider := new(MockProvider)
		tm := databasekit.NewTxManager(provider)

		mockTxOuter := new(MockTx)
		mockTxInner := new(MockTx)

		provider.On("Begin", mock.Anything).Return(mockTxOuter, nil)
		mockTxOuter.On("Commit", mock.Anything).Return(nil)
		mockTxOuter.On("Rollback", mock.Anything).Return(pgx.ErrTxClosed)

		mockTxOuter.On("Begin", mock.Anything).Return(mockTxInner, nil)
		mockTxInner.On("Rollback", mock.Anything).Return(nil)

		mockTxOuter.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item5')").Return(pgconn.CommandTag{}, nil)
		mockTxInner.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item6')").Return(pgconn.CommandTag{}, nil)

		expectedErr := fmt.Errorf("nested intentional rollback")

		err := tm.WithTx(ctx, func(ctxTx1 context.Context) error {
			q1 := tm.QuerierFromContext(ctxTx1)
			_, err := q1.Exec(ctxTx1, "INSERT INTO items (name) VALUES ('item5')")
			assert.NoError(t, err)

			innerErr := tm.WithTx(ctxTx1, func(ctxTx2 context.Context) error {
				q2 := tm.QuerierFromContext(ctxTx2)
				_, err := q2.Exec(ctxTx2, "INSERT INTO items (name) VALUES ('item6')")
				assert.NoError(t, err)
				return expectedErr
			})
			assert.EqualError(t, innerErr, expectedErr.Error())
			return nil
		})

		assert.NoError(t, err)
		provider.AssertExpectations(t)
		mockTxOuter.AssertExpectations(t)
		mockTxInner.AssertExpectations(t)
	})
	t.Run("Readyable provider ready operations", func(t *testing.T) {
		readyChan := make(chan struct{})
		close(readyChan)
		provider := &MockReadyProvider{readyChan: readyChan}
		tm := databasekit.NewTxManager(provider)

		provider.On("Exec", mock.Anything, "SELECT 1").Return(pgconn.CommandTag{}, nil)
		provider.On("Query", mock.Anything, "SELECT 1").Return(nil, nil)
		provider.On("QueryRow", mock.Anything, "SELECT 1").Return(nil)

		q := tm.QuerierFromContext(ctx)
		_, err := q.Exec(ctx, "SELECT 1")
		assert.NoError(t, err)

		_, err = q.Query(ctx, "SELECT 1")
		assert.NoError(t, err)

		row := q.QueryRow(ctx, "SELECT 1")
		assert.Nil(t, row)

		provider.AssertExpectations(t)
	})

	t.Run("Readyable provider unready context timeout", func(t *testing.T) {
		readyChan := make(chan struct{}) // never closed
		provider := &MockReadyProvider{readyChan: readyChan}
		tm := databasekit.NewTxManager(provider)

		ctxTimeout, cancel := context.WithCancel(ctx)
		cancel()

		q := tm.QuerierFromContext(ctxTimeout)
		_, err := q.Exec(ctxTimeout, "SELECT 1")
		assert.Error(t, err)

		_, err = q.Query(ctxTimeout, "SELECT 1")
		assert.Error(t, err)

		err = tm.WithTx(ctxTimeout, func(ctx context.Context) error {
			return nil
		})
		assert.ErrorContains(t, err, "pool not ready")
	})
	t.Run("Non-readyable provider operations", func(t *testing.T) {
		provider := new(MockProvider)
		tm := databasekit.NewTxManager(provider)

		provider.On("Exec", mock.Anything, "SELECT 1").Return(pgconn.CommandTag{}, nil)
		provider.On("Query", mock.Anything, "SELECT 1").Return(nil, nil)
		provider.On("QueryRow", mock.Anything, "SELECT 1").Return(nil)

		q := tm.QuerierFromContext(ctx)
		_, err := q.Exec(ctx, "SELECT 1")
		assert.NoError(t, err)

		_, err = q.Query(ctx, "SELECT 1")
		assert.NoError(t, err)

		row := q.QueryRow(ctx, "SELECT 1")
		assert.Nil(t, row)

		provider.AssertExpectations(t)
	})
}
