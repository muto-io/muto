# OpenAPI-Enriched CRD Validation Design Spec

**Date:** 2026-07-12
**Status:** Approved

---

## Summary

Add kubebuilder description markers to all CRD Go type fields, regenerate the CRD YAML so the embedded OpenAPI v3 schemas become self-documenting, and extract those schemas into a standalone `docs/api/openapi.yaml` for human reference. No new runtime components — this is purely documentation and validation enrichment.

---

## Current State

`make generate` already produces valid OpenAPI v3 schemas inside the CRD manifests (via controller-gen). Fields have `type` and `enum` constraints but no `description` fields. `kubectl explain agentjob.spec.agents` returns nothing useful.

---

## Changes

### 1. Kubebuilder description markers

Add doc comments to every exported field in:

- `platform/k8s/types/v1alpha1/agentjob_types.go` — `AgentJobSpec`, `TriggerSpec`, `AgentRoleSpec`, `JobBusSpec`, `AgentJobStatus`
- `platform/k8s/types/v1alpha1/tenant_types.go` — `TenantSpec`, `TenantBusSpec`, `TenantStatus`
- `platform/k8s/types/v1alpha1/agentfleet_types.go` — `AgentFleetSpec`, `AgentFleetStatus`

**Pattern:** Regular Go doc comments on struct fields. controller-gen picks these up and emits them as `description:` in the generated OpenAPI schema.

```go
type AgentRoleSpec struct {
    // Role is the functional name of this agent (e.g. "coordinator", "worker").
    Role string `json:"role"`
    // Image is the container image used when deploying on Kubernetes.
    Image string `json:"image,omitempty"`
    // Command is the task command used when deploying on Cloud Foundry.
    Command string `json:"command,omitempty"`
    // MaxReplicas caps the number of concurrent agent instances for this role.
    // +kubebuilder:validation:Minimum=1
    MaxReplicas int32 `json:"maxReplicas,omitempty"`
}
```

### 2. Regenerate CRD manifests

```bash
make generate
```

Produces enriched `deploy/crds/*.yaml` with `description:` fields on all properties.

### 3. Standalone OpenAPI document

`docs/api/openapi.yaml` — OpenAPI 3.0.3 document assembling the CRD schemas:

```yaml
openapi: 3.0.3
info:
  title: Muto Agent Scheduler API
  description: CRD schemas for the Muto operator (muto.io/v1alpha1)
  version: v1alpha1
  license:
    name: MIT
    url: https://github.com/muto-io/muto/blob/main/LICENSE
components:
  schemas:
    AgentJob:   # extracted from muto.io_agentjobs.yaml openAPIV3Schema
    Tenant:     # extracted from muto.io_tenants.yaml openAPIV3Schema
    AgentFleet: # extracted from muto.io_agentfleets.yaml openAPIV3Schema
```

This file is written by hand (not generated) and kept in sync with the CRDs. It is the human-readable API reference.

### 4. README update

Add an "API Reference" section pointing to `docs/api/openapi.yaml` and noting that `kubectl explain` works on any field.

---

## File Map

```
platform/k8s/types/v1alpha1/agentjob_types.go   # add field descriptions
platform/k8s/types/v1alpha1/tenant_types.go      # add field descriptions
platform/k8s/types/v1alpha1/agentfleet_types.go  # add field descriptions
deploy/crds/muto.io_agentjobs.yaml               # regenerated
deploy/crds/muto.io_tenants.yaml                 # regenerated
deploy/crds/muto.io_agentfleets.yaml             # regenerated
docs/api/openapi.yaml                            # new: standalone OpenAPI 3.0 doc
README.md                                        # add API Reference section
```

---

## Validation

After regeneration, verify with:
```bash
kubectl explain agentjob.spec.agents.role --recursive=false
# Expected: description field populated
```

And confirm the CRD YAML contains `description:` on leaf fields.
