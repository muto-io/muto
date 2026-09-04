# Webhook API Reference

Specification for Kubernetes validation and mutation webhooks in Muto. Webhooks allow custom logic to validate or modify AgentJob resources before they are persisted or applied to the cluster.

## Overview

Muto supports two types of webhooks:

1. **Validation Webhooks**: Inspect incoming AgentJob requests and reject invalid configurations
2. **Mutation Webhooks**: Modify AgentJob specifications before persistence (e.g., inject defaults, add labels)

Webhooks are implemented as HTTP services that the Kubernetes API server calls for every AgentJob admission request.

**Key Concepts:**
- **Admission Controllers**: Kubernetes extension points for resource validation/mutation
- **Synchronous**: Webhooks are called in-line during resource creation/update
- **Failure Handling**: Validation failures block resource creation; mutation failures can be configured to fail-open or fail-closed
- **Security**: Webhooks communicate via secure connections (TLS)

---

## Webhook Types

### ValidatingWebhookConfiguration

Validates AgentJob resources. Used to enforce business logic and compliance requirements.

**Typical Validations:**
- Ensure tenantRef references an existing Tenant
- Validate image format and accessibility
- Enforce resource limits
- Check for required labels/annotations
- Verify agent role names match a whitelist

### MutatingWebhookConfiguration

Modifies AgentJob resources. Used to inject defaults and standardize configurations.

**Typical Mutations:**
- Add default labels (e.g., `app=muto`, `tenant=tenant-a`)
- Inject resource requests/limits
- Set default TTL values
- Append security context configurations
- Add standardized annotations

---

## Request Format

### Webhook Request

The Kubernetes API server sends an `AdmissionReview` JSON object to the webhook HTTP endpoint.

#### Request Structure

```json
{
  "apiVersion": "admission.k8s.io/v1",
  "kind": "AdmissionReview",
  "request": {
    "uid": "12345-abcde",
    "kind": {
      "group": "muto.io",
      "kind": "AgentJob",
      "version": "v1alpha1"
    },
    "resource": {
      "group": "muto.io",
      "resource": "agentjobs",
      "version": "v1alpha1"
    },
    "requestKind": {
      "group": "muto.io",
      "kind": "AgentJob",
      "version": "v1alpha1"
    },
    "requestResource": {
      "group": "muto.io",
      "resource": "agentjobs",
      "version": "v1alpha1"
    },
    "name": "data-pipeline-job",
    "namespace": "tenant-a",
    "operation": "CREATE",
    "userInfo": {
      "username": "system:serviceaccount:default:muto-user",
      "uid": "user-uid-123",
      "groups": ["system:serviceaccounts", "system:authenticated"]
    },
    "object": {
      "apiVersion": "muto.io/v1alpha1",
      "kind": "AgentJob",
      "metadata": {
        "name": "data-pipeline-job",
        "namespace": "tenant-a",
        "creationTimestamp": "2026-09-03T10:30:45Z"
      },
      "spec": {
        "tenantRef": "tenant-a",
        "trigger": {
          "type": "manual"
        },
        "agents": [
          {
            "role": "processor",
            "image": "gcr.io/myorg/processor:v1",
            "maxReplicas": 1
          }
        ]
      }
    },
    "oldObject": null,
    "dryRun": false,
    "options": {
      "apiVersion": "meta.k8s.io/v1",
      "kind": "CreateOptions"
    }
  }
}
```

### Request Fields

#### `request.uid` (string)
- Unique identifier for this admission request
- Used for logging and tracing
- Example: `"12345-abcde"`

#### `request.kind` (object)
- Type information about the resource being admitted
- Fields: `group`, `kind`, `version`

#### `request.operation` (string)
- What operation triggered the webhook
- **Enum**: `CREATE`, `UPDATE`, `DELETE`, `CONNECT`
- Webhooks typically handle `CREATE` and `UPDATE`

#### `request.userInfo` (object)
- Information about who initiated the request
- Fields: `username`, `uid`, `groups`

#### `request.object` (object)
- The resource being created/updated (the AgentJob manifest)
- This is what validation/mutation logic inspects

#### `request.oldObject` (object, nullable)
- For UPDATE operations: the old version of the resource
- For CREATE operations: null

