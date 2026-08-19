package cf

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/muto-io/muto/core/agent"
)

// CFAdapterConfig holds isolation settings for the CF adapter.
type CFAdapterConfig struct {
	IsolationTier string // "shared" | "dedicated"
	SharedOrgName string // required when IsolationTier == "shared"
}

// CFAdapter implements core/scheduler.PlatformAdapter for Cloud Foundry.
type CFAdapter struct {
	client CFClient
	config CFAdapterConfig
	cache  *guidCache
}

// NewCFAdapter creates a CFAdapter using the provided CFClient and config.
func NewCFAdapter(client CFClient, config CFAdapterConfig) *CFAdapter {
	return &CFAdapter{
		client: client,
		config: config,
		cache:  newGUIDCache(5 * time.Minute),
	}
}

// SpawnAgent resolves the tenant's CF space, ensures a runner app exists, then
// starts a one-off task on that app.
func (a *CFAdapter) SpawnAgent(ctx context.Context, spec *agent.Spec) (string, error) {
	if len(spec.Agents) == 0 {
		return "", fmt.Errorf("no agents in spec")
	}
	role := spec.Agents[0]

	spaceGUID, err := a.resolveSpaceGUID(ctx, spec.TenantRef)
	if err != nil {
		return "", fmt.Errorf("resolve space: %w", err)
	}

	runnerName := fmt.Sprintf("muto-%s-%s", spec.TenantRef, role.Role)
	app, err := a.ensureRunnerApp(ctx, runnerName, spaceGUID, spec.TenantRef, role.Role)
	if err != nil {
		return "", fmt.Errorf("ensure runner app: %w", err)
	}

	task, err := a.client.RunTask(ctx, app.GUID, TaskRequest{
		Name:    spec.TenantRef + "-" + role.Role,
		Command: role.Command,
	})
	if err != nil {
		return "", fmt.Errorf("run task: %w", err)
	}
	return task.GUID, nil
}

// TerminateAgent cancels the CF task identified by agentID. Errors from
// already-terminal tasks are silently swallowed.
func (a *CFAdapter) TerminateAgent(ctx context.Context, agentID string) error {
	err := a.client.CancelTask(ctx, agentID)
	if err != nil && isTerminalTaskError(err) {
		return nil
	}
	return err
}

// WatchAgent polls the CF task state and emits a single event when the task
// reaches a terminal state, then closes the channel. Cancelling ctx causes the
// channel to close without an event.
func (a *CFAdapter) WatchAgent(ctx context.Context, agentID string) (<-chan agent.Event, error) {
	ch := make(chan agent.Event, 4)
	go func() {
		defer close(ch)
		for {
			task, err := a.client.GetTask(ctx, agentID)
			if err != nil {
				// Don't report context cancellation as a failure
				if errors.Is(err, context.Canceled) {
					return
				}
				ch <- agent.Event{AgentID: agentID, Type: agent.EventFailed, Message: err.Error()}
				return
			}
			switch task.State {
			case "SUCCEEDED":
				ch <- agent.Event{AgentID: agentID, Type: agent.EventCompleted}
				return
			case "FAILED", "CANCELING":
				ch <- agent.Event{AgentID: agentID, Type: agent.EventFailed}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
		}
	}()
	return ch, nil
}

// resolveSpaceGUID maps a tenant identifier to a CF space GUID, using the GUID
// cache to avoid redundant API calls.
func (a *CFAdapter) resolveSpaceGUID(ctx context.Context, tenantID string) (string, error) {
	var orgName, spaceName string
	if a.config.IsolationTier == "dedicated" {
		orgName = tenantID
		spaceName = "muto-agents"
	} else {
		orgName = a.config.SharedOrgName
		spaceName = tenantID
	}

	orgKey := orgCacheKey(orgName)
	orgGUID, ok := a.cache.get(orgKey)
	if !ok {
		org, err := a.client.GetOrgByName(ctx, orgName)
		if err != nil {
			return "", fmt.Errorf("get org %q: %w", orgName, err)
		}
		orgGUID = org.GUID
		a.cache.set(orgKey, orgGUID)
	}

	spaceKey := spaceCacheKey(orgGUID, spaceName)
	spaceGUID, ok := a.cache.get(spaceKey)
	if !ok {
		space, err := a.client.GetSpaceByName(ctx, orgGUID, spaceName)
		if err != nil {
			return "", fmt.Errorf("get space %q: %w", spaceName, err)
		}
		spaceGUID = space.GUID
		a.cache.set(spaceKey, spaceGUID)
	}
	return spaceGUID, nil
}

type cfApp struct{ GUID string }

// ensureRunnerApp returns a handle to the named CF app, pushing it first if it
// does not yet exist in the given space.
func (a *CFAdapter) ensureRunnerApp(ctx context.Context, name, spaceGUID, tenantID, role string) (*cfApp, error) {
	app, err := a.client.GetAppByName(ctx, name, spaceGUID)
	if err == nil {
		return &cfApp{GUID: app.GUID}, nil
	}
	pushed, err := a.client.PushApp(ctx, PushRequest{
		Name:      name,
		SpaceGUID: spaceGUID,
		EnvVars:   map[string]string{"MUTO_TENANT": tenantID, "MUTO_ROLE": role},
	})
	if err != nil {
		return nil, fmt.Errorf("push runner app: %w", err)
	}
	return &cfApp{GUID: pushed.GUID}, nil
}

// isTerminalTaskError returns true when the error message indicates the task
// has already reached a terminal state and cancellation is a no-op.
func isTerminalTaskError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "succeeded") || strings.Contains(msg, "failed")
}
