# Contributing to Muto

Thank you for contributing to Muto! This guide explains our contribution process, code standards, and community expectations.

## Code of Conduct

All contributors must adhere to our Code of Conduct:

- **Be respectful**: Treat all contributors with professionalism and courtesy
- **Be inclusive**: Welcome contributors from all backgrounds and experience levels
- **Be constructive**: Provide helpful feedback and be open to receiving it
- **No harassment**: We have zero tolerance for harassment, discrimination, or abusive behavior

Violations should be reported to the maintainers.

## Contributing Workflow

### 1. Fork the Repository

```bash
# Click "Fork" on GitHub
git clone https://github.com/YOUR_USERNAME/muto.git
cd muto
git remote add upstream https://github.com/muto-io/muto.git
```

### 2. Create a Feature Branch

```bash
git fetch upstream
git checkout -b feature/your-feature-name upstream/main
```

Branch naming conventions:
- `feat/...` — New features
- `fix/...` — Bug fixes
- `docs/...` — Documentation changes
- `test/...` — Test improvements
- `refactor/...` — Code refactoring
- `perf/...` — Performance improvements
- `chore/...` — Build, dependencies, tooling

### 3. Make Changes

<<<<<<< HEAD
<<<<<<< HEAD
Follow [Code Style Guide](./style.md) for code standards.
=======
Follow [Code Style Guide](./code-style.md) for code standards.
>>>>>>> 97cdc4d (docs: write development/setup.md - dev environment setup)
=======
Follow [Code Style Guide](./style.md) for code standards.
>>>>>>> 3ce5273 (fix: correct code-style references to style.md)

Write tests for your changes:
```bash
# Run tests frequently
make test-unit

# Test specific packages
go test ./path/to/package -v
```

### 4. Commit Changes

Use clear, semantic commit messages:

```
type(scope): short description

Longer explanation if needed. Explain the problem being solved
and why this change solves it.

- Bullet points for multiple changes
- Keep each point focused

Fixes #123
```

**Commit types:**
- `feat` — New feature
- `fix` — Bug fix
- `docs` — Documentation
- `test` — Test additions/changes
- `refactor` — Code refactoring
- `perf` — Performance improvement
- `chore` — Build, dependencies, tooling
- `ci` — CI/CD changes

**Examples:**

Good:
```
feat(scheduler): add priority queue support

Implement a priority queue for agent job scheduling to allow
high-priority jobs to be scheduled before lower-priority ones.

- Add PriorityQueue type in core/scheduler
- Update DefaultScheduler.Schedule() to use priority queue
- Add unit tests for priority ordering
- Update configuration docs with SCHEDULER_PRIORITY_ENABLED

Fixes #42
```

Bad:
```
fixed stuff
```

### 5. Push to Your Fork

```bash
git push origin feature/your-feature-name
```

### 6. Open a Pull Request

**On GitHub:**
1. Click "Compare & pull request"
2. Fill in the PR template with:
   - **Description**: What does this PR do?
   - **Motivation**: Why is this change needed?
   - **Testing**: How was this tested?
   - **Related Issues**: Link to issues using `Fixes #123`

**PR guidelines:**
- Descriptive title (not "Fix bug" or "Update code")
- Reference related issues
- Link to relevant documentation
- Include before/after examples if applicable
- Request review from maintainers

### 7. Respond to Feedback

