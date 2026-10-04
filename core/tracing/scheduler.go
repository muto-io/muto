// SPDX-License-Identifier: Apache-2.0
package tracing

import (
	"context"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
)

type tracedPlatformAdapter struct {
	inner scheduler.PlatformAdapter
}

// WrapPlatformAdapter returns a scheduler.PlatformAdapter that records a
// span around every call to inner.
func WrapPlatformAdapter(inner scheduler.PlatformAdapter) scheduler.PlatformAdapter {
	return &tracedPlatformAdapter{inner: inner}
}

func (t *tracedPlatformAdapter) SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error) {
	return Wrap(ctx, "PlatformAdapter.SpawnAgent", func(ctx context.Context) (string, error) {
		return t.inner.SpawnAgent(ctx, spec)
	})
}

func (t *tracedPlatformAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	return WrapErr(ctx, "PlatformAdapter.TerminateAgent", func(ctx context.Context) error {
		return t.inner.TerminateAgent(ctx, agentID)
	})
}

func (t *tracedPlatformAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	return Wrap(ctx, "PlatformAdapter.WatchAgent", func(ctx context.Context) (<-chan agent.Event, error) {
		return t.inner.WatchAgent(ctx, agentID)
	})
}

type tracedScheduler struct {
	inner scheduler.Scheduler
}

// WrapScheduler returns a scheduler.Scheduler that records a span around
// every call to inner.
func WrapScheduler(inner scheduler.Scheduler) scheduler.Scheduler {
	return &tracedScheduler{inner: inner}
}

func (t *tracedScheduler) Schedule(ctx context.Context, job *agent.Job) error {
	return WrapErr(ctx, "Scheduler.Schedule", func(ctx context.Context) error {
		return t.inner.Schedule(ctx, job)
	})
}

func (t *tracedScheduler) Cancel(ctx context.Context, jobID string) error {
	return WrapErr(ctx, "Scheduler.Cancel", func(ctx context.Context) error {
		return t.inner.Cancel(ctx, jobID)
	})
}

func (t *tracedScheduler) Status(ctx context.Context, jobID string) (*agent.Status, error) {
	return Wrap(ctx, "Scheduler.Status", func(ctx context.Context) (*agent.Status, error) {
		return t.inner.Status(ctx, jobID)
	})
}

func (t *tracedScheduler) ListActive(ctx context.Context, tenantID string) ([]*agent.Job, error) {
	return Wrap(ctx, "Scheduler.ListActive", func(ctx context.Context) ([]*agent.Job, error) {
		return t.inner.ListActive(ctx, tenantID)
	})
}