#### `request.dryRun` (boolean)
- If true, this is a dry-run request; webhook should not perform side effects
- Still must return validation results

---

## Response Format

### Webhook Response

The webhook returns an `AdmissionReview` response with an `AdmissionResponse` object.

#### Success Response (Validation Pass)

```json
{
  "apiVersion": "admission.k8s.io/v1",
  "kind": "AdmissionReview",
  "response": {
    "uid": "12345-abcde",
    "allowed": true,
    "status": {
      "message": "All validations passed"
    }
  }
}
```

#### Rejection Response (Validation Fail)

```json
{
  "apiVersion": "admission.k8s.io/v1",
  "kind": "AdmissionReview",
  "response": {
    "uid": "12345-abcde",
    "allowed": false,
    "status": {
      "code": 400,
      "message": "Validation failed: image 'invalid-image' is not accessible"
    }
  }
}
```

#### Mutation Response

```json
{
  "apiVersion": "admission.k8s.io/v1",
  "kind": "AdmissionReview",
  "response": {
    "uid": "12345-abcde",
    "allowed": true,
    "patchType": "JSONPatch",
    "patch": "W3sib3AiOiJhZGQiLCJwYXRoIjoiL3NwZWMvdHRsQWZ0ZXJDb21wbGV0aW9uIiwidmFsdWUiOjM2MDB9XQ=="
  }
}
```

### Response Fields

#### `response.uid` (string)
- MUST match the `request.uid` from the request
- Used to correlate request/response

#### `response.allowed` (boolean)
- `true`: Allow the operation (validation passed or mutation applied)
- `false`: Reject the operation (validation failed)

#### `response.status` (object, optional)
- Used when `allowed: false`
- Fields:
  - `code` (integer): HTTP-like status code (e.g., 400, 403, 500)
  - `message` (string): Human-readable error message

#### `response.patchType` (string, mutation only)
- **Required for mutations**: Type of patch being returned
- **Value**: `"JSONPatch"` (RFC 6902 JSON Patch format)

#### `response.patch` (string, mutation only)
- Base64-encoded RFC 6902 JSON Patch
- Applied to the object before persistence
- Example: `"W3sib3AiOiJhZGQiLCJwYXRoIjoiL3NwZWMvdHRsQWZ0ZXJDb21wbGV0aW9uIiwidmFsdWUiOjM2MDB9XQ=="`

---

## Validation Webhook Examples

### Example 1: Validate Tenant Reference

```python
from flask import Flask, request, jsonify
import json
import base64

app = Flask(__name__)

def validate_tenant_exists(tenant_ref):
    """Check if tenant exists in the cluster"""
    # Query K8s API for Tenant resource
    # Return True if found, False otherwise
    # (Implementation depends on your K8s client library)
    return check_tenant_exists(tenant_ref)

@app.route('/validate-agentjob', methods=['POST'])
def validate_agentjob():
    admission_review = request.get_json()
    request_uid = admission_review['request']['uid']
    
    agentjob = admission_review['request']['object']
    tenant_ref = agentjob['spec'].get('tenantRef')
    
    # Validation 1: tenantRef is required
    if not tenant_ref:
        return jsonify({
            "apiVersion": "admission.k8s.io/v1",
            "kind": "AdmissionReview",
            "response": {
                "uid": request_uid,
                "allowed": False,
                "status": {
                    "code": 400,
                    "message": "spec.tenantRef is required"
                }
            }
        })
    
    # Validation 2: tenantRef must reference existing Tenant
    if not validate_tenant_exists(tenant_ref):
        return jsonify({
            "apiVersion": "admission.k8s.io/v1",
            "kind": "AdmissionReview",
            "response": {
                "uid": request_uid,
                "allowed": False,
                "status": {
                    "code": 404,
                    "message": f"Tenant '{tenant_ref}' not found"
                }
            }
        })
    
    # All validations passed
    return jsonify({
        "apiVersion": "admission.k8s.io/v1",
        "kind": "AdmissionReview",
        "response": {
            "uid": request_uid,
            "allowed": True,
            "status": {
                "message": "All validations passed"
            }
        }
    })
```

### Example 2: Validate Image Accessibility

