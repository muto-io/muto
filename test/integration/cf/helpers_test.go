//go:build integration

package cf_test

import (
	"fmt"
)

// CFTestHelper provides common utilities for CF e2e tests.
type CFTestHelper struct {
	Counter      int
	OrgName      string
	SpacePrefix  string
	AppPrefix    string
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

// NextSpace returns a unique space name for the current test.
func (h *CFTestHelper) NextSpace() string {
	h.Counter++
	return fmt.Sprintf("%s-%d", h.SpacePrefix, h.Counter)
}

// NextTenant returns a unique tenant name.
func (h *CFTestHelper) NextTenant() string {
	h.Counter++
	return fmt.Sprintf("tenant-%d", h.Counter)
}
