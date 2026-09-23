// SPDX-License-Identifier: Apache-2.0
package a2a

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestNewClientUsesOtelhttpTransport(t *testing.T) {
	c, err := New(&Config{GatewayURL: "http://example.invalid"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, ok := c.httpClient.Transport.(*otelhttp.Transport); !ok {
		t.Errorf("httpClient.Transport = %T, want *otelhttp.Transport", c.httpClient.Transport)
	}
}

func TestNewClientPropagatesTraceContext(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"taskId":"t1","state":"submitted"}`))
	}))
	defer server.Close()

	prevPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer otel.SetTextMapPropagator(prevPropagator)

	prevProvider := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prevProvider)

	c, err := New(&Config{GatewayURL: server.URL})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	if _, err := c.SendTask(ctx, "agent-1", []byte(`{}`)); err != nil {
		t.Fatalf("SendTask: %v", err)
	}
	if gotHeader == "" {
		t.Error("no traceparent header received by server; otelhttp propagation not working")
	}
}
