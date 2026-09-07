# Kubernetes Platform Architecture

Muto's Kubernetes adapter runs as a controller in the cluster, using Kubernetes CRDs (Custom Resource Definitions) to declare and manage agent workloads. This section describes how Muto integrates with Kubernetes.

## Deployment Model

Muto runs as a set of Kubernetes deployments in the `muto-system` namespace:

```
┌─────────────────────────────────────────────────────────┐
│           Muto System Namespace                         │
│                                                         │
│  ┌──────────────────┐      ┌───────────────────────┐  │
│  │ muto-controller  │◄─────┤ CRD API Server        │  │
│  │ (pod)            │      │ (Kubernetes standard) │  │
│  └────────┬─────────┘      └───────────────────────┘  │
│           │                                             │
│           ├─ Scheduler                                  │
│           ├─ Reconcilers                                │
│           ├─ Event Watcher                              │
│           └─ Platform Adapter (Kubernetes)              │
│                                                         │
│  ┌──────────────────┐      ┌───────────────────────┐  │
│  │ muto-webhooks    │◄─────┤ ValidatingWebhook     │  │
│  │ (pod)            │      │ Configuration         │  │
│  └──────────────────┘      └───────────────────────┘  │
└─────────────────────────────────────────────────────────┘

             │
             │ Watches CRDs and Manages
             │
             ▼
┌─────────────────────────────────────────────────────────┐
│         Tenant Namespaces                               │
│                                                         │
│  agents-tenant-a/                 agents-tenant-b/     │
│  ├─ AgentJobs                      ├─ AgentJobs        │
│  ├─ AgentFleets                    ├─ AgentFleets      │
│  ├─ Pods (running agents)          ├─ Pods             │
│  ├─ Services                       ├─ Services         │
│  └─ ConfigMaps, Secrets            └─ ConfigMaps       │
└─────────────────────────────────────────────────────────┘
```

## Custom Resource Definitions (CRDs)

Muto defines three CRDs that represent the job model:

### Tenant CRD
```
muto.io/v1alpha1/Tenant
```

Represents a logical isolation boundary:
- Defines namespace and resource quotas
- Specifies message bus configuration (shared or dedicated)
- Configures RBAC and network policies
- Sets resource limits per tenant

**Example:**
```yaml
apiVersion: muto.io/v1alpha1
kind: Tenant
metadata:
  name: acme-corp
spec:
  namespace: agents-acme-corp
  isolationTier: dedicated
  messageBus:
    type: nats
  resourceQuota:
    pods: "100"
    cpu: "50"
    memory: "100Gi"
```

### AgentJob CRD
```
muto.io/v1alpha1/AgentJob
```

Represents a unit of work:
- Specifies agents to run
- Defines job triggers (manual, cron, event-based)
- Sets resource requests/limits
- Configures retry and timeout policies

**Example:**
```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: data-pipeline
spec:
  tenantRef: acme-corp
  trigger:
    type: cron
    schedule: "0 2 * * *"  # 2 AM daily
  agents:
    - name: extractor
      image: acme/extractor:v1
      resources:
        requests:
          cpu: "500m"
          memory: "512Mi"
```

### AgentFleet CRD
```
muto.io/v1alpha1/AgentFleet
```

Groups related jobs for coordinated operations:
- Tracks dependencies between jobs
- Provides fleet-level status
- Enables bulk operations

## Controller Architecture

The Muto controller follows Kubernetes control loop patterns:

```
┌─────────────────────────────────────┐
│   Kubernetes API Server             │
│   (watches CRD objects)             │
└──────────────────┬──────────────────┘
                   │
                   │ watch events
                   ▼
┌─────────────────────────────────────┐
│   Workqueue                         │
│   (pending reconciliation items)    │
└──────────────────┬──────────────────┘
                   │
                   │ dequeue
                   ▼
┌─────────────────────────────────────┐
│   Reconciler                        │
│   (drive current → desired state)   │
└──────────────────┬──────────────────┘
                   │
                   │ create/update/delete
                   ▼
┌─────────────────────────────────────┐
│   Kubernetes Resources              │
│   (Pods, Deployments, Services)     │
└─────────────────────────────────────┘
```

