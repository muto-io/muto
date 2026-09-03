# MCP Tools Reference

Complete specification of Muto's Model Context Protocol (MCP) tools for LLM integration. These tools allow Claude and other MCP clients to schedule, monitor, and manage agent jobs directly.

## Overview

Muto provides 5 primary MCP tools that allow LLMs (Claude, etc.) to interact with the agent scheduler:

1. **ScheduleJob** — Create and schedule a new agent job
2. **GetJobStatus** — Retrieve current status of a job
3. **ListJobs** — List active jobs for a tenant
4. **CancelJob** — Cancel a running or pending job
5. **DescribeTenant** — Get tenant configuration and status

All tools operate through the Muto MCP server endpoint (default: `http://localhost:3000`).

---

## ScheduleJob Tool

### Description

Creates and schedules a new agent job with the specified configuration. Returns immediately with a job ID; the actual execution is asynchronous.

### Request Parameters

#### `jobID` (string)
- **Type**: string
- **Required**: yes
- **Constraints**: Alphanumeric and hyphens only, max 255 characters
- **Description**: Unique identifier for this job (must be unique within the tenant)
- **Example**: `"data-pipeline-2026-09-03-001"`

#### `tenantID` (string)
- **Type**: string
- **Required**: yes
- **Description**: Tenant that owns this job
- **Constraints**: Must reference an existing Tenant resource
- **Example**: `"tenant-a"`

#### `image` (string)
- **Type**: string
- **Required**: yes
- **Description**: Container image to execute
- **Format**: Docker image reference (`registry/name:tag`)
- **Example**: `"gcr.io/myorg/data-processor:v1.2.3"`

#### `triggerSource` (string)
- **Type**: string
- **Required**: no
- **Default**: Empty string (manual trigger)
- **Description**: Source or context for this job trigger
- **Usage**: Can be a topic name, event ID, or other contextual information
- **Example**: `"workflow/data-ready"`

#### `ttl` (integer)
- **Type**: int32
- **Required**: no
- **Default**: 0 (no automatic cleanup)
- **Constraints**: >= 0
- **Unit**: seconds
- **Description**: Time-to-live after job completion. Job and associated resources are deleted after this duration expires
- **Example**: `3600` (delete after 1 hour)

### Request Example

```json
{
  "method": "scheduleJob",
  "params": {
    "jobID": "transform-daily-2026-09-03",
    "tenantID": "tenant-a",
    "image": "gcr.io/myorg/transformer:v2.1.0",
    "triggerSource": "schedule/daily-2am",
    "ttl": 86400
  }
}
```

### Response Format

#### Success Response (HTTP 200)

```json
{
  "jobID": "transform-daily-2026-09-03",
  "status": "Pending",
  "createdAt": "2026-09-03T10:30:45.123Z",
  "message": "Job scheduled successfully"
}
```

#### Error Response (HTTP 400-500)

```json
{
  "error": "invalid_request",
  "message": "Tenant 'tenant-x' not found",
  "details": {
    "field": "tenantID",
    "reason": "tenant_not_found"
  }
}
```

### Return Fields

#### `jobID` (string)
- The job ID passed in the request

#### `status` (string)
- Initial status, always `"Pending"`

#### `createdAt` (string)
- RFC3339 timestamp when the job was created

#### `message` (string)
- Human-readable success message

### Common Error Codes

| Code | HTTP | Reason |
|------|------|--------|
| `invalid_request` | 400 | Missing or malformed parameters |
| `job_already_exists` | 409 | JobID already exists in this tenant |
| `tenant_not_found` | 404 | TenantID doesn't reference existing Tenant |
| `invalid_image` | 400 | Image format invalid or not accessible |
| `scheduler_error` | 500 | Internal scheduler error |

---

## GetJobStatus Tool

### Description

Retrieves detailed status information for a specific job, including current phase, active agents, timing, and error details if applicable.

### Request Parameters

#### `jobID` (string)
- **Type**: string
- **Required**: yes
- **Description**: Unique identifier of the job to query
- **Example**: `"transform-daily-2026-09-03"`

### Request Example

```json
{
  "method": "getJobStatus",
  "params": {
    "jobID": "transform-daily-2026-09-03"
  }
}
```

### Response Format

#### Success Response (HTTP 200)

