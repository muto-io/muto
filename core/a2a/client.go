package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// A2AClient sends tasks to agents via an A2A gateway.
type A2AClient struct {
	gatewayURL string
	authToken  string
	httpClient *http.Client
}

// TaskResult is the response from a SendTask or GetTaskStatus call.
type TaskResult struct {
	TaskID string
	State  string // submitted | working | completed | failed
	Output []byte
}

// New creates an A2AClient. Returns an error if GatewayURL is empty.
func New(cfg *Config) (*A2AClient, error) {
	if cfg.GatewayURL == "" {
		return nil, fmt.Errorf("a2a: GatewayURL must not be empty")
	}
	return &A2AClient{
		gatewayURL: cfg.GatewayURL,
		authToken:  cfg.AuthToken,
		httpClient: &http.Client{},
	}, nil
}

// SendTask submits a task payload to the named agent via the A2A gateway.
// The caller is responsible for retry logic.
func (c *A2AClient) SendTask(ctx context.Context, agentID string, payload []byte) (*TaskResult, error) {
	body := map[string]any{"agentId": agentID, "payload": payload}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("a2a: marshal send task: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gatewayURL+"/tasks/send", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("a2a: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	return c.doRequest(req)
}

// GetTaskStatus polls the current state of a task from the gateway.
func (c *A2AClient) GetTaskStatus(ctx context.Context, taskID string) (*TaskResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/tasks/%s/status", c.gatewayURL, taskID), nil)
	if err != nil {
		return nil, fmt.Errorf("a2a: create request: %w", err)
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	return c.doRequest(req)
}

type gatewayResponse struct {
	TaskID string          `json:"taskId"`
	State  string          `json:"state"`
	Output json.RawMessage `json:"output"`
}

func (c *A2AClient) doRequest(req *http.Request) (*TaskResult, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a2a: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("a2a: gateway returned %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("a2a: read response: %w", err)
	}
	var gr gatewayResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return nil, fmt.Errorf("a2a: unmarshal response: %w", err)
	}
	return &TaskResult{
		TaskID: gr.TaskID,
		State:  gr.State,
		Output: []byte(gr.Output),
	}, nil
}
