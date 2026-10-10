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
// It mirrors the muto_* Prometheus families in platform/k8s/metrics, so
// the same domain events are visible both to a Prometheus scrape and, when
// OTLP export is enabled, to an OTLP collector - the latter is the only
// option on platforms with no Prometheus scraping (e.g. CF).
type instruments struct {
	reconciliationsTotal   otelmetric.Int64Counter
	reconciliationDuration otelmetric.Float64Histogram
	jobsTotal              otelmetric.Int64Counter
	jobDuration            otelmetric.Float64Histogram
	agentsRunning          otelmetric.Int64UpDownCounter
	jobQueueDepth          otelmetric.Int64UpDownCounter
}

// active is nil until Init builds a real OTLP meter provider. Every
// recording function below checks it and silently does nothing while it is
// nil, so calling them costs nothing when OTLP export is disabled (the
// default) and Prometheus remains the only consumer of these events.
var active atomic.Pointer[instruments]

// Init sets up the global MeterProvider from standard OTEL_* environment
// variables, the same ones read by the OTLP trace exporter and
// resource.WithFromEnv() in core/tracing. If neither
// OTEL_EXPORTER_OTLP_ENDPOINT nor OTEL_EXPORTER_OTLP_METRICS_ENDPOINT is
// set, this is a no-op: no exporter or provider is created, and the
// recording functions below stay inert. Prometheus scraping via
// platform/k8s/metrics is unaffected either way. On success, returns a
// shutdown function the caller must invoke (e.g. via defer, with a bounded
// timeout context) to flush buffered metrics before exit.
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
		// Mirrors platform/k8s/metrics.ReconciliationDuration's Buckets, so
		// the OTLP and Prometheus exports of "the same" metric bucket
		// latency identically instead of OTLP falling back to the SDK's
		// generic default boundaries.
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
		// Mirrors platform/k8s/metrics.JobDuration's Buckets; see
		// reconciliationDuration above for why.
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
// the OTLP instruments: a reconcile call named reconciler took durationSec
// seconds and ended with status ("success" or "error").
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

// RecordJobFinished mirrors platform/k8s/metrics.JobFinished. hasDuration
// must be the same "was startedAt known" condition the caller used to
// decide whether to compute durationSec, so that a genuinely-instantaneous
// job (durationSec rounds to 0 but startedAt was known) still records a
// muto.job.duration observation of 0, matching the Prometheus
// JobDuration.Observe call JobFinished always makes in that case.
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
