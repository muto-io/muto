// SPDX-License-Identifier: Apache-2.0
package tracing_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/muto-io/muto/core/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// withRecorder installs a tracetest-backed TracerProvider as the global
// provider for the duration of the test, and restores the previous one
// afterward. This only works because Wrap looks up otel.Tracer(...) fresh
// on every call rather than caching it in a package-level var - see the
// comment on Wrap in tracing.go for why a cached var would break this.
func withRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

func TestInitNoopWhenEndpointUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	shutdown, err := tracing.Init(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init returned a nil shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned error: %v", err)
	}
}

func TestInitBuildsProviderWhenEndpointSet(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	prev := otel.GetTracerProvider()
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	shutdown, err := tracing.Init(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Shutting down immediately is safe even with nothing listening on the
	// endpoint - it just means the flush attempt fails silently (the SDK
	// logs it internally), not that Shutdown itself returns an error for
	// an unreachable collector on close.
	if err := shutdown(context.Background()); err != nil {
		t.Logf("shutdown against unreachable collector returned (expected, non-fatal): %v", err)
	}
}

func TestWrapRecordsSpanAndPassesResultThrough(t *testing.T) {
	sr := withRecorder(t)

	result, err := tracing.Wrap(context.Background(), "test.op", func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	if err != nil || result != "hello" {
		t.Fatalf("Wrap: got (%q, %v), want (\"hello\", nil)", result, err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "test.op" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "test.op")
	}
	if spans[0].Status().Code != codes.Unset {
		t.Errorf("span status = %v, want Unset (no error)", spans[0].Status().Code)
	}
}

func TestWrapRecordsErrorOnSpan(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("boom")
	_, err := tracing.Wrap(context.Background(), "test.fail", func(ctx context.Context) (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Wrap returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", spans[0].Status().Code)
	}
}

func TestWrapErrSuccessAndFailure(t *testing.T) {
	sr := withRecorder(t)

	if err := tracing.WrapErr(context.Background(), "test.errfn.ok", func(ctx context.Context) error {
		return nil
	}); err != nil {
		t.Fatalf("WrapErr: %v", err)
	}

	wantErr := errors.New("errfn boom")
	err := tracing.WrapErr(context.Background(), "test.errfn.fail", func(ctx context.Context) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("WrapErr returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	if spans[0].Status().Code != codes.Unset {
		t.Errorf("spans[0] (%s) status = %v, want Unset", spans[0].Name(), spans[0].Status().Code)
	}
	if spans[1].Status().Code != codes.Error {
		t.Errorf("spans[1] (%s) status = %v, want Error", spans[1].Name(), spans[1].Status().Code)
	}
}

func TestInitInstallsPropagator(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", collector.URL)

	prevProvider := otel.GetTracerProvider()
	prevPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevProvider)
		otel.SetTextMapPropagator(prevPropagator)
	})

	shutdown, err := tracing.Init(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if carrier.Get("traceparent") == "" {
		t.Error("Init did not install a propagator that injects a traceparent header")
	}
}
