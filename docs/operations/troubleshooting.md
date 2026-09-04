# Troubleshooting Guide

Common issues in Muto deployments and how to diagnose and resolve them.

## Diagnostic Workflow

When troubleshooting issues, follow this systematic approach:

```
1. Check operator health
   ↓
2. Verify cluster/platform connectivity
   ↓
3. Check job status and events
   ↓
4. Examine logs for errors
   ↓
5. Verify configuration
   ↓
6. Check resource availability
```

## Operator Issues

### Operator Pod Not Starting

**Symptoms:**
- Pod status: `CrashLoopBackOff`, `ImagePullBackOff`, or `Error`
- No logs or immediate crash

**Diagnosis:**

```bash
# Check pod status
kubectl get pods -n muto-system

# Describe pod for events
kubectl describe pod -n muto-system <pod-name>

# View logs
kubectl logs -n muto-system <pod-name>
```

**Solutions:**

**Case 1: ImagePullBackOff**
```bash
# Verify image exists and is accessible
docker pull muto-operator:latest

# Check image pull secrets
kubectl get secrets -n muto-system

# If using private registry, verify secret
kubectl create secret docker-registry regcred \
  --docker-server=myregistry.com \
  --docker-username=user \
  --docker-password=pass \
  -n muto-system
```

**Case 2: Insufficient Resources**
```bash
# Check node resources
kubectl describe nodes

# Check operator resource requests
kubectl get deployment -n muto-system muto-operator -o yaml

# If insufficient, increase node resources or reduce replica count
kubectl scale deployment muto-operator -n muto-system --replicas=1
```

**Case 3: Configuration Error**
```bash
# Check environment variables
kubectl env deployment/muto-operator -n muto-system

# Verify ConfigMaps and Secrets
kubectl get configmaps -n muto-system
kubectl get secrets -n muto-system

# Check ConfigMap content
kubectl get cm muto-config -n muto-system -o yaml
```

### Operator Running but Not Processing Jobs

**Symptoms:**
- Operator pod is running
- Jobs stay in Pending state indefinitely
- No progress in agent execution

**Diagnosis:**

```bash
# Check operator logs
kubectl logs -n muto-system deployment/muto-operator -f

# Check reconciler status
kubectl get agentjobs

# Describe a pending job
kubectl describe agentjob <job-name>
```

**Solutions:**

**Case 1: Reconciler Not Running**
```bash
# Check if reconciler is enabled
kubectl logs -n muto-system deployment/muto-operator | grep -i reconciler

# Check for errors in reconciliation loop
kubectl logs -n muto-system deployment/muto-operator | grep -i "error"

# Increase log level to debug
kubectl set env deployment/muto-operator -n muto-system MUTO_LOG_LEVEL=debug
kubectl logs -n muto-system deployment/muto-operator -f
```

**Case 2: Scheduler Issues**
```bash
# Check scheduler logs
kubectl logs -n muto-system deployment/muto-operator | grep -i scheduler

# Verify scheduler configuration
kubectl get configmap muto-config -n muto-system -o yaml | grep -A 20 scheduler

# Check if scheduler worker count is > 0
kubectl env deployment/muto-operator -n muto-system | grep MUTO_SCHEDULER_WORKERS
```

**Case 3: Message Bus Not Connected**
```bash
# Test message bus connectivity
kubectl run -it --image=busybox --rm curl -- \
  sh -c "nc -zv nats 4222"

# Check message bus configuration
kubectl logs -n muto-system deployment/muto-operator | grep -i "message.*bus\|nats\|kafka"

# Verify credentials
kubectl get secret muto-message-bus -n muto-system -o yaml
```

## Job Execution Issues

### Jobs Stuck in Pending State

**Symptoms:**
- Job phase: `Pending`
- No Pod created on cluster
- Hours have passed

**Diagnosis:**

```bash
# Check job status
kubectl get agentjob <job-name> -o json | jq '.status'

# Check events
kubectl describe agentjob <job-name> | grep -A 20 Events:

# Check scheduler logs
kubectl logs -n muto-system deployment/muto-operator | grep <job-name>
```

**Solutions:**

**Case 1: Invalid Job Specification**
```bash
# Validate job YAML
kubectl apply -f job.yaml --dry-run=client

# Check for missing required fields
kubectl get agentjob <job-name> -o yaml | grep -E "tenantRef|agents"

# Common issues:
# - Missing tenantRef
# - Invalid agent image
# - Unsupported trigger type
# - TTL in wrong format
```

**Case 2: Tenant Not Found**
```bash
# Verify tenant exists
kubectl get tenants

# Check tenant status
kubectl get tenant <tenant-name> -o yaml

# Check if tenant namespace exists
kubectl get namespace <tenant-namespace>

# If missing, create tenant
kubectl apply -f - <<EOF
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: <tenant-name>
spec:
  namespace: <tenant-namespace>
EOF
```

