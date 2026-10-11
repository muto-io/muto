// SPDX-License-Identifier: Apache-2.0
package tracing

import (
	"context"
	"errors"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const instrumentationName = "github.com/muto-io/muto"

// Init sets up the global TracerProvider from standard OTEL_* environment
// variables (OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_EXPORTER_OTLP_HEADERS,
// OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES, ...), read automatically by
// the OTLP exporter and resource.WithFromEnv().
func Init(ctx context.Context, defaultServiceName string) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	res, err := BuildResource(ctx, defaultServiceName)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

// BuildResource builds the OTel resource describing this process, shared
// by the trace and metric SDKs, from the standard OTEL_RESOURCE_ATTRIBUTES
// / OTEL_SERVICE_NAME env vars (via resource.WithFromEnv()), falling back
// to defaultServiceName.
func BuildResource(ctx context.Context, defaultServiceName string) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(defaultServiceName)),
		resource.WithFromEnv(),
	)
	if err != nil {
		if errors.Is(err, resource.ErrPartialResource) || errors.Is(err, resource.ErrSchemaURLConflict) {
			otel.Handle(err)
			return res, nil
		}
		return nil, fmt.Errorf("build resource: %w", err)
	}
	return res, nil
}

// Wrap starts a span named name, runs fn with the span-scoped context,
// records any error fn returns on the span (RecordError + an Error
// status), ends the span, and returns fn's result and error unchanged.
func Wrap[T any](ctx context.Context, name string, fn func(context.Context) (T, error)) (T, error) {
	tracer := otel.Tracer(instrumentationName)
	ctx, span := tracer.Start(ctx, name)
	defer span.End()
	result, err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return result, err
}

// WrapErr is Wrap for functions that return only an error.
func WrapErr(ctx context.Context, name string, fn func(context.Context) error) error {
	_, err := Wrap(ctx, name, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}
