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

## Review SLAs

We maintain Service Level Agreements (SLAs) for code review to ensure timely feedback and maintain project momentum. These SLAs apply to all pull requests from external contributors and internal team members.

### PR Review Turnaround

- **First Response**: Within 2 business days
  - A review comment, question, or acknowledgment that indicates active review has begun
- **Final Decision**: Within 5 business days
  - Approval, request for changes, or clear feedback on next steps

**Note:** Business days exclude weekends and recognized holidays. For distributed teams across timezones, we aim to have at least one reviewer available during working hours.

### Dependabot PRs

Dependabot pull requests follow an expedited, automated SLA to keep dependencies current and secure:

- **Patch Updates** (e.g., 1.2.3 -> 1.2.4):
  - Automatically approved and merged if all checks pass
  - No manual review required
  - Merged within minutes of tests passing
  - Commit squashed for clean history

- **Minor Updates** (e.g., 1.2.3 -> 1.3.0):
  - Automatically approved and merged if all checks pass
  - No manual review required
  - Merged within minutes of tests passing

- **Major Version Updates** (e.g., 1.2.3 -> 2.0.0):
  - Flagged for manual review
  - Require explicit approval within 3 business days
  - Reviewer checks for breaking changes and compatibility

- **Security Updates**:
  - Treated as high priority regardless of version bump
  - Auto-merged if patch or minor; manual review for major versions
  - Target approval within 1 business day for major security patches

### Escalation Path

If a pull request is not reviewed within the SLA window:

1. **After 5 business days without review**: PR is automatically labeled `sla-warning`
2. **After 7 business days without review**: Automated reminder is posted to the PR and the `@muto-io/maintainers` group is mentioned
3. **If still blocked after 9 business days**: Issue is escalated to the project lead for manual intervention and discussed in the next team standup

### Team Availability Guidelines

To maintain responsive review coverage, we aim for:

- **Geographic Coverage**: At least 1 active reviewers available during each 8-hour window across major timezones

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

Muto uses GitHub Dependabot to keep dependencies current and secure with automated pull requests and intelligent workflows.

### Automated Dependency Updates

Dependabot creates pull requests for:

- **Go module updates** — Twice weekly (Monday and Thursday)
- **GitHub Actions updates** — Weekly
- **Docker image updates** — As available

### Dependabot PR Workflow

All Dependabot PRs follow an automated SLA for fast, secure merges:

#### Patch Updates (e.g., 1.2.3 → 1.2.4)

- Automatically approved and merged if all checks pass
- Merged within minutes of CI passing
- Commits squashed for clean history
- Entirely automated (no manual review required)

#### Minor Updates (e.g., 1.2.3 → 1.3.0)

- Automatically approved and merged if all checks pass
- Merged within minutes of CI passing
- Entirely automated (no manual review required)

#### Major Version Updates (e.g., 1.2.3 → 2.0.0)

- Flagged for manual review
- Requires explicit approval within 3 business days
- Reviewers check for breaking changes and compatibility issues
- Requires explicit approval before merge

#### Security Updates

- Treated as high priority regardless of version bump
- Auto-merged if patch or minor version
- Manual review required for major security versions
- Target approval within 1 business day for critical security patches

### Automation Configuration

**File:** `.github/workflows/dependabot-auto-merge.yaml`

The automation:
1. Monitors all Dependabot pull requests
2. Approves patch and minor updates automatically
3. Allows major updates to be reviewed manually
4. Merges approved PRs when all checks pass
5. Flags security updates for priority review

### SLA Escalation

If any pull request is not reviewed within the SLA window:

1. **After 5 business days**: Automatically labeled `sla-warning`
2. **After 7 business days**: Automated reminder posted to PR and `@muto-io/maintainers` mentioned
3. **After 9 business days**: Escalated to project lead for manual intervention

## Team Structure & Code Ownership

Muto organizes responsibilities by component and platform using GitHub's CODEOWNERS mechanism.

### Team Organization

**Maintainers** (`@muto-io/maintainers`)
- Final approval on all code changes
- Release management and versioning
- Cross-component architecture decisions

**Platform Team** (`@muto-io/platform-team`)
- Platform adapters architecture
- Multi-platform consistency
- Platform-independent abstractions

**Kubernetes Team** (`@muto-io/k8s-team`)
- Kubernetes-specific implementation
- K8s integration tests
- Kubernetes operator development

**Cloud Foundry Team** (`@muto-io/cf-team`)
- Cloud Foundry platform adapter
- CF integration tests
- CF-specific features and optimizations

**Core Team** (`@muto-io/core-team`)
- Agent runtime and execution
- Message bus integration
- Core business logic

**QA Team** (`@muto-io/qa-team`)
- Test infrastructure development
- Integration test coverage
- Test automation and frameworks

**DevOps Team** (`@muto-io/devops-team`)
- CI/CD pipeline management
- Deployment automation
- Infrastructure and Helm charts

### Code Ownership Map

```
.
├── core/                      → @core-team @maintainers
├── platform/                  → @platform-team @maintainers
│   ├── k8s/                   → @k8s-team @platform-team @maintainers
│   └── cf/                    → @cf-team @platform-team @maintainers
├── test/                      → @qa-team @maintainers
│   ├── integration/k8s/       → @qa-team @k8s-team @maintainers
│   └── integration/cf/        → @qa-team @cf-team @maintainers
├── .github/                   → @devops-team @maintainers
├── .github/workflows/         → @devops-team @maintainers
├── .github/dependabot.yml     → @devops-team @maintainers
├── Makefile                   → @devops-team @maintainers
├── deploy/                    → @devops-team @maintainers
└── deploy/helm/               → @devops-team @maintainers
```

### Review Assignment

- **Platform changes**: Multiple owners must approve
- **Core logic changes**: Core team + maintainers approval required
- **Test infrastructure**: QA team review recommended
- **Deployment/CI**: DevOps team review recommended
- **Cross-component changes**: All affected teams notified via CODEOWNERS

See [.github/CODEOWNERS](.github/CODEOWNERS) for the authoritative ownership configuration.

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