**Case 3: Resource Quota Exceeded**
```bash
# Check tenant's resource quota
kubectl describe tenant <tenant-name>

# Check current usage
kubectl top pods -n <tenant-namespace>

# Check ResourceQuota
kubectl get resourcequota -n <tenant-namespace>

# Increase quota if needed
kubectl edit resourcequota -n <tenant-namespace>
```

### Jobs Fail Immediately

**Symptoms:**
- Job phase: `Failed`
- Agent exits with non-zero code
- Minimal logs

**Diagnosis:**

```bash
# Check job status
kubectl describe agentjob <job-name>

# Check agent logs
kubectl logs job/<job-name>-<agent-name>

# Check pod events
kubectl describe pod <pod-name>
```

**Solutions:**

**Case 1: Invalid Container Image**
```bash
# Verify image exists
docker pull <image>:<tag>

# Check image pull secrets (if using private registry)
kubectl get secrets -n <namespace>

# Test image locally
docker run <image>:<tag> /bin/sh -c "echo test"
```

**Case 2: Agent Process Crashed**
```bash
# Get exit code
kubectl logs <pod-name> | tail -20

# Common exit codes:
# 1: General error
# 127: Command not found
# 137: Out of Memory (SIGKILL)
# 139: Segmentation fault (SIGSEGV)

# Increase resource limits if OOM
kubectl edit agentjob <job-name>
# Modify spec.agents[].resources.limits.memory
```

**Case 3: Dependency Not Available**
```bash
# Check agent logs for specific errors
kubectl logs <pod-name> | grep -i "error\|failed\|panic"

# Verify environment variables are set
kubectl set env pods/<pod-name> --list

# Check mounted volumes
kubectl describe pod <pod-name> | grep -A 10 "Mounts:"
```

### Jobs Timeout

**Symptoms:**
- Job runs but exceeds timeout
- Phase: `Failed` with timeout message
- Agent process still running after timeout

**Diagnosis:**

```bash
# Check job timeout setting
kubectl get agentjob <job-name> -o yaml | grep timeout

# Check how long agent actually runs
kubectl logs <pod-name> --timestamps=true | tail -20

# Calculate actual duration
# Start time: first log timestamp
# End time: last log timestamp
```

**Solutions:**

**Case 1: Legitimate Long-Running Job**
```bash
# Increase timeout
kubectl patch agentjob <job-name> --type=merge \
  -p '{"spec":{"timeout":"600s"}}'

# Rebuild job with new timeout
kubectl delete agentjob <job-name>
kubectl apply -f job-with-longer-timeout.yaml
```

**Case 2: Job Hanging**
```bash
# Check if process is actually running
kubectl describe pod <pod-name>

# Check process state
kubectl exec <pod-name> -- ps aux

# Kill hanging process
kubectl exec <pod-name> -- kill -9 <pid>

# Restart job with better error handling
```

## Message Bus Issues

### Message Bus Connection Failures

**Symptoms:**
- Logs show "connection refused" or "timeout"
- Agents cannot publish/subscribe
- Inter-agent communication fails

**Diagnosis:**

```bash
# Check message bus pod status
kubectl get pods -n message-bus-namespace

# Test connectivity
kubectl run -it --image=busybox --rm -- \
  nc -zv message-bus-host 4222

# Check message bus logs
kubectl logs -n message-bus-namespace <message-bus-pod>
```

**Solutions:**

**Case 1: Message Bus Pod Down**
```bash
# Restart message bus
kubectl delete pod -n message-bus-namespace <pod-name>

# Or scale deployment
kubectl scale deployment nats -n message-bus-namespace --replicas=3

# Verify startup
kubectl logs -n message-bus-namespace -f deployment/nats
```

**Case 2: Network Connectivity Issue**
```bash
# Check network policy
kubectl get networkpolicies -n muto-system

# Verify DNS resolution
kubectl exec <pod> -- nslookup nats.message-bus-namespace

# Test port access
kubectl port-forward svc/nats 4222:4222 -n message-bus-namespace
# Then test locally: nc -zv localhost 4222
```

**Case 3: Authentication Failure**
```bash
# Check credentials
kubectl get secret muto-message-bus -n muto-system -o yaml

# Verify they match message bus configuration
kubectl describe statefulset nats -n message-bus-namespace | grep -i auth

# Update credentials if needed
kubectl create secret generic muto-message-bus \
  --from-literal=username=newuser \
  --from-literal=password=newpass \
  -n muto-system --dry-run=client -o yaml | kubectl apply -f -
```

## Platform-Specific Issues

### Kubernetes-Specific

#### CRD Not Recognized

**Symptoms:**
- Error: `no matches for kind "AgentJob"`
- Cannot create AgentJob resources

**Solution:**
```bash
# Install or reinstall CRDs
kubectl apply -f config/crd/

# Verify CRDs installed
kubectl get crd | grep muto

# Check CRD schema
kubectl explain agentjob
```

#### RBAC Permission Denied

**Symptoms:**
- Error: `forbidden: User cannot create AgentJobs`
- Logs show authorization errors

