package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/mazzama/go-bootkit/workerkit"
)

// TestNotificationProcessor_Process verifies a framework Handler can be unit
// tested with a plain workerkit.Task — no asynq or Redis needed.
func TestNotificationProcessor_Process(t *testing.T) {
	p := NewNotificationProcessor(slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	task := workerkit.Task{Type: "notification:send", Payload: []byte(`{"to":"user@example.com"}`)}
	if err := p.Process(ctx, task); err != nil {
		t.Fatalf("Process returned error: %v", err)
	}
}

func TestNotificationProcessor_ProcessContextCancelled(t *testing.T) {
	p := NewNotificationProcessor(slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Process(ctx, workerkit.Task{Type: "notification:send"})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}
