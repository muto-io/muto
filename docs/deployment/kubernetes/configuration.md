# Kubernetes-Specific Configuration

Advanced Kubernetes configuration options for Muto.

## Custom Resource Definitions (CRDs)

Muto extends Kubernetes with three custom resource types:

### AgentJob CRD

An AgentJob represents a request to execute one or more agents.

```yaml
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: data-pipeline
  namespace: default
spec:
  # Tenant this job belongs to
  tenant: default-tenant
  
  # List of agents to execute
  agents:
    - name: extractor
      image: myregistry.io/extractor:v1.2.0
      command: ["python", "extract.py"]
      args: ["--source", "s3://data/input"]
      env:
        - name: AWS_REGION
          value: us-east-1
        - name: AWS_CREDENTIALS
          valueFrom:
            secretKeyRef:
              name: aws-credentials
              key: secret-access-key
      resources:
        requests:
          memory: "256Mi"
          cpu: "250m"
        limits:
          memory: "512Mi"
          cpu: "500m"
      timeout: 5m
      retryPolicy:
        maxRetries: 3
        backoffSeconds: 10
  
  # Global job configuration
  timeout: 30m
  parallelism: 2
  
  # Define dependencies between agents
  dependencies:
    - agent: extractor
      triggers:
        - event: completed
          topic: "workflow/extract-done"
      condition: "status == 'Completed'"
  
  # Message bus subscriptions for coordination
  subscriptions:
    - agent: processor
      topic: "workflow/extract-done"
      filter: "source == 'extractor'"

status:
  phase: Running
  conditions:
    - type: Scheduled
      status: "True"
      lastTransitionTime: "2026-09-03T10:00:00Z"
  startTime: "2026-09-03T10:00:00Z"
  completionTime: null
```

**CRD Schema Reference:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `metadata.name` | string | Yes | Unique job identifier |
| `spec.tenant` | string | Yes | Tenant namespace |
| `spec.agents` | array | Yes | List of agents to run |
| `spec.timeout` | duration | No | Overall job timeout (default: 1h) |
| `spec.parallelism` | integer | No | Max concurrent agent executions |
| `spec.dependencies` | array | No | Agent dependency definitions |
| `status.phase` | string | No | Current job state |

### Tenant CRD

A Tenant represents a logical isolation boundary.

```yaml
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: acme-corp
  namespace: muto-system
spec:
  # Kubernetes namespace for this tenant
  namespace: acme-corp-ns
  
  # Message bus topic prefix
  messageBusConfig:
    topicPrefix: "acme-corp"
    subscriptionPrefix: "acme-corp-sub"
  
  # Resource quotas
  resourceQuota:
    jobs: 1000
    agents: 5000
    cpu: "100"
    memory: "500Gi"
  
  # Network policies
  networkPolicy:
    enabled: true
    allowIngressFromNamespaces:
      - default
      - monitoring
  
  # RBAC bindings
  rbac:
    roleBindings:
      - role: admin
        subjects:
          - kind: User
            name: alice@acme.com
      - role: developer
        subjects:
          - kind: Group
            name: acme-engineers

status:
  phase: Active
  createdAt: "2026-09-03T10:00:00Z"
```

### ReconcilerConfig CRD

Configuration for reconcilers.

```yaml
apiVersion: muto.io/v1
kind: ReconcilerConfig
metadata:
  name: agent-job-reconciler
  namespace: muto-system
spec:
  reconcilerName: agent-job-reconciler
  
  # Concurrency settings
  concurrency:
    workers: 10
    maxConcurrentReconciles: 20
  
  # Retry settings
  retry:
    initialBackoffSeconds: 1
    maxBackoffSeconds: 300
    maxRetries: 15
  
  # Sync settings
  sync:
    period: 10m
    jitter: 1m
  
  # Features
  features:
    enableWebhooks: true
    enableMetrics: true
```

## Kubernetes Namespaces

### System Namespace

The muto-system namespace contains the operator and webhooks:

```bash
kubectl get all -n muto-system

# Create with proper labels
kubectl create namespace muto-system
kubectl label namespace muto-system muto.io/admission=enabled
```

### Tenant Namespaces

Each tenant gets a dedicated namespace:

```bash
# For tenant "customer-a"
kubectl create namespace customer-a
kubectl label namespace customer-a \
  muto.io/tenant=customer-a \
  muto.io/isolation=true
```

Namespace isolation enforces:
- RBAC: Only tenant users can access tenant namespace
- Network policies: Isolate traffic between tenants
- Storage: Separate PVCs per tenant

## RBAC Configuration

### Operator Service Account

The operator needs cluster-admin for CRD management:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: muto-operator
  namespace: muto-system

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: muto-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
  - kind: ServiceAccount
    name: muto-operator
    namespace: muto-system
```

### Tenant RBAC

Define per-tenant access:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: tenant-admin
  namespace: customer-a
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
  - kind: User
    name: alice@customer-a.com
  - kind: Group
    name: customer-a-admins
```

## Network Policies

