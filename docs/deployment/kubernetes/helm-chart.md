# Muto Helm Chart Reference

Complete reference for customizing the Muto Helm chart.

## Chart Overview

- **Chart name:** `muto-operator`
- **Chart version:** 0.1.0
- **App version:** 0.1.0
- **Repository:** `https://charts.muto.io`

## Default Values

The Helm chart includes sensible defaults for most deployments. Customize by creating a `values.yaml` file:

```bash
helm show values muto/muto-operator > values.yaml
# Edit values.yaml
helm install muto muto/muto-operator -f values.yaml
```

## Global Configuration

### Image Configuration

```yaml
image:
  registry: docker.io
  repository: muto-io/muto-operator
  tag: "0.1.0"
  pullPolicy: IfNotPresent

imagePullSecrets:
  - name: muto-registry  # For private registries
```

**Example:** Use specific version:
```yaml
image:
  tag: "0.1.0-rc1"
```

### Replica Configuration

```yaml
replicaCount: 1  # Control plane pods (usually 1)

operator:
  replicas: 1
  
webhook:
  replicas: 1
```

**Example:** High availability setup:
```yaml
replicaCount: 3
operator:
  replicas: 3
webhook:
  replicas: 3
```

## Operator Configuration

### Resource Limits

```yaml
operator:
  resources:
    limits:
      cpu: 1000m
      memory: 512Mi
    requests:
      cpu: 500m
      memory: 256Mi
```

**Recommendations:**
- **Development:** 500m CPU, 256Mi memory
- **Production:** 1000m CPU, 512Mi memory
- **High-traffic:** 2000m CPU, 1Gi memory

### Reconciliation Settings

```yaml
operator:
  reconciliation:
    maxConcurrentReconciles: 10
    syncPeriod: 15m
    retryBackoff:
      initialDuration: 100ms
      maxDuration: 1000ms
```

### Scheduling

```yaml
operator:
  scheduler:
    workers: 4
    batchSize: 100
    timeout: 5m
```

## Webhook Configuration

### Webhook Service

```yaml
webhook:
  service:
    type: ClusterIP
    port: 443
    targetPort: 8443
```

### Webhook Resources

```yaml
webhook:
  resources:
    limits:
      cpu: 500m
      memory: 256Mi
    requests:
      cpu: 250m
      memory: 128Mi
```

### TLS Configuration

```yaml
webhook:
  tls:
    enabled: true
    certManagerEnabled: true
    issuerName: muto-selfsigned
```

## Message Bus Configuration

### NATS Backend

```yaml
messagebus:
  type: nats
  nats:
    url: "nats://nats.default.svc.cluster.local:4222"
    maxReconnect: 10
    reconnectWait: 1s
    auth:
      enabled: false
      # username: admin
      # passwordSecret:
      #   name: nats-credentials
      #   key: password
```

**Example:** External NATS cluster:
```yaml
messagebus:
  type: nats
  nats:
    url: "nats://nats.example.com:4222"
    auth:
      enabled: true
      passwordSecret:
        name: external-nats-creds
        key: password
```

### Kafka Backend

```yaml
messagebus:
  type: kafka
  kafka:
    brokers:
      - "kafka-0.kafka.default.svc.cluster.local:9092"
      - "kafka-1.kafka.default.svc.cluster.local:9092"
      - "kafka-2.kafka.default.svc.cluster.local:9092"
    auth:
      enabled: false
      saslMechanism: PLAIN
      # username: admin
      # passwordSecret:
      #   name: kafka-credentials
      #   key: password
```

## Observability Configuration

### Metrics

```yaml
metrics:
  enabled: true
  port: 8080
  path: /metrics
  
prometheus:
  enabled: true
  serviceMonitor:
    enabled: false  # Enable if using Prometheus Operator
    interval: 30s
```

### Logging

```yaml
logging:
  level: info  # debug, info, warn, error
  format: json  # json or text
  
audit:
  enabled: true
  maxAgeInDays: 30
```

## Storage Configuration

### Persistent Volumes

```yaml
persistence:
  enabled: true
  storageClass: "default"
  size: 10Gi
  mountPath: /var/lib/muto
```

### Backups

```yaml
backup:
  enabled: false
  schedule: "0 2 * * *"  # Daily at 2 AM
  retention: 30  # days
  storageClass: "backup"
```

## Network Configuration

### Network Policies

```yaml
networkPolicy:
  enabled: true
  ingress:
    - from:
      - podSelector:
          matchLabels:
            role: client
  egress:
    - to:
      - namespaceSelector: {}
```

