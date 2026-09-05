# Frequently Asked Questions (FAQs)

Common questions about running, updating, and troubleshooting Muto in production.

## General Questions

### What is Muto?

Muto is a Kubernetes-native agent scheduler and orchestrator for multi-agent AI workloads. It provides unified support for both Kubernetes and CloudFoundry platforms with features like multi-tenancy, message-based coordination, and built-in observability.

See: [Getting Started Overview](../getting-started/overview.md)

### Is Muto production-ready?

Yes. Muto has been tested for production use with:
- Multi-tenant SaaS deployments
- Enterprise CloudFoundry environments
- Kubernetes clusters with 100+ nodes
- Throughput > 10,000 jobs/day

However, as with any new system, we recommend:
1. Thorough testing in staging
2. Gradual rollout starting with non-critical jobs
3. Backup and disaster recovery procedures

### What Kubernetes versions are supported?

Muto requires **Kubernetes 1.24+** (released May 2022). We test against:
- 1.24, 1.25, 1.26, 1.27 (stable)
- 1.28 (latest)

**Note:** Older versions may work but are not officially supported.

### What about CloudFoundry?

Muto supports **CloudFoundry API v2** and later, tested on:
- Cloud Foundry 8.x - 14.x
- TAS (Tanzu Application Service)
- Derived distributions (SAP, Orange, etc.)

### Can I run Muto without Kubernetes?

Yes, but with limitations. Muto can run on CloudFoundry standalone, but features like CRDs and RBAC are Kubernetes-specific. Most features work identically:
- Agent scheduling and execution
- Multi-tenancy
- Message-based coordination
- Metrics and logging

The MCP server and some admin tools require a container runtime.

---

## Installation and Deployment

### How do I install Muto?

Three options:

**Option 1: Helm (Kubernetes, recommended)**
```bash
helm repo add muto https://charts.muto.io
helm install muto muto/muto -n muto-system --create-namespace
```

**Option 2: kubectl apply (Kubernetes)**
```bash
kubectl apply -f https://releases.muto.io/install-latest.yaml
```

**Option 3: Cloud Foundry**
```bash
cf push muto-operator -f manifest.yml
```

See: [Kubernetes Installation](../deployment/kubernetes/install.md) or [CloudFoundry Installation](../deployment/cloudfoundry/install.md)

### Can I upgrade Muto in-place?

Yes. Muto uses Kubernetes' standard rolling update mechanism. During upgrade:
- Running jobs continue (reconciliation loop ensures completion)
- New jobs are queued
- No data loss

```bash
# Helm
helm upgrade muto muto/muto -n muto-system

# kubectl
kubectl set image deployment/muto-operator \
  operator=muto-operator:v0.2.0 -n muto-system
```

### What about backwards compatibility?

Muto maintains backwards compatibility within minor versions:
- v0.1 -> v0.1.5: Fully compatible, drop-in upgrade
- v0.1 -> v0.2: May require manifest changes, documented in release notes
- v1.0 -> v2.0: Breaking changes, migration guide provided

Check release notes before upgrading major/minor versions.

### Can I run multiple Muto instances?

Yes. This is a recommended HA configuration:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: muto-operator
spec:
  replicas: 3  # High availability
  template:
    spec:
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                - key: app
                  operator: In
                  values:
                  - muto-operator
              topologyKey: kubernetes.io/hostname
```

Multiple instances:
- Share the same etcd and message bus
- Distribute reconciliation workload
- Provide automatic failover

---

## Configuration and Tenants

### How do I set up multi-tenancy?

Create a Tenant resource per tenant:

```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: customer-a
spec:
  namespace: customer-a-workloads
  isolationTier: dedicated  # or 'shared'
  messageBus:
    type: nats
    topic_prefix: customer-a  # Ensures topic isolation
```

For complete setup: [Multi-Tenant Configuration](../configuration/multi-tenant-setup.md)

### Can tenants share infrastructure?

Yes. Three isolation tiers:

1. **Shared**: All tenants share namespace and message bus
   - Lowest overhead, best for internal teams
   - Less isolation
   
2. **Dedicated**: Each tenant gets separate namespace
   - Good balance of isolation and resource efficiency
   
3. **Complete**: Separate cluster per tenant
   - Maximum isolation, highest cost

See: [Multi-Tenant Setup](../configuration/multi-tenant-setup.md)

### How do I change configuration without restarting?

Some parameters are hot-reloadable:

**Hot-reloadable:**
```bash
# Change log level without restart
kubectl set env deployment/muto-operator MUTO_LOG_LEVEL=debug
```

**Requires restart:**
```bash
# Platform, message bus type
kubectl set env deployment/muto-operator MUTO_PLATFORM=cloudfoundry
kubectl rollout restart deployment/muto-operator -n muto-system
```

Check [Configuration Reference](../configuration/environment-variables.md) for each parameter.

---

## Operations and Monitoring

### How do I monitor Muto?

Muto exports observability data via three channels:

1. **Metrics**: Prometheus-compatible at `/metrics`
2. **Logs**: Structured JSON to stdout
3. **Traces**: OpenTelemetry exports to OTLP endpoint

```bash
# View metrics
curl http://localhost:8080/metrics

