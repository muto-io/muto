---
name: muto
description: Schedule and manage agent workloads via the Muto operator
trigger: /muto, or when user asks to schedule agents, run agent workloads, check job status, or manage multi-agent tasks
---

# Muto Agent Scheduler

Use these MCP tools to schedule and manage agent workloads on the Muto operator.

Muto runs on **Kubernetes** (primary) or **Cloud Foundry** (single instance). The operator handles multi-tenant isolation and job scheduling across both platforms.

## Standard Workflow

1. **Verify tenant** — always call `muto:describe_tenant` first to confirm the tenant is ready and note its isolation tier.
2. **Schedule job** — call `muto:schedule_agent_job` with a unique job ID, tenant ID, image, and TTL.
3. **Poll status** — call `muto:get_job_status` every 5–10 seconds until phase is `Succeeded` or `Failed`.
4. **Cancel if needed** — call `muto:cancel_job` if the user requests early termination.
5. **Fleet view** — call `muto:list_active_agents` to see all running jobs for a tenant.

## Tools

### `muto:schedule_agent_job`
Schedule a new agent job.

| Parameter | Type | Required | Description |
|---|---|---|---|
| `job_id` | string | yes | Unique identifier for this job |
| `tenant_id` | string | yes | Tenant to run the job under |
| `image` | string | yes | Container image for the worker agent |
| `trigger_source` | string | no | Event source URL (e.g. `nats://tasks.inbound`) |
| `ttl` | number | no | Seconds before cleanup after completion (default 300) |

### `muto:get_job_status`
Returns: `phase` (Pending/Running/Succeeded/Failed/Terminating), `activeAgents`, `startedAt`, `completedAt`.

### `muto:cancel_job`
Sets the job to Terminating and cleans up all agent Pods.

### `muto:list_active_agents`
Lists all non-terminal AgentJobs for a tenant.

### `muto:describe_tenant`
Returns: `isolationTier` (shared/dedicated), `messageBusType` (nats/kafka), `namespace`, `ready`.

## AgentJob Spec Reference

```yaml
job_id: my-analysis-job          # must be unique per tenant
tenant_id: acme                  # must match an existing ready Tenant
image: ghcr.io/acme/worker:latest
trigger_source: nats://tasks.inbound
ttl: 300                         # 5 minutes after completion
```

### Platform Support

| Feature | Kubernetes | Cloud Foundry |
|---------|------------|---------------|
| Multi-replica agents | ✅ Full support (maxReplicas) | ✅ Runs as CF tasks |
| Message bus types | ✅ NATS, Kafka | ✅ NATS, Kafka |
| Tenant isolation | ✅ Shared/dedicated tiers | ✅ Shared/dedicated tiers |
| Horizontal scaling | ✅ Multiple operator instances | ⚠️ Single operator instance |

## Phases

| Phase | Meaning |
|---|---|
| Pending | Job created, agents not yet spawned |
| Running | Agents are active and processing |
| Succeeded | All agents completed successfully |
| Failed | One or more agents failed |
| Terminating | Cleanup in progress, Pods being deleted |

## Deployment

For information on how to deploy and configure the Muto operator:

- **Kubernetes** – See [`deploy/helm/`](https://github.com/muto-io/muto/tree/main/deploy/helm) for Helm chart
- **Cloud Foundry** – See [`deploy/cf/README.md`](https://github.com/muto-io/muto/blob/main/deploy/cf/README.md) for CF deployment guide
- **Local development** – Use `make kind-up` to spin up a local test cluster