### Ingress

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt
  hosts:
    - host: muto.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: muto-tls
      hosts:
        - muto.example.com
```

## RBAC Configuration

```yaml
rbac:
  create: true
  
serviceAccount:
  create: true
  name: muto-operator
  annotations:
    iam.gke.io/gcp-service-account: muto@project.iam.gserviceaccount.com
```

## Security Configuration

### Pod Security

```yaml
podSecurityPolicy:
  enabled: false  # Set to true if PSP is enforced
  
securityContext:
  runAsNonRoot: true
  runAsUser: 65534
  fsReadOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

### Pod Disruption Budget

```yaml
podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

## Affinity Configuration

### Node Affinity

```yaml
affinity:
  nodeAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        preference:
          matchExpressions:
            - key: node-type
              operator: In
              values:
                - operator
```

### Pod Affinity

```yaml
affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
            - key: app
              operator: In
              values:
                - muto-operator
        topologyKey: kubernetes.io/hostname
```

## Example: Production Configuration

A complete production-ready `values.yaml`:

```yaml
replicaCount: 3

image:
  registry: docker.io
  repository: muto-io/muto-operator
  tag: "0.1.0"
  pullPolicy: IfNotPresent

operator:
  resources:
    limits:
      cpu: 2000m
      memory: 1Gi
    requests:
      cpu: 1000m
      memory: 512Mi
  
  reconciliation:
    maxConcurrentReconciles: 20
    syncPeriod: 10m

webhook:
  resources:
    limits:
      cpu: 500m
      memory: 256Mi
    requests:
      cpu: 250m
      memory: 128Mi
  tls:
    enabled: true
    certManagerEnabled: true

messagebus:
  type: kafka
  kafka:
    brokers:
      - "kafka-0.kafka.data:9092"
      - "kafka-1.kafka.data:9092"
      - "kafka-2.kafka.data:9092"
    auth:
      enabled: true
      passwordSecret:
        name: kafka-credentials
        key: password

metrics:
  enabled: true
  prometheus:
    serviceMonitor:
      enabled: true

persistence:
  enabled: true
  storageClass: "fast-ssd"
  size: 50Gi

networkPolicy:
  enabled: true

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: muto.example.com
      paths:
        - path: /
          pathType: Prefix

podDisruptionBudget:
  enabled: true
  minAvailable: 2

affinity:
  podAntiAffinity:
    requiredDuringSchedulingIgnoredDuringExecution:
      - labelSelector:
          matchExpressions:
            - key: app
              operator: In
              values:
                - muto-operator
        topologyKey: kubernetes.io/hostname
```

## Helm Upgrade Strategy

### In-Place Upgrade

Upgrade without downtime:

```bash
helm upgrade muto muto/muto-operator \
  -f values.yaml \
  --wait \
  --timeout 5m
```

Monitor rollout:

```bash
kubectl rollout status deployment/muto-operator -n muto-system
```

### Blue-Green Upgrade

For zero-downtime upgrades with multiple replicas:

```bash
# Create new release on different label
helm install muto-blue muto/muto-operator \
  -f values-prod.yaml \
  --set blueGreen.active=blue

# Switch traffic
kubectl patch svc muto-operator -p '{"spec":{"selector":{"version":"blue"}}}'

# Clean up old release
helm uninstall muto-green
```

### Rollback

If an upgrade fails:

```bash
helm rollback muto
```

Check rollback status:

```bash
kubectl rollout status deployment/muto-operator -n muto-system
```

## Advanced Customization

### Custom Image Registry

For air-gapped environments:

```yaml
image:
  registry: registry.internal.company.com
  repository: ops/muto-operator
  tag: "0.1.0"
  
imagePullSecrets:
  - name: private-registry-credentials
```

### Custom Resource Quotas

```yaml
resources:
  requests:
    memory: "1Gi"
    cpu: "1000m"
  limits:
    memory: "2Gi"
    cpu: "2000m"
```

### Custom Labels and Annotations

```yaml
labels:
  managed-by: terraform
  environment: production
  team: platform

podAnnotations:
  prometheus.io/scrape: "true"
  prometheus.io/port: "8080"
```

## Chart Values Schema

For full details on all available values, see:

```bash
helm show values muto/muto-operator
```

Or check the chart's `values.yaml` in the [Muto GitHub repository](https://github.com/muto-io/helm-charts).

---

**Last Updated:** 2026-09-03
