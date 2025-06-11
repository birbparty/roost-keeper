# Kubernetes Native Health Checks Implementation

This document describes the implementation of Kubernetes native health checks in Roost Keeper, providing comprehensive monitoring of Helm-deployed applications and their underlying Kubernetes resources.

## Overview

The Kubernetes health checker provides deep visibility into the health state of deployed applications by examining various Kubernetes resources including pods, deployments, services, dependencies, and even node health. It goes beyond basic readiness checks to provide comprehensive application health assessment.

## Features

### Core Resource Monitoring

- **Pod Health**: Monitors pod readiness, phases, restart counts, and container states
- **Deployment Health**: Checks deployment status, replica counts, and rollout progress
- **Service Health**: Validates service endpoints and connectivity
- **StatefulSet Health**: Monitors stateful applications and their replicas
- **DaemonSet Health**: Checks daemon sets across cluster nodes

### Advanced Capabilities

- **Dependency Monitoring**: Validates ConfigMaps, Secrets, and PersistentVolumeClaims
- **Node Health**: Assesses the health of nodes hosting application pods
- **Custom Resources**: Extensible monitoring for CRDs and custom Kubernetes resources
- **Intelligent Caching**: Performance optimization with configurable TTL
- **Flexible Thresholds**: Configurable ready ratios and restart limits

## Architecture

### Component Structure

```
internal/health/kubernetes/
├── checker.go      # Main health checker orchestrator
├── cache.go        # Intelligent caching system
├── pod.go          # Pod-specific health assessment
├── deployment.go   # Deployment/StatefulSet/DaemonSet monitoring
├── service.go      # Service and endpoint validation
├── resource.go     # Dependency and custom resource checking
└── node.go         # Node health assessment
```

### Health Check Flow

1. **Resource Discovery**: Automatically discovers resources using Helm labels
2. **Parallel Assessment**: Concurrent health checks across resource types
3. **Aggregation**: Combines individual component health into overall status
4. **Caching**: Stores results with configurable TTL for performance
5. **Reporting**: Provides detailed health information and recommendations

## Configuration

### Basic Configuration

```yaml
healthChecks:
- name: "kubernetes-health"
  type: "kubernetes"
  interval: "30s"
  timeout: "10s"
  kubernetes:
    checkPods: true
    checkDeployments: true
    checkServices: true
    requiredReadyRatio: 1.0  # All pods must be ready
```

### Advanced Configuration

```yaml
healthChecks:
- name: "comprehensive-k8s-health"
  type: "kubernetes"
  interval: "45s"
  timeout: "15s"
  kubernetes:
    # Core resource checks
    checkPods: true
    checkDeployments: true
    checkServices: true
    checkStatefulSets: true
    checkDaemonSets: false
    
    # Dependency and infrastructure checks
    checkDependencies: true
    checkNodeHealth: true
    
    # Custom label selector
    labelSelector:
      matchLabels:
        environment: "production"
        tier: "backend"
      matchExpressions:
      - key: "version"
        operator: "In"
        values: ["v2.1", "v2.1.1"]
    
    # Pod health configuration
    requiredReadyRatio: 0.8
    checkPodRestarts: true
    maxPodRestarts: 10
    
    # Performance tuning
    apiTimeout: "10s"
    enableCaching: true
    cacheTTL: "1m"
    
    # Custom resources monitoring
    customResources:
    - apiVersion: "networking.istio.io/v1beta1"
      kind: "VirtualService"
      healthStrategy: "exists"
    - apiVersion: "cert-manager.io/v1"
      kind: "Certificate"
      healthStrategy: "conditions"
      expectedConditions:
      - type: "Ready"
        status: "True"
```

## Configuration Options

### Core Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `checkPods` | bool | false | Enable pod health monitoring |
| `checkDeployments` | bool | false | Enable deployment health monitoring |
| `checkServices` | bool | false | Enable service health monitoring |
| `checkStatefulSets` | bool | false | Enable StatefulSet monitoring |
| `checkDaemonSets` | bool | false | Enable DaemonSet monitoring |
| `checkDependencies` | bool | false | Enable dependency resource monitoring |
| `checkNodeHealth` | bool | false | Enable node health assessment |

