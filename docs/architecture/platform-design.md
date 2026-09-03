# Platform Design: Kubernetes & CloudFoundry Adapters

Muto supports multiple execution platforms through a pluggable adapter architecture. This document describes how platform-specific details are abstracted behind a unified interface.

## Overview

Muto's core logic is **platform-agnostic** — the scheduler, reconcilers, and job lifecycle don't care whether agents run on Kubernetes or CloudFoundry. Platform differences are handled by adapters that implement a common interface.

```
Muto Core (Platform-agnostic)
    ├─ Scheduler
    ├─ Reconcilers
    ├─ Message Bus
    └─ Job Management
         │
         ├─ PlatformAdapter Interface
         │   ├─ CreateJob()
         │   ├─ GetJobStatus()
         │   ├─ DeleteJob()
         │   ├─ WatchEvents()
         │   └─ GetLogs()
         │
         ├──────────┬──────────┐
         ▼          ▼          ▼
      K8s       CloudFoundry  Future
     Adapter     Adapter      Adapters
         │          │
         ▼          ▼
    Kubernetes   CloudFoundry
    Platform     Platform
```

## PlatformAdapter Interface

The core interface that all platform adapters must implement:


Each adapter translates Muto's generic job concepts into platform-specific resources.

## Kubernetes Adapter

### Design Principles

- **Uses Kubernetes CRDs**: AgentJob resources stored in etcd
- **Namespace-based isolation**: Tenant data isolated by Kubernetes namespaces
- **Controller-based operations**: Follows Kubernetes control loop pattern
- **Native tooling**: Works with kubectl, Helm, kustomize
- **RBAC integration**: Leverages Kubernetes RBAC for multi-tenancy

### Job Execution Model

AgentJob CRD maps to Kubernetes resources:

```yaml
# Muto AgentJob CRD
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: data-pipeline
  namespace: tenant-a
spec:
  agents:
    - name: extractor
      image: myorg/extractor:v1
  timeout: 5m
  retryPolicy:
    maxRetries: 2
```

Creates Kubernetes resources:

```yaml
# Kubernetes Pod
apiVersion: v1
kind: Pod
metadata:
  name: data-pipeline-extractor-1
  namespace: tenant-a
  labels:
    muto.io/job: data-pipeline
    muto.io/agent: extractor
spec:
  containers:
  - name: extractor
    image: myorg/extractor:v1
  restartPolicy: Never
  terminationGracePeriodSeconds: 300
```

### Multi-Tenancy in Kubernetes

Tenant isolation uses Kubernetes namespaces:

```
Kubernetes Cluster
├─ Namespace: tenant-a
│  ├─ RBAC: ServiceAccount tenant-a-sa (limited permissions)
│  ├─ Resources: tenant-a/* only
│  ├─ Pods: ExtractorA-1, ProcessorA-1
│  └─ NetworkPolicy: block traffic to other namespaces
│
├─ Namespace: tenant-b
│  ├─ RBAC: ServiceAccount tenant-b-sa (limited permissions)
│  ├─ Resources: tenant-b/* only
│  ├─ Pods: ExtractorB-1
│  └─ NetworkPolicy: block traffic to other namespaces
│
└─ Namespace: muto-system
   ├─ Controller: muto-operator
   ├─ Service: muto-webhook
   └─ ConfigMap: muto-config
```

**Isolation layers:**
1. **Namespace isolation**: Each tenant in separate namespace
2. **RBAC**: Each tenant ServiceAccount has limited roles
3. **Resource quotas**: Limit CPU/memory per tenant namespace
4. **Network policies**: Block cross-tenant traffic
5. **Storage classes**: Tenant-specific PVC classes

### Implementation Details

#### Pod Creation Flow

```
AgentJob received
        │
        ▼
┌───────────────────────────┐
│ K8sReconciler.Reconcile() │
└───────────┬───────────────┘
            │
    ┌───────┴────────┐
    ▼                ▼
Validate         Check desired
spec             vs actual
    │                │
    └────┬───────────┘
         ▼
    ┌──────────────────────────┐
    │ generatePodSpec()        │
    │ - Image from agent spec  │
    │ - Resources, limits      │
    │ - Environment variables  │
    │ - Volume mounts          │
    └──────────┬───────────────┘
               │
               ▼
         ┌──────────────────────┐
         │ client.Create(pod)   │
         └──────────┬───────────┘
                    │
                    ▼
         ┌──────────────────────┐
         │ Update AgentJob      │
         │ status: Scheduled    │
         └──────────────────────┘
```

#### Event Watching

K8s adapter uses informer pattern to watch for events:


### Resource Limits and Requests

K8s adapter enforces resource constraints:

```yaml
apiVersion: muto.io/v1
kind: AgentJob
spec:
  agents:
  - name: processor
    image: myorg/processor:v1
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 1000m
        memory: 1Gi
```

Maps to Pod resource spec:

```yaml
spec:
  containers:
  - name: processor
    resources:
      requests:
        cpu: 500m
        memory: 512Mi
      limits:
        cpu: 1000m
        memory: 1Gi
```

Kubernetes scheduler uses requests for placement, enforces limits at runtime.

## CloudFoundry Adapter

### Design Principles

