CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.3
REGISTRY       ?= ghcr.io/muto-io
VERSION        ?= $(shell git describe --tags --always --dirty)
BINARY_DIR     := bin
SHELL          := /bin/bash

.PHONY: generate build test-unit test-integration test-integration-k8s test-integration-cf test-e2e test-profile kind-up kind-down docker-build docker-push

generate:
	$(CONTROLLER_GEN) crd paths="./platform/k8s/types/..." output:crd:artifacts:config=deploy/crds
	$(CONTROLLER_GEN) object paths="./platform/k8s/types/..."

build:
	mkdir -p $(BINARY_DIR)
	go build -o $(BINARY_DIR)/muto-operator ./cmd/muto-operator
	go build -o $(BINARY_DIR)/muto-mcp ./cmd/muto-mcp

test-unit:
	go test ./... -short -count=1 -coverprofile=coverage.out

test-integration-k8s:
	mkdir -p test-results/k8s
	go run github.com/onsi/ginkgo/v2/ginkgo -p -tags=integration -v -timeout=20m -json-report=report.json -output-dir=$(CURDIR)/test-results/k8s ./test/integration/k8s | tee test-results/k8s/results.log; exit $${PIPESTATUS[0]}

test-integration-cf:
	mkdir -p test-results/cf
	go run github.com/onsi/ginkgo/v2/ginkgo -p -tags=integration -v -timeout=10m -json-report=report.json -output-dir=$(CURDIR)/test-results/cf ./test/integration/cf | tee test-results/cf/results.log; exit $${PIPESTATUS[0]}

test-integration:
	go test ./test/integration/... -tags integration -v -timeout 20m

test-e2e: test-integration-k8s test-integration-cf

# Profile the integration suites; see docs/testing/test-profiling.md.
# Example: make test-profile PROFILE_ARGS="--suite k8s --top 10"
test-profile:
	scripts/test-profile.sh $(PROFILE_ARGS)

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