```python
@app.route('/validate-agentjob-image', methods=['POST'])
def validate_agentjob_image():
    admission_review = request.get_json()
    request_uid = admission_review['request']['uid']
    
    agentjob = admission_review['request']['object']
    agents = agentjob['spec'].get('agents', [])
    
    # Validate each agent image
    for agent in agents:
        image = agent.get('image')
        
        if not image:
            continue  # Image may be optional depending on platform
        
        # Check image format
        if ':' not in image:
            return jsonify({
                "apiVersion": "admission.k8s.io/v1",
                "kind": "AdmissionReview",
                "response": {
                    "uid": request_uid,
                    "allowed": False,
                    "status": {
                        "code": 400,
                        "message": f"Image '{image}' must include tag (e.g., 'image:v1.0')"
                    }
                }
            })
        
        # Attempt to pull image metadata to verify accessibility
        if not can_pull_image(image):
            return jsonify({
                "apiVersion": "admission.k8s.io/v1",
                "kind": "AdmissionReview",
                "response": {
                    "uid": request_uid,
                    "allowed": False,
                    "status": {
                        "code": 403,
                        "message": f"Image '{image}' is not accessible (check registry credentials)"
                    }
                }
            })
    
    # All images validated
    return jsonify({
        "apiVersion": "admission.k8s.io/v1",
        "kind": "AdmissionReview",
        "response": {
            "uid": request_uid,
            "allowed": True
        }
    })
```

### Example 3: Enforce Resource Limits

```python
@app.route('/validate-resource-limits', methods=['POST'])
def validate_resource_limits():
    admission_review = request.get_json()
    request_uid = admission_review['request']['uid']
    
    agentjob = admission_review['request']['object']
    
    # Define limits
    MAX_AGENTS = 10
    MAX_REPLICAS_PER_AGENT = 5
    
    agents = agentjob['spec'].get('agents', [])
    
    # Check total agent count
    if len(agents) > MAX_AGENTS:
        return jsonify({
            "apiVersion": "admission.k8s.io/v1",
            "kind": "AdmissionReview",
            "response": {
                "uid": request_uid,
                "allowed": False,
                "status": {
                    "code": 400,
                    "message": f"Too many agents: {len(agents)} > {MAX_AGENTS}"
                }
            }
        })
    
    # Check replicas per agent
    for agent in agents:
        max_replicas = agent.get('maxReplicas', 1)
        if max_replicas > MAX_REPLICAS_PER_AGENT:
            role = agent.get('role', 'unknown')
            return jsonify({
                "apiVersion": "admission.k8s.io/v1",
                "kind": "AdmissionReview",
                "response": {
                    "uid": request_uid,
                    "allowed": False,
                    "status": {
                        "code": 400,
                        "message": f"Agent '{role}': maxReplicas {max_replicas} > {MAX_REPLICAS_PER_AGENT}"
                    }
                }
            })
    
    # All limits validated
    return jsonify({
        "apiVersion": "admission.k8s.io/v1",
        "kind": "AdmissionReview",
        "response": {
            "uid": request_uid,
            "allowed": True
        }
    })
```

---

## Mutation Webhook Examples

### Example 1: Inject Default TTL

Automatically add TTL if not specified.

```python
import json
import base64

@app.route('/mutate-default-ttl', methods=['POST'])
def mutate_default_ttl():
    admission_review = request.get_json()
    request_uid = admission_review['request']['uid']
    
    agentjob = admission_review['request']['object']
    
    # Check if ttlAfterCompletion is already set
    ttl = agentjob['spec'].get('ttlAfterCompletion')
    
    if ttl is None:
        # Create JSON Patch to add default TTL (1 hour)
        patch = [
            {
                "op": "add",
                "path": "/spec/ttlAfterCompletion",
                "value": 3600
            }
        ]
        
        # Encode patch as base64
        patch_json = json.dumps(patch)
        patch_b64 = base64.b64encode(patch_json.encode()).decode()
        
        return jsonify({
            "apiVersion": "admission.k8s.io/v1",
            "kind": "AdmissionReview",
            "response": {
                "uid": request_uid,
                "allowed": True,
                "patchType": "JSONPatch",
                "patch": patch_b64
            }
        })
    
    # Already has TTL, no mutation needed
    return jsonify({
        "apiVersion": "admission.k8s.io/v1",
        "kind": "AdmissionReview",
        "response": {
            "uid": request_uid,
            "allowed": True
        }
    })
```