```json
{
  "jobID": "transform-daily-2026-09-03",
  "tenantID": "tenant-a",
  "phase": "Running",
  "activeAgents": 2,
  "startedAt": "2026-09-03T10:31:12.456Z",
  "completedAt": null,
  "progress": {
    "totalSteps": 3,
    "completedSteps": 1,
    "currentStep": "Transform"
  },
  "agents": [
    {
      "role": "extractor",
      "status": "Completed",
      "startedAt": "2026-09-03T10:31:12Z",
      "completedAt": "2026-09-03T10:35:20Z"
    },
    {
      "role": "transformer",
      "status": "Running",
      "startedAt": "2026-09-03T10:35:25Z",
      "completedAt": null
    }
  ]
}
```

#### Error Response (HTTP 404)

```json
{
  "error": "job_not_found",
  "message": "Job 'nonexistent-job' not found",
  "jobID": "nonexistent-job"
}
```

### Return Fields

#### `jobID` (string)
- The job ID queried

#### `tenantID` (string)
- Tenant that owns this job

#### `phase` (string)
- Current job phase: `Pending`, `Running`, `Succeeded`, `Failed`, `Terminating`

#### `activeAgents` (integer)
- Number of currently running agent instances

#### `startedAt` (string, nullable)
- RFC3339 timestamp when job transitioned to Running (null if still Pending)

#### `completedAt` (string, nullable)
- RFC3339 timestamp when job reached terminal phase (null if not terminal)

#### `progress` (object, optional)
- Advanced status information
- `totalSteps`: Total stages in the workflow
- `completedSteps`: Stages completed
- `currentStep`: Human-readable name of current stage

#### `agents` (array of objects)
- Detail for each agent role in the job

**Agent Status Fields:**
- `role` (string): Agent role name
- `status` (string): Current phase
- `startedAt` (string): When this agent started
- `completedAt` (string, nullable): When this agent finished

### Phase Definitions

| Phase | Meaning |
|-------|---------|
| `Pending` | Created, not yet scheduled |
| `Running` | At least one agent actively executing |
| `Succeeded` | All agents completed successfully |
| `Failed` | One or more agents failed (may retry) |
| `Terminating` | Job being cleaned up |

---

## ListJobs Tool

### Description

Lists active agent jobs for a specific tenant with optional filtering and pagination.

### Request Parameters

#### `tenantID` (string)
- **Type**: string
- **Required**: yes
- **Description**: Tenant to list jobs for
- **Example**: `"tenant-a"`

#### `filter` (object, optional)
- **Description**: Filter active jobs by phase or other criteria

**Filter Fields:**

**`filter.phase`** (string)
- **Enum**: `Pending`, `Running`, `Succeeded`, `Failed`, `Terminating`
- **Optional**: yes
- **Description**: Only return jobs in this phase
- **Example**: `"Running"`

**`filter.role`** (string)
- **Optional**: yes
- **Description**: Only return jobs that include this agent role
- **Example**: `"transformer"`

**`filter.before`** (string)
- **Format**: RFC3339 timestamp
- **Optional**: yes
- **Description**: Only return jobs created before this time
- **Example**: `"2026-09-03T10:00:00Z"`

**`filter.after`** (string)
- **Format**: RFC3339 timestamp
- **Optional**: yes
- **Description**: Only return jobs created after this time
- **Example**: `"2026-09-03T08:00:00Z"`

#### `limit` (integer)
- **Type**: int32
- **Default**: 25
- **Constraints**: 1-100
- **Description**: Maximum number of jobs to return
- **Example**: `50`

#### `offset` (integer)
- **Type**: int32
- **Default**: 0
- **Description**: Number of jobs to skip (for pagination)
- **Example**: `0`

### Request Example

```json
{
  "method": "listJobs",
  "params": {
    "tenantID": "tenant-a",
    "filter": {
      "phase": "Running"
    },
    "limit": 10,
    "offset": 0
  }
}
```

### Response Format

#### Success Response (HTTP 200)

```json
{
  "tenantID": "tenant-a",
  "totalCount": 23,
  "returnedCount": 10,
  "offset": 0,
  "limit": 10,
  "jobs": [
    {
      "jobID": "transform-2026-09-03-001",
      "phase": "Running",
      "activeAgents": 2,
      "createdAt": "2026-09-03T10:30:45Z",
      "startedAt": "2026-09-03T10:31:12Z",
      "completedAt": null
    },
    {
      "jobID": "transform-2026-09-03-002",
      "phase": "Pending",
      "activeAgents": 0,
      "createdAt": "2026-09-03T10:25:30Z",
      "startedAt": null,
      "completedAt": null
    }
  ]
}
```

