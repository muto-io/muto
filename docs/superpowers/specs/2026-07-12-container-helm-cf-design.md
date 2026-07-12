# Container Images, Helm Chart & CF Deployment Design Spec

**Date:** 2026-07-12
**Status:** Approved

---

## Summary

Add production deployment artifacts for Muto:
1. **Dockerfiles** — multi-stage distroless images for `muto-operator` and `muto-mcp`
2. **CI release workflow** — build + push to `ghcr.io/muto-io/` on `main` and semver tags
3. **Helm chart** — `deploy/helm/muto/` for K8s operator deployment with configurable values
4. **CF manifest** — `deploy/cf/manifest.yml` for `cf push` of the operator

---

## File Map

```
Dockerfile.operator                              # multi-stage: golang:1.26-alpine → distroless/static
Dockerfile.mcp                                   # same structure, builds cmd/muto-mcp
.dockerignore                                    # exclude test/, docs/, bin/, graphify-out/
Makefile                                         # back docker-build/docker-push with real commands
.github/workflows/release.yml                   # build+push on main/v* tags
.github/workflows/ci.yml                        # add helm-lint job
deploy/helm/muto/Chart.yaml
deploy/helm/muto/values.yaml
deploy/helm/muto/templates/_helpers.tpl
deploy/helm/muto/templates/deployment-operator.yaml
deploy/helm/muto/templates/serviceaccount.yaml
deploy/helm/muto/templates/clusterrole.yaml
deploy/helm/muto/templates/clusterrolebinding.yaml
deploy/helm/muto/templates/crds/agentjobs.yaml  # verbatim copy of deploy/crds/
deploy/helm/muto/templates/crds/tenants.yaml
deploy/helm/muto/templates/crds/agentfleets.yaml
deploy/helm/muto/templates/NOTES.txt
deploy/cf/manifest.yml
deploy/cf/README.md
```

---

## 1. Dockerfiles

### Dockerfile.operator

```dockerfile
# Stage 1: build
FROM golang:1.26-alpine AS builder
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /bin/muto-operator ./cmd/muto-operator

# Stage 2: runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/muto-operator /muto-operator
ENTRYPOINT ["/muto-operator"]
```

### Dockerfile.mcp

Identical to Dockerfile.operator except:
- Builds `./cmd/muto-mcp`
- Output binary `/bin/muto-mcp` → `/muto-mcp`
- `ENTRYPOINT ["/muto-mcp"]`

### .dockerignore

```
bin/
graphify-out/
test/
docs/
.github/
*.md
*.test
coverage.out
```

---

## 2. Makefile Changes

Replace the stub `docker-build` and add `docker-push`:

```makefile
REGISTRY   ?= ghcr.io/muto-io
VERSION    ?= $(shell git describe --tags --always --dirty)

docker-build:
	docker build --build-arg VERSION=$(VERSION) \
	  -t $(REGISTRY)/muto-operator:$(VERSION) -f Dockerfile.operator .
	docker build --build-arg VERSION=$(VERSION) \
	  -t $(REGISTRY)/muto-mcp:$(VERSION) -f Dockerfile.mcp .

docker-push: docker-build
	docker push $(REGISTRY)/muto-operator:$(VERSION)
	docker push $(REGISTRY)/muto-mcp:$(VERSION)
```

---

## 3. CI Release Workflow

File: `.github/workflows/release.yml`

**Triggers:** push to `main` branch, push of `v*.*.*` tags.

**Permissions:** `packages: write` (for ghcr.io push via `GITHUB_TOKEN`).

**Tags produced:**
| Trigger | Tags |
|---|---|
| Push to `main` | `latest`, `sha-<short>` |
| Push of `v1.2.3` | `1.2.3`, `1.2`, `sha-<short>` |

**Jobs:**
1. `checkout` + `docker/login-action@v3` with `registry: ghcr.io`, `password: ${{ secrets.GITHUB_TOKEN }}`
2. `docker/metadata-action@v5` for each image (operator, mcp) — generates tags from the table above
3. `docker/build-push-action@v6` for each image — builds with `BUILD_ARG VERSION=${{ github.ref_name }}`, pushes all tags

---

## 4. Helm Chart

### Chart.yaml

