# Webhook API Reference

There is no webhook delivery mechanism in Muto today - neither job-lifecycle webhooks (notifying an external HTTP endpoint when an AgentJob changes state) nor Kubernetes admission webhooks (validating/mutating AgentJob resources at apply time).

Confirmed by searching the full Go source (non-test and test) and the Helm chart (`deploy/helm/muto/`): there is no `ValidatingWebhookConfiguration`/`MutatingWebhookConfiguration` manifest, no admission webhook HTTP server, no `AdmissionReview` handling, and no outbound webhook client or `MUTO_WEBHOOK*` environment variable anywhere in the codebase. `AgentJobReconciler` validates and defaults fields in its own `Reconcile` method, not via admission webhooks.

## What would need to exist

For **admission webhooks** (validate/mutate AgentJob at the API server level):

- A webhook HTTP server registered with the controller-runtime manager (`mgr.GetWebhookServer()`), implementing `admission.Handler` or `webhook.CustomValidator`/`webhook.CustomDefaulter` for the `AgentJob` type.
- `ValidatingWebhookConfiguration`/`MutatingWebhookConfiguration` resources (and the TLS cert-management to go with them - see [Security Configuration](../configuration/tls-security.md)), templated into the Helm chart.

For **job-lifecycle webhooks** (notify an external endpoint on job state changes):

- A delivery mechanism inside `AgentJobReconciler` (or a separate component watching `AgentJob` status) that POSTs to configured URLs on phase transitions, with retry/backoff and a signing scheme for the payload.
- Configuration (env vars or a CRD field) for target URLs and which events to send.

Neither exists yet. If you need one of these today, you'd be building it from scratch against the reconcilers in `platform/k8s/reconcilers/`.

## See Also

- [CRD Types](./crd-types.md) - the actual AgentJob/Tenant field reference
- [Security Configuration](../configuration/tls-security.md) - what Muto does and doesn't do for TLS/RBAC today