- **Tasks-based execution**: CF tasks for short-lived jobs
- **Space-based isolation**: Tenant data in separate CF spaces
- **Cloud Controller API**: Uses CF API for all operations
- **CredHub for secrets**: Tenant-scoped credential storage
- **Buildpacks**: Flexible runtime support (Go, Java, Python, etc.)

### Job Execution Model

AgentJob maps to CloudFoundry Task:

```bash
# Muto AgentJob
{
  "metadata": {
    "name": "data-pipeline",
    "tenant": "tenant-a"
  },
  "spec": {
    "agents": [{
      "name": "extractor",
      "image": "myorg/extractor:v1",
      "command": ["./run.sh"]
    }]
  }
}
```

Creates CloudFoundry resources:

```bash
# CF Task creation
cf run-task app-name \
  --command='docker run myorg/extractor:v1 ./run.sh' \
  --name='data-pipeline-extractor-1' \
  --memory=512M \
  --disk=1G
```

### Multi-Tenancy in CloudFoundry

Tenant isolation uses CF spaces:

```
CF Org: muto-production
├─ Space: tenant-a
│  ├─ Apps: holding apps for tasks
│  ├─ Tasks: ExtractorA-1, ProcessorA-1
│  ├─ Secrets: tenant-a-db-password (in CredHub)
│  └─ RBAC: tenant-a-admin SpaceRole
│
├─ Space: tenant-b
│  ├─ Apps: holding apps for tasks
│  ├─ Tasks: ExtractorB-1
│  ├─ Secrets: tenant-b-db-password (in CredHub)
│  └─ RBAC: tenant-b-admin SpaceRole
│
└─ Space: muto-system
   ├─ App: muto-operator (control plane)
   ├─ CredHub: Tenant credential prefixes
   └─ Service instances: Message bus, backing services
```

**Isolation layers:**
1. **Space isolation**: Each tenant in separate space
2. **RBAC**: CF SpaceManager/SpaceDeveloper roles per tenant
3. **CredHub**: Tenant-scoped secrets (namespaced paths)
4. **Network policies**: CF ASG (application security groups) per space
5. **Resource quotas**: Space quota limits for memory/instances

### Implementation Details

#### Task Creation Flow

```
AgentJob received
        │
        ▼
┌──────────────────────────────┐
│ CFReconciler.Reconcile()     │
└───────────┬──────────────────┘
            │
    ┌───────┴────────┐
    ▼                ▼
Validate         Check desired
spec             vs actual
    │                │
    └────┬───────────┘
         ▼
┌──────────────────────────────┐
│ Create task on holding app   │
│ - Get app guid               │
│ - Call /tasks endpoint       │
│ - Set environment variables  │
│ - Set memory/disk limits     │
└──────────┬──────────────────┘
           │
           ▼
┌──────────────────────────────┐
│ cf run-task via Cloud Ctrl   │
│ API (POST /spaces/{id}/tasks)│
└──────────┬──────────────────┘
           │
           ▼
┌──────────────────────────────┐
│ Update AgentJob status:      │
│ Scheduled                    │
└──────────────────────────────┘
```

#### Event Watching (Polling)

CF doesn't have event streaming, so adapter polls:


### Environment Variables and Secrets

CF adapter exposes CredHub secrets as environment variables:

```bash
# AgentJob spec
spec:
  agents:
  - name: processor
    env:
    - name: DB_PASSWORD
      valueFrom:
        credHubRef: /muto/tenant-a/db-password
    - name: API_KEY
      valueFrom:
        secretKey: /muto/tenant-a/api-key

# CF adapter resolves from CredHub and injects as env vars:
cf run-task app \
  --env="DB_PASSWORD=<secret from credHub>" \
  --env="API_KEY=<secret from credHub>"
```

## Platform Agnostic Core

The scheduler and reconcilers are completely platform-agnostic:


The scheduler:
- Never directly creates K8s Pods
- Never directly creates CF Tasks
- Always goes through the adapter interface
- Is testable without a running platform

This allows:
1. **Easy testing**: Mock adapters for unit tests
2. **Easy extension**: Add new adapters (Nomad, YARN, custom)
3. **Easy platform migration**: Move jobs between platforms
4. **Consistent semantics**: Same AgentJob works everywhere

## Adapter Selection

Adapters are selected based on tenant configuration:

```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: tenant-a
spec:
  platform: kubernetes
  kubernetesConfig:
    namespace: tenant-a
    resources:
      requests:
        cpu: 500m
---
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: tenant-b
spec:
  platform: cloudfoundry
  cloudFoundryConfig:
    space: tenant-b
    org: muto-production
```

Muto operator selects the appropriate adapter based on `spec.platform`.

## Extensibility

Adding a new platform adapter requires:

1. **Implement PlatformAdapter interface** (6 methods)
2. **Register with adapter factory**:
   ```go
   registry.Register("nomad", &NomadAdapter{})
   ```
3. **Update Tenant CRD** with new platform config
4. **Test with adapter mock** in reconcilers

The core scheduler, reconcilers, and message bus don't change — they work with any adapter.

---

## Next Steps

- **[Agent Lifecycle](./agent-lifecycle.md)** — Job states and transitions
- **[Reconcilers](./reconcilers.md)** — How adapters are used in reconciliation loops
- **[Concepts (Platform)](../getting-started/concepts.md#platform)** — Return to core concepts

**Last Updated:** 2026-09-03
