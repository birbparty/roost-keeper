# Roost-Keeper Examples

This directory contains practical examples of ManagedRoost configurations for different use cases and complexity levels. Each example includes detailed comments and usage instructions.

## Quick Start

If you're new to Roost-Keeper, start with the [basic examples](basic/) and then explore more advanced scenarios.

### Prerequisites

- Roost-Keeper installed in your cluster ([Installation Guide](../user-guide/installation.md))
- kubectl configured for your cluster
- Basic understanding of Kubernetes and Helm

## Example Categories

### 📚 [Basic Examples](basic/)

Simple, single-purpose examples perfect for learning Roost-Keeper fundamentals.

| Example | Description | Health Checks | Teardown | Use Case |
|---------|-------------|---------------|----------|----------|
| [nginx.yaml](basic/nginx.yaml) | Basic NGINX deployment | HTTP | Timeout | Web applications, learning |
| [redis.yaml](basic/redis.yaml) | Redis cache with TCP health | TCP + Kubernetes | Timeout + Health | Databases, caching |
| [postgres.yaml](basic/postgres.yaml) | PostgreSQL database | TCP + Custom | Timeout + Manual | Stateful applications |

### 🚀 [Advanced Examples](advanced/)

Complex configurations showcasing multiple features and real-world scenarios.

| Example | Description | Features | Use Case |
|---------|-------------|----------|----------|
| [microservice-stack.yaml](advanced/microservice-stack.yaml) | Multi-service application | Multi-protocol health, Prometheus | Microservices architecture |
| [canary-deployment.yaml](advanced/canary-deployment.yaml) | Production canary testing | Metrics-based health, manual approval | Production deployments |
| [multi-region-app.yaml](advanced/multi-region-app.yaml) | Multi-region deployment | Service discovery, advanced teardown | High availability |

### 🏢 [Enterprise Examples](enterprise/)

Production-ready configurations with security, multi-tenancy, and compliance features.

| Example | Description | Features | Use Case |
|---------|-------------|----------|----------|
| [tenant-isolated-app.yaml](enterprise/tenant-isolated-app.yaml) | Multi-tenant application | RBAC, network policies, quotas | Enterprise multi-tenancy |
| [compliance-app.yaml](enterprise/compliance-app.yaml) | Compliance-focused deployment | Audit logging, data preservation | Regulated industries |
| [secure-workload.yaml](enterprise/secure-workload.yaml) | Security-hardened deployment | Pod security standards, encryption | Security-critical applications |

### 🔗 [Integration Examples](integrations/)

Examples showing integration with external systems and cloud services.

| Example | Description | Integration | Use Case |
|---------|-------------|-------------|----------|
| [prometheus-monitoring.yaml](integrations/prometheus-monitoring.yaml) | Prometheus metrics integration | Prometheus, Grafana | Observability stack |
| [external-secrets.yaml](integrations/external-secrets.yaml) | External secrets management | AWS Secrets Manager, Vault | Cloud-native secrets |
| [gitops-workflow.yaml](integrations/gitops-workflow.yaml) | GitOps deployment pattern | ArgoCD, Git webhooks | CI/CD integration |

## Usage Patterns by Environment

### Development Environment

Perfect for rapid iteration and testing:

```bash
# Quick development deployment
kubectl apply -f basic/nginx.yaml

# Monitor deployment
kubectl get managedroost basic-nginx -w

# Test application
kubectl port-forward svc/basic-nginx 8080:80
curl http://localhost:8080
```

**Characteristics:**
- Short teardown timeouts (1-4 hours)
- Simple health checks
- Reduced resource requests
- Disabled persistence for faster cleanup

### Staging Environment

Pre-production testing with production-like configuration:

```bash
# Staging deployment with comprehensive monitoring
kubectl apply -f advanced/microservice-stack.yaml

# Monitor health across all services
kubectl get managedroost -l environment=staging

# Check health status
roost status microservice-stack
```

**Characteristics:**
- Comprehensive health checks
- Manual teardown approval
- Resource limits similar to production
- Full observability enabled

### Production Environment

Production-ready with security and compliance:

```bash
# Secure production deployment
kubectl apply -f enterprise/tenant-isolated-app.yaml

# Monitor with RBAC restrictions
kubectl get managedroost -n production-tenant

# Trigger controlled teardown
roost teardown production-app --require-approval
```

**Characteristics:**
- Multi-layered security
- RBAC and network policies
- Data preservation enabled
- Comprehensive audit logging

## Common Configuration Patterns

### Health Check Strategies

#### Single Service Health

```yaml
healthChecks:
  - name: http-health
    type: http
    http:
      url: "http://my-service.default.svc.cluster.local/health"
    interval: 30s
```

#### Multi-Protocol Health

```yaml
healthChecks:
  - name: http-api
    type: http
    http:
      url: "http://my-service.default.svc.cluster.local/api/health"
    weight: 3
  - name: tcp-db
    type: tcp
    tcp:
      host: my-service-db.default.svc.cluster.local
      port: 5432
    weight: 2
  - name: kubernetes-pods
    type: kubernetes
    kubernetes:
      resources:
        - apiVersion: v1
          kind: Pod
          labelSelector:
            app: my-service
    weight: 1
```

