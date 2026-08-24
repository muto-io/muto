CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.17.3
REGISTRY       ?= ghcr.io/muto-io
VERSION        ?= $(shell git describe --tags --always --dirty)
BINARY_DIR     := bin
SHELL          := /bin/bash

.PHONY: generate build test-unit test-integration test-integration-k8s test-integration-cf test-e2e kind-up kind-down docker-build docker-push

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
	go test ./test/integration/k8s/... -tags integration -v -timeout 20m -args -ginkgo.v | tee test-results/k8s/results.log; exit $${PIPESTATUS[0]}

test-integration-cf:
	mkdir -p test-results/cf
	go test ./test/integration/cf/... -tags integration -v -timeout 10m -args -ginkgo.v | tee test-results/cf/results.log; exit $${PIPESTATUS[0]}

test-integration:
	go test ./test/integration/... -tags integration -v -timeout 20m

test-e2e: test-integration-k8s test-integration-cf

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
