CONTROLLER_GEN ?= go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.0
BINARY_DIR     := bin

.PHONY: generate build test-unit test-integration kind-up kind-down docker-build

generate:
	$(CONTROLLER_GEN) crd paths="./platform/k8s/types/..." output:crd:artifacts:config=deploy/crds
	$(CONTROLLER_GEN) object paths="./platform/k8s/types/..."

build:
	mkdir -p $(BINARY_DIR)
	go build -o $(BINARY_DIR)/muto-operator ./cmd/muto-operator
	go build -o $(BINARY_DIR)/muto-mcp ./cmd/muto-mcp

test-unit:
	go test ./... -short -count=1 -coverprofile=coverage.out

test-integration:
	go test ./test/integration/... -tags integration -v -timeout 10m

kind-up:
	kind create cluster --config deploy/kind/kind-config.yaml --name muto-dev
	kubectl apply -f deploy/crds/

kind-down:
	kind delete cluster --name muto-dev

docker-build:
	docker build -t muto-operator:dev -f Dockerfile.operator .
	docker build -t muto-mcp:dev -f Dockerfile.mcp .
