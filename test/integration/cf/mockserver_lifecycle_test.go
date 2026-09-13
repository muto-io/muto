//go:build integration

package cf_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/muto-io/muto/platform/cf"
)

var _ = Describe("Mock CF server task lifecycle", func() {
	var (
		client  *MockCFClient
		appGUID string
	)

	BeforeEach(func() {
		server := NewMockCFServer()
		DeferCleanup(server.Close)
		client = NewMockCFClient(server.URL)

		app, err := client.PushApp(ctx, cf.PushRequest{
			Name:        "lifecycle-app",
			SpaceGUID:   "lifecycle-space",
			DockerImage: "busybox:latest",
		})
		Expect(err).NotTo(HaveOccurred())
		appGUID = app.GUID
	})

	taskState := func(guid string) func() (string, error) {
		return func() (string, error) {
			task, err := client.GetTask(ctx, guid)
			if err != nil {
				return "", err
			}
			return task.State, nil
		}
	}

	DescribeTable("finishes tasks that don't only sleep within a second",
		func(command, finalState string) {
			task, err := client.RunTask(ctx, appGUID, cf.TaskRequest{Name: "lifecycle-task", Command: command})
			Expect(err).NotTo(HaveOccurred())

			Eventually(taskState(task.GUID)).
				WithTimeout(time.Second).
				WithPolling(20 * time.Millisecond).
				Should(Equal(finalState))
		},
		Entry("successful command", "echo done", "SUCCEEDED"),
		Entry("failing command", "exit 1", "FAILED"),
	)

	// Specs cancel or inspect tasks that run "sleep N" while they are still
	// running, so those tasks must outlive the regular run time by far.
	It("keeps tasks that only sleep running", func() {
		task, err := client.RunTask(ctx, appGUID, cf.TaskRequest{Name: "sleeping-task", Command: "sleep 30"})
		Expect(err).NotTo(HaveOccurred())

		Eventually(taskState(task.GUID)).
			WithTimeout(time.Second).
			WithPolling(20 * time.Millisecond).
			Should(Equal("RUNNING"))
		Consistently(taskState(task.GUID)).
			WithTimeout(300 * time.Millisecond).
			WithPolling(20 * time.Millisecond).
			Should(Equal("RUNNING"))
	})
})
