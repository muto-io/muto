# End-to-End (E2E) Testing Guide

## Overview

The Muto project uses comprehensive E2E tests to validate functionality across different deployment platforms:
- **Kubernetes (K8s):** Primary platform using `kind` (Kubernetes in Docker) for local testing
- **Cloud Foundry (CF):** Alternative platform with mocked infrastructure for CI

E2E tests run as a separate workflow (`e2e-tests.yml`) triggered by changes to core platform code, integration tests, or on a weekly schedule.

---

## Test Infrastructure

### Kubernetes E2E Tests

**Setup:**
- Uses `kind` cluster (Kubernetes in Docker)
- Requires Docker socket access
- Uses testcontainers for dependency management
- A2A Gateway integration for advanced use cases

**Environment Variables:**
```bash
MUTO_A2A_GATEWAY_IMAGE=ghcr.io/a2aprotocol/a2a-gateway:v0.1.0
DOCKER_HOST=unix:///var/run/docker.sock
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
KUBECONFIG=/tmp/kubeconfig-k8s-e2e
MUTO_USE_EXISTING_CLUSTER=true
```

**Execution:**
```bash
make test-integration-k8s
```

### Cloud Foundry E2E Tests

**Setup:**
- Mocked infrastructure for CI (no real CF required)
- Can use real CF cluster if secrets configured
- Gracefully skips when CF credentials missing

**Environment Variables (Optional for Real CF):**
```bash
CF_E2E_API_URL=${{ secrets.CF_API_URL }}
CF_E2E_USERNAME=${{ secrets.CF_USERNAME }}
CF_E2E_PASSWORD=${{ secrets.CF_PASSWORD }}
```

**Execution:**
```bash
make test-integration-cf
```

---

## Performance Analysis

### Current E2E Workflow Times

**Kubernetes E2E:**
- Average: 3:58 per run
- Range: 3:45 - 4:10 minutes
- Consistent execution with minimal variance

**CloudFoundry E2E:**
- Average: 4:10 per run (with anomalies)
- Range: 0:45 - 4:30 minutes
- High variance suggests conditional test skipping

**Combined E2E Workflow:**
- Total time: ~4 minutes (tests run in parallel at job level)
- Status check: <1 minute
- Expected P95: 4:30 minutes

### Performance Timeline

| Run | K8s Time | CF Time | Total | Notes |
|-----|----------|---------|-------|-------|
| 111 | 3:58 | 4:10 | 4:20 | Both jobs |
| 110 | ~4:02 | ~4:02 | 4:02 | Both jobs |
| 109 | ~4:14 | ~4:14 | 4:14 | Both jobs |
| 104 | - | 1:15 | 1:15 | CF only (anomaly) |
| 102 | - | 0:57 | 0:57 | CF only (anomaly) |

### Bottleneck Analysis

**K8s E2E Bottlenecks (3:58 runtime):**
1. **Kind cluster creation:** ~1 minute
2. **Test execution:** ~2:30 (integration tests with namespace setup)
3. **Teardown/cleanup:** ~0:30 (critical issue - see flakiness section)

**CF E2E Bottlenecks (4:10 runtime):**
1. **Setup/environment config:** ~0:30
2. **Test execution:** ~3:00 (mocked infrastructure)
3. **Cleanup:** <1 minute (faster than K8s)

### Resource Usage

**Current Configuration:**
- **Runner:** `ubuntu-latest` (standard GitHub Actions runner)
- **Parallelization:** K8s and CF jobs run in parallel
- **CPU:** Single core during test execution
- **Memory:** ~2-3GB during kind cluster operation
- **Disk:** ~5GB for container images and artifacts

**Cost Implications:**
- **Per run:** ~4 minutes GitHub Actions compute
- **Frequency:** Triggered on code changes + weekly schedule
- **Monthly estimate:** ~2-3 hours of compute (at ~10-15 triggers/week)

---

## Known Flakiness Issues

### A2A Gateway Namespace Cleanup (Critical)

