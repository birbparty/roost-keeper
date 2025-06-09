# Image URL to use all building/pushing image targets
IMG ?= roost-keeper:latest

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.28.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: security
security: gosec ## Run security scan
	$(GOSEC) ./...

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/manager cmd/manager/main.go

.PHONY: run
run: fmt vet ## Run a controller from your host.
	go run ./cmd/manager/main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

##@ Dependencies

.PHONY: deps
deps: ## Download and verify dependencies
	go mod download
	go mod verify

.PHONY: deps-update
deps-update: ## Update all dependencies
	go get -u ./...
	go mod tidy

.PHONY: deps-clean
deps-clean: ## Clean module cache
	go clean -modcache

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

##@ Observability & Local Development

.PHONY: dev-stack
dev-stack: ## Start local observability stack (SigNoz/OTEL)
	@echo "Starting local observability stack..."
	@if [ -f ../local-otel/docker-compose.signoz-latest.yaml ]; then \
		cd ../local-otel && docker compose -f docker-compose.signoz-latest.yaml up -d; \
	else \
		echo "Local OTEL stack not found at ../local-otel"; \
	fi

.PHONY: dev-stack-down
dev-stack-down: ## Stop local observability stack
	@echo "Stopping local observability stack..."
	@if [ -f ../local-otel/docker-compose.signoz-latest.yaml ]; then \
		cd ../local-otel && docker compose -f docker-compose.signoz-latest.yaml down; \
	else \
		echo "Local OTEL stack not found at ../local-otel"; \
	fi

.PHONY: trace-debug
trace-debug: ## Debug distributed tracing (requires running operator)
	@echo "Checking trace endpoints..."
	@curl -s http://localhost:4318/v1/traces || echo "OTLP HTTP endpoint not available"
	@curl -s http://localhost:3301 || echo "SigNoz UI not available"

.PHONY: metrics-debug
metrics-debug: ## Debug metrics collection
	@echo "Checking metrics endpoints..."
	@curl -s http://localhost:8080/metrics || echo "Operator metrics not available"

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
CONTROLLER_GEN = $(LOCALBIN)/controller-gen
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
GOSEC = $(LOCALBIN)/gosec

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.18.0
GOLANGCI_LINT_VERSION ?= v1.55.2
GOSEC_VERSION ?= v2.19.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	test -s $(LOCALBIN)/golangci-lint && $(LOCALBIN)/golangci-lint version | grep -q $(GOLANGCI_LINT_VERSION) || \
	GOBIN=$(LOCALBIN) go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: gosec
gosec: $(GOSEC) ## Download gosec locally if necessary.
$(GOSEC): $(LOCALBIN)
	test -s $(LOCALBIN)/gosec && $(LOCALBIN)/gosec -version | grep -q $(GOSEC_VERSION) || \
	GOBIN=$(LOCALBIN) go install github.com/securecodewarrior/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

##@ Testing

.PHONY: test-unit
test-unit: ## Run unit tests
	go test -v ./internal/... ./controllers/... -coverprofile=coverage-unit.out

.PHONY: test-integration
test-integration: ## Run integration tests
	@echo "Running integration tests..."
	go test -v ./test/integration/... -coverprofile=coverage-integration.out

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests (future)
	@echo "E2E tests - to be implemented in integration testing phase"

.PHONY: test-coverage
test-coverage: test ## Generate test coverage report
	go tool cover -html=cover.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

##@ Project Setup

.PHONY: clean
clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf coverage*.out coverage.html
	rm -rf $(LOCALBIN)

.PHONY: setup-dev
setup-dev: deps golangci-lint gosec ## Set up development environment
	@echo "Development environment setup complete!"
	@echo "Available commands:"
	@echo "  make run          - Run the operator locally"
	@echo "  make test         - Run tests"
	@echo "  make dev-stack    - Start observability stack"
	@echo "  make help         - Show all available targets"

##@ Verification

.PHONY: verify-deps
verify-deps: ## Verify dependency integrity
	go mod verify
	go list -m all

.PHONY: verify-build
verify-build: ## Verify project builds successfully
	go build ./...