Address review comments:
- Make requested changes
- Commit with descriptive messages
- Respond to comments (don't just force-push)
- Don't add commits after approval (rebase if needed)

### 8. Merge

Once approved, a maintainer will merge your PR.

## Pull Request Requirements

Your PR must meet these criteria to be merged:

### Code Quality

- **Tests**: All new code must include tests
  - Unit tests for logic (in same package)
  - Integration tests for cross-component interactions
  - No test coverage decrease
- **Linting**: `go vet` and `golangci-lint` pass
- **Formatting**: `go fmt` and `goimports` applied
- **No TODOs**: Fix issues or file them before submitting

### Documentation

- **Code comments**: Exported functions have godoc comments
- **User docs**: Update [docs/](../docs) if behavior changes
- **Examples**: Add code examples for new features
- **README**: Update [README.md](../../README.md) if relevant

### Review Checklist

Before requesting review, verify:

- [ ] Tests written and passing (`make test-unit`)
- [ ] Code formatted (`go fmt ./...`)
- [ ] Linted (`golangci-lint run ./...`)
- [ ] Documentation updated
- [ ] Commit messages follow convention
- [ ] No merge conflicts with main
- [ ] Related issues linked in PR description

## Development Standards

### Testing

**Unit Tests** (fast, in-process):
- Test individual functions and types
- Mock external dependencies
- File: `*_test.go` in same package
- Framework: Go `testing` package + Ginkgo/Gomega

**Integration Tests** (slow, external services):
- Test components working together
- Use real Kubernetes/CloudFoundry
- File: `test/integration/*/`
- Framework: Ginkgo/Gomega + testcontainers

**Test Organization:**
```go
// In core/scheduler/scheduler_test.go
var _ = Describe("Scheduler", func() {
    Describe("Schedule", func() {
        It("assigns job to available platform", func() {
            // test code
        })
    })
})
```

See [Testing Strategy](./testing-strategy.md) for details.

### Code Comments

**Exported functions** must have godoc comments:

```go
// Schedule assigns an agent job to a platform for execution.
// It validates tenant isolation and resource constraints before scheduling.
//
// Parameters:
//   - ctx: context for cancellation and deadlines
//   - job: the agent job to schedule (must be non-nil)
//
// Returns:
//   - jobID: unique identifier for the scheduled job
//   - err: error if scheduling fails (e.g., insufficient resources)
func (s *DefaultScheduler) Schedule(ctx context.Context, job *AgentJob) (string, error) {
    // implementation
}
```

**Inline comments** for complex logic:

```go
// Use exponential backoff to avoid thundering herd
// Start at 100ms, double each retry, cap at 30s
backoff := time.Duration(math.Min(
    30000,
    100 * math.Pow(2, float64(attempt)),
)) * time.Millisecond
```

### Code Organization

**Package structure** mirrors domain:
- `core/agent/` — Job/Agent types and validation
- `core/scheduler/` — Scheduling logic
- `platform/k8s/` — Kubernetes-specific code
- `platform/cf/` — CloudFoundry-specific code
- `mcp/tools/` — MCP tool implementations

**Interfaces** for abstraction:
```go
// MessageBus defines the interface for inter-agent communication
type MessageBus interface {
    Publish(ctx context.Context, topic string, message []byte) error
    Subscribe(ctx context.Context, topic string) (<-chan []byte, error)
}
```

<<<<<<< HEAD
<<<<<<< HEAD
See [Code Style Guide](./style.md) for naming conventions and style details.
=======
See [Code Style Guide](./code-style.md) for naming conventions and style details.
>>>>>>> 97cdc4d (docs: write development/setup.md - dev environment setup)
=======
See [Code Style Guide](./style.md) for naming conventions and style details.
>>>>>>> 3ce5273 (fix: correct code-style references to style.md)

## Common Contributions

### Bug Fix

1. Open an issue describing the bug
2. Create a branch: `git checkout -b fix/issue-name`
3. Write a test that reproduces the bug
4. Fix the bug
5. Verify test passes
6. Commit: `fix(component): brief description`

### New Feature

1. Discuss in an issue or with maintainers first
2. Create a branch: `git checkout -b feat/feature-name`
3. Implement with tests
4. Update documentation
5. Commit: `feat(component): description`

### Documentation

1. Edit the relevant markdown file in `docs/`
2. Preview with your static site generator
3. Commit: `docs: description of changes`

No formal review required for documentation-only PRs.

## Building Locally

Before submitting:

```bash
# Generate code
make generate

# Build binaries
make build

# Run all tests
make test-unit
make test-integration

# Format and lint
go fmt ./...
golangci-lint run ./...
```

## Troubleshooting

### Tests fail locally but pass in CI

<<<<<<< HEAD
1. Ensure Go version matches (1.26+)
=======
1. Ensure Go version matches (1.22+)
>>>>>>> 97cdc4d (docs: write development/setup.md - dev environment setup)
2. Clear module cache: `go clean -modcache && go mod tidy`
3. Run the exact test CI runs: `go test ./test/integration/... -timeout 20m`
4. Check for environment-specific issues (permissions, ports, disk space)

### "go mod tidy" shows errors

```bash
# Verify all dependencies
go mod verify

# Download missing modules
go mod download

# Tidy again
go mod tidy
```

### Lint errors prevent merge

Install golangci-lint:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Run and fix:
```bash
golangci-lint run ./... --fix
```

## Getting Help

- **Questions?** Open a GitHub discussion
- **Bug reports?** Open a GitHub issue with:
  - Go version (`go version`)
  - OS and Docker version
  - Steps to reproduce
  - Expected vs. actual behavior
- **Need guidance?** Ask in an issue or PR comment
- **Security issue?** Email maintainers privately

## Recognition

Contributors are recognized in:
- GitHub contributors list
- Release notes
- Project documentation

Thank you for helping make Muto better!

---

## Maintainers

Current maintainers:
- @muto-io/core-team

---

**Last Updated:** 2026-09-03
