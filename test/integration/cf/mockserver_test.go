package cf_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cloudfoundry/go-cfclient/v3/resource"
)

// MockCFServer simulates a Cloud Foundry API server for testing.
type MockCFServer struct {
	server *httptest.Server
	URL    string

	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	orgs      map[string]*resource.Organization
	spaces    map[string]*resource.Space
	apps      map[string]*resource.App
	tasks     map[string]*resource.Task
	taskCounter int
}

// NewMockCFServer creates a new mock CF API server.
func NewMockCFServer() *MockCFServer {
	ctx, cancel := context.WithCancel(context.Background())
	mcs := &MockCFServer{
		ctx:    ctx,
		cancel: cancel,
		orgs:   make(map[string]*resource.Organization),
		spaces: make(map[string]*resource.Space),
		apps:   make(map[string]*resource.App),
		tasks:  make(map[string]*resource.Task),
	}

	mcs.server = httptest.NewServer(mcs.handler())
	mcs.URL = mcs.server.URL
	return mcs
}

// Close stops the mock server and cancels all task transition goroutines.
func (mcs *MockCFServer) Close() {
	mcs.cancel()
	mcs.server.Close()
}

// handler returns the HTTP handler for the mock server.
func (mcs *MockCFServer) handler() http.Handler {
	mux := http.NewServeMux()

	// Organizations endpoints
	mux.HandleFunc("GET /v3/organizations", mcs.listOrganizations)
	mux.HandleFunc("POST /v3/organizations", mcs.createOrganization)

	// Spaces endpoints
	mux.HandleFunc("GET /v3/spaces", mcs.listSpaces)
	mux.HandleFunc("POST /v3/spaces", mcs.createSpace)

	// Apps endpoints
	mux.HandleFunc("GET /v3/apps", mcs.listApps)
	mux.HandleFunc("POST /v3/apps", mcs.createApp)
	mux.HandleFunc("PATCH /v3/apps/{guid}", mcs.updateApp)

	// Tasks endpoints
	mux.HandleFunc("GET /v3/tasks", mcs.listTasks)
	mux.HandleFunc("POST /v3/apps/{appGUID}/tasks", mcs.createTask)
	mux.HandleFunc("GET /v3/tasks/{taskGUID}", mcs.getTask)
	mux.HandleFunc("POST /v3/tasks/{taskGUID}/cancel", mcs.cancelTask)

	return mux
}

// listOrganizations returns paginated organizations.
func (mcs *MockCFServer) listOrganizations(w http.ResponseWriter, r *http.Request) {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()

	orgs := make([]*resource.Organization, 0)
	for _, org := range mcs.orgs {
		orgs = append(orgs, org)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"resources": orgs,
	})
}

// createOrganization creates a new organization.
func (mcs *MockCFServer) createOrganization(w http.ResponseWriter, r *http.Request) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	org := &resource.Organization{
		Name: req.Name,
		Resource: resource.Resource{
			GUID: fmt.Sprintf("org-%d", time.Now().UnixNano()),
		},
	}
	mcs.orgs[org.GUID] = org

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(org)
}

// listSpaces returns paginated spaces.
func (mcs *MockCFServer) listSpaces(w http.ResponseWriter, r *http.Request) {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()

	spaces := make([]*resource.Space, 0)
	for _, space := range mcs.spaces {
		spaces = append(spaces, space)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"resources": spaces,
	})
}

// createSpace creates a new space.
func (mcs *MockCFServer) createSpace(w http.ResponseWriter, r *http.Request) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	var fullReq struct {
		Name          string `json:"name"`
		Relationships map[string]interface{} `json:"relationships"`
	}
	if err := json.NewDecoder(r.Body).Decode(&fullReq); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract org GUID from relationships
	if rels, ok := fullReq.Relationships["organization"]; ok {
		if relData, ok := rels.(map[string]interface{}); ok {
			if data, ok := relData["data"]; ok {
				if dataObj, ok := data.(map[string]interface{}); ok {
					_ = dataObj["guid"] // org GUID parsed but not used in mock
				}
			}
		}
	}

	space := &resource.Space{
		Name: fullReq.Name,
		Resource: resource.Resource{
			GUID: fmt.Sprintf("space-%d", time.Now().UnixNano()),
		},
	}
	mcs.spaces[space.GUID] = space

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(space)
}

// listApps returns paginated apps.
func (mcs *MockCFServer) listApps(w http.ResponseWriter, r *http.Request) {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()

	// Parse query filters
	names := r.URL.Query()["names"]
	spaceGUIDs := r.URL.Query()["space_guids"]

	apps := make([]*resource.App, 0)
	for _, app := range mcs.apps {
		if len(names) > 0 && !slices.Contains(names, app.Name) {
			continue
		}
		if len(spaceGUIDs) > 0 && !slices.Contains(spaceGUIDs, app.Relationships.Space.Data.GUID) {
			continue
		}
		apps = append(apps, app)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"resources": apps,
	})
}

