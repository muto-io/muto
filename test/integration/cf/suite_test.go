//go:build integration

package cf_test

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	mockServer    *MockCFServer
	ctx           context.Context
	cancelCtx     context.CancelFunc
	cfCluster     *CFCluster
	cfTestOrgGUID string
	cfHelper      *CFTestHelper
)

func TestCFIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CF Integration Suite")
}

var _ = BeforeSuite(func() {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())
	cancelCtx = cancel

	// Initialize test helper
	cfHelper = NewCFTestHelper("muto-e2e-test-org")

	// Try to start CF cluster (or use existing one via env vars)
	var err error
	cfCluster, err = StartCFCluster(ctx)
	if err != nil {
		// Fall back to mock server for testing without real CF cluster
		fmt.Printf("Using mock CF server for testing (reason: %v)\n", err)
		mockServer = NewMockCFServer()

		// Create mock CF client
		mockClient := NewMockCFClient(mockServer.URL)
		cfCluster = &CFCluster{
			APIURL:   mockServer.URL,
			Username: "test",
			Password: "test",
			Client:   mockClient,
		}

		// Get or create test organization in mock server
		org, orgErr := mockClient.GetOrgByName(ctx, cfHelper.OrgName)
		if orgErr != nil {
			Skip("Failed to get or create organization in mock server: " + orgErr.Error())
		}
		cfTestOrgGUID = org.GUID
		return
	}

	// If using real CF cluster
	mockServer = NewMockCFServer()

	// Create a test organization for all e2e tests
	org, err := cfCluster.Client.GetOrgByName(ctx, cfHelper.OrgName)
	if err == nil {
		cfTestOrgGUID = org.GUID
	} else {
		// Organization doesn't exist; skip setup (pre-created expected)
		Skip("CF test organization not available: " + err.Error())
	}
})

var _ = AfterSuite(func() {
	if cancelCtx != nil {
		cancelCtx()
	}
	if mockServer != nil {
		mockServer.Close()
	}
	if cfCluster != nil {
		err := cfCluster.Close(ctx)
		Expect(err).NotTo(HaveOccurred())
	}
})