### Example 2: Add Labels and Annotations

Add standardized labels and annotations for observability.

```python
@app.route('/mutate-add-labels', methods=['POST'])
def mutate_add_labels():
    admission_review = request.get_json()
    request_uid = admission_review['request']['uid']
    
    agentjob = admission_review['request']['object']
    tenant_ref = agentjob['spec'].get('tenantRef', 'unknown')
    
    # Create patches to add labels and annotations
    patches = []
    
    # Add labels
    labels_patch = {
        "op": "add",
        "path": "/metadata/labels",
        "value": {
            "app": "muto",
            "tenant": tenant_ref,
            "created-by": "muto-webhook"
        }
    }
    
    # Add annotations for tracing
    annotations_patch = {
        "op": "add",
        "path": "/metadata/annotations",
        "value": {
            "muto.io/created-at": "2026-09-03T10:30:45Z",
            "muto.io/version": "v1alpha1"
        }
    }
    
    patches.extend([labels_patch, annotations_patch])
    
    # Encode as base64
    patch_json = json.dumps(patches)
    patch_b64 = base64.b64encode(patch_json.encode()).decode()
    
    return jsonify({
        "apiVersion": "admission.k8s.io/v1",
        "kind": "AdmissionReview",
        "response": {
            "uid": request_uid,
            "allowed": True,
            "patchType": "JSONPatch",
            "patch": patch_b64
        }
    })
```

### Example 3: Enforce Image Tag Format

Enforce a convention like only allowing specific registry or tag format.

```python
@app.route('/mutate-image-tag', methods=['POST'])
def mutate_image_tag():
    admission_review = request.get_json()
    request_uid = admission_review['request']['uid']
    
    agentjob = admission_review['request']['object']
    agents = agentjob['spec'].get('agents', [])
    
    patches = []
    
    for i, agent in enumerate(agents):
        image = agent.get('image')
        
        if image and not image.startswith('gcr.io/myorg/'):
            # Enforce company registry
            if ':' in image:
                image_name, tag = image.rsplit(':', 1)
            else:
                image_name = image
                tag = 'latest'
            
            # Reconstruct with company registry
            new_image = f"gcr.io/myorg/{image_name}:{tag}"
            
            patches.append({
                "op": "replace",
                "path": f"/spec/agents/{i}/image",
                "value": new_image
            })
    
    if patches:
        patch_json = json.dumps(patches)
        patch_b64 = base64.b64encode(patch_json.encode()).decode()
        
        return jsonify({
            "apiVersion": "admission.k8s.io/v1",
            "kind": "AdmissionReview",
            "response": {
                "uid": request_uid,
                "allowed": True,
                "patchType": "JSONPatch",
                "patch": patch_b64
            }
        })
    
    # No mutations needed
    return jsonify({
        "apiVersion": "admission.k8s.io/v1",
        "kind": "AdmissionReview",
        "response": {
            "uid": request_uid,
            "allowed": True
        }
    })
```

---

## Webhook Configuration

### ValidatingWebhookConfiguration Example

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: muto-validating-webhooks
webhooks:
  - name: validate-agentjob.muto.io
    rules:
      - operations: ["CREATE", "UPDATE"]
        apiGroups: ["muto.io"]
        apiVersions: ["v1alpha1"]
        resources: ["agentjobs"]
    clientConfig:
      service:
        name: muto-webhook
        namespace: muto-system
        path: "/validate-agentjob"
      caBundle: <base64-encoded-ca-cert>
    admissionReviewVersions: ["v1"]
    sideEffects: None
    timeoutSeconds: 5
    failurePolicy: Fail
    namespaceSelector:
      matchLabels:
        muto-webhooks: enabled
```

### MutatingWebhookConfiguration Example

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: muto-mutating-webhooks
webhooks:
  - name: mutate-defaults.muto.io
    rules:
      - operations: ["CREATE"]
        apiGroups: ["muto.io"]
        apiVersions: ["v1alpha1"]
        resources: ["agentjobs"]
    clientConfig:
      service:
        name: muto-webhook
        namespace: muto-system
        path: "/mutate-default-ttl"
      caBundle: <base64-encoded-ca-cert>
    admissionReviewVersions: ["v1"]
    sideEffects: None
    timeoutSeconds: 5
    failurePolicy: Ignore
    reinvocationPolicy: IfNeeded
    namespaceSelector:
      matchLabels:
        muto-webhooks: enabled
```

