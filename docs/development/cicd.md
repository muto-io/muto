# CI/CD Pipeline

Muto uses GitHub Actions for continuous integration and deployment. This document describes the automated workflows and how to work with them.

## Overview

The CI/CD pipeline automates:
- **Testing** — Unit tests, integration tests, E2E tests
- **Building** — Compile code, build container images
- **Quality** — Code linting, type checking, security scanning
- **Deployment** — Deploy to staging/production environments
- **Documentation** — Build and deploy API documentation

## Workflows

### Main CI Workflow (`.github/workflows/ci.yml`)

Runs on every pull request and push to main branch.

**Triggers:**
- Pull request to any branch
- Push to main branch
- Manual trigger

**Steps:**

1. **Checkout code** — Clone repository
2. **Setup Go** — Install Go toolchain (version from go.mod)
3. **Setup dependencies** — Install required tools
4. **Lint** — Run golangci-lint for code quality
5. **Format** — Check code formatting (gofmt)
6. **Build** — Compile binaries for multiple platforms
7. **Unit tests** — Run `go test ./...`
8. **Generate code** — Run code generators if needed
9. **Security scan** — Run security vulnerability scanner
10. **Upload artifacts** — Save build results

**Outputs:**
- Test results and coverage reports
- Built binaries (Linux, Darwin, Windows)
- Vulnerability scan reports

### E2E Tests Workflow (`.github/workflows/e2e-tests.yml`)

Runs end-to-end tests against live platforms.

**Triggers:**
- Push to main branch
- Weekly schedule (Monday 2 AM UTC)
- Manual trigger

**Jobs:**

**K8s E2E Job:**
- Creates kind cluster (Kubernetes in Docker)
- Deploys Muto controller
- Runs integration tests
- Collects test results
- Timeout: 15 minutes

**CloudFoundry E2E Job:**
- Connects to CF platform (mocked in CI)
- Deploys Muto application
- Runs integration tests
- Collects test results
- Timeout: 15 minutes

**Performance Analysis:**
- Average runtime: ~4 minutes (both jobs parallel)
- Expected P95: 4:30 minutes
- Flakiness: Investigated and documented

### Documentation Build Workflow (`.github/workflows/pages.yml`)

Builds and deploys documentation to GitHub Pages.

**Triggers:**
- Push to main branch (if docs/ changed)
- Manual trigger

**Steps:**

1. **Checkout** — Clone repository
2. **Setup Python** — Install Python 3.11
3. **Install dependencies** — `pip install -r docs-requirements.txt`
4. **Build docs** — `mkdocs build`
5. **Add .nojekyll** — Disable Jekyll processing
6. **Upload artifact** — Save built site
7. **Deploy to Pages** — GitHub Pages deployment

**Output:** Deployed to `muto-io.github.io/muto`

### Dependabot Workflow (`.github/workflows/dependabot-auto-merge.yml`)

Automatically merges safe dependency updates.

**Triggers:**
- Dependabot creates PR for dependency update

**Rules:**
- Auto-merge if CI passes
- Only for patch updates (semver minor/major require review)
- Squash commits before merging

## Status Checks

The following checks must pass before merging to main:

- ✅ **ci / lint** — Code quality checks
- ✅ **ci / test** — Unit test suite
- ✅ **ci / build** — Build succeeds
- ✅ **e2e-tests / k8s-e2e** — Kubernetes E2E tests
- ✅ **e2e-tests / cf-e2e** — CloudFoundry E2E tests

Failed checks block merging. Review the logs to diagnose issues.

## Running Tests Locally

Before pushing, run tests locally to catch issues early.

### Unit Tests

```bash
# Run all tests
go test ./...

# Run specific package
go test ./core/agent

# Run with coverage
go test ./... -cover

# Run with verbose output
go test ./... -v

# Run specific test
go test ./core/agent -run TestAgentLifecycle
```

### Lint

