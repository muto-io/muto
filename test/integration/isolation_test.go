//go:build integration

package integration_test

import (
	"github.com/muto-io/muto/core/tenant"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tenant isolation", func() {
	Describe("topic prefix enforcement", func() {
		It("blocks cross-tenant topic access", func() {
			topicA := tenant.TopicPrefix("tenant-a") + "job.1"
			Expect(tenant.ValidateTopic("tenant-b", topicA)).To(HaveOccurred())
		})

		It("allows same-tenant topic access", func() {
			topic := tenant.TopicPrefix("acme") + "job.42"
			Expect(tenant.ValidateTopic("acme", topic)).To(Succeed())
		})
	})
})
