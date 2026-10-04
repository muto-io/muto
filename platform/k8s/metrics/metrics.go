// SPDX-License-Identifier: Apache-2.0
package metrics

import (
	"context"
	"time"

	coremetrics "github.com/muto-io/muto/core/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var factory = promauto.With(ctrlmetrics.Registry)

var (
	ReconciliationsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "muto_reconciliations_total",
		Help: "Total number of reconcile calls, by reconciler and result.",
	}, []string{"reconciler", "result"})

	ReconciliationDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muto_reconciliation_duration_seconds",
		Help:    "Duration of reconcile calls in seconds, by reconciler.",
		Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10},
	}, []string{"reconciler"})

	JobsTotal = factory.NewCounterVec(prometheus.CounterOpts{
		Name: "muto_jobs_total",
		Help: "Total number of AgentJobs that reached a terminal state, by tenant and status.",
	}, []string{"tenant", "status"})

	JobDuration = factory.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "muto_job_duration_seconds",
		Help:    "Duration of AgentJobs from start to completion in seconds, by tenant and status.",
		Buckets: []float64{1, 5, 15, 30, 60, 300, 900, 3600},
	}, []string{"tenant", "status"})

	AgentsRunning = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "muto_agents_running",
		Help: "Number of agent pods currently running, by tenant.",
	}, []string{"tenant"})

	JobQueueDepth = factory.NewGaugeVec(prometheus.GaugeOpts{
		Name: "muto_job_queue_depth",
		Help: "Number of AgentJobs currently in the Running phase, by tenant.",
	}, []string{"tenant"})
)

// ObserveReconcile runs fn, recording its duration and result on
// ReconciliationDuration and ReconciliationsTotal under the given
// reconciler label. It also mirrors the observation to core/metrics, which
// exports it via OTLP when that's enabled - the only option for platforms
// with no Prometheus scraping (e.g. CF).
func ObserveReconcile(reconciler string, fn func() (ctrl.Result, error)) (ctrl.Result, error) {
	start := time.Now()
	result, err := fn()
	duration := time.Since(start).Seconds()
	ReconciliationDuration.WithLabelValues(reconciler).Observe(duration)
	status := "success"
	if err != nil {
		status = "error"
	}
	ReconciliationsTotal.WithLabelValues(reconciler, status).Inc()
	coremetrics.RecordReconcile(context.Background(), reconciler, status, duration)
	return result, err
}

// JobStarted records that agentCount agent pods were just created for an
// AgentJob belonging to tenant.
func JobStarted(tenant string, agentCount int32) {
	AgentsRunning.WithLabelValues(tenant).Add(float64(agentCount))
	JobQueueDepth.WithLabelValues(tenant).Inc()
	coremetrics.RecordJobStarted(context.Background(), tenant, agentCount)
}

// JobFinished records that an AgentJob belonging to tenant reached a
// terminal state.
func JobFinished(tenant, status string, startedAt time.Time, activeAgents int32) {
	JobsTotal.WithLabelValues(tenant, status).Inc()
	var duration float64
	hasDuration := !startedAt.IsZero()
	if hasDuration {
		duration = time.Since(startedAt).Seconds()
		JobDuration.WithLabelValues(tenant, status).Observe(duration)
	}
	AgentsRunning.WithLabelValues(tenant).Sub(float64(activeAgents))
	JobQueueDepth.WithLabelValues(tenant).Dec()
	coremetrics.RecordJobFinished(context.Background(), tenant, status, duration, hasDuration, activeAgents)
}
