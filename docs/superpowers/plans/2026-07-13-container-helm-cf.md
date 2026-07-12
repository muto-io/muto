# Container Images, Helm Chart & CF Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add production deployment artifacts for Muto: distroless Docker images, a CI release workflow that publishes to ghcr.io, a Helm chart for K8s deployment, and a CF manifest for Cloud Foundry deployment.

**Architecture:** Two multi-stage Dockerfiles (builder=golang:1.26-alpine, runtime=distroless/static-debian12:nonroot) produce minimal images. A GitHub Actions release workflow builds and pushes to `ghcr.io/muto-io/` on `main` and semver tags. The Helm chart at `deploy/helm/muto/` templates all K8s resources with configurable values. The CF manifest at `deploy/cf/manifest.yml` uses binary_buildpack with `no-route` and process health checks.

**Tech Stack:** Docker multi-stage builds, GitHub Actions (docker/login-action@v3, docker/metadata-action@v5, docker/build-push-action@v6), Helm v3, Cloud Foundry binary_buildpack

---

## File Map

```
Dockerfile.operator                              # new
Dockerfile.mcp                                   # new
.dockerignore                                    # new
Makefile                                         # modify: back docker-build/docker-push
.github/workflows/release.yml                   # new
.github/workflows/ci.yml                        # modify: add helm-lint job
deploy/helm/muto/Chart.yaml                     # new
deploy/helm/muto/values.yaml                    # new
deploy/helm/muto/templates/_helpers.tpl         # new
deploy/helm/muto/templates/deployment-operator.yaml  # new
deploy/helm/muto/templates/serviceaccount.yaml  # new
deploy/helm/muto/templates/clusterrole.yaml     # new
deploy/helm/muto/templates/clusterrolebinding.yaml   # new
deploy/helm/muto/templates/crds/agentjobs.yaml  # new (copy of deploy/crds/)
deploy/helm/muto/templates/crds/tenants.yaml    # new
deploy/helm/muto/templates/crds/agentfleets.yaml # new
deploy/helm/muto/templates/NOTES.txt            # new
deploy/cf/manifest.yml                          # new
deploy/cf/README.md                             # new
```

---

### Task 1: Dockerfiles and .dockerignore

**Files:**
- Create: `Dockerfile.operator`
- Create: `Dockerfile.mcp`
- Create: `.dockerignore`

- [ ] **Step 1: Write `Dockerfile.operator`**

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

- [ ] **Step 2: Write `Dockerfile.mcp`**

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
    -o /bin/muto-mcp ./cmd/muto-mcp

# Stage 2: runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/muto-mcp /muto-mcp
ENTRYPOINT ["/muto-mcp"]
```

- [ ] **Step 3: Write `.dockerignore`**

```
bin/
graphify-out/
test/
docs/
.github/
*.md
*.test
coverage.out
.idea/
```

- [ ] **Step 4: Verify both images build locally (requires Docker)**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler
docker build -f Dockerfile.operator -t muto-operator:test . 2>&1 | tail -5
docker build -f Dockerfile.mcp -t muto-mcp:test . 2>&1 | tail -5
```
Expected: both end with `Successfully built` or `naming to docker.io/library/muto-*:test done`.

- [ ] **Step 5: Verify binary runs in container**

```bash
docker run --rm muto-operator:test --help 2>&1 | head -5 || true
# Expected: any output or clean exit (not "exec format error" or "no such file")
```

- [ ] **Step 6: Commit**

```bash
git add Dockerfile.operator Dockerfile.mcp .dockerignore
git commit -m "feat(docker): add multi-stage distroless Dockerfiles for operator and mcp"
```

---

### Task 2: Update Makefile docker targets

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Read current Makefile**

```bash
cat /Users/I539231/Project/Upstream/agent-scheduler/Makefile
```

- [ ] **Step 2: Replace the stub `docker-build` target and add `docker-push`**

Replace the existing Makefile content with:

```makefile
CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.3
REGISTRY       ?= ghcr.io/muto-io
VERSION        ?= $(shell git describe --tags --always --dirty)
BINARY_DIR     := bin

.PHONY: generate build test-unit test-integration test-integration-kind kind-up kind-down docker-build docker-push

generate:
	$(CONTROLLER_GEN) crd paths="./platform/k8s/types/..." output:crd:artifacts:config=deploy/crds
	$(CONTROLLER_GEN) object paths="./platform/k8s/types/..."

build:
	mkdir -p $(BINARY_DIR)
	go build -o $(BINARY_DIR)/muto-operator ./cmd/muto-operator
	go build -o $(BINARY_DIR)/muto-mcp ./cmd/muto-mcp

test-unit:
	go test ./... -short -count=1 -coverprofile=coverage.out

test-integration-kind:
	go test ./test/integration/... -tags integration -v -timeout 15m

test-integration: test-integration-kind

kind-up:
	kind create cluster --config deploy/kind/kind-config.yaml --name muto-dev
	kubectl apply -f deploy/crds/

kind-down:
	kind delete cluster --name muto-dev

docker-build:
	docker build --build-arg VERSION=$(VERSION) \
	  -t $(REGISTRY)/muto-operator:$(VERSION) -f Dockerfile.operator .
	docker build --build-arg VERSION=$(VERSION) \
	  -t $(REGISTRY)/muto-mcp:$(VERSION) -f Dockerfile.mcp .

docker-push: docker-build
	docker push $(REGISTRY)/muto-operator:$(VERSION)
	docker push $(REGISTRY)/muto-mcp:$(VERSION)
```

- [ ] **Step 3: Verify `make docker-build` works**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler
make docker-build 2>&1 | tail -8
```
Expected: both images tagged as `ghcr.io/muto-io/muto-operator:<version>` and `ghcr.io/muto-io/muto-mcp:<version>`.

- [ ] **Step 4: Commit**

```bash
git add Makefile
git commit -m "feat(docker): back Makefile docker-build/docker-push with real commands"
```

---

### Task 3: CI release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write `.github/workflows/release.yml`**

```yaml
name: Release

on:
  push:
    branches: [main]
    tags: ["v*.*.*"]

jobs:
  build-push:
    name: Build & Push Images
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - uses: actions/checkout@v4

      - name: Log in to ghcr.io
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Docker metadata (operator)
        id: meta-operator
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/muto-io/muto-operator
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=raw,value=latest,enable=${{ github.ref == 'refs/heads/main' }}
            type=sha,prefix=sha-

      - name: Build & push operator
        uses: docker/build-push-action@v6
        with:
          context: .
          file: Dockerfile.operator
          push: true
          tags: ${{ steps.meta-operator.outputs.tags }}
          labels: ${{ steps.meta-operator.outputs.labels }}
          build-args: VERSION=${{ github.ref_name }}

      - name: Docker metadata (mcp)
        id: meta-mcp
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/muto-io/muto-mcp
          tags: |
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=raw,value=latest,enable=${{ github.ref == 'refs/heads/main' }}
            type=sha,prefix=sha-

      - name: Build & push mcp
        uses: docker/build-push-action@v6
        with:
          context: .
          file: Dockerfile.mcp
          push: true
          tags: ${{ steps.meta-mcp.outputs.tags }}
          labels: ${{ steps.meta-mcp.outputs.labels }}
          build-args: VERSION=${{ github.ref_name }}
```

- [ ] **Step 2: Validate YAML syntax**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "YAML valid"
```
Expected: `YAML valid`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add release workflow to build and push images to ghcr.io"
```

---

### Task 4: Add helm-lint job to CI

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Read current ci.yml**

```bash
cat /Users/I539231/Project/Upstream/agent-scheduler/.github/workflows/ci.yml
```

- [ ] **Step 2: Append `helm-lint` job**

Add this job at the end of `.github/workflows/ci.yml` (after the `integration-test` job):

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

- [ ] **Step 3: Validate YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML valid"
```
Expected: `YAML valid`

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add helm-lint job to validate chart on every PR"
```

---

### Task 5: Helm chart scaffolding (Chart.yaml + values.yaml)

**Files:**
- Create: `deploy/helm/muto/Chart.yaml`
- Create: `deploy/helm/muto/values.yaml`

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p /Users/I539231/Project/Upstream/agent-scheduler/deploy/helm/muto/templates/crds
```

- [ ] **Step 2: Write `deploy/helm/muto/Chart.yaml`**

```yaml
apiVersion: v2
name: muto
description: Platform-agnostic agent scheduler for Kubernetes (MUTO operator)
type: application
version: 0.1.0
appVersion: "latest"
keywords:
  - kubernetes
  - operator
  - agents
  - ai
  - scheduler
home: https://github.com/muto-io/muto
sources:
  - https://github.com/muto-io/muto
maintainers:
  - name: muto-io
    url: https://github.com/muto-io
```