### Pod Configuration

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `requiredReadyRatio` | float64 | 1.0 | Minimum ratio of pods that must be ready (0.0-1.0) |
| `checkPodRestarts` | bool | false | Enable restart count monitoring |
| `maxPodRestarts` | int32 | 5 | Maximum allowed restart count per pod |

### Performance Settings

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `apiTimeout` | duration | "5s" | Timeout for Kubernetes API calls |
| `enableCaching` | bool | true | Enable result caching |
| `cacheTTL` | duration | "30s" | Cache time-to-live |

### Advanced Selectors

| Field | Type | Description |
|-------|------|-------------|
| `labelSelector` | LabelSelector | Additional label selector for resource discovery |
| `customResources` | []CustomResourceCheck | Custom resource monitoring configuration |

## Resource Discovery

The health checker automatically discovers resources using Helm's standard labels:

- `app.kubernetes.io/instance`: Roost name
- `app.kubernetes.io/name`: Chart name

Additional resources can be discovered using custom label selectors.

## Health Assessment Logic

### Pod Health

A pod is considered healthy when:
- Phase is `Running` or `Succeeded`
- All containers are ready
- Restart count is below threshold (if configured)
- No critical conditions are present

### Deployment Health

A deployment is considered healthy when:
- Ready replicas match desired replicas
- Updated replicas match desired replicas
- Available replicas match desired replicas
- No unavailable replicas exist
- Observed generation matches current generation

### Service Health

A service is considered healthy when:
- Service exists and is accessible
- Has at least one ready endpoint (except ExternalName services)
- Endpoint addresses are reachable

### Node Health

A node is considered healthy when:
- Node is in Ready state
- No pressure conditions (Memory, Disk, PID)
- Network is available
- Not cordoned (unschedulable)

## Custom Resource Monitoring

### Health Strategies

1. **Exists**: Resource must exist
2. **Conditions**: Check specific status conditions
3. **Status**: Validate specific status field values

### Example Configurations

```yaml
customResources:
# Simple existence check
- apiVersion: "networking.istio.io/v1beta1"
  kind: "VirtualService"
  healthStrategy: "exists"

# Condition-based check
- apiVersion: "cert-manager.io/v1"
  kind: "Certificate"
  healthStrategy: "conditions"
  expectedConditions:
  - type: "Ready"
    status: "True"

# Status field check
- apiVersion: "postgresql.cnpg.io/v1"
  kind: "Cluster"
  healthStrategy: "status"
  statusPath: "phase"
  expectedStatus: "Cluster in healthy state"
```

## Caching System

### Cache Benefits

- **Performance**: Reduces Kubernetes API load
- **Efficiency**: Avoids redundant checks
- **Scalability**: Supports high-frequency monitoring

### Cache Configuration

```yaml
kubernetes:
  enableCaching: true
  cacheTTL: "1m"  # Cache results for 1 minute
```

### Cache Invalidation

- Time-based expiration (TTL)
- Manual invalidation via cache management
- Background cleanup of expired entries

## Error Handling

### Graceful Degradation

- Individual component failures don't crash entire check
- Detailed error reporting for troubleshooting
- Fallback strategies for API timeouts

### Error Categories

1. **API Errors**: Kubernetes API connectivity issues
2. **Resource Errors**: Missing or inaccessible resources
3. **Configuration Errors**: Invalid check configuration
4. **Timeout Errors**: Operations exceeding configured timeouts

## Monitoring and Observability

### Metrics Integration

Health check results integrate with the telemetry system:

- Execution time tracking
- Success/failure rates
- Resource counts and status
- Cache hit rates

### Logging

Structured logging provides insights into:

- Health check execution flow
- Resource discovery results
- Individual component health status
- Performance metrics

## Best Practices

### Configuration Guidelines

