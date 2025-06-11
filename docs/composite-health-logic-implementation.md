# Composite Health Logic Implementation

## Overview

The Composite Health Logic system provides advanced health check orchestration with sophisticated evaluation strategies including weighted scoring, dependency resolution, circuit breakers, anomaly detection, and audit trails. This implementation extends beyond simple pass/fail health checks to provide nuanced health assessment for complex distributed systems.

## Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────────┐
│                    CompositeEngine                              │
├─────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │   Scorer    │  │  Evaluator  │  │ Dependency  │            │
│  │             │  │             │  │  Resolver   │            │
│  └─────────────┘  └─────────────┘  └─────────────┘            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐            │
│  │   Cache     │  │ Circuit     │  │  Anomaly    │            │
│  │             │  │ Breaker     │  │  Detector   │            │
│  └─────────────┘  └─────────────┘  └─────────────┘            │
│  ┌─────────────┐                                               │
│  │   Auditor   │                                               │
│  │             │                                               │
│  └─────────────┘                                               │
└─────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

#### CompositeEngine
- **Primary Role**: Orchestrates health check execution and evaluation
- **Key Features**:
  - Manages individual health checker instances
  - Coordinates dependency-aware execution
  - Integrates all composite logic components
  - Provides comprehensive health results
  - Records telemetry and metrics

#### Scorer
- **Primary Role**: Calculates weighted health scores using various strategies
- **Strategies**:
  - **Weighted Average**: Traditional weighted scoring with penalties
  - **Weighted Sum**: Normalized sum-based scoring with failure weights
  - **Threshold**: Binary threshold-based evaluation
- **Features**:
  - Configurable penalty multipliers for failures
  - Intelligent weight recommendations
  - Detailed scoring breakdowns

#### Evaluator
- **Primary Role**: Processes logical expressions for complex health decisions
- **Capabilities**:
  - Parses logical expressions (AND, OR, NOT)
  - Supports parentheses and operator precedence
  - Generates truth tables for expression analysis
  - Provides detailed evaluation context
- **Expression Examples**:
  ```
  # Critical path evaluation
  (database AND api-service)
  
  # Redundancy-aware evaluation
  (primary-db OR secondary-db) AND (cache OR direct-access)
  
  # Business logic evaluation
  payment-service OR (cache AND offline-mode)
  ```

#### DependencyResolver
- **Primary Role**: Manages health check execution order based on dependencies
- **Features**:
  - Topological sorting for dependency resolution
  - Cycle detection and prevention
  - Parallel execution optimization
  - Critical path analysis
  - Dependency validation

#### CircuitBreaker
- **Primary Role**: Prevents cascading failures by isolating problematic checks
- **States**:
  - **Closed**: Normal operation, all requests pass through
  - **Open**: Failure threshold exceeded, requests immediately fail
  - **Half-Open**: Testing recovery, limited requests allowed
- **Configuration**:
  - Failure threshold for opening
  - Success threshold for closing
  - Half-open timeout duration
  - Recovery timeout settings

#### Cache
- **Primary Role**: Improves performance by caching successful health check results
- **Features**:
  - TTL-based expiration
  - Hit count tracking
  - Automatic cleanup of expired entries
  - Cache statistics and monitoring
  - Per-check TTL configuration

#### AnomalyDetector
- **Primary Role**: Identifies statistical anomalies in health check patterns
- **Detection Types**:
  - **Response Time Anomalies**: Unusual latency patterns
  - **Failure Rate Anomalies**: Unexpected failure spikes
  - **Trend Anomalies**: Degradation or improvement trends
  - **Pattern Anomalies**: Overall system health patterns
- **Statistical Methods**:
  - Z-score analysis for outlier detection
  - Moving average trend analysis
  - Confidence-based filtering

#### Auditor
- **Primary Role**: Maintains comprehensive audit trails for health check decisions
- **Event Types**:
  - Evaluation lifecycle events
  - Individual check executions
  - Circuit breaker state changes
  - Cache operations
  - Anomaly detections
  - Logic evaluations
