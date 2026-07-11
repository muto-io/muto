# Integration Tests — Ginkgo + Testcontainers-go Kind Design Spec

**Date:** 2026-07-12
**Status:** Approved

---

## Summary

Replace the current `test/integration/` suite (envtest-based, never registers reconcilers, tests time out) with a Ginkgo BDD suite backed by `testcontainers-go` kind provider. The kind cluster is managed programmatically — no external tooling required beyond Docker. All three reconcilers are registered in-process against the live cluster.

---

## Suite Architecture

```
test/integration/
├── suite_test.go               # BeforeSuite/AfterSuite: kind cluster + manager lifecycle
├── tenant_test.go              # Tenant CR → namespace + muto.io/tenant label
├── agentjob_lifecycle_test.go  # AgentJob Pending→Running→pod creation
├── ttl_cleanup_test.go         # Succeeded + TTL → AgentJob deleted
├── isolation_test.go           # cross-tenant topic enforcement (pure logic)
└── mcp_roundtrip_test.go       # schedule→status→cancel via tools.Handlers
```

**Build tag:** `//go:build integration` (unchanged — excluded from `make test-unit`)

**Makefile target:** `test-integration-kind` (new alias; existing `test-integration` updated to use same flags)

---

## suite_test.go Flow

### BeforeSuite
1. `testcontainers/kind` creates a 3-node kind cluster (1 control-plane, 2 workers) using `deploy/kind/kind-config.yaml`
2. Retrieve kubeconfig from the kind container
3. Connect via `envtest.Environment{UseExistingCluster: true}` to get a `*rest.Config`
4. Register `corev1` + `v1alpha1` schemes
5. Create `k8sClient` via `client.New`
6. Start controller-runtime manager with all three reconcilers: `TenantReconciler`, `AgentJobReconciler`, `AgentFleetReconciler`
7. Start manager in a goroutine; wait for cache sync

### AfterSuite
1. Cancel manager context
2. Terminate kind cluster via testcontainers

---

## Test Patterns

All async assertions use `Eventually`/`Consistently` from Gomega — no `time.Sleep`.

### Tenant test
```go
Eventually(func(g Gomega) {
    ns := &corev1.Namespace{}
    g.Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "acme-agents"}, ns)).To(Succeed())
    g.Expect(ns.Labels["muto.io/tenant"]).To(Equal("acme"))
}).WithTimeout(15 * time.Second).WithPolling(300 * time.Millisecond).Should(Succeed())
```

### AgentJob lifecycle test
```go
Eventually(func(g Gomega) {
    updated := &v1alpha1.AgentJob{}
    g.Expect(k8sClient.Get(ctx, jobKey, updated)).To(Succeed())
    g.Expect(updated.Status.Phase).To(Equal("Running"))
}).WithTimeout(30 * time.Second).Should(Succeed())
// assert pods exist with muto.io/job label
```

### TTL cleanup test
- Manually patch job status to `Succeeded` with `CompletedAt = now`
- `Eventually` polls until `errors.IsNotFound`

### MCP round-trip test
- Wire `K8sAdapter → DefaultScheduler → tools.Handlers` against live cluster
- `schedule_agent_job` → assert `PhaseRunning` → `cancel_job` → assert `PhaseTerminating`

### Isolation test
Pure logic — `tenant.ValidateTopic` cross-tenant rejection. No cluster needed but co-located for suite cohesion.

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/testcontainers/testcontainers-go` | Kind cluster lifecycle |
| `github.com/testcontainers/testcontainers-go/modules/kind` | Kind provider |
| `github.com/onsi/ginkgo/v2` | BDD test framework (already in go.mod) |
| `github.com/onsi/gomega` | Matchers + Eventually (already in go.mod) |

---

## CI Changes

`ci.yml` `integration-test` job:
- Remove `setup-envtest` step
- Add Docker-in-Docker (`services: docker`) or use GitHub-hosted runner (Docker pre-installed)
- Run `make test-integration-kind`
- Timeout: 15 minutes

---

## Makefile Changes

```makefile
test-integration-kind:
	go test ./test/integration/... -tags integration -v -timeout 15m
```

Update `test-integration` to alias `test-integration-kind`.