See [:octicons-book-24: **Code Style Guide**](./style.md#linting) for detailed linting guidelines.

```bash
# Run golangci-lint
golangci-lint run ./...

# Auto-fix some issues
golangci-lint run ./... --fix
```

### Format Check

See [:octicons-book-24: **Code Style Guide**](./style.md#formatting) for formatting standards.

```bash
# Check formatting
gofmt -l ./

# Auto-format
gofmt -w ./
```

### Build

```bash
# Build for current platform
go build -o muto ./cmd/muto

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o muto-linux-amd64 ./cmd/muto
GOOS=darwin GOARCH=amd64 go build -o muto-darwin-amd64 ./cmd/muto
```

### Integration Tests

```bash
# Run integration tests
make test-integration-k8s

# Run with specific cluster
export MUTO_USE_EXISTING_CLUSTER=true
export KUBECONFIG=~/.kube/config
make test-integration-k8s
```

### E2E Tests

```bash
# Create kind cluster
make kind-up

# Run E2E tests
make test-e2e

# Clean up
make kind-down
```

### Documentation Build

```bash
# Install dependencies
pip install -r docs-requirements.txt

# Build documentation
mkdocs build

# Serve locally
mkdocs serve
```

## Merge Requirements

Before merging a PR:

1. ✅ All status checks pass (CI, tests, etc.)
2. ✅ At least one approval from maintainers
3. ✅ No requested changes
4. ✅ Branch is up to date with main
5. ✅ Commits are clean (squashed if necessary)

## Deployment Pipeline

### Staging Deployment

Automatic deployment on push to `develop` branch:

```
develop push
    ↓
CI passes
    ↓
Build container image
    ↓
Push to container registry
    ↓
Deploy to staging cluster
    ↓
Run smoke tests
```

### Production Deployment

Manual deployment triggered from main branch:

```
Tag release (v1.2.3)
    ↓
CI runs full test suite
    ↓
Build container image
    ↓
Push to container registry
    ↓
Create GitHub Release
    ↓
Manual approval
    ↓
Deploy to production
```

## Environment Variables

CI workflows use these secrets (configured in GitHub):

| Secret | Purpose |
|--------|---------|
| `REGISTRY_USERNAME` | Container registry username |
| `REGISTRY_PASSWORD` | Container registry password |
| `CF_API_URL` | CloudFoundry API endpoint (E2E) |
| `CF_USERNAME` | CloudFoundry username (E2E) |
| `CF_PASSWORD` | CloudFoundry password (E2E) |
| `SLACK_WEBHOOK` | Slack notification webhook |

## Troubleshooting

### CI Fails Locally But Passes on Push

**Cause:** Different Go version, dependencies, or environment

**Fix:**
```bash
# Match CI environment
go version  # Should match go.mod
go mod tidy
go mod verify
make clean
make test
```

### E2E Tests Timeout

**Cause:** Slow cluster or resource contention

**Solution:**
```bash
# Increase timeout
export MUTO_TEST_TIMEOUT=15m
make test-e2e

# Check cluster resources
kubectl top nodes
kubectl top pods -n muto-system
```

### Documentation Build Fails

**Cause:** Missing dependencies or invalid markdown

**Fix:**
```bash
# Reinstall dependencies
pip install --upgrade -r docs-requirements.txt

# Validate markdown
mkdocs build --strict

# Check for broken links
# (MkDocs plugins can validate)
```

### Dependabot PR Keeps Failing

**Cause:** Dependency incompatibility

**Solution:**
1. Manual merge is acceptable for incompatible updates
2. Update code to use new API
3. Comment on PR to request manual merge

## Performance Metrics

### CI Workflow Times

| Component | Time | Notes |
|-----------|------|-------|
| Lint | ~30s | golangci-lint |
| Build | ~1m | go build |
| Unit tests | ~2m | All unit tests |
| Total | ~5m | Sequential execution |

### E2E Test Times

| Platform | Time | Notes |
|----------|------|-------|
| K8s E2E | ~4m | kind cluster + tests |
| CF E2E | ~4m | Mocked CF + tests |
| Both | ~4m | Parallel execution |

## Best Practices

1. **Run tests locally before pushing**
   - Catch issues early
   - Avoid wasting CI resources

2. **Keep commits atomic**
   - One logical change per commit
   - Makes bisecting easier for debugging

3. **Write meaningful commit messages**
   - First line: summary (50 chars max)
   - Blank line
   - Details and rationale

4. **Update tests with code changes**
   - Add tests for new features
   - Update tests for changed behavior
   - Maintain coverage above threshold

5. **Keep branches short-lived**
   - Merge frequently (daily if possible)
   - Reduces merge conflicts
   - Easier to review

6. **Monitor CI performance**
   - Long CI times slow development
   - Optimize slow tests
   - Consider parallelization

## Viewing Workflow Results

### GitHub Actions UI

1. Go to repository on GitHub
2. Click "Actions" tab
3. Select workflow from left panel
4. Click run to see details
5. Click job to see step logs

### Local Inspection

```bash
# View latest workflow run
gh run list --limit 10

# View specific run details
gh run view <run-id>

# View job logs
gh run view <run-id> --log
```

## Related Documentation

- [:octicons-book-24: **Code Style Guide**](./style.md) — Formatting, linting, and code standards
- [:octicons-book-24: **Testing Strategy**](./testing-strategy.md) — Test organization and best practices
- [:octicons-book-24: **Contributing Guide**](./contributing.md) — Contributing workflow
- [:octicons-book-24: **Development Setup**](./setup.md) — Local development environment
