// SPDX-License-Identifier: Apache-2.0
package tracing_test

import (
	"context"
	"errors"
	"testing"

	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/tracing"
)

type stubAdapter struct {
	spawnErr error
}

func (s *stubAdapter) SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error) {
	if s.spawnErr != nil {
		return "", s.spawnErr
	}
	return "agent-1", nil
}

func (s *stubAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	return nil
}

func (s *stubAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event)
	close(ch)
	return ch, nil
}

func TestWrapPlatformAdapterSpawnAgent(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapPlatformAdapter(&stubAdapter{})
	id, err := wrapped.SpawnAgent(context.Background(), &agent.Spec{})
	if err != nil || id != "agent-1" {
		t.Fatalf("SpawnAgent: got (%q, %v), want (\"agent-1\", nil)", id, err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "PlatformAdapter.SpawnAgent" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "PlatformAdapter.SpawnAgent")
	}
}

func TestWrapPlatformAdapterSpawnAgentError(t *testing.T) {
	sr := withRecorder(t)

	wantErr := errors.New("spawn failed")
	wrapped := tracing.WrapPlatformAdapter(&stubAdapter{spawnErr: wantErr})
	_, err := wrapped.SpawnAgent(context.Background(), &agent.Spec{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SpawnAgent returned %v, want %v", err, wantErr)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
}

func TestWrapPlatformAdapterTerminateAndWatch(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapPlatformAdapter(&stubAdapter{})
	if err := wrapped.TerminateAgent(context.Background(), "agent-1"); err != nil {
		t.Fatalf("TerminateAgent: %v", err)
	}
	if _, err := wrapped.WatchAgent(context.Background(), "agent-1"); err != nil {
		t.Fatalf("WatchAgent: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}
	names := map[string]bool{spans[0].Name(): true, spans[1].Name(): true}
	if !names["PlatformAdapter.TerminateAgent"] || !names["PlatformAdapter.WatchAgent"] {
		t.Errorf("unexpected span names: %v, %v", spans[0].Name(), spans[1].Name())
	}
}

type stubScheduler struct {
	scheduleErr error
}

func (s *stubScheduler) Schedule(ctx context.Context, job *agent.Job) error {
	return s.scheduleErr
}

func (s *stubScheduler) Cancel(ctx context.Context, jobID string) error {
	return nil
}

func (s *stubScheduler) Status(ctx context.Context, jobID string) (*agent.Status, error) {
	return &agent.Status{}, nil
}

func (s *stubScheduler) ListActive(ctx context.Context, tenantID string) ([]*agent.Job, error) {
	return nil, nil
}

func TestWrapSchedulerSchedule(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapScheduler(&stubScheduler{})
	if err := wrapped.Schedule(context.Background(), &agent.Job{}); err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "Scheduler.Schedule" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "Scheduler.Schedule")
	}
}

func TestWrapSchedulerAllMethodsProduceSpans(t *testing.T) {
	sr := withRecorder(t)

	wrapped := tracing.WrapScheduler(&stubScheduler{})
	_ = wrapped.Cancel(context.Background(), "job-1")
	_, _ = wrapped.Status(context.Background(), "job-1")
	_, _ = wrapped.ListActive(context.Background(), "tenant-1")

	spans := sr.Ended()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans, got %d", len(spans))
	}
}
