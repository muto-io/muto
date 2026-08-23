//go:build integration

package cf_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	mockServer *MockCFServer
	ctx        context.Context
)

func TestCFIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CF Integration Suite")
}

var cancelCtx context.CancelFunc

var _ = BeforeSuite(func() {
	ctx, cancelCtx = context.WithCancel(context.Background())
	mockServer = NewMockCFServer()
})

var _ = AfterSuite(func() {
	if cancelCtx != nil {
		cancelCtx()
	}
	if mockServer != nil {
		mockServer.Close()
	}
})
