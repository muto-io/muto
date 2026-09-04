# Example: Simple Hello World Job

A minimal example to get started with Muto agent jobs.

## Overview

This example demonstrates the simplest possible agent job: a single agent that prints a message and exits. This is perfect for understanding the basic concepts and verifying your Muto installation.

## Prerequisites

- Muto operator running (`kubectl get deployment -n muto-system`)
- kubectl configured to access your cluster
- Default namespace is available

## Step 1: Create the Simple Job

Save this YAML to a file named `hello-world.yaml`:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: hello-world
  namespace: default
spec:
  tenant: default
  agents:
    - name: greeter
      image: alpine:latest
      command: ["sh", "-c"]
      args:
        - |
          echo "======================================"
          echo "Hello from Muto Agent!"
          echo "======================================"
          echo "Job Name: $JOB_NAME"
          echo "Agent Name: $AGENT_NAME"
          echo "Timestamp: $(date)"
          echo "======================================"
          sleep 2
          echo "Job completed successfully!"
```

## Step 2: Apply the Job

```bash
kubectl apply -f hello-world.yaml
```

Expected output:
```
agentjob.muto.io/hello-world created
```

## Step 3: Monitor Job Execution

### Check Job Status

```bash
# Get job status
kubectl get agentjob hello-world

# Expected output:
# NAME          PHASE     READY   AGE
# hello-world   Running   1/1     5s
```

### Watch Status Changes

```bash
# Watch the job as it runs
kubectl get agentjobs --watch

# Watch until completion (Ctrl+C to stop)
```

### Get Detailed Information

```bash
# Full job details including status
kubectl describe agentjob hello-world

# Expected output includes:
# Status:
#   Phase:            Running
#   Start Time:       2026-09-03T10:30:45Z
#   Completion Time:  <not yet>
#   Duration:         45s
#   Agents:
#     Name:  greeter
#     Phase: Running
```

## Step 4: View Job Output

### Stream Logs

```bash
# Follow logs as job runs
kubectl logs agentjob/hello-world --follow

# Expected output:
# ======================================
# Hello from Muto Agent!
# ======================================
# Job Name: hello-world
# Agent Name: greeter
# Timestamp: Wed Sep 3 10:30:50 UTC 2026
# ======================================
# Job completed successfully!
```

### Get All Logs (After Completion)

```bash
# Get complete logs
kubectl logs agentjob/hello-world

# Get logs from specific agent
kubectl logs agentjob/hello-world --container=greeter
```

## Step 5: Verify Completion

```bash
# Check final status
kubectl get agentjob hello-world -o json | jq '.status.phase'

# Expected output: "Completed"

# Get detailed status
kubectl get agentjob hello-world -o json | \
  jq '.status | {phase, startTime, completionTime}'

# Expected output:
# {
#   "phase": "Completed",
#   "startTime": "2026-09-03T10:30:45Z",
#   "completionTime": "2026-09-03T10:30:50Z"
# }
```

## Step 6: Cleanup

```bash
# Delete the job
kubectl delete agentjob hello-world

# Verify deletion
kubectl get agentjobs
```

## Understanding the Example

### Job Definition

```yaml
apiVersion: muto.io/v1              # Muto API version
kind: AgentJob                       # Resource type
metadata:
  name: hello-world                  # Job identifier
  namespace: default                 # Kubernetes namespace
spec:
  tenant: default                    # Tenant ID for isolation
  agents:                            # List of agents
    - name: greeter                  # Agent name
      image: alpine:latest           # Container image
      command: ["sh", "-c"]          # Override entrypoint
      args:                          # Arguments to command
        - |                          # Multi-line script
          # Script content...
