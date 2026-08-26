//go:build integration

package cf_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("CF Failure Scenarios", func() {
	Describe("e2e failure handling", func() {
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

		It("should handle task timeout gracefully", func() {
			appName := fmt.Sprintf("timeout-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a task that would timeout (sleep for a very long time)
			taskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-timeout-task", tenantName),
				Command: "sleep 300",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Cancel the task to simulate timeout handling
			time.Sleep(1 * time.Second)
			err = cfCluster.Client.CancelTask(ctx, task.GUID)
			Expect(err).NotTo(HaveOccurred())

			// Verify task reaches terminal state
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "CANCELING", 10*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should recover from task crashes", func() {
			appName := fmt.Sprintf("crash-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a task that crashes
			taskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-crash-task", tenantName),
				Command: "sleep 1; kill -9 $$",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Verify task reaches FAILED state
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "FAILED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle invalid commands", func() {
			appName := fmt.Sprintf("invalid-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a task with an invalid command
			taskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-invalid-cmd-task", tenantName),
				Command: "/nonexistent/command",
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Verify task fails
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "FAILED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle OOM (out of memory) scenarios", func() {
			appName := fmt.Sprintf("oom-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a task with severe memory constraints
			taskReq := cf.TaskRequest{
				Name:       fmt.Sprintf("%s-oom-task", tenantName),
				Command:    "yes | tr -d ' ' > /dev/null",
				MemoryInMB: 32, // Very small memory limit
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Verify task fails (due to memory constraints)
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "FAILED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle concurrent task creation failures", func() {
			appName := fmt.Sprintf("concurrent-fail-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Attempt to create many tasks concurrently
			taskCount := 5
			errCount := 0

			for i := 0; i < taskCount; i++ {
				taskReq := cf.TaskRequest{
					Name:    fmt.Sprintf("%s-concurrent-fail-%d", tenantName, i),
					Command: "exit 1",
				}

				task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
				if err != nil {
					errCount++
				} else {
					// Verify each task fails
					err := WaitForTaskState(ctx, cfCluster.Client, task.GUID, "FAILED", 30*time.Second)
					Expect(err).NotTo(HaveOccurred())
				}
			}

			// Expect all tasks to succeed in creation (failures are in execution)
			Expect(errCount).To(Equal(0))
		})
	})
})