```yaml
apiVersion: v2
name: muto
description: Platform-agnostic agent scheduler for Kubernetes (MUTO operator)
type: application
version: 0.1.0
appVersion: "latest"
keywords: [kubernetes, operator, agents, ai, scheduler]
home: https://github.com/muto-io/muto
sources: [https://github.com/muto-io/muto]
maintainers:
  - name: muto-io
    url: https://github.com/muto-io
```

### values.yaml

```yaml
replicaCount: 1

image:
  repository: ghcr.io/muto-io/muto-operator
  tag: ""              # defaults to .Chart.AppVersion
  pullPolicy: IfNotPresent

imagePullSecrets: []
nameOverride: ""
fullnameOverride: ""

serviceAccount:
  create: true
  name: ""
  annotations: {}

rbac:
  create: true

installCRDs: true      # set false if CRDs are managed externally

env:
  MUTO_PLATFORM: k8s
  MUTO_NAMESPACE: default
  # CF platform vars — only used when MUTO_PLATFORM=cf
  CF_API_URL: ""
  CF_USERNAME: ""
  CF_PASSWORD: ""
  CF_ISOLATION_TIER: dedicated
  CF_SHARED_ORG: ""

resources:
  limits:
    cpu: 500m
    memory: 128Mi
  requests:
    cpu: 100m
    memory: 64Mi

metrics:
  enabled: true
  port: 8080

healthProbe:
  port: 8081

nodeSelector: {}
tolerations: []
affinity: {}
```

### templates/_helpers.tpl

Standard Helm helpers: `muto.name`, `muto.fullname`, `muto.labels`, `muto.selectorLabels`, `muto.serviceAccountName`.

### templates/deployment-operator.yaml

- `kind: Deployment`, namespace from `{{ .Release.Namespace }}`
- Single container `muto-operator` with image `{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}`
- Env vars from `{{ .Values.env }}` as `name/value` pairs
- `livenessProbe` and `readinessProbe` hitting `{{ .Values.healthProbe.port }}/healthz`
- Prometheus annotation `prometheus.io/scrape: "true"` on pod when `metrics.enabled`
- `resources` from values
- `serviceAccountName` from helper

### templates/clusterrole.yaml / clusterrolebinding.yaml

Templated versions of the existing `deploy/rbac/role.yaml` and `rolebinding.yaml`, gated by `{{ if .Values.rbac.create }}`. Service account name from helper.

### templates/crds/

Three files — verbatim copies of `deploy/crds/muto.io_*.yaml`, each wrapped in:
```yaml
{{- if .Values.installCRDs }}
<crd content>
{{- end }}
```

### templates/NOTES.txt

Post-install instructions showing the image tag used, how to check operator status, and a link to the README.

### Install command

```bash
helm install muto deploy/helm/muto \
  --namespace muto-system --create-namespace \
  --set image.tag=v1.0.0
```

---

## 5. CF Deployment Manifest

### deploy/cf/manifest.yml

```yaml
---
applications:
  - name: muto-operator
    memory: 128M
    disk_quota: 256M
    instances: 1
    no-route: true
    health-check-type: process
    buildpacks:
      - binary_buildpack
    command: ./muto-operator
    env:
      MUTO_PLATFORM: cf
      CF_ISOLATION_TIER: dedicated
      CF_SHARED_ORG: ""
      # Sensitive vars — set via cf set-env, not in this file:
      # CF_API_URL, CF_USERNAME, CF_PASSWORD
```

`no-route: true` and `health-check-type: process` — the operator is a controller loop, not a web server.

### deploy/cf/README.md

Step-by-step deployment guide:
1. Build Linux binary: `GOOS=linux GOARCH=amd64 make build`
2. Push: `cf push -f deploy/cf/manifest.yml -p bin/`
3. Set secrets: `cf set-env muto-operator CF_API_URL ...` + `cf restage`

---

## 6. CI helm-lint Job

Added to `.github/workflows/ci.yml`:

```yaml
  helm-lint:
    name: Helm Lint
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - name: Lint chart
        run: helm lint deploy/helm/muto
```

---

## Version Flow

| Context | Image tag |
|---|---|
| Local `make docker-build` | `ghcr.io/muto-io/muto-operator:<git-describe>` |
| CI push to `main` | `latest`, `sha-<short>` |
| CI push of `v1.2.3` | `1.2.3`, `1.2`, `sha-<short>` |
| `helm install --set image.tag=1.2.3` | Uses explicit tag, overrides `appVersion` |
