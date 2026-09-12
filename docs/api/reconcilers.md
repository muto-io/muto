# Reconcilers Reference

Reconcilers are the core components that manage Muto's resource lifecycle and orchestration. This page provides an overview of reconcilers and their role in the system.

## What are Reconcilers?

Reconcilers are control loops that watch for changes to Kubernetes resources and drive the cluster toward the desired state. They handle:

- Resource creation and deletion
- Status updates and lifecycle management
- Event-driven job scheduling
- Multi-platform coordination (Kubernetes, CloudFoundry)

## Core Reconcilers

### Tenant Reconciler
Manages Tenant resource lifecycle and isolation configuration.

**Responsibilities:**
- Create and configure tenant namespaces
- Set up message bus instances
- Configure RBAC and network policies
- Handle tenant deletion and cleanup

**Triggers:**
- Tenant resource created/updated
- Underlying namespace/message-bus state changes

### AgentJob Reconciler
Manages AgentJob scheduling, execution, and status tracking.

**Responsibilities:**
- Schedule jobs based on trigger configuration
- Create agent pods/containers
- Monitor job progress and update status
- Handle job cancellation and cleanup

**Triggers:**
- AgentJob resource created/updated
- Trigger conditions met (cron, event, manual)
- Agent pod state changes

### AgentFleet Reconciler
Manages groups of AgentJobs as a coordinated unit.

**Responsibilities:**
- Coordinate multiple jobs
- Track fleet-level status
- Handle fleet-wide lifecycle operations
- Manage inter-job communication

**Triggers:**
- AgentFleet resource created/updated
- Member job state changes

## Reconciliation Patterns

### Watch-Based Reconciliation
```
1. Controller watches for resource changes
2. Event detected (create/update/delete)
3. Reconciler triggered for affected resource
4. Current state compared against desired state
5. Actions taken to converge toward desired state
6. Status updated on resource
```

### Event-Driven Scheduling
```
1. AgentJob with trigger.type=event waits for messages
2. Message received on job topic
3. Reconciler detects trigger condition
4. Job scheduled and agents spawned
5. Job executes and completes
6. Results sent downstream
```

### Cron-Based Scheduling
```
1. AgentJob with trigger.type=cron configured
2. Cron expression evaluated by scheduler
3. At scheduled time, reconciler triggers
4. Job scheduled and agents spawned
5. Job executes on schedule
```

## Multi-Platform Reconciliation

Muto supports orchestration across multiple platforms:

### Kubernetes Reconciler
- Uses native Kubernetes controller patterns
- Manages CRDs and custom resources
- Interacts with Kubernetes API server
- Primary platform for agent scheduling

### CloudFoundry Reconciler
- Manages CF application lifecycle
- Handles service binding and configuration
- Coordinates with CF API
- Alternative platform for agent scheduling

## Reconciler Configuration

Reconcilers are configured through environment variables:

```bash
# Enable/disable reconcilers
MUTO_RECONCILER_TENANT_ENABLED=true
MUTO_RECONCILER_AGENTJOB_ENABLED=true
MUTO_RECONCILER_AGENTFLEET_ENABLED=true

# Reconciliation timing
MUTO_RECONCILE_INTERVAL=5s
MUTO_WORKER_THREADS=4

# Platform-specific settings
MUTO_PLATFORM=k8s  # or 'cf'
MUTO_KUBECONFIG=/path/to/kubeconfig
```

## Advanced Reconciliation

### Finalizers
Reconcilers use Kubernetes finalizers to ensure clean resource deletion:

```yaml
metadata:
  finalizers:
    - muto.io/agentjob-finalizer
```

Finalizers guarantee:
- Agent pods are cleaned up before resource deletion
- Message bus state is properly released
- Status is correctly recorded

### Ownership and References
Resource ownership ensures proper garbage collection:

```yaml
ownerReferences:
  - apiVersion: muto.io/v1alpha1
    kind: Tenant
    name: my-tenant
    uid: <uid>
```

### Status Conditions
Reconcilers report detailed status through conditions:

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      reason: Scheduled
      message: "Agents running"
    - type: Available
      status: "True"
      reason: Running
      message: "All agents healthy"
```

## Reconciliation Guarantees

- **At-least-once**: Actions are guaranteed to execute at least once
- **Idempotent**: Repeated reconciliation with same state produces same result
- **Progressive**: State converges toward desired configuration
- **Ordered**: Dependencies are respected (Tenant before AgentJob, etc.)

## Troubleshooting Reconcilers

### Check Reconciler Status
```bash
# View controller logs
kubectl logs -n muto-system deployment/muto-controller -f

# Check reconciler events
kubectl describe tenant my-tenant
kubectl describe agentjob my-job
```

### Common Issues

**Reconciler loops not progressing:**
- Check for finalizer conflicts
- Verify RBAC permissions
- Review controller logs for errors

**Jobs not scheduling:**
- Verify trigger conditions
- Check message bus connectivity
- Review job status conditions

## Detailed Reference

For complete reconciler implementation details and API specifications, see:

[:octicons-book-24: **Architecture: Reconcilers**](../architecture/reconcilers.md) — Deep dive into reconciler design, state machines, and implementation patterns

## Related Documentation

- **[Architecture: Agent Lifecycle](../architecture/agent-lifecycle.md)** — Agent execution model and lifecycle
- **[Usage: Scheduling Agent Jobs](../usage/scheduling-agent-jobs.md)** — Practical guide to scheduling jobs
- **[Development: Contributing](../development/contributing.md)** — Contributing improvements to reconcilers
