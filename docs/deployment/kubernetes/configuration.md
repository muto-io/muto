# Kubernetes-Specific Configuration

What the real Muto CRDs and Helm chart (`deploy/helm/muto/`) actually let you configure on Kubernetes.

## Custom Resource Definitions (CRDs)

Muto defines **three** CRDs, all under `muto.io/v1alpha1` (not `v1` - there is no `v1`): `AgentJob`, `Tenant`, `AgentFleet`. There is no `ReconcilerConfig` CRD or any other custom resource.

### AgentJob

The real schema (`platform/k8s/types/v1alpha1/agentjob_types.go`):

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentJob
metadata:
  name: data-pipeline
  namespace: tenant-acme   # must be the namespace the owning Tenant creates
spec:
  tenantRef: acme-corp       # name of the owning Tenant
  trigger:
    type: manual              # event | cron | manual
    source: ""                 # topic URL (event) or cron expression (cron)
  agents:
    - role: extractor
      image: myregistry.io/extractor:v1.2.0   # required when MUTO_PLATFORM=k8s
      maxReplicas: 1                            # defaults to 1
  messageBus:
    topic: extract-done   # namespaced to tenant.<tenantRef>.extract-done at runtime
  ttlAfterCompletion: 300  # seconds to keep the job around after it finishes; 0 = no cleanup

status:
  phase: Running   # Pending | Running | Succeeded | Failed | Terminating
  activeAgents: 1
  startedAt: "2026-09-03T10:00:00Z"
  completedAt: null
```

There is no `spec.timeout`, `spec.parallelism`, `spec.dependencies`, `spec.subscriptions`, per-agent `command`/`args`/`env`/`resources`/`timeout`/`retryPolicy`, or `status.conditions` - `AgentRoleSpec` is just `role`, `image`, `command` (CF only), and `maxReplicas`. The pod spec (image, env, resources) that actually runs is built by `AgentJobReconciler`, not supplied in the CRD.

### Tenant

See [Multi-Tenant Setup](../../configuration/multi-tenant-setup.md) for the real 3-field spec (`namespace`, `isolationTier`, `messageBus`) and what the reconciler actually does with it - it's not reproduced here to avoid the two docs drifting apart again.

### AgentFleet

```yaml
apiVersion: muto.io/v1alpha1
kind: AgentFleet
metadata:
  name: nightly-batch
spec:
  tenantRef: acme-corp
  jobRefs:
    - data-pipeline
    - cleanup-job

status:
  totalJobs: 2
  runningJobs: 1
  completedJobs: 1
```

A fleet is just a named grouping of `AgentJob` names for fleet-level status rollup - there's no fleet-level scheduling, dependency, or cancellation logic beyond what's in `AgentFleetStatus`.

## Helm Chart Configuration

Everything configurable via Helm lives in `deploy/helm/muto/values.yaml`. This is the complete set - the chart has no other templated values:

```yaml
replicaCount: 1

image:
  repository: ghcr.io/muto-io/muto-operator
  tag: ""              # defaults to .Chart.AppVersion when empty
  pullPolicy: IfNotPresent

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

serviceAccount:
  create: true
  name: ""
  annotations: {}

rbac:
  create: true

installCRDs: true        # set false if CRDs are managed externally (e.g. GitOps)

env:
  MUTO_PLATFORM: k8s
  MUTO_NAMESPACE: default
  MUTO_LOG_LEVEL: info
  MUTO_LOG_FORMAT: json
  CF_API_URL: ""
  CF_USERNAME: ""
  CF_PASSWORD: ""
  CF_ISOLATION_TIER: dedicated
  CF_SHARED_ORG: ""

resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 64Mi

metrics:
  enabled: true
  port: 8080
  serviceMonitor:
    enabled: false
    interval: 30s
    additionalLabels: {}

healthProbe:
  port: 8081

nodeSelector: {}
tolerations: []
affinity: {}
```

See [Environment Variables](../../configuration/environment-variables.md) for what each `env` entry actually does.

`replicaCount` is a plain integer with no leader-election coordination beyond what controller-runtime's manager does by default - there's no autoscaling (no `HorizontalPodAutoscaler` template), so scaling past 1 replica without external leader-election tuning isn't something this chart does for you.

### RBAC

`rbac.create: true` (with `serviceAccount.create: true`) templates a `ServiceAccount`, `ClusterRole`, and `ClusterRoleBinding` for the operator - **not** `cluster-admin`. See [Security Configuration](../../configuration/tls-security.md#kubernetes-rbac) for the exact permission table. There's no separate per-tenant RBAC template; "tenant RBAC" isn't a Muto feature (see [Multi-Tenant Setup](../../configuration/multi-tenant-setup.md) for what you'd set up yourself).

### Metrics and Monitoring

When `metrics.enabled: true` (the default), the chart creates a `-metrics` `Service` (`templates/service.yaml`) in front of the operator's `/metrics` port, plus `prometheus.io/scrape`/`port`/`path` pod annotations. Setting `metrics.serviceMonitor.enabled: true` additionally creates a real `ServiceMonitor` (`templates/servicemonitor.yaml`, Prometheus Operator CRD) selecting that Service, using `metrics.serviceMonitor.interval`/`additionalLabels`. No `PodMonitor` is needed or templated - the Service exists, so the `ServiceMonitor` has something to select.

See [Monitoring and Observability](../../operations/monitoring-observability.md) for what metrics are actually exported.

## Not Implemented

The following were previously documented here as configurable. None of them exist in `deploy/helm/muto/` or the Go source - there's no template, CRD, or code path for any of them today:

- **Admission webhooks** (validating or mutating) - no webhook server, no `ValidatingWebhookConfiguration`/`MutatingWebhookConfiguration`, no TLS/CA bundle handling. See [Security Configuration](../../configuration/tls-security.md).
- **`ReconcilerConfig` CRD**, or any per-reconciler concurrency/backoff/sync tuning - see [Reconciler Configuration](../../configuration/reconciler-config.md) for what the reconcilers actually do.
- **NetworkPolicy, PodSecurityPolicy, PersistentVolume/PersistentVolumeClaim, DaemonSet, and HorizontalPodAutoscaler templates** - none are shipped by the chart. If you need any of these (e.g. a baseline deny-all `NetworkPolicy`, or a `ResourceQuota` per tenant namespace), they're plain Kubernetes objects you apply yourself against the namespace a `Tenant` creates - see [Multi-Tenant Setup](../../configuration/multi-tenant-setup.md#what-you-still-have-to-set-up-yourself) for worked examples.
- **Tenant-level `resourceQuota`/`networkPolicy`/`rbac` spec fields** - the real `Tenant` CRD has none of these; namespace isolation (RBAC, network policies, storage) is not automatically enforced.

## Log Collection

Muto writes logs to stdout/stderr in the format set by `MUTO_LOG_FORMAT` (`json` by default) - it has no built-in log shipping. Point whatever log collector you already run (Fluentd, Fluent Bit, Vector, your cloud provider's node-level agent, etc.) at the operator pod's container logs the same way you would for any other workload; there's nothing Muto-specific to configure.