### Return Fields

#### `tenantID` (string)
- Tenant queried

#### `totalCount` (integer)
- Total number of matching jobs (after filtering)

#### `returnedCount` (integer)
- Number of jobs in this response

#### `offset` (integer)
- Pagination offset used

#### `limit` (integer)
- Pagination limit used

#### `jobs` (array of objects)
- Job summaries

**Job Summary Fields:**
- `jobID`: Job identifier
- `phase`: Current phase
- `activeAgents`: Number of running instances
- `createdAt`: RFC3339 creation timestamp
- `startedAt`: When execution began (nullable)
- `completedAt`: When job finished (nullable)

---

## CancelJob Tool

### Description

Cancels a running or pending job. Cancellation is asynchronous; use GetJobStatus to verify the job has reached the terminal `Cancelled` phase.

### Request Parameters

#### `jobID` (string)
- **Type**: string
- **Required**: yes
- **Description**: Job to cancel
- **Example**: `"transform-daily-2026-09-03"`

#### `reason` (string, optional)
- **Type**: string
- **Description**: Human-readable reason for cancellation (for logging/audit)
- **Example**: `"User requested cancellation via Claude"`

### Request Example

```json
{
  "method": "cancelJob",
  "params": {
    "jobID": "transform-daily-2026-09-03",
    "reason": "Out of memory error detected, aborting"
  }
}
```

### Response Format

#### Success Response (HTTP 200)

```json
{
  "jobID": "transform-daily-2026-09-03",
  "status": "Terminating",
  "message": "Job cancellation initiated",
  "previousPhase": "Running"
}
```

#### Error Response (HTTP 404)

```json
{
  "error": "job_not_found",
  "message": "Job 'transform-daily-2026-09-03' not found"
}
```

#### Error Response: Already Terminal (HTTP 409)

```json
{
  "error": "job_already_terminal",
  "message": "Cannot cancel job already in phase: Succeeded",
  "currentPhase": "Succeeded"
}
```

### Return Fields

#### `jobID` (string)
- The job ID being cancelled

#### `status` (string)
- New status after cancellation request (usually `"Terminating"`)

#### `message` (string)
- Human-readable confirmation

#### `previousPhase` (string)
- Phase the job was in before cancellation

### Cancellation Semantics

- **Idempotent**: Cancelling an already-cancelled job succeeds
- **Graceful**: Running agents receive SIGTERM; cleanup waits up to 30 seconds
- **Asynchronous**: Cancellation is requested immediately, but pods may take time to terminate
- **Irreversible**: Cancelled jobs cannot be resumed

---

## DescribeTenant Tool

### Description

Returns observable information about a tenant from the scheduler's perspective, including active job count and isolation configuration.

### Request Parameters

#### `tenantID` (string)
- **Type**: string
- **Required**: yes
- **Description**: Tenant to describe
- **Example**: `"tenant-a"`

### Request Example

```json
{
  "method": "describeTenant",
  "params": {
    "tenantID": "tenant-a"
  }
}
```

### Response Format

#### Success Response (HTTP 200)

```json
{
  "tenantID": "tenant-a",
  "activeJobs": 5,
  "note": "For isolation tier and bus config, check the Tenant CR: kubectl get tenant tenant-a"
}
```

#### Error Response (HTTP 404)

```json
{
  "error": "tenant_not_found",
  "message": "Tenant 'tenant-x' not found"
}
```

### Return Fields

#### `tenantID` (string)
- The tenant ID queried

#### `activeJobs` (integer)
- Number of currently active (non-terminal) jobs for this tenant

#### `note` (string)
- Guidance: Full tenant configuration (isolation tier, message bus type) is not returned here; check the Tenant CRD via kubectl

### Note on Tenant Configuration

The MCP tool intentionally returns limited tenant info. For complete tenant details, check the Tenant CRD:

```bash
kubectl get tenant <tenantID> -o yaml
```

