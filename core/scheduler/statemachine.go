// SPDX-License-Identifier: Apache-2.0
package scheduler

import (
	"fmt"
	"github.com/muto-io/muto/core/agent"
)

type Event string

const (
	EventSpawned     Event = "Spawned"
	EventAllComplete Event = "AllComplete"
	EventAgentFailed Event = "AgentFailed"
	EventTTLExpired  Event = "TTLExpired"
	EventCancelled   Event = "Cancelled"
)

type transition struct {
	from  agent.Phase
	event Event
}

var transitions = map[transition]agent.Phase{
	{agent.PhasePending, EventSpawned}:      agent.PhaseRunning,
	{agent.PhaseRunning, EventAllComplete}:  agent.PhaseSucceeded,
	{agent.PhaseRunning, EventAgentFailed}:  agent.PhaseFailed,
	{agent.PhaseRunning, EventCancelled}:    agent.PhaseTerminating,
	{agent.PhaseSucceeded, EventTTLExpired}: agent.PhaseTerminating,
	{agent.PhaseFailed, EventTTLExpired}:    agent.PhaseTerminating,
	{agent.PhaseSucceeded, EventCancelled}:  agent.PhaseTerminating,
	{agent.PhaseFailed, EventCancelled}:     agent.PhaseTerminating,
}

func Transition(current agent.Phase, event Event) (agent.Phase, error) {
	next, ok := transitions[transition{current, event}]
	if !ok {
		return current, fmt.Errorf("invalid transition: %s + %s", current, event)
	}
	return next, nil
}
