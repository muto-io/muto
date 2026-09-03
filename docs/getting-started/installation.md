# Installation Guide

Detailed steps to install Muto from source.

## System Requirements

### Minimum Requirements
- **CPU**: 2 cores
- **RAM**: 2 GB
- **Disk**: 5 GB free space
- **Network**: Outbound HTTP/HTTPS for package downloads

### Recommended Requirements (Production)
- **CPU**: 4 cores
- **RAM**: 8 GB
- **Disk**: 20 GB (for container images and logs)
- **Network**: Stable, low-latency connection to cluster

## Prerequisites

### 1. Install Go

Muto requires Go 1.26 or later.

**macOS (Homebrew):**
```bash
brew install go@1.26
```

**Linux:**
```bash
# Download from golang.org
curl -LO https://dl.google.com/go/go1.26.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

**Windows:**
Download installer from [golang.org](https://golang.org/dl/)

**Verify installation:**
```bash
go version
# Output: go version go1.26.0 linux/amd64
```

### 2. Install Docker

Muto runs containerized agents, so Docker is required.

**macOS/Windows:** Download [Docker Desktop](https://www.docker.com/products/docker-desktop)

**Linux:**
```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
```

**Verify installation:**
```bash
docker run hello-world
```

### 3. Install kubectl

Kubernetes CLI for managing clusters.

**macOS:**
```bash
brew install kubectl
```

**Linux:**
```bash
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
```

**Windows:**
```bash
choco install kubernetes-cli
```

**Verify installation:**
```bash
kubectl version --client
```

### 4. Install kind (for local development)

kind creates Kubernetes clusters in Docker.

```bash
go install sigs.k8s.io/kind@latest
```

**Verify installation:**
```bash
kind version
```

### 5. Install Helm (for Kubernetes deployments)

Helm is a package manager for Kubernetes.

**macOS:**
```bash
brew install helm
```

**Linux:**
```bash
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

**Windows:**
```bash
choco install kubernetes-helm
```

**Verify installation:**
```bash
helm version
```

### 6. Install make

Build automation tool.

**macOS:**
```bash
brew install make
```

**Linux:**
```bash
sudo apt-get install make  # Debian/Ubuntu
sudo yum install make      # RedHat/CentOS
```

**Windows:**
```bash
choco install make
```

## Clone the Repository

```bash
git clone https://github.com/muto-io/muto.git
cd muto
```

Verify directory structure:
```bash
ls -la
# Expected: bin/, cmd/, core/, deploy/, docs/, platform/, etc.
```

## Build from Source

### Development Build

For local development and testing:

```bash
make build
```

This creates:
- `bin/muto-operator` — Kubernetes operator
- `bin/muto-mcp` — MCP server

**Verify binaries:**
```bash
./bin/muto-operator --version
./bin/muto-mcp --version
```

### Production Build

For production deployment with optimizations:

```bash
make build-prod
```

This includes:
- Stripped symbols (smaller binary)
- Optimized compilation
- Static binary (portable across systems)

### Build Specific Components

Build individual components:

```bash
# Operator only
make build-operator

# MCP server only
make build-mcp
```

## Verify Installation

After building, verify everything works:

```bash
# Check Go dependencies
go mod verify

# Run unit tests
make test

# Run integration tests (requires Docker)
make test-integration
```

## Docker Images

### Build Docker Images (Local Registry)

For Kubernetes deployments, you need Docker images:

```bash
# Build all images
make docker-build

# Build specific image
make docker-build-operator
```

Images are tagged as `muto-operator:latest` and `muto-mcp:latest`.

### Push to Registry

To use images in a remote cluster:

```bash
# Set your registry
export DOCKER_REGISTRY=myregistry.com

# Build and push
make docker-build docker-push
```

## Next Steps

Congratulations! Muto is installed. Next:

1. **[Quick Start](./quick-start.md)** — Try the 5-minute local walkthrough
2. **[Kubernetes Deployment](../deployment/k8s.md)** — Deploy to production K8s
3. **[CloudFoundry Deployment](../deployment/cf.md)** — Deploy to CloudFoundry

## Troubleshooting Installation

### Go version mismatch
```bash
go version
# If not 1.26+, update:
# Download from golang.org or use version manager
```

### Docker daemon not running
```bash
# Start Docker
docker ps
# If it fails, start Docker Desktop (GUI) or:
sudo systemctl start docker
```

### kind cluster creation fails
```bash
# Ensure Docker has enough resources (8GB RAM minimum)
# Delete problematic cluster:
kind delete cluster --name muto-dev
# Try again:
make kind-up
```

### Permission denied on Linux
```bash
# Add user to docker group
sudo usermod -aG docker $USER
# Log out and back in for changes to take effect
```

---

**Support:** For issues, open a GitHub issue at [muto-io/muto/issues](https://github.com/muto-io/muto/issues)