// createApp creates a new application.
func (mcs *MockCFServer) createApp(w http.ResponseWriter, r *http.Request) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	var req struct {
		Name          string `json:"name"`
		Relationships map[string]interface{} `json:"relationships"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract space GUID
	spaceGUID := ""
	if rels, ok := req.Relationships["space"]; ok {
		if relData, ok := rels.(map[string]interface{}); ok {
			if data, ok := relData["data"]; ok {
				if dataObj, ok := data.(map[string]interface{}); ok {
					if guid, ok := dataObj["guid"].(string); ok {
						spaceGUID = guid
					}
				}
			}
		}
	}

	app := &resource.App{
		Name: req.Name,
		Resource: resource.Resource{
			GUID: fmt.Sprintf("app-%d", time.Now().UnixNano()),
		},
		Relationships: resource.AppRelationships{
			Space: resource.ToOneRelationship{
				Data: &resource.Relationship{GUID: spaceGUID},
			},
		},
	}
	mcs.apps[app.GUID] = app

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(app)
}

// updateApp updates an application.
func (mcs *MockCFServer) updateApp(w http.ResponseWriter, r *http.Request) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	appGUID := r.PathValue("guid")
	app, ok := mcs.apps[appGUID]
	if !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name != "" {
		app.Name = req.Name
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(app)
}

// listTasks returns paginated tasks.
func (mcs *MockCFServer) listTasks(w http.ResponseWriter, r *http.Request) {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()

	tasks := make([]*resource.Task, 0)
	for _, task := range mcs.tasks {
		tasks = append(tasks, task)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"resources": tasks,
	})
}

// createTask creates a new task on an app.
func (mcs *MockCFServer) createTask(w http.ResponseWriter, r *http.Request) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	appGUID := r.PathValue("appGUID")
	if _, ok := mcs.apps[appGUID]; !ok {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var req struct {
		Command string `json:"command"`
		Name    string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mcs.taskCounter++
	task := &resource.Task{
		Name:  req.Name,
		State: "PENDING",
		Resource: resource.Resource{
			GUID: fmt.Sprintf("task-%d", mcs.taskCounter),
		},
		Relationships: resource.AppRelationship{
			App: resource.ToOneRelationship{
				Data: &resource.Relationship{GUID: appGUID},
			},
		},
	}
	mcs.tasks[task.GUID] = task

	// Simulate task running after a short delay (stops if context is cancelled)
	go func(taskGUID string) {
		select {
		case <-time.After(50 * time.Millisecond):
			mcs.mu.Lock()
			if t, ok := mcs.tasks[taskGUID]; ok && t.State == "PENDING" {
				t.State = "RUNNING"
			}
			mcs.mu.Unlock()
		case <-mcs.ctx.Done():
			return
		}

		select {
		case <-time.After(100 * time.Millisecond):
			mcs.mu.Lock()
			if t, ok := mcs.tasks[taskGUID]; ok && t.State == "RUNNING" {
				t.State = "SUCCEEDED"
			}
			mcs.mu.Unlock()
		case <-mcs.ctx.Done():
			return
		}
	}(task.GUID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(task)
}

// getTask retrieves a task by GUID.
func (mcs *MockCFServer) getTask(w http.ResponseWriter, r *http.Request) {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()

	taskGUID := r.PathValue("taskGUID")
	task, ok := mcs.tasks[taskGUID]
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(task)
}

// cancelTask cancels a running task.
func (mcs *MockCFServer) cancelTask(w http.ResponseWriter, r *http.Request) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	taskGUID := r.PathValue("taskGUID")
	task, ok := mcs.tasks[taskGUID]
	if !ok {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	if task.State == "SUCCEEDED" || task.State == "FAILED" {
		http.Error(w, fmt.Sprintf("cannot cancel %s task", strings.ToLower(task.State)), http.StatusUnprocessableEntity)
		return
	}

	task.State = "CANCELING"

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(task)
}

// SetupOrg creates an org and space for testing.
func (mcs *MockCFServer) SetupOrg() (orgGUID, spaceGUID string, err error) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	org := &resource.Organization{
		Name: "test-org",
		Resource: resource.Resource{
			GUID: fmt.Sprintf("org-%d", time.Now().UnixNano()),
		},
	}
	mcs.orgs[org.GUID] = org

	space := &resource.Space{
		Name: "test-space",
		Resource: resource.Resource{
			GUID: fmt.Sprintf("space-%d", time.Now().UnixNano()),
		},
	}
	mcs.spaces[space.GUID] = space

	return org.GUID, space.GUID, nil
}
