# Installation Guide

Choose your path based on your role:

- **[👥 Users & Operators](#for-users--operators)** — Deploy Muto using Docker and Helm (recommended)
- **[👨‍💻 Developers](#for-developers--contributors)** — Build from source and contribute to Muto

---

## For Users & Operators

### Quick Start: Deploy with Docker & Helm

Muto is distributed as pre-built Docker images. No Go compilation needed.

**Prerequisites:**
- Docker (or access to a Kubernetes cluster)
- kubectl (for Kubernetes)
- Helm 3.10+ (for Kubernetes)

**Installation paths:**

1. **Kubernetes (Recommended)** -> [Kubernetes Deployment Guide](../deployment/kubernetes/install.md)
   - Use Helm charts for production deployments
   - Supports multi-tenant, high-availability setups
   - Most common production path

2. **CloudFoundry** -> [CloudFoundry Deployment Guide](../deployment/cf.md)
   - Deploy to CloudFoundry environments
   - Alternative to Kubernetes

3. **Local Testing** -> [Quick Start](./quick-start.md)
   - 5-minute walkthrough with Docker Compose

### System Requirements

**For Kubernetes deployment:**
- Kubernetes 1.24+
- 3+ nodes with 2 CPU and 4GB RAM each
- Helm 3.10+

**For local testing:**
- Docker or Docker Desktop
- 4GB RAM minimum

---

## For Developers & Contributors

### Build from Source

To contribute to Muto or customize the build, you'll need to compile from source.

#### Prerequisites

1. **Go 1.26+**

   **macOS:**
   ```bash
   brew install go@1.26
   ```

   **Linux:**
   ```bash
   curl -LO https://dl.google.com/go/go1.26.0.linux-amd64.tar.gz
   sudo tar -C /usr/local -xzf go1.26.0.linux-amd64.tar.gz
   export PATH=$PATH:/usr/local/go/bin
   ```

   **Windows:**
   Download from [golang.org](https://golang.org/dl/)

   **Verify:**
   ```bash
   go version
   ```

2. **Docker** (for building images)

   **macOS/Windows:** [Docker Desktop](https://www.docker.com/products/docker-desktop)

   **Linux:**
   ```bash
   curl -fsSL https://get.docker.com -o get-docker.sh
   sudo sh get-docker.sh
   sudo usermod -aG docker $USER
   ```

3. **Git, Make, and other tools**

   **macOS:**
   ```bash
   brew install make git
   ```

   **Linux:**
   ```bash
   sudo apt-get install build-essential git  # Debian/Ubuntu
   ```

#### Clone and Build

```bash
git clone https://github.com/muto-io/muto.git
cd muto
```

**Development build:**
```bash
make build
# Outputs: bin/muto-operator, bin/muto-mcp
```

**Production build:**
```bash
make build-prod
# Creates stripped, optimized binaries
```

**Verify:**
```bash
./bin/muto-operator --version
```

#### Build Docker Images

For local Kubernetes testing:

```bash
make docker-build
# Tags images as muto-operator:latest, muto-mcp:latest
```

Push to registry:
```bash
export DOCKER_REGISTRY=myregistry.com
make docker-build docker-push
```

#### Run Tests

```bash
# Unit tests
make test

# Integration tests (requires Docker)
make test-integration
```

#### Local Development with kind

Create a local Kubernetes cluster:

```bash
brew install kind  # or use official installer

# Create cluster
kind create cluster --name muto-dev

# Build and load images
make docker-build
kind load docker-image muto-operator:latest --name muto-dev
```

---

## Next Steps

**Users:** Follow the [Kubernetes Deployment Guide](../deployment/kubernetes/install.md) or [CloudFoundry guide](../deployment/cf.md)

**Developers:** Check out [Development Setup](../development/setup.md) and [Contributing Guidelines](../development/contributing.md)

**Everyone:** Try the [Quick Start](./quick-start.md)

---

## Troubleshooting

### Docker daemon not running
```bash
docker ps
# If it fails, start Docker Desktop or:
sudo systemctl start docker
```

### Kubernetes connection issues
```bash
kubectl cluster-info
kubectl get nodes
```

### Go version mismatch (developers only)
```bash
go version
# Update from golang.org if needed
```

### kind cluster issues (developers only)
```bash
# Ensure Docker has 8GB+ RAM
kind delete cluster --name muto-dev
kind create cluster --name muto-dev
```

---

**Support:** [GitHub Issues](https://github.com/muto-io/muto/issues) | [GitHub Discussions](https://github.com/muto-io/muto/discussions)