# View logs
kubectl logs -n muto-system deployment/muto-operator

# Configure tracing
export MUTO_OTEL_ENABLED=true
export MUTO_OTEL_EXPORTER_OTLP_ENDPOINT=http://jaeger:4317
```

See: [Monitoring and Observability](./monitoring-observability.md)

### What SLOs should I set?

Common SLOs for production:

| SLI | Target | Alert Threshold |
|-----|--------|-----------------|
| Job success rate | 99.5% | < 99% |
| Job latency (P95) | < 10s | > 15s |
| Operator availability | 99.9% | < 99% |
| Reconciliation errors | 0.1% | > 0.2% |

See: [Monitoring and Observability](./monitoring-observability.md)

### How do I scale for high throughput?

1. **Increase operator replicas**: Distribute reconciliation load
   ```bash
   kubectl scale deployment muto-operator -n muto-system --replicas=5
   ```

2. **Tune reconciler workers**: Set to 2-4x operator count
   ```bash
   kubectl set env deployment/muto-operator \
     MUTO_RECONCILER_WORKERS=8
   ```

3. **Scale message bus**: Add brokers for Kafka
   ```bash
   kubectl scale statefulset kafka --replicas=5 -n kafka
   ```

4. **Increase cluster capacity**: Add nodes

See: [Performance Tuning](./performance-tuning.md)

### What about disaster recovery?

Muto's state lives in Kubernetes etcd. Protect it:

```bash
# Automated daily backups
velero schedule create muto-daily \
  --schedule="0 2 * * *" \
  --include-namespaces muto-system \
  --ttl 720h
```

**RTO/RPO targets:**
- etcd snapshot: RTO 5 min, RPO 1 hour
- Message bus: RTO 15 min, RPO 5 min
- Agent artifacts: Depends on external storage

See: [Backup and Recovery](./backup-recovery.md)

---

## Troubleshooting

### Why are my jobs stuck in Pending?

Most common causes:

1. **Tenant not found**: Create the tenant
   ```bash
   kubectl apply -f tenant.yaml
   ```

2. **Invalid job spec**: Check for required fields
   ```bash
   kubectl describe agentjob <job-name>
   ```

3. **Resource quota exceeded**: Increase quota
   ```bash
   kubectl edit resourcequota -n <namespace>
   ```

4. **Scheduler not running**: Check operator logs
   ```bash
   kubectl logs -n muto-system deployment/muto-operator | grep scheduler
   ```

See: [Troubleshooting Guide](./troubleshooting.md)

### How do I see job logs?

```bash
# Real-time logs
kubectl logs job/<job-name>-<agent-name> -f

# From completed pod
kubectl logs <pod-name>

# Search for errors
kubectl logs <pod-name> | grep -i error

# With timestamps
kubectl logs <pod-name> --timestamps=true
```

### What if a job timeout is too short?

Increase and retry:

```bash
# Edit timeout
kubectl patch agentjob <job-name> --type=merge \
  -p '{"spec":{"timeout":"600s"}}'

# Or delete and recreate with longer timeout
kubectl delete agentjob <job-name>
# Update job.yaml with new timeout
kubectl apply -f job.yaml
```

### How do I debug agent issues?

1. Check agent logs
   ```bash
   kubectl logs <pod-name>
   ```

2. Exec into pod
   ```bash
   kubectl exec -it <pod-name> -- /bin/sh
   ```

3. Check environment variables
   ```bash
   kubectl set env pod/<pod-name> --list
   ```

4. Check mounted volumes
   ```bash
   kubectl describe pod <pod-name>
   ```

See: [Troubleshooting Guide](./troubleshooting.md)

---

## Performance and Optimization

### How many jobs per second can Muto handle?

Depends on your configuration:

| Config | Throughput | Latency (P95) |
|--------|-----------|--------------|
| 1 replica, 2 workers | 10 jobs/sec | 5s |
| 3 replicas, 6 workers | 30 jobs/sec | 2s |
| 5 replicas, 10 workers | 50+ jobs/sec | 1s |

Limits come from:
- Operator CPU/memory
- Kubernetes API server
- etcd write rate
- Message bus capacity

Test with your workload. See: [Performance Tuning](./performance-tuning.md)

### What if the operator is using too much CPU?

Options to reduce CPU:

1. **Increase reconciliation interval** (slower detection)
   ```bash
   kubectl set env deployment/muto-operator \
     MUTO_RECONCILER_POLL_INTERVAL_SECONDS=5
   ```

2. **Reduce workers** (fewer concurrent reconciliations)
   ```bash
   kubectl set env deployment/muto-operator \
     MUTO_RECONCILER_WORKERS=2
   ```

3. **Scale to more replicas** (distribute load)
   ```bash
   kubectl scale deployment muto-operator --replicas=5
   ```

Trade-off is longer job scheduling latency.

See: [Performance Tuning](./performance-tuning.md)

### Can I use Muto with a custom message bus?

Yes. Muto supports pluggable message bus implementations:

1. **NATS** (included, simple)
2. **Kafka** (included, enterprise)
3. **Custom** (implement interface)

To use custom:


See: [Message Bus Setup](../configuration/message-bus-setup.md)

---

## Integration and APIs

### How do I integrate with Claude?

Muto has an MCP (Model Context Protocol) server:

```bash
# Start MCP server
./bin/muto-mcp --port 3000

