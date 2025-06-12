# Roost-Keeper Helm Chart

A Helm chart for deploying Roost-Keeper, an advanced Kubernetes operator for managing Helm deployments with comprehensive health monitoring, observability, and lifecycle management.

## Overview

Roost-Keeper simplifies the deployment and management of Helm charts in Kubernetes environments by providing:

- **Advanced Health Monitoring**: HTTP, TCP, gRPC, Prometheus, and Kubernetes-native health checks
- **Comprehensive Observability**: Metrics, logging, and distributed tracing with OpenTelemetry
- **Multi-Environment Support**: Development, staging, and production configurations
- **High Availability**: Pod disruption budgets, topology spread constraints, and leader election
- **Security Hardening**: Pod security standards, network policies, and RBAC
- **Automated Lifecycle Management**: Intelligent teardown policies and resource management

## Prerequisites

- Kubernetes 1.20+
- Helm 3.8+
- cert-manager (optional, for webhook certificates)
- Prometheus Operator (optional, for monitoring)

## Installation

### Quick Start

```bash
# Add the Helm repository (when available)
helm repo add roost-keeper https://charts.roost-keeper.io
helm repo update

# Install with default configuration
helm install roost-keeper roost-keeper/roost-keeper
```

### From Source

```bash
# Clone the repository
git clone https://github.com/your-org/roost-keeper.git
cd roost-keeper

# Install the chart
helm install roost-keeper ./helm/roost-keeper
```

### Environment-Specific Installations

#### Development Environment
```bash
helm install roost-keeper ./helm/roost-keeper \
  -f ./helm/roost-keeper/values-development.yaml \
  --namespace roost-keeper-dev --create-namespace
```

#### Production Environment
```bash
helm install roost-keeper ./helm/roost-keeper \
  -f ./helm/roost-keeper/values-production.yaml \
  --namespace roost-keeper-system --create-namespace
```

## Configuration

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.replicas` | Number of controller replicas | `3` |
| `monitoring.enabled` | Enable monitoring | `true` |
| `webhook.enabled` | Enable admission webhooks | `true` |
| `highAvailability.enabled` | Enable HA features | `true` |
| `security.podSecurityStandards.enforce` | Pod security standard | `restricted` |

### Complete Values Reference

See [values.yaml](./values.yaml) for all available configuration options.

### Environment-Specific Values

- **Development**: [values-development.yaml](./values-development.yaml)
- **Production**: [values-production.yaml](./values-production.yaml)

## Multi-Environment Deployment

### Development
- Single replica
- Relaxed security policies
- Debug logging enabled
- Monitoring disabled
- Simplified webhook configuration

### Staging
- 2 replicas
- Moderate security
- Full monitoring enabled
- Testing features active

### Production
- 3+ replicas
- Strict security policies
- Complete observability stack
- Full HA configuration
- Performance optimizations

## Dependencies

### Optional Dependencies

The chart can install optional dependencies:

```yaml
# Enable cert-manager for webhook certificates
certManager:
  enabled: true

# Enable Prometheus monitoring
monitoring:
  prometheus:
    enabled: true
  grafana:
    enabled: true
```

### External Dependencies

When using external dependencies, ensure they are available:

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.0/cert-manager.yaml

# Install Prometheus Operator
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm install prometheus prometheus-community/kube-prometheus-stack
```

## Usage Examples

### Basic ManagedRoost Resource

```yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: example-app
  namespace: default
spec:
  chart:
    name: nginx
    repository:
      url: https://charts.bitnami.com/bitnami
    version: "13.2.23"
    values:
      inline: |
        replicaCount: 2
        service:
          type: LoadBalancer
  healthChecks:
  - name: nginx-http
    type: http
    url: "http://example-app-nginx/health"
    interval: 30s
    timeout: 5s
```

### Advanced Configuration

```yaml
apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: production-app
spec:
  chart:
    name: myapp
    repository:
      url: https://charts.example.com
      auth:
        secretRef:
          name: chart-repo-secret
    version: "2.1.0"
  healthChecks:
  - name: app-health
    type: http
    url: "http://myapp/health"
    expectedCodes: [200]
  - name: database-check
    type: tcp
    host: "postgres.database.svc.cluster.local"
    port: 5432
  - name: metrics-check
    type: prometheus
    endpoint: "http://prometheus:9090"
    query: "up{job='myapp'}"
    threshold: "1"
  observability:
    logging:
      level: info
    metrics:
      enabled: true
    tracing:
      enabled: true
      samplingRate: "0.1"
  teardownPolicy:
    requireManualApproval: true
    dataPreservation:
      enabled: true
      preserveResources:
      - persistentvolumeclaims
      - secrets
```