---

## RFC 6902 JSON Patch Operations

Mutation webhooks use RFC 6902 JSON Patch. Common operations:

### `add` Operation

Add a new field or replace existing.

```json
{
  "op": "add",
  "path": "/spec/ttlAfterCompletion",
  "value": 3600
}
```

### `remove` Operation

Remove a field.

```json
{
  "op": "remove",
  "path": "/spec/ttlAfterCompletion"
}
```

### `replace` Operation

Replace field value.

```json
{
  "op": "replace",
  "path": "/spec/agents/0/image",
  "value": "gcr.io/myorg/processor:v1"
}
```

### `move` Operation

Move a field (remove and add elsewhere).

```json
{
  "op": "move",
  "from": "/spec/oldField",
  "path": "/spec/newField"
}
```

### `copy` Operation

Copy a field value.

```json
{
  "op": "copy",
  "from": "/spec/agents/0/maxReplicas",
  "path": "/spec/agents/1/maxReplicas"
}
```

### `test` Operation

Test a value (fails patch if doesn't match).

```json
{
  "op": "test",
  "path": "/spec/tenantRef",
  "value": "tenant-a"
}
```

---

## Webhook Best Practices

### 1. Fail Gracefully

```python
@app.errorhandler(500)
def internal_error(error):
    """Return valid webhook response even on error"""
    return jsonify({
        "apiVersion": "admission.k8s.io/v1",
        "kind": "AdmissionReview",
        "response": {
            "uid": "unknown",
            "allowed": False,
            "status": {
                "code": 500,
                "message": "Webhook error: check logs"
            }
        }
    }), 500
```

### 2. Set Appropriate Timeouts

Webhooks should complete quickly (< 5 seconds recommended):

```yaml
timeoutSeconds: 5  # In webhook config
```

### 3. Use Labels for Selective Webhooks

Only invoke webhooks for labeled namespaces:

```yaml
namespaceSelector:
  matchLabels:
    muto-webhooks: enabled  # Only namespaces with this label
```

### 4. Log All Decisions

```python
import logging

logging.info(f"Webhook {request_uid}: {allowed=} reason={message}")
```

### 5. Handle Dry-Run Requests

Don't perform side effects during dry-run:

```python
if admission_review['request'].get('dryRun'):
    # Skip external API calls
    return allow_response(request_uid)
```

### 6. Validate Your Patches

Test JSON Patch operations:

```bash
# Test patch offline
jq --slurpfile patch patch.json '.patch = $patch | .' input.json
```

---

## Troubleshooting Webhooks

### Webhook Not Being Called

1. Check webhook is registered:
   ```bash
   kubectl get validatingwebhookconfigurations
   kubectl get mutatingwebhookconfigurations
   ```

2. Check namespace label:
   ```bash
   kubectl get ns -L muto-webhooks
   kubectl label ns tenant-a muto-webhooks=enabled
   ```

3. Verify webhook service is running:
   ```bash
   kubectl get svc -n muto-system muto-webhook
   ```

### Webhook Rejecting Valid Requests

1. Check webhook logs:
   ```bash
   kubectl logs -n muto-system -l app=muto-webhook
   ```

2. Test webhook with dry-run:
   ```bash
   kubectl create --dry-run=server -f agentjob.yaml
   ```

3. Check admission event:
   ```bash
   kubectl describe agentjob job-name
   # Look for Events section
   ```

### Mutation Not Applied

1. Check patch syntax (must be valid RFC 6902)
2. Verify base64 encoding:
   ```bash
   echo "<patch_value>" | base64 -d | jq .
   ```

3. Check patch operations are correct:
   ```bash
   # Each operation must reference valid paths
   # Paths must be JSON pointers (per RFC 6901)
   ```

---

## Related Documentation

- **[CRD Types](./crd-types.md)** — AgentJob and Tenant field reference
- **[Architecture: Security Model](../architecture/security-model.md)** — Webhook security considerations
- **[Development: Contributing](../development/contributing.md)** — Writing custom webhooks

---

**Last Updated:** 2026-09-03  
**Webhook API Version**: admission.k8s.io/v1  
**JSON Patch Format**: RFC 6902