#### Metrics-Based Health

```yaml
healthChecks:
  - name: error-rate
    type: prometheus
    prometheus:
      query: 'rate(http_requests_total{status=~"5.."}[5m])'
      threshold: "0.01"
      operator: lt
    weight: 2
  - name: response-time
    type: prometheus
    prometheus:
      query: 'histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))'
      threshold: "0.5"
      operator: lt
    weight: 1
```

### Teardown Strategies

#### Development: Aggressive Cleanup

```yaml
teardownPolicy:
  triggers:
    - type: timeout
      timeout: 2h
    - type: failure_count
      failureCount: 3
  cleanup:
    gracePeriod: 30s
    force: true
```

#### Staging: Controlled Cleanup

```yaml
teardownPolicy:
  triggers:
    - type: timeout
      timeout: 24h
    - type: schedule
      schedule: "0 2 * * *"  # Daily cleanup
  requireManualApproval: true
  cleanup:
    gracePeriod: 5m
    force: false
```

#### Production: Protected Cleanup

```yaml
teardownPolicy:
  triggers:
    - type: failure_count
      failureCount: 10
    - type: resource_threshold
      resourceThreshold:
        cpu: "5m"
        memory: "10Mi"
  requireManualApproval: true
  dataPreservation:
    enabled: true
    preserveResources: ["PersistentVolumeClaim", "Secret"]
```

## Best Practices

### Chart Configuration

1. **Use Specific Versions**: Always pin chart versions for reproducibility
2. **Resource Limits**: Set appropriate resource requests and limits
3. **Security Context**: Configure security contexts for production workloads
4. **Persistence**: Disable persistence for ephemeral workloads

### Health Check Design

1. **Layer Health Checks**: Use multiple types (HTTP, TCP, Kubernetes)
2. **Set Realistic Timeouts**: Balance responsiveness with false positives
3. **Weight Critical Checks**: Higher weights for business-critical health checks
4. **Use Prometheus for SLIs**: Monitor business metrics and SLOs

### Teardown Policy Design

1. **Environment-Appropriate Policies**: Different policies for dev/staging/prod
2. **Multiple Triggers**: Combine timeout, health, and resource triggers
3. **Data Protection**: Enable preservation for stateful applications
4. **Manual Gates**: Require approval for production workloads

### Security Considerations

1. **Least Privilege**: Minimal required permissions
2. **Network Policies**: Restrict network access where possible
3. **Secret Management**: Use external secret management systems
4. **Pod Security**: Enforce pod security standards

## Testing Your Configurations

### Validation Steps

```bash
# 1. Dry-run validation
kubectl apply --dry-run=server -f your-example.yaml

# 2. Apply and monitor
kubectl apply -f your-example.yaml
kubectl get managedroost your-roost -w

# 3. Health check validation
roost health check your-roost
kubectl describe managedroost your-roost

# 4. Test application functionality
kubectl port-forward svc/your-service 8080:80
curl http://localhost:8080/health

# 5. Teardown testing (non-production)
roost teardown your-roost --dry-run
```

### Common Issues and Solutions

#### Health Checks Failing

```bash
# Debug connectivity
kubectl run debug-pod --image=curlimages/curl -i --tty --rm -- sh

# Check service endpoints
kubectl get endpoints your-service

# Verify DNS resolution
nslookup your-service.namespace.svc.cluster.local
```

#### Teardown Not Triggered

```bash
# Check teardown status
kubectl get managedroost your-roost -o jsonpath='{.status.teardown}'

# Review controller logs
kubectl logs -n roost-keeper-system deployment/roost-keeper-controller
```

#### Permission Issues

```bash
# Check RBAC
kubectl auth can-i create managedroosts
kubectl describe rolebinding roost-keeper

# Verify service account
kubectl get serviceaccount roost-keeper -o yaml
```

## Contributing Examples

We welcome contributions of new examples! Please:

1. Follow the existing structure and naming conventions
2. Include comprehensive comments and usage instructions
3. Test examples in multiple environments
4. Update this README with your new example
5. Submit a pull request with detailed description

### Example Template

```yaml
# Title: Brief description
# Perfect for: Use cases, target audience
# Features: Key features demonstrated

apiVersion: roost.birb.party/v1alpha1
kind: ManagedRoost
metadata:
  name: example-name
  namespace: default
  labels:
    example: category
spec:
  # ... configuration

---
# Usage instructions
# Testing steps
# Cleanup commands
```

## Support and Community

- 📚 [Documentation](../README.md)
- 💬 [GitHub Discussions](https://github.com/birbparty/roost-keeper/discussions)
- 🐛 [Report Issues](https://github.com/birbparty/roost-keeper/issues)
- 📧 [Community Mailing List](mailto:community@roost-keeper.io)

Happy deploying with Roost-Keeper! 🚀
