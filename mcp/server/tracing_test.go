// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func withRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

func TestWrapToolHandlerRecordsSpan(t *testing.T) {
	sr := withRecorder(t)

	handler := WrapToolHandler("mcp.test_tool", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok"), nil
	})

	result, err := handler(context.Background(), mcp.CallToolRequest{})
	if err != nil || result == nil {
		t.Fatalf("handler: got (%v, %v), want a non-nil result and nil error", result, err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "mcp.test_tool" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "mcp.test_tool")
	}
}

func TestWrapToolHandlerRecordsError(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("tool failed")
	handler := WrapToolHandler("mcp.failing_tool", func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, wantErr
	})

	_, err := handler(context.Background(), mcp.CallToolRequest{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("handler returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}
