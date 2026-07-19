package core

import (
	"context"
	"io"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler wraps an slog.Handler to automatically inject OpenTelemetry
// trace_id and span_id into log records when a valid span context is present.
type TraceHandler struct {
	inner slog.Handler
}

// NewTraceHandler creates a new TraceHandler wrapping the provided inner handler.
func NewTraceHandler(inner slog.Handler) *TraceHandler {
	return &TraceHandler{inner: inner}
}

// Enabled delegates to the inner handler.
func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle injects trace_id and span_id attributes if a valid span context is found,
// then delegates to the inner handler.
func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new TraceHandler with the additional attributes.
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TraceHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup returns a new TraceHandler with the additional group.
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	return &TraceHandler{inner: h.inner.WithGroup(name)}
}

type LoggerConfig struct {
	Level  slog.Level
	Writer io.Writer
}

type LoggerOption func(*LoggerConfig)

// WithLogLevel sets the minimum log level for the logger.
func WithLogLevel(level slog.Level) LoggerOption {
	return func(c *LoggerConfig) {
		c.Level = level
	}
}

// WithLogWriter sets the output writer for the logger.
func WithLogWriter(writer io.Writer) LoggerOption {
	return func(c *LoggerConfig) {
		c.Writer = writer
	}
}

// NewLogger creates a new *slog.Logger configured with JSON output
// and trace context correlation.
func NewLogger(options ...LoggerOption) *slog.Logger {
	config := &LoggerConfig{
		Level:  slog.LevelInfo,
		Writer: os.Stdout,
	}

	for _, opt := range options {
		opt(config)
	}

	jsonHandler := slog.NewJSONHandler(config.Writer, &slog.HandlerOptions{
		Level: config.Level,
	})
	
	traceHandler := NewTraceHandler(jsonHandler)

	return slog.New(traceHandler)
}
