// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/muto-io/muto/core/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

const instrumentationName = "github.com/muto-io/muto"

// instruments holds every OTLP metric instrument this package records.
type instruments struct {
	reconciliationsTotal   otelmetric.Int64Counter
	reconciliationDuration otelmetric.Float64Histogram
	jobsTotal              otelmetric.Int64Counter
	jobDuration            otelmetric.Float64Histogram
	agentsRunning          otelmetric.Int64UpDownCounter
	jobQueueDepth          otelmetric.Int64UpDownCounter
}

// active is nil until Init builds a real OTLP meter provider.
var active atomic.Pointer[instruments]

// Init sets up the global MeterProvider from standard OTEL_* environment
// variables, the same ones read by the OTLP trace exporter and
// resource.WithFromEnv() in core/tracing.
func Init(ctx context.Context, defaultServiceName string) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlpmetrichttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create otlp metric exporter: %w", err)
	}

	res, err := tracing.BuildResource(ctx, defaultServiceName)
	if err != nil {
		_ = exporter.Shutdown(ctx)
		return nil, err
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
	)

	inst, err := newInstruments(mp.Meter(instrumentationName))
	if err != nil {
		_ = mp.Shutdown(ctx)
		return nil, fmt.Errorf("create otlp metric instruments: %w", err)
	}

	otel.SetMeterProvider(mp)
	active.Store(inst)
	return mp.Shutdown, nil
}

func newInstruments(meter otelmetric.Meter) (*instruments, error) {
	reconciliationsTotal, err := meter.Int64Counter("muto.reconciliations.total",
		otelmetric.WithDescription("Total number of reconcile calls, by reconciler and result."))
	if err != nil {
		return nil, err
	}
	reconciliationDuration, err := meter.Float64Histogram("muto.reconciliation.duration",
		otelmetric.WithDescription("Duration of reconcile calls in seconds, by reconciler."),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(.001, .005, .01, .05, .1, .5, 1, 5, 10))
	if err != nil {
		return nil, err
	}
	jobsTotal, err := meter.Int64Counter("muto.jobs.total",
		otelmetric.WithDescription("Total number of AgentJobs that reached a terminal state, by tenant and status."))
	if err != nil {
		return nil, err
	}
	jobDuration, err := meter.Float64Histogram("muto.job.duration",
		otelmetric.WithDescription("Duration of AgentJobs from start to completion in seconds, by tenant and status."),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(1, 5, 15, 30, 60, 300, 900, 3600))
	if err != nil {
		return nil, err
	}
	agentsRunning, err := meter.Int64UpDownCounter("muto.agents.running",
		otelmetric.WithDescription("Number of agent pods currently running, by tenant."))
	if err != nil {
		return nil, err
	}
	jobQueueDepth, err := meter.Int64UpDownCounter("muto.job.queue_depth",
		otelmetric.WithDescription("Number of AgentJobs currently in the Running phase, by tenant."))
	if err != nil {
		return nil, err
	}
	return &instruments{
		reconciliationsTotal:   reconciliationsTotal,
		reconciliationDuration: reconciliationDuration,
		jobsTotal:              jobsTotal,
		jobDuration:            jobDuration,
		agentsRunning:          agentsRunning,
		jobQueueDepth:          jobQueueDepth,
	}, nil
}

// RecordReconcile mirrors platform/k8s/metrics.ObserveReconcile's effect on
// the OTLP instruments.
func RecordReconcile(ctx context.Context, reconciler, status string, durationSec float64) {
	inst := active.Load()
	if inst == nil {
		return
	}
	attrs := otelmetric.WithAttributes(attribute.String("reconciler", reconciler))
	inst.reconciliationDuration.Record(ctx, durationSec, attrs)
	inst.reconciliationsTotal.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("reconciler", reconciler), attribute.String("result", status),
	))
}

// RecordJobStarted mirrors platform/k8s/metrics.JobStarted.
func RecordJobStarted(ctx context.Context, tenant string, agentCount int32) {
	inst := active.Load()
	if inst == nil {
		return
	}
	inst.agentsRunning.Add(ctx, int64(agentCount), otelmetric.WithAttributes(attribute.String("tenant", tenant)))
	inst.jobQueueDepth.Add(ctx, 1, otelmetric.WithAttributes(attribute.String("tenant", tenant)))
}

// RecordJobFinished mirrors platform/k8s/metrics.JobFinished.
func RecordJobFinished(ctx context.Context, tenant, status string, durationSec float64, hasDuration bool, activeAgents int32) {
	inst := active.Load()
	if inst == nil {
		return
	}
	attrs := otelmetric.WithAttributes(attribute.String("tenant", tenant), attribute.String("status", status))
	inst.jobsTotal.Add(ctx, 1, attrs)
	if hasDuration {
		inst.jobDuration.Record(ctx, durationSec, attrs)
	}
	inst.agentsRunning.Add(ctx, -int64(activeAgents), otelmetric.WithAttributes(attribute.String("tenant", tenant)))
	inst.jobQueueDepth.Add(ctx, -1, otelmetric.WithAttributes(attribute.String("tenant", tenant)))
}
