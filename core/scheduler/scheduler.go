package scheduler

import (
	"context"
	"fmt"
	"sync"

	"github.com/muto-io/muto/core/agent"
)

type PlatformAdapter interface {
	SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error)
	TerminateAgent(ctx context.Context, agentID string) error
	WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error)
}

type Scheduler interface {
	Schedule(ctx context.Context, job *agent.Job) error
	Cancel(ctx context.Context, jobID string) error
	Status(ctx context.Context, jobID string) (*agent.Status, error)
	ListActive(ctx context.Context, tenantID string) ([]*agent.Job, error)
}

type jobRecord struct {
	job      *agent.Job
	agentIDs []string
}

type DefaultScheduler struct {
	mu      sync.RWMutex
	adapter PlatformAdapter
	jobs    map[string]*jobRecord
}

func NewDefaultScheduler(adapter PlatformAdapter) *DefaultScheduler {
	return &DefaultScheduler{
		adapter: adapter,
		jobs:    make(map[string]*jobRecord),
	}
}

func (s *DefaultScheduler) Schedule(ctx context.Context, job *agent.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var agentIDs []string
	for _, role := range job.Spec.Agents {
		spec := &agent.Spec{
			TenantRef: job.Spec.TenantRef,
			Agents:    []agent.AgentRole{role},
		}
		id, err := s.adapter.SpawnAgent(ctx, spec)
		if err != nil {
			return fmt.Errorf("spawn agent %s: %w", role.Role, err)
		}
		agentIDs = append(agentIDs, id)
	}

	job.Status.Phase = agent.PhaseRunning
	s.jobs[job.ID] = &jobRecord{job: job, agentIDs: agentIDs}
	return nil
}

func (s *DefaultScheduler) Cancel(ctx context.Context, jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job %q not found", jobID)
	}
	for _, id := range rec.agentIDs {
		_ = s.adapter.TerminateAgent(ctx, id)
	}
	next, err := Transition(rec.job.Status.Phase, EventCancelled)
	if err != nil {
		return err
	}
	rec.job.Status.Phase = next
	return nil
}

func (s *DefaultScheduler) Status(_ context.Context, jobID string) (*agent.Status, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.jobs[jobID]
	if !ok {
		return nil, fmt.Errorf("job %q not found", jobID)
	}
	st := rec.job.Status
	return &st, nil
}

func (s *DefaultScheduler) ListActive(_ context.Context, tenantID string) ([]*agent.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*agent.Job
	for _, rec := range s.jobs {
		if rec.job.TenantID == tenantID && !rec.job.Status.Phase.IsTerminal() {
			result = append(result, rec.job)
		}
	}
	return result, nil
}
