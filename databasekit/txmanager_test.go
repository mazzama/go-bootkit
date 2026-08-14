package databasekit_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mazzama/go-bootkit/core"
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

var _ core.Readyable = (*MockReadyProvider)(nil)

func (m *MockProvider) Begin(ctx context.Context) (databasekit.Tx, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(databasekit.Tx), args.Error(1)
}

func (m *MockProvider) Exec(ctx context.Context, sql string, arguments ...any) (int64, error) {
	callArgs := append([]any{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockProvider) Query(ctx context.Context, sql string, arguments ...any) (databasekit.Rows, error) {
	callArgs := append([]any{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(databasekit.Rows), args.Error(1)
}

func (m *MockProvider) QueryRow(ctx context.Context, sql string, arguments ...any) databasekit.Row {
	callArgs := append([]any{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(databasekit.Row)
}

type MockTx struct {
	mock.Mock
}

func (m *MockTx) Begin(ctx context.Context) (databasekit.Tx, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(databasekit.Tx), args.Error(1)
}

func (m *MockTx) Commit(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockTx) Rollback(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockTx) Exec(ctx context.Context, sql string, arguments ...any) (int64, error) {
	callArgs := append([]any{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockTx) Query(ctx context.Context, sql string, arguments ...any) (databasekit.Rows, error) {
	callArgs := append([]any{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(databasekit.Rows), args.Error(1)
}

func (m *MockTx) QueryRow(ctx context.Context, sql string, arguments ...any) databasekit.Row {
	callArgs := append([]any{ctx, sql}, arguments...)
	args := m.Called(callArgs...)
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(databasekit.Row)
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
		mockTx.On("Rollback", mock.Anything).Return(databasekit.ErrTxClosed)

		// Assert query logic inside the transaction works
		mockTx.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item1')").Return(int64(0), nil)

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

		mockTx.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item2')").Return(int64(0), nil)

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
		mockTxOuter.On("Rollback", mock.Anything).Return(databasekit.ErrTxClosed)

		mockTxOuter.On("Begin", mock.Anything).Return(mockTxInner, nil)
		mockTxInner.On("Commit", mock.Anything).Return(nil)
		mockTxInner.On("Rollback", mock.Anything).Return(databasekit.ErrTxClosed)

		mockTxOuter.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item3')").Return(int64(0), nil)
		mockTxInner.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item4')").Return(int64(0), nil)

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
		mockTxOuter.On("Rollback", mock.Anything).Return(databasekit.ErrTxClosed)

		mockTxOuter.On("Begin", mock.Anything).Return(mockTxInner, nil)
		mockTxInner.On("Rollback", mock.Anything).Return(nil)

		mockTxOuter.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item5')").Return(int64(0), nil)
		mockTxInner.On("Exec", mock.Anything, "INSERT INTO items (name) VALUES ('item6')").Return(int64(0), nil)

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

		provider.On("Exec", mock.Anything, "SELECT 1").Return(int64(0), nil)
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

		provider.On("Exec", mock.Anything, "SELECT 1").Return(int64(0), nil)
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
