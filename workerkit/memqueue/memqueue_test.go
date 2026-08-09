package memqueue_test

import (
	"context"
	"testing"

	"github.com/mazzama/go-bootkit/workerkit"
	"github.com/mazzama/go-bootkit/workerkit/memqueue"
)

func TestInMemoryClientEnqueue(t *testing.T) {
	c := memqueue.New()

	task := workerkit.Task{Type: "email:send", Payload: []byte(`{"to":"a@b.com"}`)}
	info, err := c.Enqueue(task, workerkit.WithQueue("critical"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Type != "email:send" {
		t.Errorf("expected email:send, got %s", info.Type)
	}
	if info.State != "enqueued" {
		t.Errorf("expected enqueued, got %s", info.State)
	}

	tasks := c.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Task.Type != "email:send" {
		t.Errorf("expected email:send, got %s", tasks[0].Task.Type)
	}
	if string(tasks[0].Task.Payload) != `{"to":"a@b.com"}` {
		t.Errorf("unexpected payload: %s", tasks[0].Task.Payload)
	}
}

func TestInMemoryClientEnqueueContext(t *testing.T) {
	c := memqueue.New()

	ctx := context.Background()
	task := workerkit.Task{Type: "report:generate"}
	info, err := c.EnqueueContext(ctx, task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Type != "report:generate" {
		t.Errorf("expected report:generate, got %s", info.Type)
	}

	if len(c.Tasks()) != 1 {
		t.Fatalf("expected 1 task, got %d", len(c.Tasks()))
	}
}

func TestInMemoryClientEnqueueContextCancelled(t *testing.T) {
	c := memqueue.New()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	task := workerkit.Task{Type: "noop"}
	_, err := c.EnqueueContext(ctx, task)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}

	if len(c.Tasks()) != 0 {
		t.Fatalf("expected 0 tasks for cancelled context, got %d", len(c.Tasks()))
	}
}

func TestInMemoryClientMultipleTasks(t *testing.T) {
	c := memqueue.New()

	for i := range 3 {
		_, err := c.Enqueue(workerkit.Task{Type: "task"}, workerkit.WithQueue("default"))
		if err != nil {
			t.Fatalf("unexpected error for task %d: %v", i, err)
		}
	}

	tasks := c.Tasks()
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestInMemoryClientReset(t *testing.T) {
	c := memqueue.New()

	_, _ = c.Enqueue(workerkit.Task{Type: "task"})
	if len(c.Tasks()) != 1 {
		t.Fatal("expected 1 task before reset")
	}

	c.Reset()
	if len(c.Tasks()) != 0 {
		t.Fatalf("expected 0 tasks after reset, got %d", len(c.Tasks()))
	}
}

func TestInMemoryClientSatisfiesEnqueuer(t *testing.T) {
	var e workerkit.Enqueuer = memqueue.New()
	_, err := e.Enqueue(workerkit.Task{Type: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
