package cf_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cloudfoundry/go-cfclient/v3/resource"
	"github.com/muto-io/muto/platform/cf"
)

// MockCFClient wraps the MockCFServer and implements the cf.CFClient interface.
type MockCFClient struct {
	baseURL string
	client  *http.Client
}

// NewMockCFClient creates a client that talks to the mock CF server.
func NewMockCFClient(serverURL string) *MockCFClient {
	return &MockCFClient{
		baseURL: serverURL,
		client:  &http.Client{},
	}
}

// GetOrgByName returns the first organization matching the given name.
// If the org doesn't exist, it creates it (for testing).
func (c *MockCFClient) GetOrgByName(ctx context.Context, name string) (*resource.Organization, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v3/organizations", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list organizations: status %d", resp.StatusCode)
	}

	var result struct {
		Resources []*resource.Organization `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	for _, org := range result.Resources {
		if org.Name == name {
			return org, nil
		}
	}

	// Create the org if it doesn't exist
	body := map[string]interface{}{"name": name}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	createReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v3/organizations", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := c.client.Do(createReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = createResp.Body.Close() }()

	var org resource.Organization
	if err := json.NewDecoder(createResp.Body).Decode(&org); err != nil {
		return nil, err
	}

	return &org, nil
}

// GetSpaceByName returns the first space matching name inside the given org.
// If the space doesn't exist, it creates it (for testing).
func (c *MockCFClient) GetSpaceByName(ctx context.Context, orgGUID, name string) (*resource.Space, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v3/spaces", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list spaces: status %d", resp.StatusCode)
	}

	var result struct {
		Resources []*resource.Space `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	for _, space := range result.Resources {
		if space.Name == name {
			return space, nil
		}
	}

	// Create the space if it doesn't exist
	body := map[string]interface{}{
		"name": name,
		"relationships": map[string]interface{}{
			"organization": map[string]interface{}{
				"data": map[string]string{
					"guid": orgGUID,
				},
			},
		},
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	createReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v3/spaces", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := c.client.Do(createReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = createResp.Body.Close() }()

	var space resource.Space
	if err := json.NewDecoder(createResp.Body).Decode(&space); err != nil {
		return nil, err
	}

	return &space, nil
}

// GetAppByName returns the first app matching name in the given space.
func (c *MockCFClient) GetAppByName(ctx context.Context, name, spaceGUID string) (*resource.App, error) {
	q := url.Values{}
	q.Add("names", name)
	q.Add("space_guids", spaceGUID)

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v3/apps?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get app: status %d", resp.StatusCode)
	}

	var result struct {
		Resources []*resource.App `json:"resources"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Resources) == 0 {
		return nil, fmt.Errorf("app %q not found in space %q", name, spaceGUID)
	}

	return result.Resources[0], nil
}

// PushApp creates an application in CF from the supplied request.
func (c *MockCFClient) PushApp(ctx context.Context, req cf.PushRequest) (*resource.App, error) {
	body := map[string]interface{}{
		"name": req.Name,
		"relationships": map[string]interface{}{
			"space": map[string]interface{}{
				"data": map[string]string{
					"guid": req.SpaceGUID,
				},
			},
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v3/apps", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to push app: status %d", resp.StatusCode)
	}

	var app resource.App
	if err := json.NewDecoder(resp.Body).Decode(&app); err != nil {
		return nil, err
	}

	return &app, nil
}

// RunTask starts a one-off task on the specified app.
func (c *MockCFClient) RunTask(ctx context.Context, appGUID string, req cf.TaskRequest) (*resource.Task, error) {
	body := map[string]interface{}{
		"command": req.Command,
		"name":    req.Name,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v3/apps/"+appGUID+"/tasks", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to run task: status %d", resp.StatusCode)
	}

	var task resource.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, err
	}

	return &task, nil
}

// GetTask fetches the current state of a task by its GUID.
func (c *MockCFClient) GetTask(ctx context.Context, taskGUID string) (*resource.Task, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v3/tasks/"+taskGUID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("task not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get task: status %d", resp.StatusCode)
	}

	var task resource.Task
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return nil, err
	}

	return &task, nil
}

// CancelTask requests cancellation of a running task.
func (c *MockCFClient) CancelTask(ctx context.Context, taskGUID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v3/tasks/"+taskGUID+"/cancel", nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("task not found")
	}

	if resp.StatusCode == http.StatusUnprocessableEntity {
		return fmt.Errorf("cannot cancel succeeded task")
	}

	return nil
}
