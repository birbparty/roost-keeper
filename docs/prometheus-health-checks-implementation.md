# Prometheus Health Checks Implementation

## Overview

The Prometheus health checks implementation provides sophisticated, metrics-driven health monitoring for Roost-Keeper. This system leverages PromQL queries to evaluate application health based on time-series data, offering deeper insights than simple connectivity checks.

## Features

### ✅ Core Capabilities
- **PromQL Query Execution**: Execute complex Prometheus queries with proper error handling and timeouts
- **Threshold Evaluation**: Support all mathematical operators (>, >=, <, <=, ==, !=) with precise float comparison
- **Service Discovery**: Automatic Prometheus endpoint discovery via Kubernetes services
- **Authentication**: Bearer token and basic authentication with secret reference support
- **Query Templating**: Dynamic query generation with roost-specific variables
- **Result Caching**: TTL-based caching to reduce load on Prometheus servers
- **Comprehensive Logging**: Detailed observability with OpenTelemetry integration

### 🚀 Advanced Features
- **Trend Analysis**: Time-series trend detection with improvement/degradation classification
- **Multiple Result Handling**: Support for both vector and scalar query results
- **Query Optimization**: Efficient query construction and client connection pooling
- **Fallback Mechanisms**: Service discovery with explicit endpoint fallback
- **Custom Labels**: Add metadata to health checks for enhanced monitoring

## Architecture

### Component Structure

```
internal/health/prometheus/
├── checker.go           # Main Prometheus health checker
├── discovery.go         # Service discovery for Prometheus endpoints
├── auth.go             # Authentication handling (bearer token, basic auth)
├── cache.go            # Query result caching with TTL
└── trend.go            # Time-series trend analysis
```

### Integration Points

1. **Health Check Framework**: Seamless integration with `internal/health/checker.go`
2. **Telemetry**: Full OpenTelemetry tracing and metrics
3. **Kubernetes API**: Service discovery and secret resolution
4. **Prometheus API**: Direct integration with Prometheus HTTP API

## Configuration

### Basic Configuration

```yaml
apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: basic-prometheus-example
spec:
  chart:
    repository:
      url: "https://charts.example.com"
    name: "my-app"
    version: "1.0.0"
  healthChecks:
  - name: "memory-usage-check"
    type: "prometheus"
    interval: "30s"
    timeout: "10s"
    prometheus:
      endpoint: "http://prometheus.monitoring:9090"
      query: 'avg(container_memory_usage_bytes{pod=~"{{.ServiceName}}-.*"}) / 1024 / 1024'
      threshold: "512"  # 512 MB
      operator: "lt"    # Less than threshold means healthy
```

### Service Discovery Configuration

```yaml
prometheus:
  serviceDiscovery:
    serviceName: "prometheus-server"
    serviceNamespace: "monitoring"
    servicePort: "9090"
    pathPrefix: "/prometheus"
    enableFallback: true
  endpoint: "http://prometheus.monitoring:9090"  # Fallback endpoint
  query: 'histogram_quantile(0.95, rate(http_request_duration_seconds_bucket{service="{{.ServiceName}}"}[5m]))'
  threshold: "0.5"
  operator: "lt"
```

### Authentication Configuration

#### Bearer Token Authentication
```yaml
prometheus:
  endpoint: "https://prometheus.secure.com:9090"
  auth:
    bearerToken: "{{.Secret.prometheus-token.token}}"
  query: 'up{job="{{.ServiceName}}"}'
  threshold: "1"
  operator: "eq"
```

#### Basic Authentication
```yaml
prometheus:
  endpoint: "https://prometheus.internal.com:9090"
  auth:
    basicAuth:
      username: "monitoring"
      password: "{{.Secret.prometheus-basic-auth.password}}"
  query: 'sum(rabbitmq_queue_messages{queue=~"{{.ServiceName}}-.*"})'
  threshold: "1000"
  operator: "lt"
```

#### Custom Headers
```yaml
prometheus:
  endpoint: "https://prometheus.company.com:9090"
  auth:
    headers:
      "X-API-Key": "{{.Secret.prometheus-api.key}}"
      "X-Tenant-ID": "{{.RoostNamespace}}"
  query: 'rate(http_requests_total{service="{{.ServiceName}}"}[5m])'
  threshold: "10"
  operator: "gt"
```

### Advanced Configuration with Trend Analysis

