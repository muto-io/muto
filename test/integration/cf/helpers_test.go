//go:build integration

package cf_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
)

// CFTestHelper provides common utilities for CF e2e tests.
type CFTestHelper struct {
	Counter     int
	OrgName     string
	SpacePrefix string
	AppPrefix   string
}

// NewCFTestHelper returns a new helper for CF tests.
func NewCFTestHelper(orgName string) *CFTestHelper {
	return &CFTestHelper{
		Counter:     0,
		OrgName:     orgName,
		SpacePrefix: "muto-test",
		AppPrefix:   "muto-runner",
	}
}

// NextSpace returns a unique space name for the current test. Against a real
// CF instance (unlike the mock server, which is started fresh per process),
// space/org names are global, so the name is offset by the Ginkgo parallel
// process number on first use to stay unique under `ginkgo -p`.
func (h *CFTestHelper) NextSpace() string {
	h.Counter = nextCounter(&h.Counter)
	return fmt.Sprintf("%s-%d", h.SpacePrefix, h.Counter)
}

// NextTenant returns a unique tenant name.
func (h *CFTestHelper) NextTenant() string {
	h.Counter = nextCounter(&h.Counter)
	return fmt.Sprintf("tenant-%d", h.Counter)
}

// nextCounter increments a per-suite counter, offsetting it by the Ginkgo
// parallel process number on first use so names built from it stay unique
// under `ginkgo -p`. See the identical helper in test/integration/k8s for
// the full rationale.
func nextCounter(counter *int) int {
	if *counter == 0 {
		*counter = GinkgoParallelProcess() * 1_000_000
	}
	*counter++
	return *counter
}