## Monitoring and Observability

### Metrics

The controller exposes metrics on port 8080:

```bash
# Port-forward to access metrics
kubectl port-forward -n roost-keeper-system deployment/roost-keeper-controller-manager 8080:8080

# View metrics
curl http://localhost:8080/metrics
```

### Health Checks

Health endpoints are available:

```bash
# Health check
curl http://localhost:8081/healthz

# Readiness check  
curl http://localhost:8081/readyz
```

### Logging

Configure logging levels:

```yaml
observability:
  logging:
    level: debug  # debug, info, warn, error
    format: json  # json, console
    development: false
```

### Distributed Tracing

Enable OpenTelemetry tracing:

```yaml
observability:
  tracing:
    enabled: true
    samplingRate: 0.1
    endpoint: "http://jaeger-collector:14268/api/traces"
```

## Security

### Pod Security Standards

The chart enforces restricted pod security standards by default:

```yaml
security:
  podSecurityStandards:
    enforce: restricted
    audit: restricted
    warn: restricted
```

### Network Policies

Network policies are enabled by default in production:

```yaml
security:
  networkPolicies:
    enabled: true
    denyAll: true
    allowRoostKeeperSystem: true
    allowMonitoring: true
```

### RBAC

The chart creates minimal RBAC permissions required for operation. Review the generated ClusterRole for details.

## High Availability

### Leader Election

Multiple replicas use leader election for active-passive HA:

```yaml
controller:
  replicas: 3
  leaderElection:
    enabled: true
    leaseDuration: "15s"
    renewDeadline: "10s"
```

### Pod Disruption Budget

Ensures minimum availability during updates:

```yaml
highAvailability:
  podDisruptionBudget:
    enabled: true
    minAvailable: 2
```

### Topology Spread Constraints

Distributes pods across availability zones:

```yaml
highAvailability:
  topologySpreadConstraints:
    enabled: true
    maxSkew: 1
    topologyKey: topology.kubernetes.io/zone
```

## Upgrades

### Rolling Updates

The chart supports rolling updates:

```bash
# Upgrade to new version
helm upgrade roost-keeper ./helm/roost-keeper \
  --set image.tag=v0.2.0
```

### Pre/Post Upgrade Hooks

Configure upgrade hooks:

```yaml
upgrade:
  preUpgrade:
    enabled: true
    backup: true
    validate: true
  postUpgrade:
    enabled: true
    verify: true
    test: true
```

## Testing

### Helm Tests

Run the included tests:

```bash
# Run tests
helm test roost-keeper

# View test logs
kubectl logs -l app.kubernetes.io/component=test
```

### Manual Testing

```bash
# Check controller status
kubectl get deployment -n roost-keeper-system

# View controller logs
kubectl logs -n roost-keeper-system -l app.kubernetes.io/component=controller

# Test CRD installation
kubectl get crd managedroosts.roost.birb.party

# Create a test ManagedRoost
kubectl apply -f examples/simple-managedroost.yaml
```

## Troubleshooting

### Common Issues

#### Controller Not Starting

```bash
# Check events
kubectl describe deployment -n roost-keeper-system roost-keeper-controller-manager

# Check logs
kubectl logs -n roost-keeper-system -l app.kubernetes.io/component=controller
```

#### Webhook Issues

```bash
# Check webhook configuration
kubectl get validatingwebhookconfiguration

# Check certificate status
kubectl get certificate -n roost-keeper-system

# Check webhook logs
kubectl logs -n roost-keeper-system -l app.kubernetes.io/component=controller | grep webhook
```

#### RBAC Issues

```bash
# Check service account
kubectl get serviceaccount -n roost-keeper-system

# Check cluster role binding
kubectl get clusterrolebinding | grep roost-keeper
```

### Debug Mode

Enable debug logging:

```yaml
observability:
  logging:
    level: debug
    development: true
debug:
  enabled: true
  verbose: true
```

## Uninstallation

```bash
# Uninstall the chart
helm uninstall roost-keeper

# Remove CRDs (if desired)
kubectl delete crd managedroosts.roost.birb.party

# Remove namespace
kubectl delete namespace roost-keeper-system
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test with different configurations
5. Submit a pull request

## Support

- **Documentation**: [https://docs.roost-keeper.io](https://docs.roost-keeper.io)
- **Issues**: [GitHub Issues](https://github.com/your-org/roost-keeper/issues)
- **Discussions**: [GitHub Discussions](https://github.com/your-org/roost-keeper/discussions)

## License

This chart is licensed under the Apache 2.0 License. See [LICENSE](../../LICENSE) for details.