This separation of concerns maintains security (MCP clients don't need to know detailed configuration) and directs users to the authoritative source (Kubernetes API).

---

## Tool Response Status Codes

### HTTP 200 OK
- Request succeeded, response body contains result

### HTTP 400 Bad Request
- Request parameters invalid or malformed
- Example: Missing required parameter, invalid enum value

### HTTP 404 Not Found
- Resource doesn't exist
- Example: Job not found, Tenant not found

### HTTP 409 Conflict
- Operation conflicts with current state
- Example: Job ID already exists, job already terminal

### HTTP 500 Internal Server Error
- Unexpected server error
- Example: Scheduler panic, database unavailable

---

## Authentication & Authorization

The MCP server should be deployed with appropriate authentication:

- **In-Cluster**: Use Kubernetes service account authentication (bearer token)
- **Remote**: Use TLS client certificates or API keys
- **RBAC**: Each tool should verify the caller has appropriate tenant permissions

Example: A caller should not be able to `DescribeTenant` for a tenant they don't have access to.

---

## Tool Usage Examples with Claude

### Example 1: Schedule a Daily Data Pipeline

```
Claude: "Schedule a daily ETL pipeline that runs at 2 AM UTC"

Tool Call:
{
  "method": "scheduleJob",
  "params": {
    "jobID": "daily-etl-2026-09-03",
    "tenantID": "tenant-a",
    "image": "gcr.io/myorg/etl:v1",
    "triggerSource": "schedule/daily-2am-utc",
    "ttl": 86400
  }
}

Response:
{
  "jobID": "daily-etl-2026-09-03",
  "status": "Pending",
  "createdAt": "2026-09-03T10:30:45.123Z",
  "message": "Job scheduled successfully"
}

Claude: "Daily ETL pipeline scheduled with job ID 'daily-etl-2026-09-03'. 
It's currently in Pending state and will start when the trigger fires."
```

### Example 2: Check Running Jobs

```
Claude: "What's the status of running jobs in tenant-a?"

Tool Call 1:
{
  "method": "listJobs",
  "params": {
    "tenantID": "tenant-a",
    "filter": { "phase": "Running" },
    "limit": 25
  }
}

Response:
{
  "tenantID": "tenant-a",
  "totalCount": 3,
  "returnedCount": 3,
  "jobs": [
    {
      "jobID": "job1",
      "phase": "Running",
      "activeAgents": 2,
      "createdAt": "2026-09-03T10:00:00Z",
      "startedAt": "2026-09-03T10:05:00Z"
    }
  ]
}

Tool Call 2 (for details):
{
  "method": "getJobStatus",
  "params": { "jobID": "job1" }
}

Claude: "There are 3 running jobs in tenant-a:
1. job1 - Running with 2 active agents since 10:05 UTC"
```

### Example 3: Cancel a Long-Running Job

```
Claude: "The job 'transform-2026-09-03' seems to be stuck. Cancel it."

Tool Call:
{
  "method": "cancelJob",
  "params": {
    "jobID": "transform-2026-09-03",
    "reason": "Job appeared stuck, user requested cancellation"
  }
}

Response:
{
  "jobID": "transform-2026-09-03",
  "status": "Terminating",
  "message": "Job cancellation initiated",
  "previousPhase": "Running"
}

Claude: "Cancellation initiated for job 'transform-2026-09-03'. 
The job is now in Terminating state and will be cleaned up shortly."
```

---

## Rate Limiting & Quotas

The MCP server should enforce reasonable limits:

### Suggested Defaults

- **Per-tenant job creation rate**: 10 jobs/second
- **Per-client request rate**: 100 requests/second
- **Concurrent jobs per tenant**: 1000
- **Maximum job lifetime**: 30 days (jobs auto-deleted after)

### Rate Limit Response

```json
{
  "error": "rate_limit_exceeded",
  "message": "Exceeded 10 jobs/second for tenant",
  "retryAfter": 5
}
```

HTTP Status: 429 Too Many Requests

---

## Versioning

**Current MCP Tool Version**: v1

Breaking changes will follow semantic versioning:
- v1.x.y: Backward compatible additions
- v2: Breaking changes (will support both v1 and v2 during transition)

---

## Related Documentation

- **[Usage: Scheduling Agent Jobs](../usage/scheduling-agent-jobs.md)** — Practical job creation patterns
- **[Message API](./message-api.md)** — Inter-agent messaging details
- **[CRD Types](./crd-types.md)** — Complete CRD field reference

---

**Last Updated:** 2026-09-03  
**Tool Version**: v1  
**MCP Protocol Version**: 1.0+
