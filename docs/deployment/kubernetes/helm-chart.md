# Muto Helm Chart Reference

What the `deploy/helm/muto` chart actually contains and installs today.

## Chart Overview

- **Chart name:** `muto`
- **Chart version:** `0.1.0` (`Chart.yaml` `version`)
- **App version:** `latest` (`Chart.yaml` `appVersion` - the default image tag; see [Image](#image))
- **Published to:** `oci://ghcr.io/muto-io/charts` - pushed by `.github/workflows/release.yml`'s `publish-helm` job, but only on a `v*.*.*` tag push, not on every merge to `main`.

```bash
helm show values oci://ghcr.io/muto-io/charts/muto > values.yaml
# edit values.yaml
helm install muto-operator oci://ghcr.io/muto-io/charts/muto -f values.yaml -n muto-system --create-namespace
```

## Chart Structure

The chart has exactly these templates (`deploy/helm/muto/templates/`):

| Template | Creates | Gated by |
|---|---|---|
| `deployment-operator.yaml` | One `Deployment` running `muto-operator` | always |
| `serviceaccount.yaml` | `ServiceAccount` for the operator | `serviceAccount.create` |
| `clusterrole.yaml` + `clusterrolebinding.yaml` | Cluster-wide RBAC for the operator (see [Security Configuration](../../configuration/tls-security.md#kubernetes-rbac) for the exact permission table) | `rbac.create` |
| `service.yaml` | A `Service` exposing the metrics port | `metrics.enabled` |
| `servicemonitor.yaml` | A Prometheus Operator `ServiceMonitor` | `metrics.enabled` **and** `metrics.serviceMonitor.enabled` |
| `crds/tenants.yaml`, `crds/agentjobs.yaml`, `crds/agentfleets.yaml` | The three `muto.io` CRDs (`Tenant`, `AgentJob`, `AgentFleet`) | `installCRDs` |
| `NOTES.txt` | Post-install message (image, namespace, platform, a sample `Tenant` manifest) | n/a |

There is no `muto-mcp` deployment, no webhook `Service`/`ValidatingWebhookConfiguration`/`MutatingWebhookConfiguration`, no `ConfigMap`, no `PodDisruptionBudget`, and no `NetworkPolicy` template in this chart. `muto-mcp` runs over stdio (see its own docs) and isn't deployed by this chart at all.

## Values Reference

The real top-level keys in `values.yaml`:

```yaml
replicaCount: 1

image:
  repository: ghcr.io/muto-io/muto-operator
  tag: ""              # defaults to .Chart.AppVersion ("latest") when empty
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

installCRDs: true       # set false if CRDs are managed externally (e.g. GitOps)

env:                     # becomes the operator container's env vars - see Environment Variables below
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
  limits: { cpu: 500m, memory: 128Mi }
  requests: { cpu: 100m, memory: 64Mi }

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

That's the entire schema - there is no `operator.*`, `webhook.*`, `messagebus.*`, `persistence.*`, `backup.*`, `networkPolicy.*`, `ingress.*`, `podSecurityPolicy.*`, `securityContext.*`, `podDisruptionBudget.*`, or `labels`/`podAnnotations` key. Setting any of those via `--set` or an extra values file is a no-op (Helm ignores unknown keys by default).

### Image

`image.tag` is empty by default, which makes the template fall back to `.Chart.AppVersion` (`"latest"`). Pin a real version for production:

```yaml
image:
  tag: "sha-abc1234"   # or a semver tag published by release.yml, e.g. "0.1.0"
```

### `env`

Every key under `env` becomes a literal env var on the operator container (empty values are skipped). This is the same variable set documented in [Environment Variables](../../configuration/environment-variables.md) - the chart doesn't introduce any chart-specific naming for them. `MUTO_METRICS_BIND_ADDRESS` and `MUTO_HEALTH_PROBE_BIND_ADDRESS` are set separately by the template from `metrics.port`/`healthProbe.port`/`metrics.enabled`, not from the `env` map.

### Metrics and health probes

`metrics.enabled: false` sets `MUTO_METRICS_BIND_ADDRESS=0`, which disables the metrics server entirely (not just the Service/annotations). `metrics.port`/`healthProbe.port` only change the container port *declarations* - the operator's own bind addresses are effectively fixed by what the chart passes in via `env`, so don't change these without also changing the env values they drive. See [Monitoring and Observability](../../operations/monitoring-observability.md) for what's actually exported.

## CRDs

`installCRDs: true` (default) installs the three CRDs listed above as part of the release. If another tool (e.g. a GitOps controller) manages CRDs for you, set it to `false` and apply `deploy/helm/muto/templates/crds/*.yaml` through that tool instead - Helm doesn't upgrade or delete CRDs on `helm upgrade`/`helm uninstall` by default, chart-managed or not.

## Upgrading and Uninstalling

Standard Helm operations - nothing Muto-specific here:

```bash
helm upgrade muto-operator oci://ghcr.io/muto-io/charts/muto -f values.yaml -n muto-system
kubectl rollout status deployment/muto-operator -n muto-system

helm rollback muto-operator -n muto-system   # revert to the previous release
helm uninstall muto-operator -n muto-system  # leaves CRDs and any Tenants/AgentJobs/AgentFleets in place
```

Because the chart has no `PodDisruptionBudget` and `replicaCount` defaults to `1`, there's no built-in zero-downtime rollout story beyond whatever `kubectl rollout` gives you for a single-replica `Deployment`.

## Known Limitations

- No TLS, mTLS, or webhook support - see [Security Configuration](../../configuration/tls-security.md).
- No persistence/backup values - the operator is stateless (all state lives in the K8s API server via the CRDs).
- No `NetworkPolicy` or `Ingress` templates - add your own if you need them.
- No `PodDisruptionBudget`, pod affinity/anti-affinity presets, or `securityContext` values beyond the bare `nodeSelector`/`tolerations`/`affinity` passthroughs already listed above.
