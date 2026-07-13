# Deploying Muto on Cloud Foundry

The Muto operator runs as a long-lived CF app using the binary buildpack.
It has no HTTP route (`no-route: true`) and uses process health checks.

## Prerequisites

- CF CLI installed and logged in (`cf login`)
- A CF space with the binary buildpack available
- CF API credentials for the CF adapter (the operator manages CF tasks for agent workloads)

## Steps

### 1. Build the Linux binary

```bash
GOOS=linux GOARCH=amd64 make build
```

### 2. Push to CF

```bash
cf push -f deploy/cf/manifest.yml -p bin/
```

This pushes the `muto-operator` binary to your current CF space.

### 3. Set sensitive environment variables

Never commit API credentials to `manifest.yml`. Set them after push:

```bash
cf set-env muto-operator CF_API_URL https://api.cf.example.com
cf set-env muto-operator CF_USERNAME admin
cf set-env muto-operator CF_PASSWORD your-password
cf restage muto-operator
```

### 4. Verify the operator is running

```bash
cf app muto-operator
cf logs muto-operator --recent
```

## Configuration Reference

| Env var | Description | Default |
|---|---|---|
| `MUTO_PLATFORM` | Platform adapter (`cf` or `k8s`) | `cf` |
| `CF_API_URL` | CF API endpoint | required |
| `CF_USERNAME` | CF username | required |
| `CF_PASSWORD` | CF password | required |
| `CF_ISOLATION_TIER` | `dedicated` (org per tenant) or `shared` (space per tenant) | `dedicated` |
| `CF_SHARED_ORG` | Org name when `CF_ISOLATION_TIER=shared` | — |

## Updating

```bash
GOOS=linux GOARCH=amd64 make build
cf push -f deploy/cf/manifest.yml -p bin/
```
