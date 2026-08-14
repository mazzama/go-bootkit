package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"github.com/mazzama/go-bootkit/core"
	"go.opentelemetry.io/otel/trace"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) Bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Bytes()
}

func TestTraceHandler_WithSpanContext(t *testing.T) {
	var buf syncBuffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(core.NewTraceHandler(handler))

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")

	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	logger.InfoContext(ctx, "test message")

	var output map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("failed to parse log JSON: %v, raw: %s", err, string(buf.Bytes()))
	}

	if output["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected trace_id, got %v", output["trace_id"])
	}
	if output["span_id"] != "00f067aa0ba902b7" {
		t.Errorf("expected span_id, got %v", output["span_id"])
	}
}

func TestTraceHandler_WithoutSpanContext(t *testing.T) {
	var buf syncBuffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(core.NewTraceHandler(handler))

	logger.InfoContext(context.Background(), "test message")

	var output map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}

	if _, ok := output["trace_id"]; ok {
		t.Errorf("did not expect trace_id")
	}
	if _, ok := output["span_id"]; ok {
		t.Errorf("did not expect span_id")
	}
}

func TestTraceHandler_WithAttrsAndGroup(t *testing.T) {
	var buf syncBuffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(core.NewTraceHandler(handler))

	l := logger.With("component", "test").WithGroup("group1")

	traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")

	spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})

	ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)

	l.InfoContext(ctx, "test message", "key", "value")

	var output map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &output); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}

	if output["component"] != "test" {
		t.Errorf("expected component attr, got %v", output["component"])
	}

	if output["trace_id"] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("expected trace_id, got %v", output["trace_id"])
	}

	group, ok := output["group1"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected group1")
	}

	if group["key"] != "value" {
		t.Errorf("expected group1.key=value, got %v", group["key"])
	}
}

func TestTraceHandler_ConcurrentSafety(t *testing.T) {
	var buf syncBuffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(core.NewTraceHandler(handler))

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			traceID, _ := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
			spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")

			spanCtx := trace.NewSpanContext(trace.SpanContextConfig{
				TraceID:    traceID,
				SpanID:     spanID,
				TraceFlags: trace.FlagsSampled,
			})

			ctx := trace.ContextWithSpanContext(context.Background(), spanCtx)
			logger.InfoContext(ctx, "concurrent message")
		}(i)
	}
	wg.Wait()
	// Test passes if -race does not detect any data races
}
