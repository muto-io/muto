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
	job         *agent.Job
	agentIDs    []string
	cancelWatch context.CancelFunc // stops all WatchAgent goroutines for this job
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
	// Spawn agents outside the lock — I/O must not block other scheduler operations.
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

	// context.Background() — watch goroutines must outlive the request context.
	watchCtx, cancelWatch := context.WithCancel(context.Background())
	var watchChans []<-chan agent.Event
	for _, id := range agentIDs {
		ch, err := s.adapter.WatchAgent(watchCtx, id)
		if err != nil {
			cancelWatch()
			return fmt.Errorf("watch agent %s: %w", id, err)
		}
		watchChans = append(watchChans, ch)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	job.Status.Phase = agent.PhaseRunning
	s.jobs[job.ID] = &jobRecord{job: job, agentIDs: agentIDs, cancelWatch: cancelWatch}
	go s.watchJob(job.ID, watchChans)
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
		if err := s.adapter.TerminateAgent(ctx, id); err != nil {
			return fmt.Errorf("terminate agent %s: %w", id, err)
		}
	}
	rec.cancelWatch()
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
			// Return a copy to avoid races with concurrent writers.
			jobCopy := *rec.job
			result = append(result, &jobCopy)
		}
	}
	return result, nil
}

// watchJob fans in all per-agent event channels and transitions the job's
// in-memory phase once all agents have reported a terminal event.
func (s *DefaultScheduler) watchJob(jobID string, chans []<-chan agent.Event) {
	total := len(chans)
	results := make(chan agent.Event, total*2)
	for _, ch := range chans {
		go func(c <-chan agent.Event) {
			for ev := range c {
				results <- ev
			}
		}(ch)
	}
	completed, failed := 0, 0
	for completed+failed < total {
		ev, ok := <-results
		if !ok {
			return
		}
		switch ev.Type {
		case agent.EventCompleted:
			completed++
		case agent.EventFailed:
			failed++
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.jobs[jobID]
	if !ok {
		return
	}
	if failed > 0 {
		rec.job.Status.Phase = agent.PhaseFailed
	} else {
		rec.job.Status.Phase = agent.PhaseSucceeded
	}
}