**Solution:**
```bash
# Check current RBAC
kubectl get rolebindings -n muto-system
kubectl get clusterrolebindings | grep muto

# Grant permissions
kubectl create rolebinding muto-admin \
  --clusterrole=cluster-admin \
  --serviceaccount=muto-system:muto-operator \
  -n muto-system
```

### CloudFoundry-Specific

#### Service Instance Not Binding

**Symptoms:**
- Error: `service instance not found`
- Cannot authenticate to CloudFoundry

**Solution:**
```bash
# List available services
cf services

# Create service instance if missing
cf create-service nats shared-vm muto-nats

# Bind service
cf bind-service muto-operator muto-nats

# Restart app
cf restart muto-operator
```

#### Task Deployment Fails

**Symptoms:**
- Task fails to start on CloudFoundry
- Memory or disk quota exceeded

**Solution:**
```bash
# Check space quota
cf space-quota <space-name>

# View current usage
cf org-users

# Increase quota if needed
cf update-space-quota <quota-name> --memory 512M --instances 10
```

## Log Analysis for Debugging

### Finding Errors in Logs

```bash
# Show only errors
kubectl logs -n muto-system deployment/muto-operator | grep -i error

# Show errors with context (3 lines before and after)
kubectl logs -n muto-system deployment/muto-operator | grep -i error -B 3 -A 3

# Show specific job's errors
kubectl logs -n muto-system deployment/muto-operator | grep "job-abc123" | grep -i error
```

### Tracing a Job Through Logs

```bash
# Get all log entries for a job
export JOB_NAME=job-abc123
kubectl logs -n muto-system deployment/muto-operator | grep "$JOB_NAME"

# Show timeline
kubectl logs -n muto-system deployment/muto-operator | grep "$JOB_NAME" | grep -E "Scheduled|Running|Completed|Failed"

# Show with timestamps
kubectl logs -n muto-system deployment/muto-operator --timestamps=true | grep "$JOB_NAME"
```

## Performance Diagnostics

### High CPU Usage

**Diagnosis:**
```bash
# Check CPU usage
kubectl top pods -n muto-system

# Find hot reconcilers
kubectl logs -n muto-system deployment/muto-operator | grep -i "duration" | sort -t= -k3 -rn | head -5
```

**Solutions:**
```bash
# Increase reconciliation interval
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_RECONCILER_POLL_INTERVAL_SECONDS=5

# Reduce worker count
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_SCHEDULER_WORKERS=2
```

### High Memory Usage

**Diagnosis:**
```bash
# Check memory usage
kubectl top pods -n muto-system

# Enable memory profiling
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_PROFILE_MEMORY=true
```

**Solutions:**
```bash
# Increase memory limits
kubectl edit deployment -n muto-system muto-operator
# Modify spec.template.spec.containers[0].resources.limits.memory

# Reduce job history retention
kubectl set env deployment/muto-operator -n muto-system \
  MUTO_COMPLETED_JOB_TTL_SECONDS=3600
```

## Recovery Procedures

### Operator Restart

```bash
# Graceful restart
kubectl rollout restart deployment/muto-operator -n muto-system

# Wait for rollout to complete
kubectl rollout status deployment/muto-operator -n muto-system

# Verify operator is running
kubectl logs -n muto-system deployment/muto-operator -f
```

### Clear Job Queue

```bash
# Delete pending jobs
kubectl delete agentjob -n <namespace> --field-selector=status.phase=Pending

# Delete failed jobs
kubectl delete agentjob -n <namespace> --field-selector=status.phase=Failed

# Delete all jobs (careful!)
kubectl delete agentjob -n <namespace> --all
```

### Reset Tenant

```bash
# Delete tenant (cascade deletes jobs)
kubectl delete tenant <tenant-name>

# Verify namespace deleted
kubectl get namespace <tenant-namespace>

# Recreate tenant
kubectl apply -f - <<EOF
apiVersion: muto.io/v1
kind: Tenant
metadata:
  name: <tenant-name>
spec:
  namespace: <tenant-namespace>
EOF
```

---

## Getting Help

If issues persist:

1. **Collect diagnostics:**
   ```bash
   # Export cluster info
   kubectl cluster-info dump --all-namespaces --output-directory=./cluster-info
   
   # Export operator logs
   kubectl logs -n muto-system deployment/muto-operator > operator.log
   
   # Export job status
   kubectl get agentjobs -o yaml > jobs.yaml
   ```

2. **Consult documentation:**
   - [Configuration Reference](../configuration/environment-variables.md)
   - [Architecture Overview](../architecture/overview.md)
   - [Monitoring and Observability](./monitoring-observability.md)

3. **Open an issue:**
   - Include cluster info dump
   - Include operator logs (last 1000 lines)
   - Include job YAML and status
   - Describe expected vs. actual behavior

---

**See Also:**
- [Monitoring and Observability](./monitoring-observability.md) — Setting up metrics and logs
- [Performance Tuning](./performance-tuning.md) — Optimizing for your workload
- [Configuration Reference](../configuration/environment-variables.md) — All configuration options
