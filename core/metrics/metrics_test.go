// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// withInstruments builds a MeterProvider backed by a ManualReader, wires it
// into the package's active instrument set the same way Init does, and
// restores the previous state afterward. Recording functions stay inert
// (active is nil) until a test calls this, mirroring how they behave in
// production until Init runs.
func withInstruments(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	inst, err := newInstruments(mp.Meter(instrumentationName))
	if err != nil {
		t.Fatalf("newInstruments: %v", err)
	}
	active.Store(inst)
	t.Cleanup(func() { active.Store(nil) })
	return reader
}

func collect(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	return rm
}

func metricNames(rm metricdata.ResourceMetrics) []string {
	var names []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			names = append(names, m.Name)
		}
	}
	return names
}

func TestRecordingFunctionsAreInertBeforeInit(t *testing.T) {
	// active is nil at this point (no prior test left it set, per
	// withInstruments' cleanup) - these must not panic.
	RecordReconcile(context.Background(), "tenant", "success", 0.01)
	RecordJobStarted(context.Background(), "acme", 2)
	RecordJobFinished(context.Background(), "acme", "succeeded", 1.5, true, 2)
}

func TestRecordReconcileEmitsCounterAndHistogram(t *testing.T) {
	reader := withInstruments(t)

	RecordReconcile(context.Background(), "agentjob", "success", 0.25)

	names := metricNames(collect(t, reader))
	want := map[string]bool{"muto.reconciliations.total": false, "muto.reconciliation.duration": false}
	for _, n := range names {
		if _, ok := want[n]; ok {
			want[n] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("expected metric %q to be recorded, got %v", name, names)
		}
	}
}

func TestRecordJobLifecycleEmitsAllFamilies(t *testing.T) {
	reader := withInstruments(t)

	RecordJobStarted(context.Background(), "acme", 3)
	RecordJobFinished(context.Background(), "acme", "succeeded", 5.0, true, 3)

	names := metricNames(collect(t, reader))
	want := []string{"muto.agents.running", "muto.job.queue_depth", "muto.jobs.total", "muto.job.duration"}
	for _, name := range want {
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected metric %q to be recorded, got %v", name, names)
		}
	}
}

func TestRecordJobFinishedRecordsZeroDurationWhenKnown(t *testing.T) {
	reader := withInstruments(t)

	// hasDuration=true with durationSec==0 models a genuinely-instantaneous
	// job (startedAt was known), which must still be observed - mirroring
	// platform/k8s/metrics.JobFinished, which always calls
	// JobDuration.Observe once startedAt is non-zero, regardless of value.
	RecordJobFinished(context.Background(), "acme", "succeeded", 0, true, 1)

	found := false
	for _, name := range metricNames(collect(t, reader)) {
		if name == "muto.job.duration" {
			found = true
		}
	}
	if !found {
		t.Error("expected muto.job.duration to be recorded for a known zero duration, but it was skipped")
	}
}

func TestRecordJobFinishedSkipsDurationWhenUnknown(t *testing.T) {
	reader := withInstruments(t)

	// hasDuration=false models an unknown startedAt, which must not record
	// a duration observation at all - mirroring JobFinished, which skips
	// JobDuration.Observe entirely when startedAt.IsZero().
	RecordJobFinished(context.Background(), "acme", "succeeded", 0, false, 1)

	for _, name := range metricNames(collect(t, reader)) {
		if name == "muto.job.duration" {
			t.Errorf("expected muto.job.duration to be skipped for an unknown duration, but it was recorded")
		}
	}
}

func TestInitNoopWhenEndpointUnset(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", "")
	t.Cleanup(func() { active.Store(nil) })

	shutdown, err := Init(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init returned a nil shutdown func")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("no-op shutdown returned error: %v", err)
	}
	if active.Load() != nil {
		t.Error("Init populated active instruments even though OTLP export is disabled")
	}

	// Recording must still be inert: no instruments were created.
	RecordJobStarted(context.Background(), "acme", 1)
}

func TestInitBuildsProviderWhenEndpointSet(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")
	t.Cleanup(func() { active.Store(nil) })

	shutdown, err := Init(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	if active.Load() == nil {
		t.Fatal("Init did not populate active instruments despite OTLP export being enabled")
	}
}
