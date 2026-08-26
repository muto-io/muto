//go:build integration

package cf_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("CF Multi-Agent Coordination", func() {
	Describe("e2e multi-agent jobs", func() {
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

		It("should coordinate tasks with a coordinator role", func() {
			// Create coordinator app
			coordinatorAppName := fmt.Sprintf("coordinator-%s", tenantName)
			coordinatorReq := cf.PushRequest{
				Name:        coordinatorAppName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
				EnvVars: map[string]string{
					"MUTO_ROLE": "coordinator",
				},
			}

			coordinatorApp, err := cfCluster.Client.PushApp(ctx, coordinatorReq)
			Expect(err).NotTo(HaveOccurred())

			// Create worker app
			workerAppName := fmt.Sprintf("worker-%s", tenantName)
			workerReq := cf.PushRequest{
				Name:        workerAppName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
				EnvVars: map[string]string{
					"MUTO_ROLE": "worker",
				},
			}

			workerApp, err := cfCluster.Client.PushApp(ctx, workerReq)
			Expect(err).NotTo(HaveOccurred())

			// Run coordinator task
			coordinatorTaskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-coordinator", tenantName),
				Command: "echo 'Coordinator starting'; sleep 2; echo 'Coordinator done'",
			}

			coordinatorTask, err := cfCluster.Client.RunTask(ctx, coordinatorApp.GUID, coordinatorTaskReq)
			Expect(err).NotTo(HaveOccurred())

			// Run worker tasks
			workerCount := 2
			workerTaskGUIDs := make([]string, 0, workerCount)

			for i := 0; i < workerCount; i++ {
				workerTaskReq := cf.TaskRequest{
					Name:    fmt.Sprintf("%s-worker-%d", tenantName, i),
					Command: fmt.Sprintf("echo 'Worker %d processing'; sleep 1; echo 'Worker %d done'", i, i),
				}

				workerTask, err := cfCluster.Client.RunTask(ctx, workerApp.GUID, workerTaskReq)
				Expect(err).NotTo(HaveOccurred())
				workerTaskGUIDs = append(workerTaskGUIDs, workerTask.GUID)
			}

			// Verify coordinator completes
			err = WaitForTaskState(ctx, cfCluster.Client, coordinatorTask.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			// Verify all workers complete
			for _, workerGUID := range workerTaskGUIDs {
				err := WaitForTaskState(ctx, cfCluster.Client, workerGUID, "SUCCEEDED", 30*time.Second)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		It("should handle worker failure gracefully", func() {
			workerAppName := fmt.Sprintf("worker-fail-%s", tenantName)
			workerReq := cf.PushRequest{
				Name:        workerAppName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			workerApp, err := cfCluster.Client.PushApp(ctx, workerReq)
			Expect(err).NotTo(HaveOccurred())

			// Run a failing worker task
			workerTaskReq := cf.TaskRequest{
				Name:    fmt.Sprintf("%s-failing-worker", tenantName),
				Command: "echo 'Worker failed'; exit 1",
			}

			workerTask, err := cfCluster.Client.RunTask(ctx, workerApp.GUID, workerTaskReq)
			Expect(err).NotTo(HaveOccurred())

			// Verify task fails
			err = WaitForTaskState(ctx, cfCluster.Client, workerTask.GUID, "FAILED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should run tasks with resource constraints", func() {
			appName := fmt.Sprintf("constrained-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Run task with memory constraint
			taskReq := cf.TaskRequest{
				Name:       fmt.Sprintf("%s-constrained-task", tenantName),
				Command:    "echo 'Running with constraints'; sleep 1",
				MemoryInMB: 128,
				DiskInMB:   512,
			}

			task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
			Expect(err).NotTo(HaveOccurred())

			// Verify task completes with constraints
			err = WaitForTaskState(ctx, cfCluster.Client, task.GUID, "SUCCEEDED", 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