- **Features**:
  - Structured logging with severity levels
  - Event filtering and querying
  - Export capabilities (JSON, CSV)
  - Retention management

## Health Check Types Integration

The composite system integrates with all health check types:

### HTTP Health Checks
```yaml
- name: api-service
  type: http
  weight: 8
  http:
    url: "http://api-service:8080/health"
    expectedCodes: [200]
    headers:
      Authorization: "Bearer {{ .token }}"
```

### TCP Health Checks
```yaml
- name: database
  type: tcp
  weight: 10
  tcp:
    host: "postgres.db.svc.cluster.local"
    port: 5432
    sendData: ""
    enablePooling: true
```

### gRPC Health Checks
```yaml
- name: worker-service
  type: grpc
  weight: 4
  grpc:
    host: "worker-service"
    port: 9090
    service: "health"
    circuitBreaker:
      enabled: true
      failureThreshold: 5
```

### Prometheus Health Checks
```yaml
- name: metrics-endpoint
  type: prometheus
  weight: 2
  prometheus:
    query: 'up{job="api-server"}'
    threshold: "1"
    operator: gte
    trendAnalysis:
      enabled: true
```

### Kubernetes Health Checks
```yaml
- name: kubernetes-resources
  type: kubernetes
  weight: 7
  kubernetes:
    checkPods: true
    checkDeployments: true
    requiredReadyRatio: 0.8
```

## Configuration

### Global Health Evaluation Settings

```yaml
# ConfigMap: composite-health-config
health_evaluation:
  scoring_strategy: "weighted_average"
  healthy_threshold: 0.85
  degraded_threshold: 0.60
  enable_penalties: true
  penalty_multiplier: 1.5
  
  circuit_breaker:
    failure_threshold: 5
    success_threshold: 3
    timeout: 60s
  
  cache:
    default_ttl: 30s
    max_entries: 1000
  
  anomaly_detection:
    enabled: true
    confidence_threshold: 0.8
  
  audit:
    enabled: true
    max_events: 1000
```

### Weight Configuration

```yaml
check_weights:
  primary-database: 10    # Most critical
  api-service: 8         # Core application
  kubernetes-resources: 7 # Infrastructure
  cache-service: 5       # Supporting service
  metrics-endpoint: 2    # Monitoring
```

### Logical Expressions

```yaml
expressions:
  critical_path: "(primary-database AND api-service)"
  business_critical: "(payment-service OR (cache-service AND primary-database))"
  complete_health: |
    (primary-database AND api-service AND kubernetes-resources) AND
    (cache-service OR message-queue) AND
    (load-balancer OR cdn-endpoint)
```

## Usage Examples

### Basic Composite Health Check

```yaml
apiVersion: roost.birbparty.com/v1alpha1
kind: ManagedRoost
metadata:
  name: web-application
spec:
  healthChecks:
    - name: database
      type: tcp
      weight: 10
      tcp:
        host: "postgres"
        port: 5432
    
    - name: api
      type: http  
      weight: 8
      http:
        url: "http://api:8080/health"
    
    - name: cache
      type: tcp
      weight: 5
      tcp:
        host: "redis"
        port: 6379
```

### Advanced Health Check with Dependencies

```yaml
healthChecks:
  - name: load-balancer
    type: http
    weight: 2
    # Dependencies: none (entry point)
    
  - name: api-service
    type: http
    weight: 8
    # Dependencies: database, cache (implicit)
    
  - name: database
    type: tcp
    weight: 10
    # Dependencies: none (foundational)
    
  - name: worker-service
    type: grpc
    weight: 4
    # Dependencies: database, message-queue
```

### Logical Expression Evaluation

```yaml
# Define complex health logic
expressions:
  # Core system must be available
  core_available: "(database AND api-service)"
  
  # At least one data path must work
  data_path: "(cache-service OR direct-database)"
  
  # Overall health combines core + data path
  overall_health: "core_available AND data_path"
```