```yaml
prometheus:
  serviceDiscovery:
    labelSelector:
      app: "prometheus"
      component: "server"
    servicePort: "web"
  query: 'rate(http_requests_total{service="{{.ServiceName}}", status=~"5.."}[5m]) / rate(http_requests_total{service="{{.ServiceName}}"}[5m])'
  threshold: "0.01"  # 1% error rate
  operator: "lt"
  trendAnalysis:
    enabled: true
    timeWindow: "15m"
    improvementThreshold: 20  # 20% improvement needed
    degradationThreshold: 30  # 30% degradation triggers concern
    allowImprovingUnhealthy: true  # Consider healthy if improving despite threshold
  queryTimeout: "30s"
  cacheTTL: "3m"
```

## Query Templating

The Prometheus health checker supports dynamic query templating with roost-specific variables:

### Available Template Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `{{.ServiceName}}` | ManagedRoost name | `my-service` |
| `{{.Namespace}}` | ManagedRoost namespace | `production` |
| `{{.RoostName}}` | Alias for ServiceName | `my-service` |
| `{{.RoostNamespace}}` | Alias for Namespace | `production` |
| `{{.Release.Name}}` | Helm release name | `my-service-v1-2-3` |
| `${ROOST_NAME}` | Environment-style variable | `my-service` |
| `${NAMESPACE}` | Environment-style variable | `production` |

### Template Examples

```yaml
# Memory usage by pod prefix
query: 'avg(container_memory_usage_bytes{pod=~"{{.ServiceName}}-.*"})'

# HTTP request rate by service label
query: 'rate(http_requests_total{service="{{.ServiceName}}"}[5m])'

# Database connections by release
query: 'mysql_global_status_threads_connected{instance=~".*{{.Release.Name}}.*"}'

# Custom metrics with namespace
query: 'custom_metric{namespace="{{.Namespace}}", app="{{.ServiceName}}"}'
```

## Threshold Evaluation

### Supported Operators

| Operator | Symbol | Description | Example |
|----------|--------|-------------|---------|
| `gt` | `>` | Greater than | `value > 100` |
| `gte` | `>=` | Greater than or equal | `value >= 100` |
| `lt` | `<` | Less than | `value < 100` |
| `lte` | `<=` | Less than or equal | `value <= 100` |
| `eq` | `==` | Equal (with epsilon) | `value == 100` |
| `ne` | `!=` | Not equal | `value != 100` |

### Float Comparison

The system handles floating-point comparisons properly:
- Equality comparisons use epsilon (0.0001) for tolerance
- All other operators use standard mathematical comparison
- Threshold values are parsed as float64 for precision

### Examples

```yaml
# CPU usage should be less than 80%
threshold: "80"
operator: "lt"

# Service should be exactly up (1)
threshold: "1"
operator: "eq"

# Error rate should be less than 1%
threshold: "0.01"
operator: "lt"

# Request rate should be greater than 100 RPS
threshold: "100"
operator: "gt"
```

## Service Discovery

### Discovery Methods

#### 1. Service Name Discovery
```yaml
serviceDiscovery:
  serviceName: "prometheus"
  serviceNamespace: "monitoring"  # Optional, defaults to roost namespace
  servicePort: "9090"            # Port name or number
```

#### 2. Label Selector Discovery
```yaml
serviceDiscovery:
  labelSelector:
    app: "prometheus"
    component: "server"
  serviceNamespace: "monitoring"
  servicePort: "web"
```

#### 3. Fallback Configuration
```yaml
serviceDiscovery:
  serviceName: "prometheus"
  serviceNamespace: "monitoring"
  enableFallback: true
endpoint: "http://backup-prometheus:9090"  # Used if discovery fails
```

### Port Resolution

The service discovery system handles port resolution intelligently:

1. **Explicit Port Number**: `servicePort: "9090"`
2. **Named Port**: `servicePort: "web"`
3. **Auto-detection**: Searches for common Prometheus port names
4. **Default**: Uses first available port if none specified

### Hostname Construction

Services are accessed via cluster-internal DNS:
```
<service-name>.<namespace>.svc.cluster.local
```

## Authentication

### Secret Reference Patterns

#### Template Syntax
```yaml
# Full template with secret name and key
bearerToken: "{{.Secret.prometheus-token.token}}"

# Simplified template (assumes 'token' key)
bearerToken: "{{.Secret.prometheus-token}}"

# Custom headers with secret references
headers:
  "X-API-Key": "{{.Secret.api-credentials.key}}"
  "Authorization": "Bearer {{.Secret.auth-token.value}}"
```

#### Secret Auto-Detection
When using `secretRef` without specifying a key, the system automatically detects:

1. **Bearer Tokens**: `token`, `bearer-token`, `auth-token`
2. **Basic Auth**: `username` + `password`
3. **Authorization Headers**: `authorization`