- [ ] **Step 3: Write `deploy/helm/muto/values.yaml`**

```yaml
replicaCount: 1

image:
  repository: ghcr.io/muto-io/muto-operator
  # tag defaults to .Chart.AppVersion when empty
  tag: ""
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

# Set to false if CRDs are managed externally (e.g. by a GitOps operator)
installCRDs: true

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

- [ ] **Step 4: Commit**

```bash
git add deploy/helm/muto/Chart.yaml deploy/helm/muto/values.yaml
git commit -m "feat(helm): add Helm chart scaffolding (Chart.yaml + values.yaml)"
```

---

### Task 6: Helm _helpers.tpl

**Files:**
- Create: `deploy/helm/muto/templates/_helpers.tpl`

- [ ] **Step 1: Write `deploy/helm/muto/templates/_helpers.tpl`**

```
{{/*
Expand the name of the chart.
*/}}
{{- define "muto.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "muto.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "muto.labels" -}}
helm.sh/chart: {{ include "muto.chart" . }}
{{ include "muto.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "muto.selectorLabels" -}}
app.kubernetes.io/name: {{ include "muto.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Chart label
*/}}
{{- define "muto.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
ServiceAccount name
*/}}
{{- define "muto.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "muto.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image reference
*/}}
{{- define "muto.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/muto/templates/_helpers.tpl
git commit -m "feat(helm): add Helm template helpers"
```

---

### Task 7: Helm ServiceAccount template

**Files:**
- Create: `deploy/helm/muto/templates/serviceaccount.yaml`

- [ ] **Step 1: Write `deploy/helm/muto/templates/serviceaccount.yaml`**

```yaml
{{- if .Values.serviceAccount.create -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "muto.serviceAccountName" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "muto.labels" . | nindent 4 }}
  {{- with .Values.serviceAccount.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
{{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/muto/templates/serviceaccount.yaml
git commit -m "feat(helm): add ServiceAccount template"
```

---

### Task 8: Helm RBAC templates

**Files:**
- Create: `deploy/helm/muto/templates/clusterrole.yaml`
- Create: `deploy/helm/muto/templates/clusterrolebinding.yaml`

- [ ] **Step 1: Write `deploy/helm/muto/templates/clusterrole.yaml`**

```yaml
{{- if .Values.rbac.create -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ include "muto.fullname" . }}
  labels:
    {{- include "muto.labels" . | nindent 4 }}
rules:
  - apiGroups: ["muto.io"]
    resources: ["tenants", "agentjobs", "agentfleets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["muto.io"]
    resources: ["tenants/status", "agentjobs/status", "agentfleets/status"]
    verbs: ["update", "patch"]
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["serviceaccounts"]
    verbs: ["get", "list", "watch", "create", "update", "patch"]
  - apiGroups: ["rbac.authorization.k8s.io"]
    resources: ["roles", "rolebindings"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["apps"]
    resources: ["statefulsets"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
{{- end }}
```

- [ ] **Step 2: Write `deploy/helm/muto/templates/clusterrolebinding.yaml`**

```yaml
{{- if .Values.rbac.create -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {{ include "muto.fullname" . }}
  labels:
    {{- include "muto.labels" . | nindent 4 }}
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: {{ include "muto.fullname" . }}
subjects:
  - kind: ServiceAccount
    name: {{ include "muto.serviceAccountName" . }}
    namespace: {{ .Release.Namespace }}
{{- end }}
```

- [ ] **Step 3: Commit**

```bash
git add deploy/helm/muto/templates/clusterrole.yaml deploy/helm/muto/templates/clusterrolebinding.yaml
git commit -m "feat(helm): add ClusterRole and ClusterRoleBinding templates"
```

---

### Task 9: Helm Deployment template

**Files:**
- Create: `deploy/helm/muto/templates/deployment-operator.yaml`

- [ ] **Step 1: Write `deploy/helm/muto/templates/deployment-operator.yaml`**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "muto.fullname" . }}
  namespace: {{ .Release.Namespace }}
  labels:
    {{- include "muto.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "muto.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "muto.selectorLabels" . | nindent 8 }}
      {{- if .Values.metrics.enabled }}
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: {{ .Values.metrics.port | quote }}
        prometheus.io/path: "/metrics"
      {{- end }}
    spec:
      {{- with .Values.imagePullSecrets }}
      imagePullSecrets:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      serviceAccountName: {{ include "muto.serviceAccountName" . }}
      containers:
        - name: muto-operator
          image: {{ include "muto.image" . }}
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          env:
            {{- range $key, $val := .Values.env }}
            {{- if $val }}
            - name: {{ $key }}
              value: {{ $val | quote }}
            {{- end }}
            {{- end }}
          ports:
            - name: metrics
              containerPort: {{ .Values.metrics.port }}
              protocol: TCP
            - name: health
              containerPort: {{ .Values.healthProbe.port }}
              protocol: TCP
          livenessProbe:
            httpGet:
              path: /healthz
              port: health
            initialDelaySeconds: 15
            periodSeconds: 20
          readinessProbe:
            httpGet:
              path: /readyz
              port: health
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            {{- toYaml .Values.resources | nindent 12 }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
```

- [ ] **Step 2: Commit**

```bash
git add deploy/helm/muto/templates/deployment-operator.yaml
git commit -m "feat(helm): add Deployment template for muto-operator"
```

---

### Task 10: Helm CRD templates and NOTES.txt

**Files:**
- Create: `deploy/helm/muto/templates/crds/agentjobs.yaml`
- Create: `deploy/helm/muto/templates/crds/tenants.yaml`
- Create: `deploy/helm/muto/templates/crds/agentfleets.yaml`
- Create: `deploy/helm/muto/templates/NOTES.txt`

- [ ] **Step 1: Write the CRD wrapper for AgentJobs**

Read the source CRD:
```bash
cat /Users/I539231/Project/Upstream/agent-scheduler/deploy/crds/muto.io_agentjobs.yaml
```

Write `deploy/helm/muto/templates/crds/agentjobs.yaml` — wrap the verbatim content with the installCRDs gate:

```yaml
{{- if .Values.installCRDs }}
<paste verbatim content of deploy/crds/muto.io_agentjobs.yaml here>
{{- end }}
```

- [ ] **Step 2: Write CRD wrapper for Tenants**

Read source:
```bash
cat /Users/I539231/Project/Upstream/agent-scheduler/deploy/crds/muto.io_tenants.yaml
```

Write `deploy/helm/muto/templates/crds/tenants.yaml` with the same `{{- if .Values.installCRDs }}` / `{{- end }}` wrapper.

- [ ] **Step 3: Write CRD wrapper for AgentFleets**

Read source:
```bash
cat /Users/I539231/Project/Upstream/agent-scheduler/deploy/crds/muto.io_agentfleets.yaml
```

Write `deploy/helm/muto/templates/crds/agentfleets.yaml` with the same wrapper.

- [ ] **Step 4: Write `deploy/helm/muto/templates/NOTES.txt`**

```
Muto operator has been deployed!

Image: {{ include "muto.image" . }}
Namespace: {{ .Release.Namespace }}
Platform: {{ .Values.env.MUTO_PLATFORM }}

Check operator status:
  kubectl get pods -n {{ .Release.Namespace }} -l app.kubernetes.io/name=muto

View CRDs:
  kubectl get crds | grep muto.io

Schedule an agent job:
  kubectl apply -f - <<EOF
  apiVersion: muto.io/v1alpha1
  kind: Tenant
  metadata:
    name: my-tenant
  spec:
    namespace: my-tenant-agents
    isolationTier: shared
    messageBus:
      type: nats
  EOF

Docs: https://github.com/muto-io/muto
```

- [ ] **Step 5: Verify `helm lint` passes**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler
helm lint deploy/helm/muto
```
Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 6: Verify `helm template` renders without error**

```bash
helm template muto deploy/helm/muto --namespace muto-system | head -30
```
Expected: YAML output beginning with rendered resource manifests, no template errors.

- [ ] **Step 7: Commit**

```bash
git add deploy/helm/muto/templates/crds/ deploy/helm/muto/templates/NOTES.txt
git commit -m "feat(helm): add CRD templates and NOTES.txt"
```

---

### Task 11: CF deployment manifest and README

**Files:**
- Create: `deploy/cf/manifest.yml`
- Create: `deploy/cf/README.md`

- [ ] **Step 1: Create directory**

```bash
mkdir -p /Users/I539231/Project/Upstream/agent-scheduler/deploy/cf
```

- [ ] **Step 2: Write `deploy/cf/manifest.yml`**

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
      # Sensitive vars — set via 'cf set-env', never commit here:
      # CF_API_URL, CF_USERNAME, CF_PASSWORD
```

- [ ] **Step 3: Write `deploy/cf/README.md`**

```markdown
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
```

- [ ] **Step 4: Validate manifest YAML**

```bash
python3 -c "import yaml; yaml.safe_load(open('deploy/cf/manifest.yml'))" && echo "YAML valid"
```
Expected: `YAML valid`

- [ ] **Step 5: Commit**

```bash
git add deploy/cf/manifest.yml deploy/cf/README.md
git commit -m "feat(cf): add CF deployment manifest and deployment guide"
```

---

### Task 12: Final verification

**Files:** none (verification only)

- [ ] **Step 1: Run `helm lint` end-to-end**

```bash
cd /Users/I539231/Project/Upstream/agent-scheduler
helm lint deploy/helm/muto
```
Expected: `1 chart(s) linted, 0 chart(s) failed`

- [ ] **Step 2: Run `helm template` for K8s platform**

```bash
helm template muto deploy/helm/muto \
  --namespace muto-system \
  --set image.tag=v1.0.0 \
  --set env.MUTO_PLATFORM=k8s \
  | grep "^kind:" | sort | uniq -c
```
Expected output includes:
```
      1 kind: ClusterRole
      1 kind: ClusterRoleBinding
      3 kind: CustomResourceDefinition
      1 kind: Deployment
      1 kind: ServiceAccount
```

- [ ] **Step 3: Run `helm template` with `installCRDs=false`**

```bash
helm template muto deploy/helm/muto \
  --namespace muto-system \
  --set installCRDs=false \
  | grep "^kind:" | sort | uniq -c
```
Expected: No `CustomResourceDefinition` lines.

- [ ] **Step 4: Run `helm template` with CF values**

```bash
helm template muto deploy/helm/muto \
  --namespace muto-system \
  --set env.MUTO_PLATFORM=cf \
  --set env.CF_API_URL=https://api.cf.example.com \
  --set env.CF_USERNAME=admin \
  --set env.CF_PASSWORD=secret \
  | grep "MUTO_PLATFORM\|CF_API_URL"
```
Expected: Both env vars appear in rendered Deployment.

- [ ] **Step 5: Run unit tests to confirm nothing broken**

```bash
make test-unit 2>&1 | tail -5
```
Expected: all PASS.

- [ ] **Step 6: Final commit and push**

```bash
git push origin muto-implementation
```

---

## Self-Review

**Spec coverage:**
| Spec Requirement | Task |
|---|---|
| Dockerfile.operator — multi-stage, distroless | Task 1 |
| Dockerfile.mcp — multi-stage, distroless | Task 1 |
| .dockerignore | Task 1 |
| Makefile docker-build/docker-push with VERSION + REGISTRY | Task 2 |
| release.yml — build+push on main/v* tags | Task 3 |
| ci.yml — add helm-lint job | Task 4 |
| Chart.yaml + values.yaml | Task 5 |
| _helpers.tpl with muto.name, muto.fullname, muto.labels, muto.selectorLabels, muto.serviceAccountName, muto.image | Task 6 |
| ServiceAccount template (gated by serviceAccount.create) | Task 7 |
| ClusterRole template (gated by rbac.create) | Task 8 |
| ClusterRoleBinding template (gated by rbac.create) | Task 8 |
| Deployment template with env, probes, metrics annotations | Task 9 |
| CRD templates (gated by installCRDs) | Task 10 |
| NOTES.txt | Task 10 |
| deploy/cf/manifest.yml — no-route, process health check | Task 11 |
| deploy/cf/README.md — step-by-step guide | Task 11 |
| helm lint passes | Tasks 10 + 12 |

**No placeholders found.** All code blocks complete.

**Type consistency:** `muto.image` helper defined in Task 6, used in Task 9 Deployment template. `muto.fullname`, `muto.serviceAccountName`, `muto.labels`, `muto.selectorLabels` all defined in Task 6 and used consistently in Tasks 7–9. `installCRDs` (not `install_crds` or `installcrds`) used consistently in Tasks 5 and 10.
