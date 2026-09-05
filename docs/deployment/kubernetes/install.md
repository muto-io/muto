# Installing Muto on Kubernetes

Deploy Muto to a production Kubernetes cluster.

## Prerequisites

### Kubernetes Cluster Requirements

- **Kubernetes version:** 1.24 or later
- **Cluster size:** Minimum 3 nodes (1 control-plane, 2 workers)
- **Resources per node:** 2 CPU, 4GB memory minimum
- **Network:** Ingress controller installed (optional, for external access)
- **Storage:** Persistent volumes available (for logs, state)
- **RBAC:** Cluster-admin access for initial setup

### Required Tools

- **kubectl:** Version matching your cluster (within 1 minor version)
- **Helm:** Version 3.10 or later
- **Networking:** Cluster-wide networking enabled (CNI plugin installed)

### Image Registry Access

Muto images must be accessible to your cluster:

- **Public registry:** Docker Hub images (`muto-io/muto-operator:v0.1.0`, etc.)
- **Private registry:** Configure image pull secrets if using private registry
- **Air-gapped:** Build and push images to local registry (see [building images](../../getting-started/installation.md#docker-images))

## Step 1: Prepare the Cluster

### Create Namespace

Create a dedicated namespace for Muto:

```bash
kubectl create namespace muto-system
```

### Label the Namespace

Label the namespace for admission webhooks:

```bash
kubectl label namespace muto-system muto.io/admission=enabled
```

### Verify Cluster Access

Ensure you have cluster-admin access:

```bash
kubectl auth can-i create clusterrolebindings --as=system:serviceaccount:muto-system:muto-operator
```

Expected output: `yes`

## Step 2: Create Image Pull Secrets (If Using Private Registry)

If using a private Docker registry, create an image pull secret:

```bash
kubectl create secret docker-registry muto-registry \
  --docker-server=<registry.example.com> \
  --docker-username=<username> \
  --docker-password=<password> \
  --docker-email=<email> \
  -n muto-system
```

## Step 3: Install Muto Using Helm

### Add Muto Helm Repository

```bash
helm repo add muto https://charts.muto.io
helm repo update
```

### Install Muto

Basic installation with default values:

```bash
helm install muto muto/muto-operator \
  --namespace muto-system \
  --create-namespace
```

**Custom namespace:**

```bash
helm install muto muto/muto-operator \
  --namespace my-operators \
  --create-namespace
```

**With custom values file:**

```bash
helm install muto muto/muto-operator \
  -f values.yaml \
  --namespace muto-system
```

### Verify Installation

Check that all pods are running:

```bash
kubectl get pods -n muto-system

# Expected output:
# NAME                              READY   STATUS    RESTARTS   AGE
# muto-operator-7d8f9c5d4b-abc12    1/1     Running   0          2m
# muto-webhook-7c3e2a1b9f-def45     1/1     Running   0          2m
```

Check CRDs are registered:

```bash
kubectl get crd | grep muto.io

# Expected output:
# agentjobs.muto.io                     2026-09-03T10:00:00Z
# tenants.muto.io                       2026-09-03T10:00:00Z
# reconcilerconfigs.muto.io             2026-09-03T10:00:00Z
```

Verify webhook is responsive:

```bash
kubectl get validatingwebhookconfiguration | grep muto

# Expected output:
# muto-operator-validation   1          2m
```

## Step 4: Configure Muto

### Set Up Message Bus

Configure message bus (NATS or Kafka) in `values.yaml`:

```yaml
messagebus:
  type: nats
  nats:
    url: nats://nats.default.svc.cluster.local:4222
    maxReconnect: 10
```

See [Message Bus Configuration](./helm-chart.md#message-bus-configuration) for details.

### Create Tenant

Create your first tenant:

```bash
kubectl apply -f - <<'EOF'
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: default-tenant
  namespace: muto-system
spec:
  namespace: default
  messageBusConfig:
    topicPrefix: "default-tenant"
  resourceQuota:
    jobs: 1000
    agents: 5000
EOF
```

Verify tenant is created:

```bash
kubectl get tenants -n muto-system
```

## Step 5: Verify Deployment

### Health Checks

Check operator health endpoint:

```bash
kubectl port-forward -n muto-system svc/muto-operator 8080:8080 &
curl http://localhost:8080/healthz
# Expected: "ok"
```

### Test Basic Job Scheduling

Create a test agent job:

```bash
kubectl apply -f - <<'EOF'
apiVersion: muto.io/v1
kind: AgentJob
metadata:
  name: test-deployment
spec:
  tenant: default-tenant
  agents:
    - name: test-agent
      image: alpine:latest
      command: ["echo"]
      args: ["Muto deployment successful!"]
  timeout: 30s
EOF
```

Monitor job:

```bash
kubectl get agentjobs
kubectl describe agentjob test-deployment
kubectl logs agentjob/test-deployment
```

### Expected Success Indicators

- [ ] All Muto pods are in Running state
- [ ] CRDs are registered (`kubectl get crd | grep muto.io`)
- [ ] Webhook is operational (`kubectl get validatingwebhookconfiguration`)
- [ ] Tenant created successfully
- [ ] Test job transitions through states: Pending -> Scheduled -> Running -> Completed
- [ ] Health check returns 200 OK

## Upgrade Muto

To upgrade to a new version:

```bash
helm upgrade muto muto/muto-operator \
  --namespace muto-system \
  --version 0.2.0
```

Wait for rollout to complete:

```bash
kubectl rollout status deployment/muto-operator -n muto-system
```

## Uninstall Muto

To remove Muto from your cluster:

```bash
helm uninstall muto --namespace muto-system
```

**Note:** This does not delete CRDs or custom resources. To clean up completely:

```bash
kubectl delete crd agentjobs.muto.io tenants.muto.io reconcilerconfigs.muto.io
```

## Troubleshooting

### Operator pod not starting

Check logs:
```bash
kubectl logs -n muto-system deployment/muto-operator --tail=50
```

Check pod events:
```bash
kubectl describe pod -n muto-system -l app=muto-operator
```

### CRDs not registered

Verify cluster has permission:
```bash
kubectl api-resources | grep muto
```

If missing, reinstall Helm chart or manually apply CRDs:
```bash
helm get values muto -n muto-system | grep installCRDs
```

### Webhook not working

Check webhook configuration:
```bash
kubectl get validatingwebhookconfiguration muto-operator-validation -o yaml
```

Verify webhook service is accessible:
```bash
kubectl get svc -n muto-system | grep webhook
```

### Jobs stuck in Pending

Check tenant exists:
```bash
kubectl get tenants
```

Check node resources:
```bash
kubectl describe nodes
```

Check operator logs for scheduling errors:
```bash
kubectl logs -n muto-system deployment/muto-operator | grep -i "pending\|schedule"
```

## Next Steps

- **[Helm Chart Reference](./helm-chart.md)** — Customize values and options
- **[Kubernetes Configuration](./configuration.md)** — Advanced K8s-specific settings
- **[Production Checklist](../production-checklist.md)** — Pre-launch verification
- **[Configuration Guide](../../configuration/)** — Environment variables and tuning

---

**Last Updated:** 2026-09-03
