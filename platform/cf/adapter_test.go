package cf_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/muto-io/muto/core/agent"
	"github.com/muto-io/muto/platform/cf"
)

type mockCFClient struct {
	apps      map[string]*resource.App
	tasks     map[string]*resource.Task
	orgs      map[string]*resource.Organization
	spaces    map[string]*resource.Space
	pushCalls int
	taskCalls int
	orgCalls  int
}

func newMockCFClient() *mockCFClient {
	return &mockCFClient{
		apps:   make(map[string]*resource.App),
		tasks:  make(map[string]*resource.Task),
		orgs:   make(map[string]*resource.Organization),
		spaces: make(map[string]*resource.Space),
	}
}

func (m *mockCFClient) GetOrgByName(_ context.Context, name string) (*resource.Organization, error) {
	m.orgCalls++
	if o, ok := m.orgs[name]; ok {
		return o, nil
	}
	return nil, fmt.Errorf("org %q not found", name)
}

func (m *mockCFClient) GetSpaceByName(_ context.Context, orgGUID, name string) (*resource.Space, error) {
	key := orgGUID + ":" + name
	if s, ok := m.spaces[key]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("space %q not found", name)
}

func (m *mockCFClient) GetAppByName(_ context.Context, name, spaceGUID string) (*resource.App, error) {
	key := name + ":" + spaceGUID
	if a, ok := m.apps[key]; ok {
		return a, nil
	}
	return nil, fmt.Errorf("app %q not found", name)
}

func (m *mockCFClient) PushApp(_ context.Context, req cf.PushRequest) (*resource.App, error) {
	m.pushCalls++
	app := &resource.App{Name: req.Name}
	app.GUID = "app-" + req.Name
	m.apps[req.Name+":"+req.SpaceGUID] = app
	return app, nil
}

func (m *mockCFClient) RunTask(_ context.Context, _ string, req cf.TaskRequest) (*resource.Task, error) {
	m.taskCalls++
	task := &resource.Task{Name: req.Name, State: "RUNNING"}
	task.GUID = "task-" + req.Name
	m.tasks[task.GUID] = task
	return task, nil
}

func (m *mockCFClient) GetTask(_ context.Context, taskGUID string) (*resource.Task, error) {
	if t, ok := m.tasks[taskGUID]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("task %q not found", taskGUID)
}

func (m *mockCFClient) CancelTask(_ context.Context, taskGUID string) error {
	if t, ok := m.tasks[taskGUID]; ok {
		t.State = "CANCELING"
		return nil
	}
	return fmt.Errorf("task %q not found", taskGUID)
}

func setupMock() (*mockCFClient, *cf.CFAdapter) {
	mock := newMockCFClient()
	org := &resource.Organization{Name: "acme"}
	org.GUID = "org-acme"
	mock.orgs["acme"] = org
	space := &resource.Space{Name: "muto-agents"}
	space.GUID = "space-acme"
	mock.spaces["org-acme:muto-agents"] = space

	adapter := cf.NewCFAdapter(mock, cf.CFAdapterConfig{
		IsolationTier: "dedicated",
	})
	return mock, adapter
}

func TestSpawnAgentRunnerExists(t *testing.T) {
	mock, adapter := setupMock()
	existing := &resource.App{Name: "muto-acme-worker"}
	existing.GUID = "app-existing"
	mock.apps["muto-acme-worker:space-acme"] = existing

	spec := &agent.Spec{
		TenantRef:  "acme",
		Agents:     []agent.AgentRole{{Role: "worker", Command: "run-agent", MaxReplicas: 1}},
		MessageBus: agent.MessageBusConfig{Topic: "tenant.acme.job1"},
	}
	id, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Error("expected non-empty task GUID")
	}
	if mock.pushCalls != 0 {
		t.Errorf("expected no push, got %d", mock.pushCalls)
	}
	if mock.taskCalls != 1 {
		t.Errorf("expected 1 task, got %d", mock.taskCalls)
	}
}

func TestSpawnAgentRunnerMissing(t *testing.T) {
	mock, adapter := setupMock()

	spec := &agent.Spec{
		TenantRef:  "acme",
		Agents:     []agent.AgentRole{{Role: "worker", Command: "run-agent", MaxReplicas: 1}},
		MessageBus: agent.MessageBusConfig{Topic: "tenant.acme.job1"},
	}
	_, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if mock.pushCalls != 1 {
		t.Errorf("expected 1 push, got %d", mock.pushCalls)
	}
	if mock.taskCalls != 1 {
		t.Errorf("expected 1 task, got %d", mock.taskCalls)
	}
}

func TestWatchAgentSucceeded(t *testing.T) {
	mock, adapter := setupMock()
	task := &resource.Task{State: "SUCCEEDED"}
	task.GUID = "task-done"
	mock.tasks["task-done"] = task

	ch, err := adapter.WatchAgent(context.Background(), "task-done")
	if err != nil {
		t.Fatal(err)
	}
	ev := <-ch
	if ev.Type != agent.EventCompleted {
		t.Errorf("expected EventCompleted, got %v", ev.Type)
	}
}

func TestWatchAgentFailed(t *testing.T) {
	mock, adapter := setupMock()
	task := &resource.Task{State: "FAILED"}
	task.GUID = "task-fail"
	mock.tasks["task-fail"] = task

	ch, err := adapter.WatchAgent(context.Background(), "task-fail")
	if err != nil {
		t.Fatal(err)
	}
	ev := <-ch
	if ev.Type != agent.EventFailed {
		t.Errorf("expected EventFailed, got %v", ev.Type)
	}
}

func TestWatchAgentContextCancel(t *testing.T) {
	mock, adapter := setupMock()
	task := &resource.Task{State: "RUNNING"}
	task.GUID = "task-running"
	mock.tasks["task-running"] = task

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := adapter.WatchAgent(ctx, "task-running")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Error("channel did not close after context cancel")
	}
}

func TestTerminateAgentHappyPath(t *testing.T) {
	mock, adapter := setupMock()
	task := &resource.Task{State: "RUNNING"}
	task.GUID = "task-alive"
	mock.tasks["task-alive"] = task

	if err := adapter.TerminateAgent(context.Background(), "task-alive"); err != nil {
		t.Fatal(err)
	}
	if mock.tasks["task-alive"].State != "CANCELING" {
		t.Errorf("expected CANCELING, got %s", mock.tasks["task-alive"].State)
	}
}

func TestTerminateAgentAlreadyDone(t *testing.T) {
	_, adapter := setupMock()
	err := adapter.TerminateAgent(context.Background(), "nonexistent-task")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestGUIDCacheHit(t *testing.T) {
	mock, adapter := setupMock()
	existing := &resource.App{Name: "muto-acme-worker"}
	existing.GUID = "app-existing"
	mock.apps["muto-acme-worker:space-acme"] = existing

	spec := &agent.Spec{
		TenantRef:  "acme",
		Agents:     []agent.AgentRole{{Role: "worker", Command: "run-agent"}},
		MessageBus: agent.MessageBusConfig{Topic: "tenant.acme.j1"},
	}
	_, err := adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	firstOrgCalls := mock.orgCalls

	spec.MessageBus.Topic = "tenant.acme.j2"
	_, err = adapter.SpawnAgent(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if mock.orgCalls != firstOrgCalls {
		t.Errorf("expected no additional org lookups on cache hit, got %d extra", mock.orgCalls-firstOrgCalls)
	}
}
