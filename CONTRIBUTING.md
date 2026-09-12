# Contributing to Muto

Thank you for your interest in contributing to Muto! This document outlines our contribution guidelines and review process.

## Code of Conduct

We are committed to providing a welcoming and inclusive environment for all contributors. Please review and abide by our Code of Conduct.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<your-username>/muto.git`
3. Create a feature branch: `git checkout -b feature/your-feature-name`
4. Make your changes and commit with clear messages
5. Push to your fork and open a Pull Request

## Development Workflow

### Prerequisites

- Go 1.26+
- Docker
- kind (Kubernetes in Docker)
- kubectl

### Running Tests

Muto has a comprehensive test suite covering unit tests, integration tests, and end-to-end tests across multiple platforms.

#### Quick Start

```bash
# Unit tests
make test-unit

# Kubernetes E2E tests (requires Docker and testcontainers)
make test-integration-k8s

# Cloud Foundry E2E tests (requires CF credentials or mocked setup)
make test-integration-cf

# All tests
make test-e2e
```

#### Detailed Test Instructions

See [test/integration/README.md](test/integration/README.md) for comprehensive testing documentation including:
- Detailed Kubernetes and Cloud Foundry test setup
- How to add new tests
- Test debugging and troubleshooting
- Performance baselines and timeout configuration

## Review Process & Team Structure

For detailed information about:
- **Review SLAs and expectations**: See [Team Onboarding Guide - Review Process](docs/team/onboarding.md#review-process)
- **Team structure and responsibilities**: See [Team Onboarding Guide - Team Structure](docs/team/onboarding.md#team-structure-overview)
- **Code ownership and CODEOWNERS**: See [Team Onboarding Guide - Code Ownership](docs/team/onboarding.md#code-ownership--codeowners)
- **Dependabot workflow**: See [Team Onboarding Guide - Understanding Dependabot](docs/team/onboarding.md#understanding-dependabot)
- **Escalation procedures**: See [Team Onboarding Guide - Escalation Path](docs/team/onboarding.md#escalation-path)

**Quick reference**: Pull requests are reviewed by code owners from our team structure. All required reviewers must approve before merge. Expected response time is within 4 hours during business hours, or next business day outside business hours.

## Pull Request Guidelines

### Before Submitting

- Ensure all tests pass locally: `make test-e2e`
- Keep commits focused and logically organized
- Write clear, descriptive commit messages
- Update documentation if your changes affect user-facing behavior

### PR Description

Include the following in your PR description:

- **What**: Brief summary of the change
- **Why**: Motivation and context for the change
- **How**: Key implementation details (if non-obvious)
- **Testing**: How to verify the change works
- **Checklist**:
  - [ ] Tests pass locally
  - [ ] Documentation updated (if applicable)
  - [ ] No breaking changes (or clearly documented if intentional)

### Commit Messages

Follow conventional commits format:

```
type(scope): subject

body

footer
```

Examples:
- `feat: Add priority queue support`
- `fix: Resolve pod reconciliation race condition`
- `docs: Update AgentJob CRD examples`
- `feat: Bump sigs.k8s.io/controller-runtime`

## E2E Test Infrastructure

Muto features a comprehensive end-to-end test infrastructure supporting multiple platforms:

### Kubernetes (K8s) Testing

Tests located in `test/integration/k8s/` validate Muto's Kubernetes platform adapter:

- **Multi-Agent Coordination**: Agent orchestration with multiple roles, replica scaling, and message bus communication
- **Failure Scenarios**: Resource constraints, pod lifecycle failures, and concurrent failure handling
- **Stress Testing**: High-volume job creation, concurrent operations, and resource exhaustion scenarios
- **Core Lifecycle**: Job creation, execution, completion, and resource cleanup

#### Running K8s Tests

```bash
# Run all K8s tests
make test-integration-k8s

# Run specific test suite
ginkgo -v --focus="Multi-Agent" ./test/integration/k8s
ginkgo -v --focus="Failure" ./test/integration/k8s
ginkgo -v --focus="Stress" ./test/integration/k8s
```

The K8s test suite uses testcontainers to spin up a k3s cluster automatically. Tests run in approximately 15-20 minutes.

### Cloud Foundry (CF) Testing

Tests located in `test/integration/cf/` validate Muto's Cloud Foundry platform adapter:

- **Job Lifecycle**: Task creation, execution, and completion handling
- **Multi-Agent Coordination**: Cross-role orchestration and communication patterns
- **Failure Scenarios**: Timeouts, crashes, and out-of-memory conditions
- **Tenant Isolation**: Multi-tenancy verification and security boundaries
- **Stress Testing**: High-volume concurrent operations

#### Running CF Tests

```bash
# Using existing CF instance
export CF_E2E_API_URL=https://api.cf.your-domain.com
export CF_E2E_USERNAME=admin
export CF_E2E_PASSWORD=password
make test-integration-cf

