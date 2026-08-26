//go:build integration

package cf_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("CF Stress Testing", func() {
	Describe("e2e stress scenarios", func() {
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

		It("should handle high volume of concurrent task creation", func() {
			// Create a single runner app for many tasks
			appName := fmt.Sprintf("stress-runner-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			// Spawn many concurrent tasks
			taskCount := 20
			taskGUIDs := make([]string, 0, taskCount)
			var mu sync.Mutex
			var wg sync.WaitGroup
			var successCount int32

			for i := 0; i < taskCount; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					taskReq := cf.TaskRequest{
						Name:    fmt.Sprintf("%s-stress-task-%d", tenantName, index),
						Command: fmt.Sprintf("echo 'Stress test task %d'; sleep 1", index),
					}

					task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
					if err == nil {
						mu.Lock()
						taskGUIDs = append(taskGUIDs, task.GUID)
						mu.Unlock()
						atomic.AddInt32(&successCount, 1)
					}
				}(i)
			}

			wg.Wait()

			// Verify most tasks were created successfully
			Expect(atomic.LoadInt32(&successCount)).To(BeNumerically(">", int32(taskCount/2)))

			// Wait for at least some tasks to complete
			completedCount := 0
			for _, taskGUID := range taskGUIDs {
				err := WaitForTaskState(ctx, cfCluster.Client, taskGUID, "SUCCEEDED", 30*time.Second)
				if err == nil {
					completedCount++
				}
			}

			// Expect significant portion to complete
			Expect(completedCount).To(BeNumerically(">", len(taskGUIDs)/2))
		})

		It("should handle rapid task submission and cancellation", func() {
			appName := fmt.Sprintf("rapid-cancel-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			taskCount := 10
			var wg sync.WaitGroup
			var successCount, cancelCount int32

			for i := 0; i < taskCount; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					taskReq := cf.TaskRequest{
						Name:    fmt.Sprintf("%s-rapid-task-%d", tenantName, index),
						Command: "sleep 30",
					}

					task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
					if err == nil {
						atomic.AddInt32(&successCount, 1)

						// Immediately cancel the task
						time.Sleep(100 * time.Millisecond)
						cancelErr := cfCluster.Client.CancelTask(ctx, task.GUID)
						if cancelErr == nil {
							atomic.AddInt32(&cancelCount, 1)
						}
					}
				}(i)
			}

			wg.Wait()

			Expect(atomic.LoadInt32(&successCount)).To(Equal(int32(taskCount)))
			Expect(atomic.LoadInt32(&cancelCount)).To(BeNumerically(">", 0))
		})

		It("should handle many concurrent apps in the same space", func() {
			appCount := 5
			var wg sync.WaitGroup
			appGUIDs := make([]string, 0, appCount)
			var mu sync.Mutex

			// Create multiple apps concurrently
			for i := 0; i < appCount; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					appName := fmt.Sprintf("multi-app-stress-%d-%s", index, tenantName)
					appReq := cf.PushRequest{
						Name:        appName,
						SpaceGUID:   spaceGUID,
						DockerImage: "busybox:latest",
					}

					app, err := cfCluster.Client.PushApp(ctx, appReq)
					if err == nil {
						mu.Lock()
						appGUIDs = append(appGUIDs, app.GUID)
						mu.Unlock()
					}
				}(i)
			}

			wg.Wait()

			// Verify all apps were created
			Expect(len(appGUIDs)).To(Equal(appCount))

			// Run tasks on all apps concurrently
			for i, appGUID := range appGUIDs {
				wg.Add(1)
				go func(index int, guid string) {
					defer wg.Done()

					taskReq := cf.TaskRequest{
						Name:    fmt.Sprintf("%s-multi-task-%d", tenantName, index),
						Command: "echo 'Task from app ' && sleep 1",
					}

					_, _ = cfCluster.Client.RunTask(ctx, guid, taskReq)
				}(i, appGUID)
			}

			wg.Wait()
		})

		It("should recover from task failure storms", func() {
			appName := fmt.Sprintf("failure-storm-%s", tenantName)
			appReq := cf.PushRequest{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				DockerImage: "busybox:latest",
			}

			app, err := cfCluster.Client.PushApp(ctx, appReq)
			Expect(err).NotTo(HaveOccurred())

			taskCount := 15
			var wg sync.WaitGroup
			var failureCount int32

			// Spawn many failing tasks rapidly
			for i := 0; i < taskCount; i++ {
				wg.Add(1)
				go func(index int) {
					defer wg.Done()

					taskReq := cf.TaskRequest{
						Name:    fmt.Sprintf("%s-fail-task-%d", tenantName, index),
						Command: "exit 1",
					}

					task, err := cfCluster.Client.RunTask(ctx, app.GUID, taskReq)
					if err == nil {
						// Wait for task to fail
						failErr := WaitForTaskState(ctx, cfCluster.Client, task.GUID, "FAILED", 30*time.Second)
						if failErr == nil {
							atomic.AddInt32(&failureCount, 1)
						}
					}
				}(i)
			}

			wg.Wait()

			// Expect most tasks to fail as expected
			Expect(atomic.LoadInt32(&failureCount)).To(BeNumerically(">", int32(taskCount/2)))
		})
	})
})
