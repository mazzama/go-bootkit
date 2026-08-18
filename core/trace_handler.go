package core

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TraceHandler wraps an slog.Handler to automatically inject OpenTelemetry
// trace_id and span_id into log records when a valid span context is present.
type TraceHandler struct {
	base slog.Handler
	ops  []handlerOp
}

type handlerOp struct {
	isGroup bool
	name    string
	attrs   []slog.Attr
}

// NewTraceHandler creates a new TraceHandler wrapping the provided base handler.
func NewTraceHandler(base slog.Handler) *TraceHandler {
	return &TraceHandler{base: base}
}

// Enabled delegates to the inner handler.
func (h *TraceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

// Handle injects trace_id and span_id attributes at the root of the log record
// before applying any groups or attributes added via WithGroup/WithAttrs.
//
//nolint:gocritic // slog.Handler interface mandates Record by value.
func (h *TraceHandler) Handle(ctx context.Context, r slog.Record) error {
	handler := h.base

	spanCtx := trace.SpanContextFromContext(ctx)
	if spanCtx.IsValid() {
		handler = handler.WithAttrs([]slog.Attr{
			slog.String("trace_id", spanCtx.TraceID().String()),
			slog.String("span_id", spanCtx.SpanID().String()),
		})
	}

	for _, op := range h.ops {
		if op.isGroup {
			handler = handler.WithGroup(op.name)
		} else {
			handler = handler.WithAttrs(op.attrs)
		}
	}

	return handler.Handle(ctx, r)
}

// WithAttrs returns a new TraceHandler with the additional attributes.
func (h *TraceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newOps := append([]handlerOp(nil), h.ops...)
	newOps = append(newOps, handlerOp{isGroup: false, attrs: attrs})
	return &TraceHandler{base: h.base, ops: newOps}
}

// WithGroup returns a new TraceHandler with the additional group.
func (h *TraceHandler) WithGroup(name string) slog.Handler {
	newOps := append([]handlerOp(nil), h.ops...)
	newOps = append(newOps, handlerOp{isGroup: true, name: name})
	return &TraceHandler{base: h.base, ops: newOps}
}