.PHONY: verify-imports
verify-imports: ## Verify no unused imports
	@echo "Checking for unused imports..."
	@if command -v goimports &> /dev/null; then \
		diff -u <(echo -n) <(goimports -d .); \
	else \
		echo "goimports not found, install with: go install golang.org/x/tools/cmd/goimports@latest"; \
	fi

.PHONY: verify-all
verify-all: verify-deps verify-build fmt vet lint security test ## Run all verification checks

##@ CRD Management

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: validate-crd
validate-crd: manifests ## Validate CRD against Kubernetes schema
	@echo "Validating CRD manifests..."
	@if command -v kubectl &> /dev/null; then \
		kubectl --dry-run=client apply -f config/crd/bases/ >/dev/null 2>&1 && echo "✅ CRD validation passed" || echo "❌ CRD validation failed"; \
	else \
		echo "kubectl not found, skipping CRD validation"; \
	fi

.PHONY: install-crd
install-crd: manifests ## Install CRDs into the cluster
	kubectl apply -f config/crd/bases/

.PHONY: uninstall-crd
uninstall-crd: ## Uninstall CRDs from the cluster
	kubectl delete -f config/crd/bases/ --ignore-not-found=true

.PHONY: apply-samples
apply-samples: ## Apply sample ManagedRoost resources
	kubectl apply -f config/samples/simple_managedroost.yaml
	kubectl apply -f config/samples/complex_managedroost.yaml

.PHONY: delete-samples
delete-samples: ## Delete sample ManagedRoost resources
	kubectl delete -f config/samples/simple_managedroost.yaml --ignore-not-found=true
	kubectl delete -f config/samples/complex_managedroost.yaml --ignore-not-found=true

##@ Documentation

.PHONY: docs
docs: ## Generate API documentation
	@echo "Generating API documentation..."
	@mkdir -p docs/api
	@echo "# Roost-Keeper API Documentation" > docs/api/README.md
	@echo "" >> docs/api/README.md
	@echo "This directory contains generated API documentation." >> docs/api/README.md

.PHONY: api-docs
api-docs: controller-gen ## Generate API reference documentation
	@echo "Generating API reference documentation..."
	@mkdir -p docs/api
	$(CONTROLLER_GEN) crd:generateEmbeddedObjectMeta=true paths="./api/..." output:stdout > docs/api/managedroost-crd.yaml
	@echo "API reference generated: docs/api/managedroost-crd.yaml"

##@ Helm Development

.PHONY: helm-lint
helm-lint: ## Validate Helm chart configurations (future)
	@echo "Helm lint functionality - to be implemented in Helm SDK integration phase"

##@ Enterprise Features

.PHONY: check-security
check-security: gosec ## Run comprehensive security checks
	$(GOSEC) -fmt json -out security-report.json ./...
	@echo "Security report generated: security-report.json"

.PHONY: check-dependencies
check-dependencies: ## Check for dependency vulnerabilities
	@echo "Checking dependencies for vulnerabilities..."
	@if command -v govulncheck &> /dev/null; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not found, install with: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
	fi

##@ Release

.PHONY: pre-release
pre-release: verify-all ## Run pre-release checks
	@echo "Pre-release verification complete!"

.PHONY: version
version: ## Show version information
	@echo "Roost-Keeper Dependency Information:"
	@echo "Go version: $(shell go version)"
	@echo "Project: $(shell go list -m)"
	@echo "Dependencies:"
	@go list -m all | head -20

##@ Development Workflow

.PHONY: quick-test
quick-test: ## Quick test for development
	go test -short ./...

.PHONY: watch
watch: ## Watch for changes and run tests (requires entr)
	@if command -v entr &> /dev/null; then \
		find . -name "*.go" | entr -r make quick-test; \
	else \
		echo "entr not found, install with your package manager"; \
	fi

.PHONY: dev-deps
dev-deps: ## Install development dependencies
	@echo "Installing development dependencies..."
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Development dependencies installed!"

##@ CI/CD Support

.PHONY: ci-test
ci-test: ## CI test target
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

.PHONY: ci-lint
ci-lint: golangci-lint ## CI lint target
	$(GOLANGCI_LINT) run --timeout=5m

.PHONY: ci-security
ci-security: gosec ## CI security scan target
	$(GOSEC) -fmt junit-xml -out security-report.xml ./...

.PHONY: ci-all
ci-all: ci-test ci-lint ci-security ## Run all CI checks
