// SPDX-License-Identifier: Apache-2.0
package tracing_test

import (
	"context"
	"errors"
	"testing"

	k8stracing "github.com/muto-io/muto/platform/k8s/tracing"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

type stubReconciler struct {
	err error
}

func (s *stubReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, s.err
}

func TestWrapReconcilerRecordsSpan(t *testing.T) {
	sr := withRecorder(t)

	wrapped := k8stracing.WrapReconciler("tenant", &stubReconciler{})
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "tenant" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "tenant")
	}
}

func TestWrapReconcilerRecordsError(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("reconcile failed")
	wrapped := k8stracing.WrapReconciler("agentjob", &stubReconciler{err: wantErr})
	_, err := wrapped.Reconcile(context.Background(), reconcile.Request{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reconcile returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}
