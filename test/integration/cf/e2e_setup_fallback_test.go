//go:build integration

package cf_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CF E2E setup", func() {
	Describe("StartCFClusterViaKind", func() {
		It("fails fast when the CF API host does not resolve", func() {
			// Mirrors the E2E workflow: KIND_DEPLOYMENT_PATH points to a
			// directory without a CF deployment and the API URL is a
			// placeholder, so the suite has to fall back to the mock server.
			GinkgoT().Setenv("KIND_DEPLOYMENT_PATH", GinkgoT().TempDir())
			GinkgoT().Setenv("CF_E2E_API_URL", "https://api.muto-e2e.invalid")
			GinkgoT().Setenv("KUBECONFIG", "")

			setupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			start := time.Now()
			_, err := StartCFClusterViaKind(setupCtx)

			Expect(err).To(HaveOccurred())
			Expect(time.Since(start)).To(BeNumerically("<", 5*time.Second))
		})
	})
})