```yaml
auth:
  secretRef:
    name: "prometheus-credentials"
    # Auto-detects authentication type from secret keys
```

### Authentication Precedence

1. **Bearer Token** (highest priority)
2. **Basic Auth**
3. **Secret Reference**
4. **Custom Headers** (lowest priority)

## Caching

### Cache Configuration

```yaml
prometheus:
  cacheTTL: "5m"  # Cache results for 5 minutes
  query: 'up{job="{{.ServiceName}}"}'
  # ... other configuration
```

### Cache Behavior

- **Cache Key**: Based on endpoint, query, roost name/namespace, and check name
- **TTL**: Configurable per health check (default: 5 minutes)
- **Automatic Cleanup**: Expired entries are removed every 5 minutes
- **Cache Miss**: Results in fresh Prometheus query execution
- **Successful Results Only**: Only healthy results are cached

### Cache Bypass

Set `cacheTTL: "0s"` to disable caching for a specific health check.

## Trend Analysis

### Configuration

```yaml
trendAnalysis:
  enabled: true
  timeWindow: "15m"              # How far back to analyze
  improvementThreshold: 10       # % improvement needed
  degradationThreshold: 20       # % degradation threshold
  allowImprovingUnhealthy: true  # Override threshold if improving
```

### How It Works

1. **Range Query**: Executes query over specified time window
2. **Linear Regression**: Calculates slope and correlation
3. **Trend Classification**: Determines if improving, degrading, or stable
4. **Health Override**: Can consider improving metrics as healthy

### Trend Categories

- **Improving**: Slope indicates positive trend with sufficient confidence
- **Degrading**: Slope indicates negative trend exceeding threshold
- **Stable**: Minimal change or insufficient confidence in trend

## Error Handling

### Common Error Scenarios

1. **Prometheus Unreachable**: Network connectivity issues
2. **Authentication Failure**: Invalid credentials or tokens
3. **Query Syntax Error**: Invalid PromQL syntax
4. **No Results**: Query returns empty result set
5. **Multiple Results**: Vector queries with unexpected result count
6. **Threshold Parse Error**: Invalid threshold value format

### Error Response Format

```json
{
  "healthy": false,
  "message": "Prometheus query failed: connection refused",
  "details": {
    "error_type": "connection_error",
    "endpoint": "http://prometheus:9090",
    "query": "up{job=\"my-service\"}",
    "execution_time_ms": 5000
  }
}
```

### Timeout Handling

```yaml
# Health check level timeout
timeout: "30s"

# Prometheus-specific query timeout (overrides health check timeout)
prometheus:
  queryTimeout: "15s"
```

## Performance Considerations

### Query Optimization

1. **Use Specific Labels**: Include service/namespace labels in queries
2. **Limit Time Ranges**: Use appropriate time ranges for rate/increase functions
3. **Avoid Expensive Operations**: Be cautious with regex and complex aggregations
4. **Pre-aggregate When Possible**: Use recording rules for complex calculations

### Connection Management

- **Connection Pooling**: Automatic connection reuse per endpoint
- **Client Caching**: One client per unique endpoint
- **Timeout Configuration**: Proper timeout settings prevent resource leaks

### Resource Usage

```yaml
# Recommended configuration for production
prometheus:
  queryTimeout: "10s"    # Keep queries fast
  cacheTTL: "2m"         # Balance freshness vs. load
  interval: "30s"        # Don't check too frequently
```

## Monitoring and Observability

### OpenTelemetry Integration

All Prometheus health checks are fully instrumented with:

- **Spans**: Detailed execution tracing
- **Attributes**: Query details, endpoint info, results
- **Metrics**: Execution time, success/failure rates
- **Errors**: Comprehensive error recording

### Log Messages

```
INFO  Starting Prometheus health check
  check_name=memory-check
  check_type=prometheus
  query=avg(container_memory_usage_bytes{pod=~"my-service-.*"})
  endpoint=http://prometheus:9090

INFO  Prometheus health check completed
  healthy=true
  threshold_met=true
  value=256.5
  execution_time=1.2s
  endpoint=http://prometheus:9090
```

### Health Check Results

```json
{
  "healthy": true,
  "message": "Prometheus health check passed",
  "query_result": {
    "value": 256.5,
    "timestamp": "2025-06-10T21:00:00Z",
    "labels": {
      "job": "my-service"
    }
  },
  "threshold_met": true,
  "execution_time": "1.2s",
  "from_cache": false,
  "prometheus_url": "http://prometheus:9090",
  "discovery_method": "service_discovery"
}
```

