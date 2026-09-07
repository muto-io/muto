# Running Tests Locally

Guide to setting up your development environment and running tests on your machine.

## Prerequisites

### System Requirements

- **OS:** Linux, macOS, or Windows with WSL2
- **RAM:** 8GB minimum (16GB recommended for integration tests)
- **Disk:** 20GB free space
- **CPU:** 4 cores (8+ recommended)

### Required Tools

Install these tools before running tests:

```bash
# Go (1.26+)
go version

# Docker
docker --version

# Kind (Kubernetes in Docker)
kind version

# kubectl
kubectl version

# Make
make --version

# Git
git --version
```

### Installation

**macOS:**
```bash
brew install go docker kind kubernetes-cli
brew install --cask docker  # Docker Desktop GUI
```

**Linux (Ubuntu/Debian):**
```bash
# Go
wget https://go.dev/dl/go1.26.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.26.linux-amd64.tar.gz

# Docker
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER

# Kind
curl -Lo ./kind https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
chmod +x ./kind
sudo mv ./kind /usr/local/bin/kind

# kubectl
curl -LO https://dl.k8s.io/release/stable.txt
VERSION=$(cat stable.txt)
curl -L "https://dl.k8s.io/release/${VERSION}/bin/linux/amd64/kubectl" -o kubectl
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl
```

**Windows (WSL2):**
Use Ubuntu/Debian steps above

## Development Environment Setup

### Clone Repository

```bash
git clone https://github.com/muto-io/muto.git
cd muto
```

### Install Dependencies

```bash
# Download Go modules
go mod download

# Install development tools
make install-tools
```

### Create Kind Cluster

```bash
# Create single-node cluster
kind create cluster --name muto-test

# Verify cluster
kubectl cluster-info --context kind-muto-test
kubectl get nodes
```

### Verify Setup

```bash
# Run quick diagnostics
make setup-verify
```

Expected output:
```
✓ Go 1.26.0
✓ Docker running
✓ Kind cluster ready
✓ kubectl configured
✓ All tools installed
```

## Running Tests

### Quick Test Suite (5 min)

Test just the core logic without platform integration:

```bash
make test-unit
```

**What it tests:**
- Job validation
- State machine logic
- Scheduler logic
- Message formatting

### Full Test Suite (30 min)

Unit tests + integration tests:

```bash
make test
```

**What it includes:**
1. Unit tests (5 min)
2. Integration tests - Kubernetes (15 min)
3. Integration tests - CloudFoundry (10 min)

### Platform-Specific Tests

**Kubernetes only:**
```bash
make test-k8s
```

**CloudFoundry only:**
```bash
make test-cf
```

### Specific Test

Run a single test:

```bash
# By function name
go test ./core/agent -run TestValidate -v

# By pattern
go test ./... -run "JobScheduling" -v

# By label (integration tests)
go test ./test/integration/k8s/... -v
```

### Watch Mode

Rerun tests on file changes:

```bash
# Install watcher
go install github.com/cosmtrek/air@latest

# Run in watch mode
air
```

## Debugging Tests

### Verbose Output

See detailed test logs:

```bash
go test ./core/agent -v
```

### Show Ginkgo Output

For integration tests:

```bash
go test ./test/integration/k8s/... -v -ginkgo.v
```

### Debug Single Test

Stop on first failure and show full output:

```bash
go test ./core/agent -run TestName -v -ginkgo.fail-fast
```

### Print Logs During Test

```go
It("does something", func() {
    fmt.Println("Debug output here")
    By("when this happens")
    // Your test
})
```

Run with output:
```bash
go test ./... -v -args -ginkgo.v
```

### Attach Debugger

Use your IDE's debugger or Delve:

```bash
# Install Delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Run test under debugger
dlv test ./core/agent -- -test.run TestValidate
```

## Troubleshooting Common Issues

### Docker Not Running

```bash
# macOS
open -a Docker

# Linux (systemd)
sudo systemctl start docker

# Verify
docker ps
```

### Kind Cluster Issues

```bash
# Recreate cluster
kind delete cluster --name muto-test
kind create cluster --name muto-test

# Verify cluster is ready
kubectl cluster-info
kubectl get nodes
```

### Out of Memory

Increase Docker memory limit:

**Docker Desktop (GUI):**
1. Preferences -> Resources
2. Set Memory to 8GB+

**Command line:**
```bash
docker run --memory 8g --memory-swap 8g
```

### Port Already in Use

If NATS or other services fail to start:

```bash
# Find what's using the port
lsof -i :4222  # NATS
lsof -i :6379  # Redis

# Kill process
kill -9 <PID>
```

### Tests Timeout

Increase timeout for slow systems:

```bash
go test ./... -timeout 40m
```

### Dependency Download Failures

```bash
# Clear cache
go clean -modcache

# Re-download
go mod download
```

### Tests Pass Locally but Fail in CI

1. **Check Go version:**
   ```bash
   go version  # Should be 1.26+
   ```

2. **Check Docker:**
   ```bash
   docker ps
   ```

3. **Clear cache and retry:**
   ```bash
   go clean -modcache
   go mod tidy
   make test-unit
   ```

## Testing Workflow

### Before Committing

```bash
# Run quick tests
make test-unit

# Check formatting
make fmt

# Run linter
make lint
```

### Before Pushing

```bash
# Full test suite
make test

# Check coverage
make coverage
```

### Before Opening PR

```bash
# Full validation
make validate
```

## Viewing Test Coverage

### Generate Coverage Report

```bash
# All tests
go test ./... -coverprofile=coverage.out

# Specific package
go test ./core/agent -coverprofile=coverage.out
```

### View Results

**HTML Report:**
```bash
go tool cover -html=coverage.out
```

**Command Line:**
```bash
go tool cover -func=coverage.out
```

**By threshold:**
```bash
# Find functions below 80% coverage
go tool cover -func=coverage.out | awk '$NF < 80'
```

## Continuous Development

### Test-Driven Development (TDD)

```bash
# 1. Write failing test
# vim core/agent/job_test.go
# Add: It("should do X", func() { ... })

# 2. Run test to see it fail
go test ./core/agent -run TestX -v

# 3. Write minimal code to pass
# vim core/agent/job.go

# 4. Run test again
go test ./core/agent -run TestX -v

# 5. Refactor if needed
# Repeat
```

### Incremental Testing

Test only what changed:

```bash
# Show what files changed
git diff --name-only

# Test only modified packages
git diff --name-only | xargs -I {} dirname {} | sort -u | \
  xargs -I {} go test ./{}/...
```

## Performance Tips

### Speed Up Tests

1. **Run in parallel:**
   ```bash
   go test -parallel 8 ./...
   ```

2. **Skip slow tests during development:**
   ```bash
   go test -short ./...
   ```

3. **Use `-run` to skip tests:**
   ```bash
   go test -run "^TestFast" ./...
   ```

4. **Cache test results:**
   ```bash
   go test -count=1 ./...  # Force rerun (don't cache)
   ```

### Improve Resource Usage

```bash
# Limit concurrent tests
go test -p 1 ./...

# Reduce test verbosity
go test ./... -q
```

## IDE Integration

### Visual Studio Code

Install Go extension:
1. Open Extensions (Cmd/Ctrl + Shift + X)
2. Search "Go" -> Install official extension
3. Cmd/Ctrl + Shift + P -> "Go: Install/Update Tools"

**Run tests in VSCode:**
- Right-click test file -> "Run Tests"
- Click CodeLens on test function
- Cmd/Ctrl + Shift + T to toggle test file

### GoLand / IntelliJ IDEA

Built-in Go support:
1. Open test file
2. Click green play icon next to test name
3. Or right-click -> "Run"

### Vim/Neovim

```vim
" Install vim-go plugin
" Then use: :GoTest
```

## GitHub Actions Locally

### Simulate CI Environment

Use `act` to run GitHub Actions locally:

```bash
# Install act
brew install act

# Run all workflows
act

# Run specific workflow
act -j test
```

## Making Changes

### Typical Workflow

```bash
# 1. Create branch
git checkout -b feature/my-feature

# 2. Make changes
vim core/agent/job.go
vim core/agent/job_test.go

# 3. Run tests
make test-unit

# 4. If tests pass, run full suite
make test

# 5. Format code
make fmt

# 6. Commit
git add -A
git commit -m "feat: add my feature"

# 7. Push and create PR
git push -u origin feature/my-feature
```

### Common Make Targets

```bash
make test-unit        # Unit tests only
make test-k8s        # Kubernetes integration tests
make test-cf         # CloudFoundry integration tests
make test            # All tests
make fmt             # Format code
make lint            # Run linter
make build           # Build binaries
make clean           # Clean build artifacts
make help            # Show all targets
```

---

## Related Documentation

- [:octicons-book-24: **Testing Overview**](./overview.md) — Testing strategy
- [:octicons-book-24: **Unit Tests**](./unit-tests.md) — Unit testing guide
- [:octicons-book-24: **Integration Tests**](./integration-tests.md) — Integration testing guide
- [:octicons-book-24: **Development Setup**](../development/setup.md) — Full development setup
- [:octicons-book-24: **Contributing Guide**](../development/contributing.md) — Contribution workflow

---

**Last Updated:** 2026-09-06