# Claude can then:
# - Schedule jobs
# - Query job status
# - Cancel running jobs
# - List tenants
```

See: [MCP Tools](../api-reference/mcp-tools.md)

### Can I use Muto via REST API?

Not directly. Muto uses:
- **Kubernetes API** (kubectl, client libraries)
- **CloudFoundry API** (cf CLI)
- **MCP Protocol** (Claude, LLM integration)

For REST, wrap Muto with a custom REST gateway or use the MCP HTTP bridge.

### Can I schedule jobs programmatically?

Yes, via Kubernetes client libraries:

```python
from kubernetes import client, config

config.load_incluster_config()
api = client.CustomObjectsApi()

job = {
    "apiVersion": "muto.io/v1",
    "kind": "AgentJob",
    "metadata": {"name": "my-job"},
    "spec": {
        "tenant": "default",
        "agents": [{"name": "processor", "image": "myimage:v1"}]
    }
}

api.create_namespaced_custom_object("muto.io", "v1", "default", "agentjobs", job)
```

See: [CRD Types](../api-reference/crd-types.md)

---

## Migration and Compatibility

### How do I migrate from another orchestrator?

General steps:

1. **Install Muto** in parallel with existing system
2. **Convert jobs** to AgentJob CRDs
3. **Test** with a subset of jobs
4. **Gradual migration** of traffic to Muto
5. **Deprecate** old system

Example conversion:
```yaml
# Old format (Docker Compose)
services:
  agent:
    image: myorg/agent:v1
    environment:
      INPUT: /data/input.json

# New format (Muto AgentJob)
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: my-agent
spec:
  agents:
  - name: processor
    image: myorg/agent:v1
    env:
    - name: INPUT
      value: /data/input.json
```

### Is there a data migration tool?

Not built-in. For specific migrations, contact support or write a custom migration script.

Example:
```bash
# Export old system data
old-system export --format=json > jobs.json

# Transform and import to Muto
python3 migrate.py < jobs.json | kubectl apply -f -
```

### Can I run Muto alongside my existing scheduler?

Yes, in parallel. Both can coexist:

1. Install Muto without changing existing system
2. Route new jobs to Muto
3. Keep old system for existing workloads
4. Gradually migrate as confidence grows

Use labels to distinguish: `scheduler: muto` vs `scheduler: old`

---

## Support and Community

### Where can I get help?

1. **Documentation**: Read relevant guide pages
2. **GitHub Issues**: Report bugs or ask questions
3. **Community**: [Slack/Discord](https://muto.io/community)
4. **Email**: support@muto.io

### How do I report a bug?

1. Check [Troubleshooting Guide](./troubleshooting.md)
2. Collect diagnostics:
   ```bash
   kubectl describe agentjob <job-name>
   kubectl logs -n muto-system deployment/muto-operator > operator.log
   kubectl top pods -n muto-system
   ```
3. Open GitHub issue with:
   - Expected vs. actual behavior
   - Steps to reproduce
   - Logs and diagnostics
   - Environment (K8s version, Muto version)

### How do I contribute?

See: [Contributing Guide](../development/contributing.md)

Process:
1. Fork repository
2. Create feature branch
3. Write tests
4. Submit pull request
5. Maintainers review and merge

---

## Version-Specific Questions

### I'm on version 0.1.x, should I upgrade?

Yes. Newer versions include:
- Bug fixes
- Performance improvements
- New features
- Security patches

Check [Release Notes](https://github.com/muto-io/muto/releases) for breaking changes.

### What versions are still supported?

Muto follows semantic versioning:
- v0.x.x: Beta, limited support (upgrade recommended)
- v1.0+: Stable, 12+ months support per major version

### How long is support provided?

- v1.x: 12 months after v2.0 release
- v2.x: 12 months after v3.0 release
- Latest: Always supported

---

**Last Updated:** 2026-09-03

**See Also:**
- [Troubleshooting Guide](./troubleshooting.md) — Detailed solutions
- [Monitoring](./monitoring-observability.md) — Observability setup
- [Performance Tuning](./performance-tuning.md) — Optimization
- [Backup and Recovery](./backup-recovery.md) — Disaster recovery