# Run specific test suite
ginkgo -v --focus="Lifecycle" ./test/integration/cf
ginkgo -v --focus="Isolation" ./test/integration/cf
ginkgo -v --focus="Stress" ./test/integration/cf
```

### Test Helpers and Utilities

Common test utilities are shared between K8s and CF tests:

- **`K8sTestHelper`**: Manages unique namespaces and test counters
- **`CFTestHelper`**: Generates unique spaces, apps, and tenant names
- **`WaitFor`**: Generic polling utility for async verification
- **`WaitForJobPhase`**: K8s-specific job phase waiter
- **`WaitForTaskState`**: CF-specific task state waiter

See [test/integration/README.md](test/integration/README.md) for complete test infrastructure documentation including templates for adding new tests.

## CI/CD Workflow

Muto uses GitHub Actions to automate testing, building, and deployment processes.

### Workflow Overview

**File:** `.github/workflows/ci.yml`

The CI workflow runs on every push and pull request:

1. **Lint** — Code style validation using `golangci-lint`
2. **Unit Tests** — Fast unit test suite with coverage reporting
3. **Build** — Compilation verification for binaries and integration tests
4. **Kubernetes Integration Tests** — Full K8s platform adapter validation
5. **Cloud Foundry Integration Tests** — CF platform adapter validation (continue-on-error if CF unavailable)
6. **Helm Lint** — Chart syntax validation

#### Triggers

- **Pull Requests** — When PR targets main branch
- **All Branches** — On every push
- **Weekly** — Scheduled runs for regression testing

### Test Artifact Retention

- Test results are uploaded as GitHub artifacts
- Retention period: 30 days
- Accessible via Actions tab on pull request or branch view

### E2E Test Workflow

**File:** `.github/workflows/e2e-tests.yml`

Dedicated workflow for end-to-end testing:

- **Triggers**: PR changes to platform/core/test code, pushes to main, weekly schedule
- **K8s Tests**: Always run (required for PR approval)
- **CF Tests**: Run only if CF credentials configured in GitHub secrets

#### Enabling CF Testing in CI

To enable Cloud Foundry testing in GitHub Actions:

1. Navigate to repository Settings → Secrets and variables → Actions
2. Add these secrets:
   - `CF_API_URL` — Cloud Foundry API endpoint
   - `CF_USERNAME` — CF admin username
   - `CF_PASSWORD` — CF admin password

### Local CI Simulation

To verify your changes pass CI locally:

```bash
# Run linter
golangci-lint run ./...

# Run all tests
make test-e2e

# Verify build
make build

