# Quick Start: Get Running in 5 Minutes

Get Muto up and running locally to see it in action.

## Prerequisites

- **Go 1.26+**: Download from [golang.org](https://golang.org/dl/)
- **Docker**: [Download Docker Desktop](https://www.docker.com/products/docker-desktop)
- **kind**: Kubernetes in Docker — `go install sigs.k8s.io/kind@latest`
- **kubectl**: Kubernetes CLI — `curl -LO https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl`
- **make**: Standard build tool (included on macOS/Linux; Windows users: `choco install make`)

**Verify installations:**
```bash
go version          # Should show 1.26+
docker version      # Should show Docker CLI and server
kind version        # Should show kind v0.x.x
kubectl version     # Should show client version
make --version      # Should show GNU Make
```

## Step 1: Create a Local Kubernetes Cluster (1 min)

Create a local Kubernetes cluster using kind:

```bash
cd /path/to/muto
make kind-up
```

This command:
- Creates a kind cluster named `muto-dev`
- Installs required CRDs
- Sets kubeconfig to use the new cluster

**Verify the cluster is running:**
```bash
kubectl cluster-info
kubectl get nodes
```

Expected output: One node named `muto-dev-control-plane` in Ready state.

## Step 2: Build Muto Binaries (1.5 min)

Build the Muto operator and MCP server:

```bash
make build
```

This creates:
- `./bin/muto-operator` — Kubernetes controller that manages agent jobs
- `./bin/muto-mcp` — MCP server for Claude/LLM integration

**Verify binaries exist:**
```bash
ls -lh bin/
```

## Step 3: Run the Muto Operator (1 min)

Start the Muto operator:

```bash
./bin/muto-operator
```

You should see output like:
```
2026-09-03T10:30:45.123Z    INFO    muto-operator started    {"version": "0.1.0"}
2026-09-03T10:30:45.456Z    INFO    reconcilers configured   {"reconcilers": ["tenant", "agentjob"]}
```

**In another terminal, verify the operator is running:**
```bash
kubectl get pods -n muto-system
```

Expected: One pod named `muto-operator-*` with status Running.

## Step 4: Create Your First Agent Job (1 min)

Create a sample agent job:

```bash
cat << 'YAML' | kubectl apply -f -
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: hello-world
  namespace: default
spec:
  tenant: default
  agents:
    - name: printer
      image: alpine:latest
      command: ["sh", "-c"]
      args: ["echo 'Hello from Muto agent!'; sleep 5"]
  timeout: 60s
  retryPolicy:
    maxRetries: 2
    backoffSeconds: 5
YAML
```

**Monitor the job:**
```bash
# Check job status
kubectl get agentjobs

# Follow logs
kubectl logs job.muto.io/hello-world

# Get detailed status
kubectl describe agentjob hello-world
```

Expected: Job transitions from Pending → Scheduled → Running → Completed.

## Step 5: Run the MCP Server (Optional, 1 min)

In another terminal, run the MCP server to integrate with Claude:

```bash
./bin/muto-mcp
```

You should see:
```
2026-09-03T10:35:20.123Z    INFO    MCP server started    {"port": 3000}
```

This allows Claude or other MCP clients to:
- Schedule agent jobs
- Query job status
- Cancel running jobs

## What's Next?

You now have Muto running! Next steps:

- **[Install on Kubernetes](../deployment/k8s.md)** — Deploy to a real K8s cluster
- **[Configuration](../configuration/env-vars.md)** — Customize behavior
- **Architecture Overview** — Understand how it works (coming in Phase 2)
- **Usage Patterns** — Build complex workflows (coming in Phase 5)

## Troubleshooting

### Cluster creation fails
```bash
# If kind fails, clean up old clusters
kind delete cluster --name muto-dev
make kind-up
```

### Operator won't start
```bash
# Check Docker is running
docker ps

# Check cluster is accessible
kubectl cluster-info
```

### Job stuck in Pending
```bash
# Check operator logs
kubectl logs -n muto-system deployment/muto-operator

# Check job status
kubectl describe agentjob hello-world

# Check resource availability
kubectl describe nodes
```

---

**Time to production:** This is a local dev setup. For production, see [Kubernetes Deployment](../deployment/k8s.md) or [CloudFoundry Deployment](../deployment/cf.md).