### Deny All by Default

Create a baseline network policy that denies all traffic:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: default-deny-all
  namespace: muto-system
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
```

### Allow Operator to Webhook

Allow communication from operator to webhook:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-operator-to-webhook
  namespace: muto-system
spec:
  podSelector:
    matchLabels:
      app: muto-webhook
  policyTypes:
    - Ingress
  ingress:
    - from:
      - podSelector:
          matchLabels:
            app: muto-operator
      ports:
      - protocol: TCP
        port: 8443
```

### Allow Agents to Message Bus

Allow tenant agents to communicate with message bus:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-agents-to-messagebus
  namespace: customer-a
spec:
  podSelector:
    matchLabels:
      muto.io/agent: "true"
  policyTypes:
    - Egress
  egress:
    - to:
      - namespaceSelector:
          matchLabels:
            name: message-bus
      ports:
      - protocol: TCP
        port: 4222  # NATS default
```

## Storage Configuration

### Persistent Volumes for Muto State

Create a PV for operator state and logs:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: muto-state-pv
spec:
  capacity:
    storage: 100Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Retain
  storageClassName: fast-ssd
  hostPath:
    path: /data/muto-state
```

### Persistent Volume Claim

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: muto-state
  namespace: muto-system
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: fast-ssd
  resources:
    requests:
      storage: 50Gi
```

## Admission Webhooks

Muto uses admission webhooks for validation and mutation.

### Validating Webhook

Validates AgentJob specs before creation:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: muto-operator-validation
webhooks:
  - name: validation.muto.io
    admissionReviewVersions: ["v1"]
    clientConfig:
      service:
        name: muto-webhook
        namespace: muto-system
        path: "/validate/v1/agentjobs"
      caBundle: <base64-encoded-ca-cert>
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["muto.io"]
        apiVersions: ["v1"]
        resources: ["agentjobs"]
    failurePolicy: Fail
    sideEffects: None
```

### Mutating Webhook

Mutates AgentJob specs (adds defaults, injects config):

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: muto-operator-mutation
webhooks:
  - name: mutation.muto.io
    admissionReviewVersions: ["v1"]
    clientConfig:
      service:
        name: muto-webhook
        namespace: muto-system
        path: "/mutate/v1/agentjobs"
      caBundle: <base64-encoded-ca-cert>
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["muto.io"]
        apiVersions: ["v1"]
        resources: ["agentjobs"]
    failurePolicy: Fail
    sideEffects: NoneOnDryRun
```

## Pod Security Policies

Define security constraints for tenant pods:

```yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: muto-agents
spec:
  privileged: false
  allowPrivilegeEscalation: false
  requiredDropCapabilities:
    - ALL
  runAsUser:
    rule: MustRunAsNonRoot
  fsGroup:
    rule: MustRunAs
    ranges:
      - min: 1000
        max: 65535
  readOnlyRootFilesystem: false
  volumes:
    - configMap
    - emptyDir
    - projected
    - secret
    - downwardAPI
    - persistentVolumeClaim
```

## Resource Quotas

Per-tenant resource quotas:

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: customer-a-quota
  namespace: customer-a
spec:
  hard:
    requests.cpu: "100"
    requests.memory: "500Gi"
    limits.cpu: "200"
    limits.memory: "1000Gi"
    pods: "5000"
    persistentvolumeclaims: "10"
  scopes:
    - Terminating
```

## Quality of Service (QoS) Classes

Ensure Muto pods have proper QoS:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  containers:
  - name: operator
    image: muto-io/muto-operator:0.1.0
    resources:
      requests:
        memory: "256Mi"
        cpu: "250m"
      limits:
        memory: "512Mi"
        cpu: "500m"
  # This pod gets Guaranteed QoS (requests == limits)
```

## DaemonSets for Node-Level Components

For node-local agents:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: muto-node-agent
  namespace: muto-system
spec:
  selector:
    matchLabels:
      app: muto-node-agent
  template:
    metadata:
      labels:
        app: muto-node-agent
    spec:
      nodeSelector:
        muto.io/agent-capable: "true"
      containers:
      - name: agent
        image: muto-io/muto-node-agent:0.1.0
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
```

## Scaling Configuration

### Horizontal Pod Autoscaling

Auto-scale the operator based on metrics:

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: muto-operator-hpa
  namespace: muto-system
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: muto-operator
  minReplicas: 3
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

## Monitoring and Observability

### Service Monitor for Prometheus

Enable Prometheus scraping:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: muto-operator
  namespace: muto-system
spec:
  selector:
    matchLabels:
      app: muto-operator
  endpoints:
    - port: metrics
      interval: 30s
      path: /metrics
```

### Log Collection with Fluentd

Collect Muto logs:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluentd-muto-config
  namespace: muto-system
data:
  input-kubernetes-muto.conf: |
    <source>
      @type tail
      path /var/log/containers/*muto*.log
      tag kubernetes.*
      read_from_head true
    </source>
```

---

**Last Updated:** 2026-09-03