```

### Key Concepts

**Agent**: The `greeter` agent runs a simple shell script in an Alpine Linux container. The script:
- Prints formatted output
- References environment variables (automatically set by Muto)
- Sleeps briefly to simulate work
- Exits successfully

**Tenant**: `default` tenant provides isolation. In multi-tenant clusters, this prevents the job from interfering with other tenants.

**Image**: `alpine:latest` is a minimal Linux distribution (5 MB). Perfect for simple examples. In production, use your own application images.

## Variations

### Print Environment Variables

```yaml
spec:
  agents:
    - name: env-printer
      image: alpine:latest
      command: ["sh", "-c"]
      args:
        - |
          echo "All environment variables:"
          env | sort
          echo ""
          echo "Muto-provided variables:"
          env | grep -i muto
```

### Run a Python Script

```yaml
spec:
  agents:
    - name: python-agent
      image: python:3.11-slim
      command: ["python", "-c"]
      args:
        - |
          import json
          from datetime import datetime
          print(json.dumps({
              'message': 'Hello from Python',
              'timestamp': datetime.now().isoformat()
          }, indent=2))
```

### Add Environment Variables

```yaml
spec:
  agents:
    - name: greeter
      image: alpine:latest
      command: ["sh", "-c"]
      args:
        - |
          echo "Name: $USER_NAME"
          echo "Email: $USER_EMAIL"
          echo "Config: $APP_CONFIG"
      env:
        - name: USER_NAME
          value: "Alice"
        - name: USER_EMAIL
          value: "alice@example.com"
        - name: APP_CONFIG
          value: "production"
```

### Set Resource Limits

```yaml
spec:
  agents:
    - name: greeter
      image: alpine:latest
      command: ["sh", "-c"]
      args:
        - echo "Hello World"
      resources:
        requests:          # Minimum resources guaranteed
          cpu: "100m"      # 0.1 CPU cores
          memory: "64Mi"   # 64 MB RAM
        limits:            # Maximum resources allowed
          cpu: "500m"      # 0.5 CPU cores
          memory: "256Mi"  # 256 MB RAM
```

### Add Timeouts

```yaml
spec:
  timeout: 5m             # Timeout for job
  agents:
    - name: greeter
      image: alpine:latest
      timeout: 5m         # Agent-specific timeout
      command: ["sh", "-c"]
      args:
        - |
          echo "Starting..."
          sleep 2
          echo "Done!"
```

### Enable Retries

```yaml
spec:
  retryPolicy:
    maxRetries: 2         # Retry up to 2 times
    backoffSeconds: 5     # Start with 5 second backoff
  agents:
    - name: greeter
      image: alpine:latest
      command: ["sh", "-c"]
      args:
        - echo "Hello World"
```

## Common Issues

### Job Stuck in Pending

```bash
# Check operator logs
kubectl logs -n muto-system deployment/muto-operator

# Check job events
kubectl describe agentjob hello-world

# Verify node resources
kubectl describe nodes
```

**Solution**: Usually means insufficient cluster resources or operator not running.

### Image Pull Failed

```bash
# Check which image pull error occurred
kubectl describe agentjob hello-world | grep -A 5 "Events"

# Verify image exists and is accessible
docker pull alpine:latest
```

**Solution**: Use public images or ensure private registry credentials are configured.

### Agent Exits Immediately

```bash
# Check logs for error
kubectl logs agentjob/hello-world

# Check exit code
kubectl get agentjob hello-world -o json | \
  jq '.status.agents[0].exitCode'
```

**Solution**: Agent script has error. Check logs and fix script.

## Next Steps

Now that you understand basic job execution:

1. **[Multi-Agent Workflow Example](./multi-agent-workflow.md)** — Build multi-agent jobs
2. **[Scheduling Agent Jobs](../scheduling-agent-jobs.md)** — Learn job configuration options
3. **[Multi-Agent Patterns](../multi-agent-patterns.md)** — Learn orchestration patterns
4. **[Best Practices](../best-practices.md)** — Optimize for production

## Full Example Files

For reference, complete example files are available in the Muto repository:
- `examples/jobs/hello-world.yaml` — This example
- `examples/jobs/multi-agent-pipeline.yaml` — Complex workflow
- `examples/jobs/error-handling.yaml` — Error recovery patterns

---

**Last Updated:** 2026-09-03