# Lint Helm chart
helm lint deploy/helm/muto
```

## Dependabot Automation

Muto uses GitHub Dependabot to keep dependencies current and secure. See [Team Onboarding Guide - Understanding Dependabot](docs/team/onboarding.md#understanding-dependabot) for:

- How Dependabot creates and manages pull requests
- Automated SLA for patch and minor updates
- Manual review process for major version updates
- Security update handling and prioritization
- Troubleshooting common Dependabot scenarios

**Configuration file:** `.github/dependabot.yml`

## Team Structure & Code Ownership

See [Team Onboarding Guide - Team Structure Overview](docs/team/onboarding.md#team-structure-overview) for detailed team organization, responsibilities, and code ownership information.

The authoritative code ownership configuration is at [.github/CODEOWNERS](.github/CODEOWNERS).

## Testing Best Practices

### General Guidelines

1. **Test Before Submitting** — Run the full test suite before opening a PR:
   ```bash
   make test-e2e
   ```

2. **Write Focused Tests** — Each test should verify one behavior
   - Avoid testing multiple concerns in one test
   - Use descriptive test names that explain what is being verified
   - Keep test setup minimal and focused

3. **Use Table-Driven Tests** — For multiple similar scenarios in unit tests:
   ```go
   testCases := []struct {
       input    string
       expected string
   }{
       {"case1", "result1"},
       {"case2", "result2"},
   }
   for _, tc := range testCases {
       t.Run(tc.input, func(t *testing.T) {
           // test logic
       })
   }
   ```

### Unit Test Best Practices

- **Isolation**: Mock external dependencies (HTTP, databases, message buses)
- **Deterministic**: Tests should always produce the same result
- **Fast**: Unit tests should complete in milliseconds
- **Coverage**: Aim for meaningful coverage; prioritize critical paths over coverage numbers
- **No Sleeps**: Use conditional polling instead of `time.Sleep()` in tests

### Integration Test Best Practices

#### Kubernetes Tests

- **Unique Namespaces**: Always use unique namespaces per test to avoid conflicts:
  ```go
  testCounter++
  nsName := fmt.Sprintf("test-namespace-%d", testCounter)
  ```

- **Clean Up Resources**: Always delete created resources in `AfterEach`:
  ```go
  AfterEach(func() {
      ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
      _ = k8sClient.Delete(ctx, ns)
  })
  ```

- **Use Eventually for Async Operations**: Don't use `time.Sleep()` for async verification:
  ```go
  Eventually(func(g Gomega) {
      job := &v1alpha1.AgentJob{}
      g.Expect(k8sClient.Get(ctx, key, job)).To(Succeed())
      g.Expect(job.Status.Phase).To(Equal("Running"))
  }).WithTimeout(30 * time.Second).Should(Succeed())
  ```

- **Proper Resource Labeling**: Always label resources with standard labels:
  ```yaml
  labels:
    muto.io/job: my-job
    muto.io/role: worker
    muto.io/tenant: my-tenant
  ```

#### Cloud Foundry Tests

- **Skip Gracefully**: Handle missing CF clusters gracefully:
  ```go
  BeforeEach(func() {
      if cfCluster == nil {
          Skip("CF cluster not available")
      }
  })
  ```

- **Generate Unique Names**: Use the CF test helper to generate unique resource names:
  ```go
  spaceName := cfHelper.NextSpace()    // → "muto-test-1", "muto-test-2", etc.
  tenantName := cfHelper.NextTenant()  // → "tenant-1", "tenant-2", etc.
  ```

- **Use Docker Images**: Always specify container images:
  ```go
  DockerImage: "busybox:latest"
  ```

- **Set Resource Constraints**: Test with realistic resource limits:
  ```go
  taskReq := cf.TaskRequest{
      Name:       "constrained-task",
      Command:    "my-command",
      MemoryInMB: 128,
      DiskInMB:   512,
  }
  ```

### CI/CD Best Practices

1. **Local Testing** — Always run `make test-e2e` before pushing
2. **Clear Commit Messages** — Use conventional commit format for clarity
3. **Minimal Changes** — Keep PRs focused; split large changes into multiple PRs
4. **Test Artifact Review** — Check uploaded test artifacts if tests fail in CI
5. **Address Failures Promptly** — Fix CI failures quickly to maintain code quality

### Test Naming Conventions

#### File Names
- K8s: Descriptive like `k8s_multiagent_coordination_test.go`
- CF: Prefixed with `e2e_` like `e2e_failure_scenarios_test.go`

#### Test Descriptions
```go
Describe("Feature Category", func() {
    Describe("specific scenario", func() {
        It("should verify specific behavior", func() {
            // test implementation
        })
    })
})
```

Example:
```
Describe("K8s Multi-Agent Coordination")
  Describe("multi-agent job orchestration")
    It("should coordinate coordinator and worker agents")
```

### Test Troubleshooting

**Tests hang on pod creation:**
```bash
# Check if k3s cluster is running
docker ps | grep k3s

# Increase timeout
ginkgo -timeout 30m ./test/integration/k8s
```

**CF tests skip unexpectedly:**
```bash
# Verify CF credentials
echo $CF_E2E_API_URL
echo $CF_E2E_USERNAME

# Create test org and spaces
cf create-org muto-e2e-test-org
cf create-space -o muto-e2e-test-org muto-test-1
```

**Coverage gaps in tests:**
```bash
# Generate coverage report
make test-unit  # Creates coverage.out

# View coverage
go tool cover -html=coverage.out
```

## Code Review Process

### What Reviewers Look For

- **Correctness**: Does the code work as intended?
- **Testing**: Are there sufficient test cases for new behavior?
- **Performance**: Could this introduce performance regressions?
- **Security**: Are there potential security concerns?
- **Maintainability**: Is the code clear and maintainable?

### Author Response

When addressing feedback:

1. Acknowledge each piece of feedback
2. Make necessary changes or explain why a suggestion isn't adopted
3. Reply to each review comment
4. Request re-review after making changes

## Deployment & Releases

Muto follows semantic versioning (MAJOR.MINOR.PATCH). Releases are created by maintainers and published to:

- GitHub Releases
- Container registries (Docker Hub, GHCR)
- Go module repositories

Release notes should summarize breaking changes, new features, and bug fixes.

## Reporting Issues

When reporting bugs, please include:

- Go version (`go version`)
- Kubernetes/Cloud Foundry version (if applicable)
- Steps to reproduce
- Expected vs. actual behavior
- Relevant logs or error messages

## Questions?

- Check existing [issues](https://github.com/muto-io/muto/issues) and [discussions](https://github.com/muto-io/muto/discussions)
- Open a new discussion for questions or ideas
- Reach out to the maintainers team for urgent concerns

Thank you for contributing to Muto!
