# CF Adapter Implementation Design Spec

**Date:** 2026-07-12
**Status:** Approved

---

## Summary

Implement `platform/cf/` as a real Cloud Foundry `PlatformAdapter` using `cloudfoundry/go-cfclient/v3`. The adapter maps Muto's agent lifecycle to CF's runner-app + task model: a long-lived CF App per tenant+role acts as the execution host; each `AgentJob` triggers a CF Task on it. Task GUIDs are used as agent IDs throughout.

---

## File Layout

```
platform/cf/
├── client.go          # CFClient interface + NewRealCFClient (wraps go-cfclient)
├── adapter.go         # CFAdapter — SpawnAgent, WatchAgent, TerminateAgent
├── adapter_test.go    # unit tests with mockCFClient
└── cache.go           # org/space GUID cache (sync.Map, 5-min TTL)

platform/k8s/types/v1alpha1/
└── agentjob_types.go  # add Command field to AgentRoleSpec (backward compatible)

core/agent/
└── types.go           # add Command field to AgentRole (mirrors CRD change)
```

---

## CFClient Interface

```go
type CFClient interface {
    GetAppByName(ctx context.Context, name, spaceGUID string) (*resource.App, error)
    PushApp(ctx context.Context, req PushRequest) (*resource.App, error)
    RunTask(ctx context.Context, appGUID string, req TaskRequest) (*resource.Task, error)
    GetTask(ctx context.Context, taskGUID string) (*resource.Task, error)
    CancelTask(ctx context.Context, taskGUID string) error
    GetOrgByName(ctx context.Context, name string) (*resource.Organization, error)
    GetSpaceByName(ctx context.Context, orgGUID, name string) (*resource.Space, error)
}
```

`NewRealCFClient(apiURL, username, password string) (CFClient, error)` returns a wrapper around the real `go-cfclient` v3 client.

---

## Tenant Isolation Mapping

| Muto tier | CF mapping |
|---|---|
| `dedicated` | One CF Org per tenant; runner apps in a dedicated Space named `muto-agents` |
| `shared` | One shared Org (`SharedOrgName` from config); one Space per tenant |

---

## CFAdapterConfig

```go
type CFAdapterConfig struct {
    APIURL        string
    Username      string
    Password      string
    IsolationTier string // "shared" | "dedicated"
    SharedOrgName string // required when IsolationTier == "shared"
}
```

---

## SpawnAgent Flow

1. Resolve tenant org + space GUIDs (via cache, TTL 5 min)
2. Runner app name: `muto-<tenantID>-<role>`
3. If runner app not found → push with `sleep infinity` command (binary_buildpack)
4. Trigger CF Task on runner app:
   - Name: `<jobID>`
   - Command: `AgentRole.Command` (CF-specific field)
   - Env: `MUTO_TENANT`, `MUTO_ROLE`, `MUTO_JOB_ID`, `MUTO_BUS_TOPIC`
   - Memory: 512M, Disk: 1G
5. Return `task.GUID` as agentID

**Runner app push spec:**
```go
PushRequest{
    Name:      runnerAppName,
    SpaceGUID: spaceGUID,
    Instances: 1,
    Memory:    "256M",
    DiskQuota: "512M",
    Buildpack: "binary_buildpack",
    Command:   "sleep infinity",
    Env:       map[string]string{"MUTO_TENANT": tenantID, "MUTO_ROLE": role},
}
```

---

## WatchAgent Flow

Polls CF Task state every 5 seconds until terminal:

| CF Task state | Emitted event |
|---|---|
| `SUCCEEDED` | `agent.EventCompleted` |
| `FAILED` | `agent.EventFailed` |
| `RUNNING` / `PENDING` | continue polling |
| context cancelled | close channel, return |

---

## TerminateAgent Flow

Calls `CancelTask(ctx, agentID)`. Errors indicating the task is already terminal (`SUCCEEDED`, `FAILED`) are silently ignored.

---

## AgentRoleSpec Extension

Add `Command` field (backward compatible — `omitempty`):

```go
// platform/k8s/types/v1alpha1/agentjob_types.go
type AgentRoleSpec struct {
    Role        string `json:"role"`
    Image       string `json:"image,omitempty"`    // K8s: container image
    Command     string `json:"command,omitempty"`  // CF: task command
    MaxReplicas int32  `json:"maxReplicas,omitempty"`
}

// core/agent/types.go
type AgentRole struct {
    Role        string
    Image       string  // K8s
    Command     string  // CF
    MaxReplicas int32
}
```

After adding: run `make generate` to regenerate deepcopy + CRD YAML.

---

## GUID Cache

`platform/cf/cache.go` — avoids repeated org/space API calls:

```go
type guidCache struct {
    mu      sync.Mutex
    entries map[string]cacheEntry  // key: "org:<name>" or "space:<orgGUID>:<name>"
    ttl     time.Duration          // 5 minutes
}
```

---

## Platform Selection

`cmd/muto-operator/main.go` selects adapter via `MUTO_PLATFORM` env var (default: `k8s`):

```
MUTO_PLATFORM=k8s   → K8sAdapter (default)
MUTO_PLATFORM=cf    → CFAdapter (requires CF_API_URL, CF_USERNAME, CF_PASSWORD,
                                  CF_ISOLATION_TIER, CF_SHARED_ORG env vars)
```

---

## Unit Tests

All in `platform/cf/adapter_test.go` using `mockCFClient`:

| Test | Scenario |
|---|---|
| `TestSpawnAgentRunnerExists` | Runner app found → no push, task created |
| `TestSpawnAgentRunnerMissing` | Runner app not found → push then task |
| `TestWatchAgentSucceeded` | Task SUCCEEDED → EventCompleted emitted |
| `TestWatchAgentFailed` | Task FAILED → EventFailed emitted |
| `TestWatchAgentContextCancel` | ctx cancelled → channel closed |
| `TestTerminateAgentHappyPath` | CancelTask called with correct GUID |
| `TestTerminateAgentAlreadyDone` | Terminal error ignored |
| `TestGUIDCacheHit` | Second SpawnAgent for same tenant skips org/space lookup |

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/cloudfoundry/go-cfclient/v3` | CF API v3 client |
