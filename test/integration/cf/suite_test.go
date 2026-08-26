//go:build integration

package cf_test

import (
	"context"
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

	mockServer = NewMockCFServer()

	// Start CF cluster (or use existing one via env vars)
	var err error
	cfCluster, err = StartCFCluster(ctx)
	if err != nil {
		// Skip CF e2e tests if no CF cluster available
		Skip("CF cluster not available: " + err.Error())
	}

	// Initialize test helper
	cfHelper = NewCFTestHelper("muto-e2e-test-org")

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