**Reconciliation flow:**
1. Controller watches CRD objects for changes
2. Changes enqueued to workqueue
3. Reconciler pops item from queue
4. Compares desired state (CRD spec) with current state (K8s resources)
5. Creates/updates/deletes K8s resources to match desired state
6. Updates CRD status with current state
7. Requeues if necessary (error or periodic resync)

## Job Execution on Kubernetes

When a Tenant and AgentJob are created, the controller:

1. **Creates tenant namespace** with ResourceQuota and NetworkPolicy
2. **Creates RBAC** (ServiceAccount, Role, RoleBinding)
3. **Creates message bus** (if dedicated isolation tier)
4. **For each agent in AgentJob:**
   - Creates a Deployment or Pod
   - Sets resource requests/limits
   - Configures environment variables
   - Mounts ConfigMaps/Secrets for configuration
5. **Watches Pod events** and updates AgentJob status
6. **On completion** cleans up resources (configurable retention)

## Scaling Model

Kubernetes-native scaling capabilities:

### Horizontal Pod Autoscaling (HPA)
The controller can configure HPA for agents:
```yaml
agents:
  - name: worker
    image: acme/worker:v1
    replicas: 1
    maxReplicas: 10
    cpuThreshold: "80%"
```

### Resource Quotas
Each tenant namespace has enforced quotas:
```yaml
spec:
  resourceQuota:
    pods: "100"
    cpu: "50"
    memory: "100Gi"
```

### Cluster Scaling
Muto can integrate with cluster autoscaling:
- Pod requests trigger cluster autoscaling
- Nodes added/removed based on demand
- Placement controlled via nodeSelectors and affinities

## Multi-Tenancy on Kubernetes

Tenant isolation is enforced via:

### Namespace Isolation
- Each tenant gets a dedicated namespace
- RBAC prevents cross-tenant access
- Resource quotas limit tenant resource usage

### Network Policies
Restrict traffic between tenants:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: tenant-isolation
spec:
  podSelector:
    matchLabels:
      muto.io/tenant: tenant-a
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          muto.io/tenant: tenant-a
```

### RBAC
- ServiceAccount per tenant
- Role limited to tenant's namespace
- RoleBinding grants minimum necessary permissions

## Observability

### Status Tracking
AgentJob status reflects Kubernetes Pod status:
```yaml
status:
  phase: Running
  conditions:
  - type: Ready
    status: "True"
  - type: Available
    status: "True"
  activeAgents: 3
  startedAt: "2026-09-05T10:30:00Z"
```

### Events
Kubernetes Events track job lifecycle:
```
LAST SEEN   TYPE      REASON          MESSAGE
1m          Normal    Scheduled       AgentJob scheduled on tenant-a
30s         Normal    Started         Pod created for agent-1
15s         Normal    Running         Job executing
```

### Metrics
Controller exports Prometheus metrics:
- `muto_job_duration_seconds` — Job execution time
- `muto_job_retries_total` — Retry count by outcome
- `muto_pods_created_total` — Pod creation rate
- `muto_tenant_resource_usage` — Tenant resource consumption

## Advantages of Kubernetes Platform

✅ **Native integration** with existing Kubernetes infrastructure  
✅ **Standard tooling** (kubectl, Helm, kustomize) for operations  
✅ **Rich ecosystem** (monitoring, networking, storage solutions)  
✅ **RBAC** for access control  
✅ **Resource quotas** for cost control  
✅ **Automatic scaling** via HPA and cluster autoscaling  
✅ **High availability** via pod replicas and node distribution  

## Related Documentation

- [:octicons-book-24: **Platform Design**](./platform-design.md) — Adapter abstraction
- [:octicons-book-24: **Architecture Overview**](./overview.md) — System architecture
- [:octicons-book-24: **Deployment**](../deployment/kubernetes/install.md) — K8s installation guide
