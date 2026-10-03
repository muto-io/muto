// SPDX-License-Identifier: Apache-2.0
package metrics_test

import (
	"errors"
	"testing"
	"time"

	"github.com/muto-io/muto/platform/k8s/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	ctrl "sigs.k8s.io/controller-runtime"
)

// histogramSampleCount returns how many observations a HistogramVec's
// specific label combination has recorded. testutil.ToFloat64 only supports
// Gauge/Counter/Untyped (it panics on a Histogram), so this reads the
// observation count via the metric's protobuf Write method instead.
func histogramSampleCount(t *testing.T, o prometheus.Observer) uint64 {
	t.Helper()
	m, ok := o.(prometheus.Metric)
	if !ok {
		t.Fatalf("observer %T does not implement prometheus.Metric", o)
	}
	pb := &dto.Metric{}
	if err := m.Write(pb); err != nil {
		t.Fatalf("write metric: %v", err)
	}
	return pb.GetHistogram().GetSampleCount()
}

func TestObserveReconcileSuccess(t *testing.T) {
	const reconciler = "test-observe-success"

	_, err := metrics.ObserveReconcile(reconciler, func() (ctrl.Result, error) {
		return ctrl.Result{}, nil
	})
	if err != nil {
		t.Fatalf("ObserveReconcile: %v", err)
	}

	if got := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues(reconciler, "success")); got != 1 {
		t.Errorf("ReconciliationsTotal{%s,success} = %v, want 1", reconciler, got)
	}
	if got := histogramSampleCount(t, metrics.ReconciliationDuration.WithLabelValues(reconciler)); got != 1 {
		t.Errorf("ReconciliationDuration{%s} sample count = %d, want 1", reconciler, got)
	}
}

func TestObserveReconcileError(t *testing.T) {
	const reconciler = "test-observe-error"
	wantErr := errors.New("boom")

	_, err := metrics.ObserveReconcile(reconciler, func() (ctrl.Result, error) {
		return ctrl.Result{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("ObserveReconcile returned %v, want %v", err, wantErr)
	}

	if got := testutil.ToFloat64(metrics.ReconciliationsTotal.WithLabelValues(reconciler, "error")); got != 1 {
		t.Errorf("ReconciliationsTotal{%s,error} = %v, want 1", reconciler, got)
	}
}

func TestJobStartedIncrementsGauges(t *testing.T) {
	const tenant = "test-job-started"

	metrics.JobStarted(tenant, 3)

	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues(tenant)); got != 3 {
		t.Errorf("AgentsRunning{%s} = %v, want 3", tenant, got)
	}
	if got := testutil.ToFloat64(metrics.JobQueueDepth.WithLabelValues(tenant)); got != 1 {
		t.Errorf("JobQueueDepth{%s} = %v, want 1", tenant, got)
	}
}

func TestJobFinishedRecordsOutcomeAndDrainsGauges(t *testing.T) {
	const tenant = "test-job-finished"

	metrics.JobStarted(tenant, 2)
	startedAt := time.Now().Add(-10 * time.Second)
	metrics.JobFinished(tenant, "succeeded", startedAt, 2)

	if got := testutil.ToFloat64(metrics.JobsTotal.WithLabelValues(tenant, "succeeded")); got != 1 {
		t.Errorf("JobsTotal{%s,succeeded} = %v, want 1", tenant, got)
	}
	if got := histogramSampleCount(t, metrics.JobDuration.WithLabelValues(tenant, "succeeded")); got != 1 {
		t.Errorf("JobDuration{%s,succeeded} sample count = %d, want 1", tenant, got)
	}
	if got := testutil.ToFloat64(metrics.AgentsRunning.WithLabelValues(tenant)); got != 0 {
		t.Errorf("AgentsRunning{%s} = %v, want 0 (drained back to baseline)", tenant, got)
	}
	if got := testutil.ToFloat64(metrics.JobQueueDepth.WithLabelValues(tenant)); got != 0 {
		t.Errorf("JobQueueDepth{%s} = %v, want 0 (drained back to baseline)", tenant, got)
	}
}
