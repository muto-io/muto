package scheduler_test

import (
	"testing"
	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/core/scheduler"
)

func TestTransitionPendingToRunning(t *testing.T) {
	next, err := scheduler.Transition(agent.PhasePending, scheduler.EventSpawned)
	if err != nil {
		t.Fatal(err)
	}
	if next != agent.PhaseRunning {
		t.Errorf("expected Running, got %s", next)
	}
}

func TestTransitionRunningToSucceeded(t *testing.T) {
	next, err := scheduler.Transition(agent.PhaseRunning, scheduler.EventAllComplete)
	if err != nil {
		t.Fatal(err)
	}
	if next != agent.PhaseSucceeded {
		t.Errorf("expected Succeeded, got %s", next)
	}
}

func TestTransitionRunningToFailed(t *testing.T) {
	next, err := scheduler.Transition(agent.PhaseRunning, scheduler.EventAgentFailed)
	if err != nil {
		t.Fatal(err)
	}
	if next != agent.PhaseFailed {
		t.Errorf("expected Failed, got %s", next)
	}
}

func TestTransitionToTerminating(t *testing.T) {
	for _, from := range []agent.Phase{agent.PhaseSucceeded, agent.PhaseFailed} {
		next, err := scheduler.Transition(from, scheduler.EventTTLExpired)
		if err != nil {
			t.Fatalf("from %s: %v", from, err)
		}
		if next != agent.PhaseTerminating {
			t.Errorf("from %s: expected Terminating, got %s", from, next)
		}
	}
}

func TestInvalidTransition(t *testing.T) {
	_, err := scheduler.Transition(agent.PhaseSucceeded, scheduler.EventSpawned)
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}
