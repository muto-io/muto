//go:build integration

package cf_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/cloudfoundry/go-cfclient/v3/resource"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("CF Agent Job Lifecycle", func() {
	Describe("e2e job lifecycle", func() {
		var (
			spaceName  string
			spaceGUID  string
			tenantName string
		)

		BeforeEach(func() {
			if cfCluster == nil {
				Skip("CF cluster not available")
			}

			spaceName = cfHelper.NextSpace()
			tenantName = cfHelper.NextTenant()

			// Create a test space in the org
			space, err := cfCluster.Client.GetSpaceByName(ctx, cfTestOrgGUID, spaceName)
			if err == nil {
				// Space already exists
				spaceGUID = space.GUID
			} else {
				// In a real scenario, would create space via CF API
				// For now, assume space exists or was created by test setup
				Skip("test space " + spaceName + " not found")
			}
		})

		It("should handle job lifecycle transitions", func() {
			// Create a runner app for this test
			appName := fmt.Sprintf("runner-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
				Buildpacks:  nil,
				EnvVars: map[string]string{
					"MUTO_TENANT": tenantName,
				},
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(app).NotTo(BeNil())

			// Run a task on the app
			taskReq := cf.TaskRequest{
				Name:    tenantName + "-task",
				Command: "echo 'test task'; sleep 1; exit 0",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())
			Expect(task).NotTo(BeNil())

			// Verify task transitions to SUCCEEDED
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify final task state
			finalTask, err := cfCluster.Client.GetTask(ctx, task.GUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalTask.State).To(Equal("SUCCEEDED"))
		})

		It("should handle failing tasks", func() {
			appName := fmt.Sprintf("runner-fail-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a task that fails
			taskReq := cf.TaskRequest{
				Name:    tenantName + "-fail-task",
				Command: "exit 1",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Verify task transitions to FAILED
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "FAILED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			finalTask, err := cfCluster.Client.GetTask(ctx, task.GUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalTask.State).To(Equal("FAILED"))
		})

		It("should allow cancelling tasks", func() {
			appName := fmt.Sprintf("runner-cancel-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a long-running task
			taskReq := cf.TaskRequest{
				Name:    tenantName + "-cancel-task",
				Command: "sleep 60",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Cancel the task
			err = cfCluster.Client.CancelTask(ctx, task.GUID)
			Expect(err).NotTo(HaveOccurred())

			// Verify task reaches terminal state (CANCELING or FAILED)
			// Note: CF uses CANCELING as an intermediate state
			maxAttempts := 30
			var finalTask *resource.Task
			for i := 0; i < maxAttempts; i++ {
				finalTask, err = cfCluster.Client.GetTask(ctx, task.GUID)
				Expect(err).NotTo(HaveOccurred())

				if finalTask.State == "CANCELING" || finalTask.State == "FAILED" {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
			Expect(finalTask.State).To(Or(Equal("CANCELING"), Equal("FAILED")))
		})

		It("should run multiple tasks concurrently", func() {
			appName := fmt.Sprintf("runner-multi-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run multiple tasks
			taskCount := 3
			taskGUIDs := make([]string, 0, taskCount)

			for i := 0; i < taskCount; i++ {
				taskReq := cf.TaskRequest{
					Name:    fmt.Sprintf("%s-task-%d", tenantName, i),
					Command: fmt.Sprintf("echo 'Task %d'; sleep 1", i),
				}

				task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
				Expect(err).NotTo(HaveOccurred())
				taskGUIDs = append(taskGUIDs, task.GUID)
			}

			// Wait for all tasks to complete
			for _, taskGUID := range taskGUIDs {
				err := WaitForTaskState(ctx, cfCluster.Client, taskGUID, "SUCCEEDED", 30*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})
})
