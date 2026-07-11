package agent_test

import (
	"testing"
	"github.com/muto-io/muto/core/agent"
)

func TestPhaseIsTerminal(t *testing.T) {
	if agent.PhaseSucceeded.IsTerminal() != true {
		t.Error("Succeeded should be terminal")
	}
	if agent.PhaseFailed.IsTerminal() != true {
		t.Error("Failed should be terminal")
	}
	if agent.PhaseRunning.IsTerminal() != false {
		t.Error("Running should not be terminal")
	}
	if agent.PhasePending.IsTerminal() != false {
		t.Error("Pending should not be terminal")
	}
}
