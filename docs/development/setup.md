# Development Environment Setup

This guide helps you set up a local development environment to contribute to Muto.

## Prerequisites

### System Requirements

- **OS**: Linux, macOS, or Windows with WSL2
- **CPU**: 2+ cores (4+ recommended)
- **RAM**: 4GB minimum (8GB+ recommended)
- **Disk**: 20GB free space (for Docker images and build artifacts)

### Required Tools

<<<<<<< HEAD
#### Go 1.26+

The project uses Go 1.26 or later. Install from [golang.org](https://golang.org/dl/).
=======
#### Go 1.22+

The project uses Go 1.22 or later. Install from [golang.org](https://golang.org/dl/).
>>>>>>> 97cdc4d (docs: write development/setup.md - dev environment setup)

**Verify installation:**
```bash
go version
<<<<<<< HEAD
# Expected: go version go1.26.0 (or later)
=======
# Expected: go version go1.22.0 (or later)
>>>>>>> 97cdc4d (docs: write development/setup.md - dev environment setup)
```

#### Docker

Required for building container images and running integration tests.

**macOS/Windows:** Install [Docker Desktop](https://www.docker.com/products/docker-desktop)

**Linux:**
```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
newgrp docker
```

**Verify installation:**
```bash
docker run hello-world
```

#### kubectl

Kubernetes CLI for managing test clusters.

**macOS:**
```bash
brew install kubectl
```

**Linux:**
```bash
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

**Windows (WSL2):**
```bash
choco install kubernetes-cli
```

**Verify installation:**
```bash
kubectl version --client
```

#### kind

Kubernetes in Docker—creates local K8s clusters for testing.

```bash
go install sigs.k8s.io/kind@latest
```

**Verify installation:**
```bash
kind version
```

#### make

Build automation tool.

**macOS:**
```bash
brew install make
```

**Linux (Debian/Ubuntu):**
```bash
sudo apt-get install make
```

**Windows:** Install via [GNU Make for Windows](https://gnuwin32.sourceforge.net/packages/make.htm) or WSL2

**Verify installation:**
```bash
make --version
```

#### Optional: Code Editor/IDE

- **VS Code** with [Go extension](https://marketplace.visualstudio.com/items?itemName=golang.Go)
- **GoLand** (JetBrains)
- **vim/neovim** with gopls

## Clone the Repository

```bash
git clone https://github.com/muto-io/muto.git
cd muto
```

## Build from Source

### Install Dependencies

```bash
go mod download
go mod tidy
```

### Generate Code

Generate Kubernetes CRDs and code from type definitions:

```bash
make generate
```

This runs `controller-gen` to create:
- CRD manifests in `deploy/crds/`
- Go deepcopy methods in `platform/k8s/types/`

### Build Binaries

Development build (includes debug symbols):

```bash
make build
```

Creates:
- `bin/muto-operator` — Kubernetes operator
- `bin/muto-mcp` — MCP server

Verify binaries:
```bash
./bin/muto-operator --version
./bin/muto-mcp --version
```

### Build Docker Images

For local testing:

```bash
make docker-build
```

This builds:
- `muto-operator:latest`
- `muto-mcp:latest`

To push to a registry:

```bash
export DOCKER_REGISTRY=myregistry.com
make docker-build docker-push
```

## Local Kubernetes Cluster

### Create a Test Cluster

```bash
make kind-up
```

This creates a kind cluster named `muto-dev` with:
- Kubernetes v1.30+
- Required CRDs pre-installed
- Kubeconfig automatically configured

**Verify cluster:**
```bash
kubectl cluster-info
kubectl get nodes
```

### Deploy Muto to the Cluster

Option 1: Use Helm chart (recommended):
```bash
kubectl create namespace muto-system
helm install muto ./deploy/helm/muto \
  --namespace muto-system \
  --values ./deploy/helm/values.yaml
```

Option 2: Apply CRDs and run operator locally:
```bash
kubectl apply -f deploy/crds/
./bin/muto-operator
```

**Verify operator is running:**
```bash
kubectl get pods -n muto-system
# Or if running locally, check logs in terminal
```

### Clean Up Cluster

```bash
make kind-down
```

## Running Tests

### Unit Tests

Fast, in-memory tests with no dependencies:

```bash
make test-unit
```

Generates coverage report in `coverage.out`:
```bash
go tool cover -html=coverage.out
```

### Integration Tests

Tests that use real Kubernetes and CloudFoundry environments. Requires Docker and may take 10-20 minutes.

**All integration tests:**
```bash
make test-integration
```

**Kubernetes integration tests only:**
```bash
make test-integration-k8s
```

**CloudFoundry integration tests only:**
```bash
make test-integration-cf
```

**Run specific test:**
```bash
go test ./test/integration/k8s/... -run TestAgentJobLifecycle -v
```

### End-to-End Tests

Runs both K8s and CF integration test suites:

```bash
make test-e2e
```

### Test with Coverage

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

## IDE Setup

### VS Code

**Install Go extension:**
1. Open Extensions (Ctrl+Shift+X)
2. Search for "Go"
3. Install the official Go extension by Google

**Configure settings.json:**
```json
{
  "go.lintOnSave": "package",
  "go.lintTool": "golangci-lint",
  "go.lintArgs": ["--enable-all"],
  "go.formatTool": "goimports",
  "go.useLanguageServer": true,
  "[go]": {
    "editor.formatOnSave": true,
    "editor.codeActionsOnSave": {
      "source.organizeImports": true
    }
  }
}
```

### GoLand

1. Open project settings (Cmd+, on macOS or Ctrl+Alt+S on Linux/Windows)
2. Go -> Code Style -> Go
3. Set formatting to "goimports"
4. Enable inspections for common issues

### vim/neovim with gopls

Use [coc.nvim](https://github.com/neoclide/coc.nvim) or [vim-lsp](https://github.com/prabirshrestha/vim-lsp) with gopls configured.

## Development Workflow

### 1. Create a feature branch

```bash
git checkout -b feature/your-feature-name
```

### 2. Make changes

Edit code, write tests, commit regularly.

### 3. Run tests locally

```bash
make test-unit
# Test specific package:
go test ./core/agent -v
```

### 4. Format and lint

```bash
go fmt ./...
go vet ./...
```

Or use `golangci-lint` (if installed):
```bash
golangci-lint run ./...
```

### 5. Commit with proper message format

```bash
git commit -m "feat: add new scheduler optimization

- Improved job assignment algorithm
- Added unit tests for edge cases
- Updated documentation"
```

See [Contributing](./contributing.md) for commit message guidelines.

### 6. Push and open PR

```bash
git push origin feature/your-feature-name
```

Then open a pull request on GitHub.

## Troubleshooting

### Module tidy fails

```bash
# Clear module cache
go clean -modcache
# Re-download dependencies
go mod tidy
```

### kind cluster creation fails

```bash
# Check Docker is running and has resources
docker ps
docker system df

# Delete problematic cluster
kind delete cluster --name muto-dev

# Try again with explicit config
make kind-up
```

### Tests timeout

Increase test timeout:
```bash
go test ./test/integration/... -timeout 30m -v
```

### Docker image build fails

Check available disk space:
```bash
df -h
docker system prune  # Clean up dangling images
```

### golangci-lint not found

Install it:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Next Steps

- Read [Contributing Guidelines](./contributing.md) for code contribution process
- See [Testing Strategy](./testing-strategy.md) for testing best practices
<<<<<<< HEAD
<<<<<<< HEAD
- Check [Code Style Guide](./style.md) for Go conventions
=======
- Check [Code Style Guide](./code-style.md) for Go conventions
>>>>>>> 97cdc4d (docs: write development/setup.md - dev environment setup)
=======
- Check [Code Style Guide](./style.md) for Go conventions
>>>>>>> 3ce5273 (fix: correct code-style references to style.md)
- Review [Debugging Guide](./debugging.md) for troubleshooting techniques

---

**Last Updated:** 2026-09-03
