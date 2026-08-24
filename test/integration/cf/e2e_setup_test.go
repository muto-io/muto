//go:build integration

package cf_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/testcontainers/testcontainers-go"

	"github.com/muto-io/muto/platform/cf"
)

// CFCluster represents a CloudFoundry cluster running for e2e testing.
type CFCluster struct {
	Container  testcontainers.Container
	APIURL     string
	Username   string
	Password   string
	AdminOrg   string
	AdminSpace string
	Client     cf.CFClient
}

// StartCFCluster starts a CF cluster. Priority:
// 1. Environment-provided CF instance (e.g., kind-deployment)
// 2. Fall back to mock server for local testing
func StartCFCluster(ctx context.Context) (*CFCluster, error) {
	// Check if kind-deployment is configured
	kindDeploymentPath := os.Getenv("KIND_DEPLOYMENT_PATH")
	if kindDeploymentPath != "" {
		return StartCFClusterViaKind(ctx)
	}

	// Try to use environment-provided CF instance (for manual setup)
	if url := os.Getenv("CF_E2E_API_URL"); url != "" && url != "https://api.cf.local" {
		client, err := cf.NewRealCFClient(
			url,
			os.Getenv("CF_E2E_USERNAME"),
			os.Getenv("CF_E2E_PASSWORD"),
		)
		if err != nil {
			return nil, fmt.Errorf("create cf client: %w", err)
		}
		return &CFCluster{
			APIURL:   url,
			Username: os.Getenv("CF_E2E_USERNAME"),
			Password: os.Getenv("CF_E2E_PASSWORD"),
			Client:   client,
		}, nil
	}

	// Fallback: use mock server for local/test environments
	return nil, fmt.Errorf("CF_E2E_API_URL not set; use kind-deployment or configure CF_E2E_* env vars")
}

// Close terminates the CF cluster.
func (c *CFCluster) Close(ctx context.Context) error {
	if c.Container != nil {
		return c.Container.Terminate(ctx)
	}
	return nil
}


// WaitForTaskState polls until the task reaches the desired state.
func WaitForTaskState(ctx context.Context, client cf.CFClient, taskGUID, targetState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		task, err := client.GetTask(ctx, taskGUID)
		if err != nil {
			return fmt.Errorf("get task: %w", err)
		}
		if task.State == targetState {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for task %s to reach state %s", taskGUID, targetState)
}

// StartCFClusterViaKind starts a CF cluster using kind-deployment from CloudFoundry.
// kind-deployment should be pre-deployed and running in the CI/CD environment.
func StartCFClusterViaKind(ctx context.Context) (*CFCluster, error) {
	kindDeploymentPath := os.Getenv("KIND_DEPLOYMENT_PATH")
	if kindDeploymentPath == "" {
		return nil, fmt.Errorf("KIND_DEPLOYMENT_PATH env var not set")
	}

	// Verify kind-deployment directory exists
	if _, err := os.Stat(kindDeploymentPath); err != nil {
		return nil, fmt.Errorf("kind-deployment path not found at %s: %w", kindDeploymentPath, err)
	}

	// Get CF API URL and credentials
	apiURL := os.Getenv("CF_E2E_API_URL")
	if apiURL == "" {
		apiURL = "https://api.cf.local"
	}

	username := os.Getenv("CF_E2E_USERNAME")
	if username == "" {
		username = "admin"
	}

	password := os.Getenv("CF_E2E_PASSWORD")
	if password == "" {
		password = "password"
	}

	// If running in kind cluster in CI, try to find the actual CF API endpoint
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		fmt.Printf("Detected KUBECONFIG: %s\n", kubeconfig)
		// Try to get CF API service endpoint from the cluster
		if cfURL := getCFAPIFromCluster(ctx, kubeconfig); cfURL != "" {
			fmt.Printf("Found CF API in cluster: %s\n", cfURL)
			apiURL = cfURL
		}
	}

	// Wait for CF API to be ready
	if err := waitForCFReady(ctx, apiURL); err != nil {
		// Log but continue - CF might be deployed but not fully ready
		fmt.Printf("CF API not immediately ready: %v, will attempt connection anyway\n", err)
	}

	// Try to create CF client
	client, err := cf.NewRealCFClient(apiURL, username, password)
	if err != nil {
		return nil, fmt.Errorf("create cf client: %w", err)
	}

	return &CFCluster{
		APIURL:   apiURL,
		Username: username,
		Password: password,
		Client:   client,
	}, nil
}

// getCFAPIFromCluster tries to find the CF API service endpoint in the Kubernetes cluster.
func getCFAPIFromCluster(ctx context.Context, kubeconfig string) string {
	// Check for common CF service endpoints in the cluster
	// Try to find ingress or service that exposes the CF API
	svc := ""

	// Look for CF API ingress or service
	// Common names: cf-api, api, cf-api-ingress, etc.
	for _, namespace := range []string{"cf", "cloudfoundry", "default"} {
		for _, svcName := range []string{"cf-api", "api", "diego-api", "ccapi"} {
			// Try https://svc.namespace.svc.cluster.local:443
			testURL := fmt.Sprintf("https://%s.%s.svc.cluster.local", svcName, namespace)
			if isServiceReachable(ctx, testURL, 2*time.Second) {
				return testURL
			}
		}
	}

	return svc
}

// isServiceReachable checks if a service is reachable via TCP.
func isServiceReachable(ctx context.Context, url string, timeout time.Duration) bool {
	host := parseHost(url)
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:443", host), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// waitForCFReady polls the CF API until it responds.
// Times out quickly (2 minutes) to avoid blocking test suite.
func waitForCFReady(ctx context.Context, apiURL string) error {
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to resolve the host
		host := parseHost(apiURL)
		if addrs, err := net.LookupHost(host); err == nil && len(addrs) > 0 {
			// Host resolves; API should be ready
			return nil
		}

		// Try direct TCP connection to API
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:443", host), 2*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}

		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("timeout waiting for CF API to be ready at %s (CF cluster not available)", apiURL)
}

func parseHost(apiURL string) string {
	// Simple parsing; handles https://api.cf.local format
	start := len("https://")
	if len(apiURL) > start {
		if end := len(apiURL); end > 0 {
			return apiURL[start:end]
		}
	}
	return apiURL
}
