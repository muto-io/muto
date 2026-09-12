# End-to-End (E2E) Testing Guide

## Overview

The Muto project uses comprehensive E2E tests to validate functionality across different deployment platforms:
- **Kubernetes (K8s):** Primary platform using `kind` (Kubernetes in Docker) for local testing
- **Cloud Foundry (CF):** Alternative platform with mocked infrastructure for CI

E2E tests run as a separate workflow (`e2e-tests.yml`) triggered by changes to core platform code, integration tests, or on a weekly schedule.

---

## Test Infrastructure

### Parallel Execution

K8s and CF E2E tests run in parallel (no job dependencies) using isolated kubeconfigs and environments.

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

## Performance Baseline

**Kubernetes E2E:** Average 3:58 per run  
**CloudFoundry E2E:** Average 4:10 per run  
**Combined Workflow:** ~4 minutes (parallel execution)

**Key Bottlenecks:**
- K8s: Kind cluster creation (~1m) + tests (~2:30) + cleanup (~0:30)
- CF: Setup (~0:30) + tests (~3:00) + cleanup (<1m)

## Known Flakiness Issues

### A2A Gateway Namespace Cleanup (Critical)

**Issue:** Kubernetes namespace stuck in "Terminating" state after A2A Gateway tests

**Symptoms:**
- Cleanup timeout set to 300 seconds (increased from 60s)
- Tests occasionally fail waiting for namespace deletion
- Resource accumulation if not properly cleaned

**Current Mitigation:**
- Force delete enabled as fallback
- Proper finalizer cleanup sequence

**Track:** See Issue #59 for optimization plan

### E2E Test Anomalies

**Issue:** CF E2E runs sometimes complete in <1 minute

**Root Cause:** CF tests skip gracefully when GitHub secrets are missing
- Tests check for `CF_API_URL`, `CF_USERNAME`, `CF_PASSWORD` secrets
- When missing, tests skip with success status (intended behavior)
- This is expected behavior for CI without real CF credentials

**Track:** See Issue #56 for investigation details

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

## Related Documentation

- [:octicons-book-24: **Integration Tests**](./integration-tests.md) — Integration testing guide
- [:octicons-book-24: **Testing Overview**](./overview.md) — Testing strategy
- [:octicons-book-24: **Running Tests Locally**](./running-locally.md) — Local test execution
- [:octicons-book-24: **Unit Tests**](./unit-tests.md) — Unit testing best practices

**Implementation:**
- GitHub Workflow: `.github/workflows/e2e-tests.yml`
- Test Code: `test/integration/k8s/` and `test/integration/cf/`

---
