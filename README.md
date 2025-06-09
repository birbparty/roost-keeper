# Roost-Keeper

[![Go Version](https://img.shields.io/badge/go-1.24+-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Domain](https://img.shields.io/badge/domain-roost.birb.party-purple.svg)](https://roost.birb.party)

A Kubernetes operator for intelligent Helm lifecycle management with comprehensive health checking and automated teardown policies.

## Overview

Roost-Keeper manages ephemeral Helm deployments that automatically tear down based on configurable health check failures. Perfect for development environments, testing, and cost-conscious cloud deployments.

### Key Features

🚀 **Intelligent Lifecycle Management**
- Automated Helm chart deployment and teardown
- Health-based lifecycle decisions
- Configurable teardown policies (manual, health-failure, TTL)

🔍 **Multi-Protocol Health Checks**
- HTTP/HTTPS with custom headers and paths
- TCP/UDP socket connectivity
- gRPC health checking protocol
- Prometheus metrics evaluation
- Kubernetes resource condition checking
- Composite logic (AND/OR combinations)

📊 **Enterprise Observability**
- OpenTelemetry integration with distributed tracing
- Structured logging with correlation IDs
- Prometheus metrics export
- SigNoz stack integration

🔒 **Security & Compliance**
- RBAC with least-privilege principles
- ConfigMap/Secret integration for values
- Admission webhook validation
- TLS certificate management

## Quick Start

### Prerequisites

- Go 1.24+
- Docker (for local development)
- kubectl with cluster access
- Helm 3.x

### Development Setup

```bash
# Clone the repository
git clone https://github.com/birbparty/roost-keeper.git
cd roost-keeper

# Set up development environment
make setup-dev

# Start local observability stack (optional)
make dev-stack

# Run tests
make test

# Run operator locally
make run
```

### Basic Usage

```yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: nginx-demo
  namespace: default
spec:
  helmChart:
    repository: "https://charts.bitnami.com/bitnami"
    chart: "nginx"
    version: "13.2.23"
    values:
      service:
        type: LoadBalancer
  healthChecks:
    enabled: true
    http:
      path: "/"
      port: 80
      interval: 30s
      timeout: 10s
  teardownPolicy:
    triggerCondition: HealthFailure
    gracePeriod: 5m
```

## Architecture

### Components

- **ManagedRoost CRD**: Defines ephemeral Helm deployments
- **Controller**: Reconciliation loop with health checking
- **Health Framework**: Multi-protocol health verification
- **Observability**: OTEL integration with SigNoz
- **Helm SDK**: Direct chart management

### Directory Structure

```
roost-keeper/
├── api/v1alpha1/           # CRD definitions
├── cmd/manager/            # Operator entry point
├── controllers/            # Reconciliation logic
├── internal/
│   ├── telemetry/         # Observability
│   ├── helm/              # Helm operations
│   ├── health/            # Health checks
│   └── webhook/           # Admission control
├── observability/         # OTEL configs
├── config/                # K8s manifests
└── docs/                  # Documentation
```

## Development

### Available Commands

```bash
# Development
make run                    # Run operator locally
make test                   # Run tests
make build                  # Build binary
make manifests generate     # Generate CRDs and code

# Observability
make dev-stack             # Start SigNoz stack
make trace-debug           # Debug tracing
make metrics-debug         # Debug metrics

# Quality
make lint                  # Run linters
make test-coverage         # Generate coverage

# Infrastructure
make infra-request         # Request birb-home infrastructure
```

### Observability Endpoints

- **SigNoz UI**: http://localhost:3301
- **Operator Metrics**: http://localhost:8080/metrics
- **OTLP HTTP**: http://localhost:4318/v1/traces
- **OTLP gRPC**: http://localhost:4317

## Current Status

### ✅ Completed (Phase 1: Project Initialization)

- [x] Go module with proper naming (`github.com/birbparty/roost-keeper`)
- [x] Standard operator directory structure
- [x] Comprehensive ManagedRoost CRD with all health check types
- [x] Basic controller with observability integration
- [x] OpenTelemetry SDK setup with SigNoz integration
- [x] Production-ready Makefile with all essential targets
- [x] Dockerfile for containerized deployment
- [x] AI agent knowledge base (`proompts/docs/`)
- [x] Infrastructure request generator for birb-home team

### 🔄 Next Steps (Phase 2: Dependency Management)

- [ ] Add Kubernetes client libraries to go.mod
- [ ] Install Helm SDK dependencies
- [ ] Set up testing framework with testify
- [ ] Configure controller-runtime dependencies
- [ ] Add OpenTelemetry dependencies

### 🚧 Future Phases

- **Phase 3**: Observability foundation
- **Phase 4**: CRD definition expansion
- **Phase 5**: Controller runtime enhancement
- **Phase 6**: ManagedRoost controller implementation
- **Phases 7-30**: Health checks, webhooks, CLI, production features

## Integration

### Birb Infrastructure

The project is designed for birbparty infrastructure:

- **Domain**: roost.birb.party
- **Container Registry**: birbparty registry
- **Observability**: Production SigNoz stack
- **Secrets**: Birb secret management

Use `make infra-request` to generate infrastructure requirements for the birb-home team.

### Local Development

Integrates with existing local-otel stack at `../local-otel/`:

```bash
# Use your existing SigNoz stack
make dev-stack

# Or integrate with running stack
export SIGNOZ_ENDPOINT=http://localhost:4317
make run
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make changes following the development workflow
4. Run tests (`make test`)
5. Commit changes (`git commit -m 'feat: add amazing feature'`)
6. Push to branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

### Code Standards

- Follow Go best practices and conventions
- Write comprehensive tests for new features
- Update documentation for API changes
- Use conventional commit messages
- Ensure `make lint` passes

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Support

- **Documentation**: [Architecture](proompts/docs/architecture.md) | [Development](proompts/docs/development-workflow.md)
- **Issues**: GitHub Issues
- **Discussions**: GitHub Discussions

---

**Built with ❤️ by the birbparty team**
