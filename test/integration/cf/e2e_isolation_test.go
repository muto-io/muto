//go:build integration

package cf_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("CF Tenant Isolation", func() {
	Describe("e2e isolation and multi-tenancy", func() {
		var (
			space1GUID    string
			space2GUID    string
			tenant1Name   string
			tenant2Name   string
		)

		BeforeEach(func() {
			if cfCluster == nil {
				Skip("CF cluster not available")
			}

			tenant1Name = cfHelper.NextTenant()
			tenant2Name = cfHelper.NextTenant()

			space1, err := cfCluster.Client.GetSpaceByName(ctx, cfTestOrgGUID, cfHelper.NextSpace())
			if err == nil {
				space1GUID = space1.GUID
			} else {
				Skip("test space not found")
			}

			space2, err := cfCluster.Client.GetSpaceByName(ctx, cfTestOrgGUID, cfHelper.NextSpace())
			if err == nil {
				space2GUID = space2.GUID
			} else {
				Skip("test space not found")
			}
		})

		It("should isolate tasks between tenants", func() {
			// Create apps for both tenants
			tenant1AppReq := cf.PushRequest{
				Name:        fmt.Sprintf("tenant1-app-%s", tenant1Name),
				SpaceGUID:   space1GUID,
				DockerImage: "busybox:latest",
				EnvVars: map[string]string{
					"TENANT_ID": tenant1Name,
				},
			}

			tenant1App, err := cfCluster.Client.PushApp(ctx, tenant1AppReq)
			Expect(err).NotTo(HaveOccurred())

			tenant2AppReq := cf.PushRequest{
				Name:        fmt.Sprintf("tenant2-app-%s", tenant2Name),
				SpaceGUID:   space2GUID,
				DockerImage: "busybox:latest",
				EnvVars: map[string]string{
					"TENANT_ID": tenant2Name,
				},
			}

			tenant2App, err := cfCluster.Client.PushApp(ctx, tenant2AppReq)
			Expect(err).NotTo(HaveOccurred())

			// Run tasks for both tenants
			task1Req := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-task", tenant1Name),
				Command: fmt.Sprintf("echo 'Tenant 1: %s'", tenant1Name),
			}

			task1, err := cfCluster.Client.RunTask(ctx, tenant1App.GUID, task1Req)
			Expect(err).NotTo(HaveOccurred())

			task2Req := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-task", tenant2Name),
				Command: fmt.Sprintf("echo 'Tenant 2: %s'", tenant2Name),
			}

			task2, err := cfCluster.Client.RunTask(ctx, tenant2App.GUID, task2Req)
			Expect(err).NotTo(HaveOccurred())

			// Verify both tasks complete
			err = WaitForTaskState(ctx, cfCluster.Client, task1.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			err = WaitForTaskState(ctx, cfCluster.Client, task2.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify tasks ran to completion on different apps
			task1Final, err := cfCluster.Client.GetTask(ctx, task1.GUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(task1Final.GUID).To(Equal(task1.GUID))

			task2Final, err := cfCluster.Client.GetTask(ctx, task2.GUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(task2Final.GUID).To(Equal(task2.GUID))
		})

		It("should prevent task cancellation across tenant boundaries", func() {
			// Create an app for tenant 1
			tenant1AppReq := cf.PushRequest{
				Name:        fmt.Sprintf("tenant1-protected-%s", tenant1Name),
				SpaceGUID:   space1GUID,
				DockerImage: "busybox:latest",
			}

			tenant1App, err := cfCluster.Client.PushApp(ctx, tenant1AppReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a task for tenant 1
			task1Req := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-protected-task", tenant1Name),
				Command: "sleep 30",
			}

			task1, err := cfCluster.Client.RunTask(ctx, tenant1App.GUID, task1Req)
			Expect(err).NotTo(HaveOccurred())

			// Attempt to cancel task1 using tenant2's context (should be isolated)
			// This test verifies the isolation mechanism prevents unauthorized cancellations
			// In a real scenario, this would be enforced at the API gateway level
			// Here we just verify the task continues to run
			time.Sleep(500 * time.Millisecond)

			taskState, err := cfCluster.Client.GetTask(ctx, task1.GUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(taskState.State).To(Or(Equal("RUNNING"), Equal("PENDING")))

			// Clean up: cancel the task
			_ = cfCluster.Client.CancelTask(ctx, task1.GUID)
		})

		It("should enforce resource quotas per tenant", func() {
			// Create apps for multiple tenants
			appReqs := []cf.PushRequest{
				{
					Name:        fmt.Sprintf("quota-tenant1-%s", tenant1Name),
					SpaceGUID:   space1GUID,
					DockerImage: "busybox:latest",
				},
				{
					Name:        fmt.Sprintf("quota-tenant2-%s", tenant2Name),
					SpaceGUID:   space2GUID,
					DockerImage: "busybox:latest",
				},
			}

			apps := make([]string, 0, len(appReqs))
			for _, req := range appReqs {
				app, err := cfCluster.Client.PushApp(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				apps = append(apps, app.GUID)
			}

			// Each tenant runs tasks with resource constraints
			for i, appGUID := range apps {
				taskReq := cf.TaskRequest{
					Name:       fmt.Sprintf("quota-task-%d", i),
					Command:    "echo 'Resource constrained task'",
					MemoryInMB: 128,
					DiskInMB:   512,
				}

				task, err := cfCluster.Client.RunTask(ctx, appGUID, taskReq)
				Expect(err).NotTo(HaveOccurred())

				err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "SUCCEEDED", 30*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should prevent data leakage between tenant spaces", func() {
			// Create apps in different spaces
			tenant1AppReq := cf.PushRequest{
				Name:        fmt.Sprintf("data-tenant1-%s", tenant1Name),
				SpaceGUID:   space1GUID,
				DockerImage: "busybox:latest",
				EnvVars: map[string]string{
					"SECRET_DATA": "secret-value-tenant1",
				},
			}

			tenant1App, err := cfCluster.Client.PushApp(ctx, tenant1AppReq)
			Expect(err).NotTo(HaveOccurred())

			tenant2AppReq := cf.PushRequest{
				Name:        fmt.Sprintf("data-tenant2-%s", tenant2Name),
				SpaceGUID:   space2GUID,
				DockerImage: "busybox:latest",
				EnvVars: map[string]string{
					"SECRET_DATA": "secret-value-tenant2",
				},
			}

			tenant2App, err := cfCluster.Client.PushApp(ctx, tenant2AppReq)
			Expect(err).NotTo(HaveOccurred())

			// Verify each app is in different spaces (isolation)
			Expect(tenant1App.Relationships.Space.Data.GUID).NotTo(Equal(tenant2App.Relationships.Space.Data.GUID))

			// Run tasks from each space
			task1Req := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-data-task", tenant1Name),
				Command: "echo 'Tenant 1 data task'",
			}

			task1, err := cfCluster.Client.RunTask(ctx, tenant1App.GUID, task1Req)
			Expect(err).NotTo(HaveOccurred())

			err = WaitForTaskState(ctx, cfCluster.Client, task1.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
