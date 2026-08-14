package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/mazzama/go-bootkit/workerkit"
)

// NotificationProcessor simulates sending an email or push notification asynchronously.
type NotificationProcessor struct {
	logger *slog.Logger
}

func NewNotificationProcessor(logger *slog.Logger) *NotificationProcessor {
	return &NotificationProcessor{
		logger: logger,
	}
}

func (p *NotificationProcessor) Process(ctx context.Context, t workerkit.Task) error {
	p.logger.Info("sending notification", "payload", string(t.Payload))

	// Simulate some work being done (e.g., calling an external API)
	select {
	case <-time.After(100 * time.Millisecond):
		p.logger.Info("notification sent successfully", "payload", string(t.Payload))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