1. **Start Simple**: Begin with basic pod and deployment checks
2. **Tune Thresholds**: Adjust ready ratios based on application characteristics
3. **Enable Caching**: Use caching for frequently checked applications
4. **Monitor Timeouts**: Set appropriate timeouts for your cluster size

### Performance Optimization

1. **Selective Monitoring**: Only enable checks you need
2. **Appropriate Intervals**: Balance freshness with API load
3. **Cache Strategy**: Use longer TTL for stable applications
4. **Timeout Tuning**: Set timeouts based on cluster responsiveness

### Production Recommendations

```yaml
kubernetes:
  # Core monitoring
  checkPods: true
  checkDeployments: true
  checkServices: true
  
  # Dependency validation
  checkDependencies: true
  
  # Conservative thresholds
  requiredReadyRatio: 0.8
  checkPodRestarts: true
  maxPodRestarts: 5
  
  # Performance optimization
  apiTimeout: "10s"
  enableCaching: true
  cacheTTL: "2m"
```

## Troubleshooting

### Common Issues

1. **No Resources Found**
   - Verify Helm deployment labels
   - Check label selector configuration
   - Confirm namespace targeting

2. **API Timeouts**
   - Increase `apiTimeout` setting
   - Check cluster performance
   - Verify network connectivity

3. **Cache Issues**
   - Clear cache manually if needed
   - Adjust TTL settings
   - Monitor cache statistics

### Debug Configuration

```yaml
kubernetes:
  apiTimeout: "30s"      # Longer timeout for debugging
  enableCaching: false   # Disable cache for testing
  checkPods: true
  # Enable only essential checks for debugging
```

## Integration Examples

### Development Environment

```yaml
healthChecks:
- name: "dev-k8s-health"
  type: "kubernetes"
  interval: "60s"
  timeout: "30s"
  failureThreshold: 5
  kubernetes:
    checkPods: true
    checkDeployments: true
    requiredReadyRatio: 0.5  # Relaxed for development
    maxPodRestarts: 20
    enableCaching: false     # Faster feedback
```

### Production Environment

```yaml
healthChecks:
- name: "prod-k8s-health"
  type: "kubernetes"
  interval: "30s"
  timeout: "15s"
  kubernetes:
    checkPods: true
    checkDeployments: true
    checkServices: true
    checkStatefulSets: true
    checkDependencies: true
    checkNodeHealth: true
    requiredReadyRatio: 1.0
    checkPodRestarts: true
    maxPodRestarts: 3
    enableCaching: true
    cacheTTL: "1m"
```

### Microservices Stack

```yaml
healthChecks:
- name: "microservices-k8s"
  type: "kubernetes"
  interval: "45s"
  timeout: "20s"
  kubernetes:
    checkPods: true
    checkDeployments: true
    checkServices: true
    checkDependencies: true
    labelSelector:
      matchLabels:
        environment: "production"
        stack: "microservices"
    customResources:
    - apiVersion: "networking.istio.io/v1beta1"
      kind: "VirtualService"
      healthStrategy: "exists"
    - apiVersion: "networking.istio.io/v1beta1"
      kind: "DestinationRule"
      healthStrategy: "exists"
```

## Future Enhancements

### Planned Features

1. **Resource Quotas**: Monitor resource usage against quotas
2. **Network Policies**: Validate network policy configurations
3. **Security Contexts**: Check security policy compliance
4. **Horizontal Pod Autoscaler**: Monitor HPA status and metrics
5. **Persistent Volume**: Advanced storage health checking

### Extension Points

The system is designed for extensibility:

- Custom health strategies for resources
- Pluggable resource checkers
- Configurable assessment criteria
- Integration with external monitoring systems

## Conclusion

The Kubernetes native health checks provide comprehensive monitoring capabilities that go far beyond basic readiness checks. By integrating deeply with Kubernetes APIs and Helm deployment patterns, they offer detailed insights into application health across multiple dimensions.

The modular architecture ensures maintainability and extensibility, while the caching system provides the performance characteristics needed for production environments. Whether monitoring simple web applications or complex microservices architectures, the Kubernetes health checker provides the visibility needed to ensure application reliability.
