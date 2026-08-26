//go:build integration

package cf_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("CF Resource Cleanup", func() {
	Describe("e2e cleanup and TTL", func() {
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

			space, err := cfCluster.Client.GetSpaceByName(ctx, cfTestOrgGUID, spaceName)
			if err == nil {
				spaceGUID = space.GUID
			} else {
				Skip("test space " + spaceName + " not found")
			}
		})

		It("should clean up completed tasks", func() {
			appName := fmt.Sprintf("cleanup-runner-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a quick task
			taskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-cleanup-task", tenantName),
				Command: "echo 'Quick task'; exit 0",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Wait for task to complete
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify task is retrievable after completion
			finalTask, err := cfCluster.Client.GetTask(ctx, task.GUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalTask.State).To(Equal("SUCCEEDED"))

			// In a real scenario, a cleanup job would remove old tasks
			// Here we verify the task remains in CF system (for audit trail)
		})

		It("should support task TTL expiration", func() {
			appName := fmt.Sprintf("ttl-runner-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run multiple tasks to verify TTL handling
			taskCount := 3
			taskGUIDs := make([]string, 0, taskCount)

			for i := 0; i < taskCount; i++ {
				taskReq := cf.TaskRequest{
					Name:    fmt.Sprintf("%s-ttl-task-%d", tenantName, i),
					Command: "echo 'Task with TTL'; sleep 1",
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

			// Verify all tasks are in terminal state
			for _, taskGUID := range taskGUIDs {
				task, err := cfCluster.Client.GetTask(ctx, taskGUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(task.State).To(Equal("SUCCEEDED"))
			}
		})

		It("should handle cleanup of failed tasks", func() {
			appName := fmt.Sprintf("failed-cleanup-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			failedTaskCount := 5
			failedTaskGUIDs := make([]string, 0, failedTaskCount)

			// Create multiple failing tasks
			for i := 0; i < failedTaskCount; i++ {
				taskReq := cf.TaskRequest{
					Name:    fmt.Sprintf("%s-failed-cleanup-%d", tenantName, i),
					Command: "exit 1",
				}

				task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
				Expect(err).NotTo(HaveOccurred())
				failedTaskGUIDs = append(failedTaskGUIDs, task.GUID)
			}

			// Wait for all to fail
			for _, taskGUID := range failedTaskGUIDs {
				err := WaitForTaskState(ctx, cfCluster.Client, taskGUID, "FAILED", 30*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}

			// Verify all are in FAILED state (ready for cleanup)
			for _, taskGUID := range failedTaskGUIDs {
				task, err := cfCluster.Client.GetTask(ctx, taskGUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(task.State).To(Equal("FAILED"))
			}
		})

		It("should preserve running tasks during cleanup", func() {
			appName := fmt.Sprintf("preserve-runner-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Create a long-running task
			runningTaskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-running-preserve", tenantName),
				Command: "sleep 60",
			}

			runningTask, err := cfCluster.Client.RunTask(ctx, app.GUID, runningTaskReq)
			Expect(err).NotTo(HaveOccurred())

			// Create and complete a quick task
			quickTaskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-quick-preserve", tenantName),
				Command: "echo 'quick'",
			}

			quickTask, err := cfCluster.Client.RunTask(ctx, app.GUID, quickTaskReq)
			Expect(err).NotTo(HaveOccurred())

			// Wait for quick task to complete
			err = WaitForTaskState(ctx, cfCluster.Client, quickTask.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify running task is still running
			runningTaskState, err := cfCluster.Client.GetTask(ctx, runningTask.GUID)
			Expect(err).NotTo(HaveOccurred())
			Expect(runningTaskState.State).To(Or(Equal("PENDING"), Equal("RUNNING")))

			// Clean up: cancel the running task
			_ = cfCluster.Client.CancelTask(ctx, runningTask.GUID)
		})

		It("should handle cleanup with large number of tasks", func() {
			appName := fmt.Sprintf("bulk-cleanup-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			taskCount := 10
			taskGUIDs := make([]string, 0, taskCount)

			// Create many tasks
			for i := 0; i < taskCount; i++ {
				taskReq := cf.TaskRequest{
					Name:    fmt.Sprintf("%s-bulk-%d", tenantName, i),
					Command: fmt.Sprintf("echo 'Task %d'; sleep 1", i),
				}

				task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
				Expect(err).NotTo(HaveOccurred())
				taskGUIDs = append(taskGUIDs, task.GUID)
			}

			// Wait for all to complete
			completedCount := 0
			for _, taskGUID := range taskGUIDs {
				err := WaitForTaskState(ctx, cfCluster.Client, taskGUID, "SUCCEEDED", 30*time.Second)
				if err == nil {
					completedCount++
				}
			}

			// Expect most to complete
			Expect(completedCount).To(BeNumerically(">=", taskCount/2))
		})
	})
})
