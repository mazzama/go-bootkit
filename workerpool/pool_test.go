package workerpool_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/core"
	"github.com/mazzama/go-bootkit/workerpool"
	"github.com/mazzama/go-bootkit/workerpool/memory"
	"github.com/stretchr/testify/assert"
)

type mockProcessor struct {
	fn func(ctx context.Context, payload []byte) error
}

func (m *mockProcessor) Process(ctx context.Context, payload []byte) error {
	return m.fn(ctx, payload)
}

func TestWorkerPool_SubmitAndProcess(t *testing.T) {
	q := memory.NewQueue(10)

	var processed [][]byte
	var mu sync.Mutex

	p := &mockProcessor{
		fn: func(ctx context.Context, payload []byte) error {
			mu.Lock()
			processed = append(processed, payload)
			mu.Unlock()
			return nil
		},
	}

	pool := workerpool.NewWorkerPool("test-pool", q, p, workerpool.WithWorkers(2))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start pool in runner
	runner := core.NewApplicationRunner(core.WithServices(pool))

	go func() {
		_ = runner.Run(ctx)
	}()

	// Wait for ready
	<-pool.Ready()

	jobID, err := pool.Submit(context.Background(), []byte("hello"))
	assert.NoError(t, err)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	status, err := pool.Status(context.Background(), jobID)
	assert.NoError(t, err)
	assert.Equal(t, workerpool.StatusCompleted, status)

	mu.Lock()
	assert.Len(t, processed, 1)
	assert.Equal(t, []byte("hello"), processed[0])
	mu.Unlock()
}

func TestWorkerPool_ProcessorErrorNacksJob(t *testing.T) {
	q := memory.NewQueue(10)

	p := &mockProcessor{
		fn: func(ctx context.Context, payload []byte) error {
			return errors.New("boom")
		},
	}

	pool := workerpool.NewWorkerPool("test-pool", q, p, workerpool.WithWorkers(1))

	go func() {
		_ = pool.Start(context.Background())
	}()
	<-pool.Ready()
	defer func() { _ = pool.Stop(context.Background()) }()

	jobID, _ := pool.Submit(context.Background(), []byte("fail"))

	time.Sleep(100 * time.Millisecond)

	status, _ := pool.Status(context.Background(), jobID)
	assert.Equal(t, workerpool.StatusFailed, status)
}

func TestWorkerPool_ProcessorPanicIsRecovered(t *testing.T) {
	q := memory.NewQueue(10)

	p := &mockProcessor{
		fn: func(ctx context.Context, payload []byte) error {
			panic("unexpected boom")
		},
	}

	pool := workerpool.NewWorkerPool("test-pool", q, p, workerpool.WithWorkers(1))

	go func() {
		_ = pool.Start(context.Background())
	}()
	<-pool.Ready()
	defer func() { _ = pool.Stop(context.Background()) }()

	jobID, _ := pool.Submit(context.Background(), []byte("panic"))

	time.Sleep(100 * time.Millisecond)

	status, _ := pool.Status(context.Background(), jobID)
	assert.Equal(t, workerpool.StatusFailed, status)
}