**Issue:** Kubernetes namespace stuck in "Terminating" state after A2A Gateway tests

**Symptoms:**
- Cleanup timeout gradually increased: 60s -> 120s -> 300s
- Tests occasionally fail waiting for namespace deletion
- Resource accumulation if not properly cleaned

**Related Commits:**
- `e5646e7` - Fix A2A Gateway test cleanup to prevent namespace stuck in Terminating
- `4b11e9a` - Increase A2A gateway test cleanup timeout (#3)
- `0f470e6` - Increase namespace cleanup timeout to 300s
- `e036307` - Increase namespace cleanup timeout to 120s
- `2235129` - Fix A2A gateway integration test cleanup sequence

**Current Mitigation:**
- Cleanup timeout set to 300 seconds (5 minutes)
- Force delete enabled as fallback
- Proper finalizer cleanup sequence

**Recommended Investigation:**
- Profile actual cleanup times to determine root cause
- Evaluate operator-level improvements to prevent namespace blocking
- Consider test scope reduction to avoid cleanup delays

### E2E Test Anomalies

**Issue:** CF E2E runs completing in <1 minute (runs 104, 102, 100, 99, 98, 97)

**Root Cause:** CF tests skip gracefully when GitHub secrets are missing
- Tests check for `CF_API_URL`, `CF_USERNAME`, `CF_PASSWORD` secrets
- When missing, tests skip with success status (intended behavior)
- Mocked tests allow CI to run without real CF infrastructure

**Validation:**
- Review test code in `test/integration/cf/` for skip conditions
- Expected behavior for CI environment without CF credentials
- Not a bug, but should be clearly documented

---

## Running E2E Tests Locally

### Prerequisites

```bash
# Install required tools
kind version                           # Check if installed
kubectl version                        # Check if installed
docker ps                              # Verify Docker daemon

# Install kind if needed
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind
```

### Kubernetes E2E Tests

```bash
# Create kind cluster (if needed)
make kind-up

# Run tests against existing cluster
export MUTO_USE_EXISTING_CLUSTER=true
export KUBECONFIG=$HOME/.kube/config
make test-integration-k8s

# Cleanup
make kind-down
```

### Cloud Foundry E2E Tests (Mocked)

```bash
# Run with mocked infrastructure (no CF required)
make test-integration-cf

# With real CF credentials (optional)
export CF_E2E_API_URL="https://api.your-cf.com"
export CF_E2E_USERNAME="admin"
export CF_E2E_PASSWORD="password"
make test-integration-cf
```

### Full E2E Suite

```bash
# Run both K8s and CF E2E tests
make test-e2e

# Run with specific timeout
make test-integration-k8s TIMEOUT=15m
```

---

## CI/CD Integration

### Workflow Configuration

**Trigger Events:**
- Pull request with changes to:
  - `platform/**`
  - `core/**`
  - `test/integration/**`
  - `.github/workflows/e2e-tests.yml`
  - `go.mod` or `go.sum`
- Push to `main` branch (same paths)
- Weekly schedule: Monday 2:00 AM UTC

**Job Timeouts:**
```yaml
k8s-e2e:
  timeout-minutes: 15  # Total job timeout
  steps:
    - Create cluster: 10m
    - Run tests: 10m   # Individual step timeout

cf-e2e:
  timeout-minutes: 15  # Total job timeout
  steps:
    - Setup: 5m
    - Run tests: 10m   # Individual step timeout
```

### Artifacts

Both K8s and CF jobs upload test results:
- Location: `test-results/{platform}/`
- Retention: 30 days
- Format: Log files from Ginkgo test framework

---

## Test Coverage

### Kubernetes E2E Tests

**Areas Covered:**
- Operator deployment and lifecycle
- Custom Resource Definition (CRD) validation
- A2A Gateway integration
- Tenant management
- Agent fleet operations
- Networking and service discovery

**Test Files:**
```
test/integration/k8s/
├── operator_test.go           # Operator lifecycle tests
├── a2a_gateway_test.go        # A2A Gateway integration
├── tenant_management_test.go  # Tenant CRUD operations
├── agent_fleet_test.go        # Fleet management
└── ...
```

### Cloud Foundry E2E Tests

**Areas Covered:**
- CF deployment compatibility
- Mocked infrastructure validation
- Service binding and configuration
- Application lifecycle

**Test Files:**
```
test/integration/cf/
├── deployment_test.go     # CF deployment tests
├── service_test.go        # Service binding tests
└── ...
```

---

## Optimization Recommendations

### Quick Wins (1-2 Days)

1. **Verify test skip conditions**
   - Validate CF tests skip gracefully when secrets missing
   - Document expected behavior
   - Confirm no tests are unintentionally skipped

2. **Monitor namespace cleanup times**
   - Add logging to A2A Gateway test cleanup
   - Track actual cleanup duration vs timeout
   - Identify if 300s timeout is necessary

### Medium-term (1-2 Weeks)

3. **Profile individual test cases**
   ```bash
   go test ./test/integration/k8s -v -cpuprofile=cpu.prof
   go tool pprof cpu.prof
   ```
   - Identify slowest test cases
   - Look for parallelization opportunities
   - Create optimization tickets for top 5 tests

4. **Optimize A2A Gateway cleanup**
   - Investigate root cause of namespace termination delays
   - Consider operator-level improvements
   - Potentially reduce timeout from 300s to 60s

### Long-term (1 Month+)

5. **Parallelize K8s and CF E2E tests**
   - Current: Jobs already run in parallel (good)
   - Opportunity: Split K8s tests into focused suites
   - Expected savings: 30-40% with better parallelization

6. **Implement test execution profiling**
   - Create dashboard for test timing trends
   - Alert on regressions (>10% increase)
   - Monitor P95 execution times

---

## Troubleshooting

### Kind Cluster Issues

**Problem:** Cluster creation times out
```bash
# Solution: Increase timeout or recreate cluster
make kind-down
sleep 5
make kind-up
```

**Problem:** Docker socket not accessible
```bash
# Solution: Check Docker daemon
docker ps
# Ensure DOCKER_HOST is set correctly
export DOCKER_HOST=unix:///var/run/docker.sock
```

### Namespace Stuck in Terminating

**Problem:** Tests hang during namespace cleanup
```bash
# Workaround: Check namespace status
kubectl get namespace muto-test -o json | jq .status

# Force delete if necessary (last resort)
kubectl delete namespace muto-test --grace-period=0 --force
```

### Memory Issues

**Problem:** Kind cluster runs out of memory
```bash
# Solution: Increase Docker resource limits
# Edit Docker Desktop settings or:
docker stats  # Monitor usage
```

---

## Performance Targets

### Execution Time Goals

| Metric | Current | Target | Status |
|--------|---------|--------|--------|
| K8s E2E avg | 3:58 | 3:30 | Monitor |
| CF E2E avg | 4:10 | 3:30 | Monitor |
| P95 combined | 4:30 | 4:00 | Monitor |
| Total job timeout | 15m | 15m | ✓ Optimized |

### Success Metrics

- [x] E2E execution times documented
- [x] Bottlenecks identified (K8s cleanup)
- [x] Flakiness issues tracked (A2A Gateway, test anomalies)
- [ ] 30% performance improvement (pending medium-term optimizations)
- [ ] <1% test flakiness rate (in progress)

---

## References

- **Performance Analysis:** See `performance-analysis.md` for detailed metrics
- **Optimization Roadmap:** See `optimization-roadmap.md` for implementation plan
- **K8s Integration Tests:** See `integration-tests.md`
- **GitHub Workflows:** `.github/workflows/e2e-tests.yml`
- **Test Code:** `test/integration/k8s/` and `test/integration/cf/`

---

**Last Updated:** September 2, 2026  
**Related Issue:** #17 - Phase 4.2: Review and Optimize Test Times
