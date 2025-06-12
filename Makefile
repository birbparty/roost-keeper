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

##@ Release Management

.PHONY: release-check
release-check: ## Check if ready for release
	@echo "Checking release readiness..."
	@git status --porcelain | grep -q . && echo "❌ Working directory not clean" && exit 1 || echo "✅ Working directory clean"
	@make test > /dev/null 2>&1 && echo "✅ Tests passing" || (echo "❌ Tests failing" && exit 1)
	@make lint > /dev/null 2>&1 && echo "✅ Lint passing" || (echo "❌ Lint failing" && exit 1)
	@make security > /dev/null 2>&1 && echo "✅ Security scan passing" || (echo "❌ Security scan failing" && exit 1)
	@echo "🚀 Ready for release!"

.PHONY: release-prepare
release-prepare: ## Prepare for release (update versions, run checks)
	@read -p "Enter version (e.g., v1.0.0): " VERSION; \
	if [[ ! "$$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$$ ]]; then \
		echo "❌ Invalid version format. Use v1.2.3 or v1.2.3-alpha.1"; \
		exit 1; \
	fi; \
	echo "Preparing release $$VERSION..."; \
	make release-check; \
	echo "✅ Release $$VERSION is ready to be tagged"

.PHONY: release-notes
release-notes: ## Generate release notes for current changes
	@echo "# Release Notes"
	@echo ""
	@echo "## Changes since last release"
	@LAST_TAG=$$(git describe --tags --abbrev=0 2>/dev/null || echo "HEAD~10"); \
	echo "Comparing against: $$LAST_TAG"; \
	echo ""; \
	git log $$LAST_TAG..HEAD --pretty=format:"- %s" --reverse

.PHONY: tag-release
tag-release: ## Create and push release tag
	@read -p "Enter version to tag (e.g., v1.0.0): " VERSION; \
	if [[ ! "$$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$$ ]]; then \
		echo "❌ Invalid version format. Use v1.2.3 or v1.2.3-alpha.1"; \
		exit 1; \
	fi; \
	echo "Creating and pushing tag $$VERSION..."; \
	git tag -a "$$VERSION" -m "Release $$VERSION"; \
	git push origin "$$VERSION"; \
	echo "✅ Tag $$VERSION created and pushed"

##@ Container Development

.PHONY: docker-build-dev
docker-build-dev: ## Build development container image
	$(CONTAINER_TOOL) build -t ${IMG}-dev \
		--build-arg VERSION=dev \
		--build-arg COMMIT=$$(git rev-parse HEAD) \
		--build-arg BUILD_DATE=$$(date -u +%Y-%m-%dT%H:%M:%SZ) \
		.

.PHONY: docker-run-dev
docker-run-dev: docker-build-dev ## Run development container
	$(CONTAINER_TOOL) run --rm -it \
		-v $$(pwd)/config:/workspace/config \
		-v ~/.kube:/home/nonroot/.kube:ro \
		${IMG}-dev

.PHONY: docker-scan
docker-scan: ## Scan container image for vulnerabilities
	@if command -v trivy >/dev/null 2>&1; then \
		trivy image ${IMG}; \
	else \
		echo "Trivy not installed. Install with: curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin"; \
	fi

.PHONY: docker-sbom
docker-sbom: ## Generate SBOM for container image
	@if command -v syft >/dev/null 2>&1; then \
		syft ${IMG} -o spdx-json > container-sbom.json; \
		echo "SBOM generated: container-sbom.json"; \
	else \
		echo "Syft not installed. Install with: curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin"; \
	fi

##@ GitHub Integration

.PHONY: gh-check
gh-check: ## Check GitHub CLI and repository status
	@if ! command -v gh >/dev/null 2>&1; then \
		echo "❌ GitHub CLI not installed. Install from: https://github.com/cli/cli#installation"; \
		exit 1; \
	fi
	@echo "✅ GitHub CLI available"
	@gh auth status
	@echo "Repository: $$(gh repo view --json nameWithOwner -q .nameWithOwner)"

.PHONY: gh-release-create
gh-release-create: ## Create GitHub release (requires gh CLI)
	@make gh-check
	@read -p "Enter version (e.g., v1.0.0): " VERSION; \
	read -p "Enter title (default: Release $$VERSION): " TITLE; \
	TITLE=$${TITLE:-"Release $$VERSION"}; \
	read -p "Is this a pre-release? (y/N): " PRERELEASE; \
	PRERELEASE_FLAG=""; \
	if [[ "$$PRERELEASE" =~ ^[Yy]$$ ]]; then PRERELEASE_FLAG="--prerelease"; fi; \
	echo "Creating GitHub release..."; \
	gh release create "$$VERSION" \
		--title "$$TITLE" \
		--generate-notes \
		$$PRERELEASE_FLAG

.PHONY: gh-workflow-status
gh-workflow-status: ## Check GitHub Actions workflow status
	@make gh-check
	@echo "Recent workflow runs:"
	@gh run list --limit 10

.PHONY: gh-workflow-logs
gh-workflow-logs: ## View logs for latest workflow run
	@make gh-check
	@gh run view --log

##@ Development Productivity

.PHONY: dev-reset
dev-reset: ## Reset development environment
	@echo "Resetting development environment..."
	make clean
	make deps
	make manifests generate
	@echo "✅ Development environment reset"

.PHONY: dev-update
dev-update: ## Update development dependencies and tools
	@echo "Updating development environment..."
	make deps-update
	make dev-deps
	@echo "✅ Development environment updated"

.PHONY: quick-start
quick-start: ## Quick start for new developers
	@echo "🚀 Setting up Roost-Keeper development environment..."
	@echo ""
	@echo "1. Installing dependencies..."
	make setup-dev
	@echo ""
	@echo "2. Running tests..."
	make test
	@echo ""
	@echo "3. Building project..."
	make build
	@echo ""
	@echo "✅ Quick start complete!"
	@echo ""
	@echo "Next steps:"
	@echo "  - make run          # Run the operator locally"
	@echo "  - make dev-stack    # Start observability stack"
	@echo "  - make help         # Show all available commands"

.PHONY: benchmark
benchmark: ## Run performance benchmarks
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./internal/performance/...
	go test -bench=. -benchmem ./internal/health/...

.PHONY: profile
profile: ## Generate CPU and memory profiles
	@echo "Generating profiles..."
	@mkdir -p profiles/
	go test -cpuprofile=profiles/cpu.prof -memprofile=profiles/mem.prof -bench=. ./internal/performance/...
	@echo "Profiles generated in profiles/ directory"
	@echo "View with: go tool pprof profiles/cpu.prof"

.PHONY: mockgen
mockgen: ## Generate mocks for testing
	@echo "Generating mocks..."
	@if command -v mockgen >/dev/null 2>&1; then \
		find ./internal -name "*.go" -exec grep -l "//go:generate.*mockgen" {} \; | xargs -I {} go generate {}; \
		echo "✅ Mocks generated"; \
	else \
		echo "Installing mockgen..."; \
		go install github.com/golang/mock/mockgen@latest; \
		find ./internal -name "*.go" -exec grep -l "//go:generate.*mockgen" {} \; | xargs -I {} go generate {}; \
		echo "✅ Mocks generated"; \
	fi