## Monitoring and Observability

### Metrics

The composite health system exposes comprehensive metrics:

```prometheus
# Overall health score (0.0 to 1.0)
roost_composite_health_score{roost="app", namespace="prod"} 0.85

# Individual check status
roost_health_check_status{check="database", roost="app"} 1

# Check execution duration
roost_health_check_duration_seconds{check="api", roost="app"} 0.123

# Circuit breaker states
roost_circuit_breaker_state{check="api", state="closed"} 1

# Cache hit ratio
roost_cache_hit_ratio{roost="app"} 0.75

# Anomaly detection events
roost_anomaly_detected_total{type="response_time", check="api"} 1
```

### Logs

Structured logs capture detailed health check information:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "component": "composite_engine",
  "roost": "web-application",
  "namespace": "production",
  "event": "health_evaluation_completed",
  "overall_healthy": true,
  "health_score": 0.85,
  "individual_results": {
    "database": {"healthy": true, "response_time": "5ms"},
    "api": {"healthy": true, "response_time": "12ms"},
    "cache": {"healthy": true, "response_time": "2ms"}
  },
  "execution_order": ["database", "cache", "api"],
  "anomalies_detected": false,
  "evaluation_time": "45ms"
}
```

### Audit Trail

Comprehensive audit events track all health check decisions:

```json
{
  "type": "evaluation_started",
  "timestamp": "2024-01-15T10:30:00.000Z",
  "context": {
    "roost": "web-application",
    "namespace": "production",
    "checks": 3
  }
}
```

## Integration with ManagedRoost Controller

The composite health system integrates seamlessly with the ManagedRoost controller:

### Controller Integration

```go
// In managedroost_controller.go
func (r *ManagedRoostReconciler) evaluateHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost) error {
    // Use composite engine for health evaluation
    result, err := r.compositeEngine.EvaluateCompositeHealth(ctx, roost)
    if err != nil {
        return err
    }
    
    // Update roost status with composite results
    roost.Status.Health = result.OverallHealthy
    roost.Status.HealthScore = result.HealthScore
    roost.Status.HealthDetails = result.CheckDetails
    
    return nil
}
```

### Status Updates

The composite health results are reflected in the ManagedRoost status:

```yaml
status:
  phase: Ready
  health: healthy
  healthScore: 0.85
  healthChecks:
    - name: database
      status: healthy
      lastCheck: "2024-01-15T10:30:00Z"
    - name: api
      status: healthy
      lastCheck: "2024-01-15T10:30:00Z"
  conditions:
    - type: Healthy
      status: "True"
      reason: "CompositeHealthPassed"
      message: "All critical health checks passing with score 0.85"