## Best Practices

### Query Design

1. **Use Efficient Queries**: Leverage Prometheus best practices
2. **Include Context**: Add relevant labels for filtering
3. **Test Queries**: Validate PromQL in Prometheus UI first
4. **Document Intent**: Use clear query comments and descriptions

```yaml
# Good: Specific and efficient
query: 'avg(container_memory_usage_bytes{pod=~"{{.ServiceName}}-.*", namespace="{{.Namespace}}"})'

# Avoid: Too broad and expensive
query: 'avg(container_memory_usage_bytes)'
```

### Threshold Setting

1. **Based on SLIs**: Align thresholds with Service Level Indicators
2. **Include Buffer**: Set thresholds with safety margins
3. **Consider Trends**: Use trend analysis for gradual degradation
4. **Regular Review**: Adjust thresholds based on operational experience

### Authentication Security

1. **Use Secrets**: Never embed credentials in YAML
2. **Least Privilege**: Grant minimal required permissions
3. **Rotate Regularly**: Implement credential rotation
4. **Audit Access**: Monitor authentication attempts

```yaml
# Secure: Uses secret reference
auth:
  bearerToken: "{{.Secret.prometheus-token.token}}"

# Insecure: Hardcoded credential
auth:
  bearerToken: "actual-token-value"  # DON'T DO THIS
```

## Troubleshooting

### Common Issues and Solutions

#### 1. Service Discovery Fails

**Symptoms**: `failed to discover Prometheus endpoint`

**Solutions**:
- Verify service exists in specified namespace
- Check label selectors match service labels
- Ensure service has correct ports defined
- Enable fallback with explicit endpoint

#### 2. Authentication Errors

**Symptoms**: `authentication failed: unauthorized`

**Solutions**:
- Verify secret exists and contains correct keys
- Check secret template syntax
- Validate credentials against Prometheus directly
- Ensure proper RBAC permissions for secret access

#### 3. Query Execution Timeouts

**Symptoms**: `Prometheus query failed: context deadline exceeded`

**Solutions**:
- Increase `queryTimeout` value
- Optimize PromQL query for better performance
- Check Prometheus server load and performance
- Consider using recording rules for complex queries

#### 4. No Query Results

**Symptoms**: `query returned no results`

**Solutions**:
- Verify metric names and labels exist in Prometheus
- Check time ranges in rate/increase functions
- Validate template variable substitution
- Test query directly in Prometheus UI

#### 5. Cache Issues

**Symptoms**: Stale results or unexpected caching behavior

**Solutions**:
- Verify `cacheTTL` configuration
- Check cache key uniqueness
- Restart health checker to clear cache
- Disable caching temporarily for debugging

### Debug Techniques

1. **Query Testing**: Test PromQL queries directly in Prometheus
2. **Log Analysis**: Review health checker logs for detailed error information
3. **Template Verification**: Check variable substitution in logs
4. **Network Testing**: Verify connectivity to Prometheus endpoints

## Examples

### Complete Configuration Examples

See `config/samples/prometheus_health_checks.yaml` for comprehensive examples including:

- Basic Prometheus health checks
- Service discovery configurations
- Authentication methods
- Trend analysis setup
- Multi-check configurations
- Composite health checks

### Integration Examples

```yaml
# Combining Prometheus with other health check types
healthChecks:
- name: "http-liveness"
  type: "http"
  weight: 10
  http:
    url: "http://{{.ServiceName}}:8080/health"
    
- name: "prometheus-performance"
  type: "prometheus"
  weight: 8
  prometheus:
    query: 'avg(rate(http_requests_total{service="{{.ServiceName}}"}[5m]))'
    threshold: "100"
    operator: "gt"
```

## API Reference

### PrometheusHealthCheckSpec

Complete API specification for Prometheus health check configuration:

```go
type PrometheusHealthCheckSpec struct {
    Query            string                     // PromQL query (supports templating)
    Threshold        string                     // Threshold value as string
    Operator         string                     // Comparison operator
    Endpoint         string                     // Explicit Prometheus endpoint
    ServiceDiscovery *PrometheusDiscoverySpec   // Service discovery config
    Auth             *PrometheusAuthSpec        // Authentication config
    QueryTimeout     *metav1.Duration           // Query execution timeout
    CacheTTL         *metav1.Duration           // Cache time-to-live
    TrendAnalysis    *TrendAnalysisSpec         // Trend analysis config
    Labels           map[string]string          // Custom labels
}
```

For complete API documentation, see the generated CRD specification and Go package documentation.
