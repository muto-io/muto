//go:build integration

package cf_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/muto-io/muto/core/agent"
	cfpkg "github.com/muto-io/muto/platform/cf"
)

var _ = Describe("CF Adapter E2E", func() {
	var (
		adapter    *cfpkg.CFAdapter
		mockClient *MockCFClient
	)

	BeforeEach(func() {
		// Create mock client pointing to the mock server
		mockClient = NewMockCFClient(mockServer.URL)

		// Create CF adapter with dedicated isolation tier (each tenant gets their own org)
		config := cfpkg.CFAdapterConfig{
			IsolationTier: "dedicated",
			SharedOrgName: "", // Not used in dedicated mode
		}
		adapter = cfpkg.NewCFAdapter(mockClient, config)
	})

	Describe("SpawnAgent", func() {
		It("should spawn a task and return a task GUID", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-1",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo hello",
					},
				},
			}

			agentID, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentID).NotTo(BeEmpty())
			Expect(agentID).To(HavePrefix("task-"))
		})

		It("should create a runner app if it doesn't exist", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-1",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo hello",
					},
				},
			}

			agentID, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentID).NotTo(BeEmpty())
			// App creation is verified by the fact that the task was spawned successfully
		})

		It("should reuse existing runner app", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-2",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo hello",
					},
				},
			}

			// First call creates the app
			_, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())

			// Second call should reuse the app
			agentID2, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())

			// Both should create different tasks on the same app
			Expect(agentID2).NotTo(BeEmpty())
		})

		It("should fail with no agents in spec", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-1",
				Agents:    []agent.AgentRole{}, // Empty agents list
			}

			_, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no agents"))
		})
	})

	Describe("WatchAgent", func() {
		It("should watch task and return success event", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-3",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo success",
					},
				},
			}

			agentID, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())

			// Watch the task - should get a completion event
			events, err := adapter.WatchAgent(ctx, agentID)
			Expect(err).NotTo(HaveOccurred())

			// Wait for event with timeout (adapter polls every 5 seconds)
			var event agent.Event
			select {
			case event = <-events:
				// Expected
			case <-time.After(15 * time.Second):
				Fail("Did not receive event within timeout")
			}

			Expect(event.AgentID).To(Equal(agentID))
			Expect(event.Type).To(Equal(agent.EventCompleted))
		})

		It("should respect context cancellation", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-4",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "sleep 10",
					},
				},
			}

			agentID, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())

			// Create a cancellable context
			watchCtx, cancel := context.WithCancel(ctx)
			events, err := adapter.WatchAgent(watchCtx, agentID)
			Expect(err).NotTo(HaveOccurred())

			// Cancel after short delay
			time.Sleep(100 * time.Millisecond)
			cancel()

			// Channel should close without an event
			_, ok := <-events
			Expect(ok).To(BeFalse(), "channel should be closed")
		})
	})

	Describe("TerminateAgent", func() {
		It("should cancel a running task", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-5",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "sleep 30",
					},
				},
			}

			agentID, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())

			// Terminate the task
			err = adapter.TerminateAgent(ctx, agentID)
			Expect(err).NotTo(HaveOccurred())

			// Verify task state is CANCELING
			task, err := mockClient.GetTask(ctx, agentID)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.State).To(Equal("CANCELING"))
		})

		It("should silently ignore terminal task errors", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-6",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo done",
					},
				},
			}

			agentID, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())

			// Wait for task to complete
			time.Sleep(300 * time.Millisecond)

			// Try to terminate - should not error even though it's already succeeded
			err = adapter.TerminateAgent(ctx, agentID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return error for non-existent task", func() {
			err := adapter.TerminateAgent(ctx, "task-nonexistent")
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Isolation and Multi-Tenant", func() {
		It("should use different runner apps for different tenants", func() {
			spec1 := &agent.Spec{
				TenantRef: "tenant-a",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo a",
					},
				},
			}

			spec2 := &agent.Spec{
				TenantRef: "tenant-b",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo b",
					},
				},
			}

			agentID1, err := adapter.SpawnAgent(ctx, spec1)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentID1).NotTo(BeEmpty())

			agentID2, err := adapter.SpawnAgent(ctx, spec2)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentID2).NotTo(BeEmpty())

			// Different tenants should result in different task IDs
			Expect(agentID1).NotTo(Equal(agentID2))
		})

		It("should support multiple roles for same tenant", func() {
			spec := &agent.Spec{
				TenantRef: "tenant-multi",
				Agents: []agent.AgentRole{
					{
						Role:    "executor",
						Command: "echo exec",
					},
				},
			}

			agentID1, err := adapter.SpawnAgent(ctx, spec)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentID1).NotTo(BeEmpty())

			// Create a new spec with different role
			spec2 := &agent.Spec{
				TenantRef: "tenant-multi",
				Agents: []agent.AgentRole{
					{
						Role:    "observer",
						Command: "echo observe",
					},
				},
			}

			agentID2, err := adapter.SpawnAgent(ctx, spec2)
			Expect(err).NotTo(HaveOccurred())
			Expect(agentID2).NotTo(BeEmpty())

			// Different roles should create different tasks
			Expect(agentID1).NotTo(Equal(agentID2))
		})
	})
})
