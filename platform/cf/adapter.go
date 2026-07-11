package cf

import (
	"context"
	"fmt"

	"github.com/muto-io/muto/core/agent"
)

// CFAdapter implements core/scheduler.PlatformAdapter for Cloud Foundry.
// SpawnAgent maps to `cf push` into a tenant org/space.
// TerminateAgent maps to `cf stop` + `cf delete`.
// Tenant isolation: org = hard isolation, space = soft isolation.
// Not implemented in v1 — returns ErrNotImplemented for all methods.
var ErrNotImplemented = fmt.Errorf("CF adapter not implemented in v1")

type CFAdapter struct {
	APIURL   string
	Username string
	Password string
}

func NewCFAdapter(apiURL, username, password string) *CFAdapter {
	return &CFAdapter{APIURL: apiURL, Username: username, Password: password}
}

func (a *CFAdapter) SpawnAgent(_ context.Context, _ *agent.Spec) (string, error) {
	return "", ErrNotImplemented
}

func (a *CFAdapter) TerminateAgent(_ context.Context, _ string) error {
	return ErrNotImplemented
}

func (a *CFAdapter) WatchAgent(_ context.Context, _ string) (<-chan agent.Event, error) {
	return nil, ErrNotImplemented
}