```

## Performance Considerations

### Caching Strategy

The cache component significantly improves performance:

- **TTL Configuration**: Balance between freshness and performance
- **Hit Ratio Monitoring**: Track cache effectiveness
- **Memory Management**: Automatic cleanup prevents memory leaks

### Parallel Execution

The dependency resolver enables parallel execution:

- **Dependency Levels**: Checks at the same level run in parallel
- **Resource Limits**: Configurable parallelism prevents resource exhaustion
- **Timeout Management**: Per-check timeouts prevent blocking

### Circuit Breaker Benefits

Circuit breakers improve system resilience:

- **Fail Fast**: Immediate failures for problematic checks
- **Resource Conservation**: Prevents wasted resources on failing checks
- **Gradual Recovery**: Half-open state allows gradual recovery testing

## Best Practices

### Weight Assignment

1. **Critical Infrastructure**: Assign highest weights (8-10)
   - Primary databases
   - Core API services
   - Authentication systems

2. **Supporting Services**: Medium weights (4-7)
   - Caches
   - Message queues
   - Background workers

3. **Auxiliary Services**: Lower weights (1-3)
   - Monitoring endpoints
   - CDN services
   - Optional features

### Expression Design

1. **Favor Clarity**: Use parentheses to make operator precedence explicit
2. **Consider Redundancy**: Account for backup systems and failover scenarios
3. **Business Logic**: Align expressions with actual business requirements
4. **Testing**: Use truth table generation to verify expression logic

### Circuit Breaker Configuration

1. **Failure Threshold**: Set based on expected reliability
   - Critical services: Lower threshold (3-5)
   - Non-critical services: Higher threshold (5-10)

2. **Recovery Timeout**: Allow sufficient time for problem resolution
   - Transient issues: 30-60 seconds
   - Infrastructure issues: 2-5 minutes

### Anomaly Detection Tuning

1. **Sample Size**: Ensure sufficient historical data (minimum 10-20 samples)
2. **Confidence Threshold**: Start conservative (0.8) and adjust based on false positives
3. **Baseline Period**: Allow adequate time to establish normal patterns

## Troubleshooting

### Common Issues

#### Low Health Scores

**Symptoms**: Health score consistently below expected threshold

**Debugging Steps**:
1. Check individual health check results
2. Review weight assignments
3. Examine penalty multiplier settings
4. Validate logical expressions

**Example Investigation**:
```bash
# Check individual health check status
kubectl get managedroost web-app -o jsonpath='{.status.healthChecks}'

# Review composite health details
kubectl logs deployment/roost-keeper-controller | grep "composite_health"
```

#### Circuit Breaker Issues

**Symptoms**: Checks failing due to open circuit breakers

**Debugging Steps**:
1. Review circuit breaker state history
2. Check failure threshold configuration
3. Investigate underlying health check issues
4. Verify recovery timeout settings

**Resolution**:
```yaml
# Adjust circuit breaker settings
circuitBreaker:
  failureThreshold: 10  # Increase tolerance
  recoveryTimeout: 120s # Allow more recovery time
```

#### Anomaly False Positives

**Symptoms**: Frequent anomaly alerts without actual issues

**Debugging Steps**:
1. Review anomaly detection sensitivity
2. Check historical data sufficiency
3. Validate baseline establishment period
4. Adjust confidence thresholds

**Tuning**:
```yaml
anomaly_detection:
  confidence_threshold: 0.9  # Increase for fewer false positives
  min_sample_size: 20       # Require more historical data
```

### Monitoring Health Check Performance

Use these queries to monitor composite health system performance:

```prometheus
# Average health evaluation time
avg(roost_health_evaluation_duration_seconds)

# Health check success rate by type
sum(rate(roost_health_check_success_total[5m])) by (type) / 
sum(rate(roost_health_check_total[5m])) by (type)

# Cache hit ratio trend
avg_over_time(roost_cache_hit_ratio[1h])

# Circuit breaker activation frequency
sum(rate(roost_circuit_breaker_state_changes_total[5m])) by (check)
```

## Future Enhancements

### Planned Features

1. **Machine Learning Integration**: AI-powered anomaly detection and pattern recognition
2. **Predictive Health**: Forecast potential health issues based on trends
3. **Custom Scoring Algorithms**: Plugin architecture for custom scoring strategies
4. **Health Check Recommendations**: Automatic suggestions for optimal health check configurations
5. **Integration Testing**: Synthetic transaction-based health validation
6. **Multi-Cluster Health**: Federated health checking across multiple clusters

### Extensibility Points

The composite health system is designed for extensibility:

1. **Custom Evaluators**: Implement additional logical expression evaluators
2. **Scoring Strategies**: Add new weighted scoring algorithms  
3. **Anomaly Detectors**: Implement domain-specific anomaly detection
4. **Cache Backends**: Support for external cache systems (Redis, Memcached)
5. **Audit Exporters**: Additional audit trail export formats and destinations

This implementation provides a robust foundation for sophisticated health checking in complex distributed systems while maintaining flexibility for future enhancements and customizations.
