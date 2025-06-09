# Roost-Keeper Observability Framework

This directory contains the comprehensive observability foundation for Roost-Keeper, implementing OpenTelemetry with local-otel compatible file-based exporters for development and testing.

## Architecture

The observability framework provides three pillars of observability:

### 1. Traces (`observability/traces/`)
- **Format**: JSON Lines (JSONL) 
- **File**: `traces.jsonl`
- **Content**: Distributed traces showing the flow of operations through the operator
- **Includes**: Correlation IDs, span hierarchy, timing, attributes, and events

### 2. Metrics (`observability/metrics/`)
- **Format**: JSON Lines (JSONL)
- **File**: `metrics.jsonl` 
- **Content**: Performance and business metrics for monitoring operator health
- **Types**: Counters, gauges, and histograms for comprehensive monitoring

### 3. Logs (`observability/logs/`)
- **Format**: JSON Lines (JSONL)
- **File**: `operator.jsonl`
- **Content**: Structured logs with correlation IDs and trace context
- **Features**: Multi-output (console + file), automatic trace/span correlation

## Key Features

### 🔗 Correlation ID Tracking
Every request gets a unique correlation ID that flows through all telemetry:
- Traces include correlation IDs as attributes
- Logs automatically include correlation IDs
- Metrics can be filtered by correlation ID
- End-to-end request tracking across components

### 📊 Comprehensive Metrics
**Controller Metrics:**
- `roost_keeper_reconcile_total` - Total reconcile operations
- `roost_keeper_reconcile_duration_seconds` - Reconcile timing
- `roost_keeper_reconcile_errors_total` - Reconcile failures

**ManagedRoost Metrics:**
- `roost_keeper_roosts_total` - Total managed roosts
- `roost_keeper_roosts_healthy` - Healthy roost count
- `roost_keeper_roosts_by_phase` - Roosts grouped by phase
- `roost_keeper_roost_generation_lag` - Spec/status generation differences

**Helm Operation Metrics:**
- `roost_keeper_helm_install_total` - Helm install operations
- `roost_keeper_helm_install_duration_seconds` - Install timing
- `roost_keeper_helm_install_errors_total` - Install failures
- (Similar for upgrade and uninstall operations)

**Health Check Metrics:**
- `roost_keeper_health_check_total` - Health checks performed
- `roost_keeper_health_check_duration_seconds` - Check timing
- `roost_keeper_health_check_errors_total` - Check failures

**Performance Metrics:**
- `roost_keeper_memory_usage_bytes` - Memory consumption
- `roost_keeper_cpu_usage_ratio` - CPU utilization
- `roost_keeper_goroutine_count` - Active goroutines

### 🔍 Distributed Tracing
**Span Types:**
- **Controller spans**: Reconciliation operations
- **Helm spans**: Chart installation/upgrade/uninstall
- **Health check spans**: Service health validation  
- **Kubernetes API spans**: API server interactions
- **Generic spans**: Custom operations

**Span Attributes:**
- Operation type and target resource
- Correlation IDs for request tracking
- Resource names and namespaces
- Generation numbers and status

### 📝 Structured Logging
**Log Levels:**
- `INFO`: Important operational events
- `DEBUG`: Detailed operational information
- `ERROR`: Error conditions with full context

**Log Context:**
- Automatic trace ID and span ID inclusion
- Correlation ID propagation
- Service and component identification
- Structured fields for easy querying

## Middleware Integration

### Controller Middleware
Wraps controller operations with complete observability:
```go
wrappedReconcile := telemetry.ControllerMiddleware(
    reconcileFunc,
    "reconcile",
    metrics,
    roostName,
    namespace,
)
```

### Helm Middleware
Instruments Helm operations:
```go
wrappedHelmOp := telemetry.HelmMiddleware(
    helmFunc,
    "install",
    chartName,
    chartVersion,
    namespace,
    metrics,
)
```

### Kubernetes API Middleware
Tracks API server interactions:
```go
wrappedAPICall := telemetry.KubernetesAPIMiddleware(
    apiFunc,
    "GET",
    "ManagedRoost", 
    namespace,
    metrics,
)
```

## Local-OTEL Compatibility

The framework is designed for compatibility with the local-otel stack:

### File Format
- **JSON Lines**: Each line is a complete JSON object
- **Timestamps**: Unix timestamps for consistent ordering
- **Structure**: Follows OpenTelemetry semantic conventions

### Directory Structure
```
observability/
├── traces/
│   └── traces.jsonl     # Distributed traces
├── metrics/
│   └── metrics.jsonl    # Performance metrics
├── logs/
│   └── operator.jsonl   # Structured logs
├── configs/
│   └── otel-collector.yaml  # OTEL collector config
└── dashboards/
    └── (dashboard configs)
```

## Performance Characteristics

### Design Goals
- **< 5% Performance Impact**: Minimal overhead in production
- **Async Processing**: Non-blocking telemetry export
- **Graceful Degradation**: Operator functions even if observability fails
- **Resource Limits**: Bounded memory usage for telemetry buffers

### Optimizations
- **Batched Exports**: Reduce I/O overhead
- **Sampling**: Configurable trace sampling rates
- **Circuit Breaker**: Automatic telemetry disable on errors
- **Efficient Serialization**: Optimized JSON encoding

## Development Workflow

### Local Testing
1. **Start Operator**: Observability automatically initializes
2. **Generate Telemetry**: Perform operations (reconcile, etc.)
3. **Check Files**: Examine JSON files in `observability/` directories
4. **Analyze Data**: Use JSON tools or local-otel viewers

### Integration Testing
```bash
# Run observability integration tests
go test ./test/integration/observability_test.go -v

# Check telemetry output
ls -la observability/*/
head observability/traces/traces.jsonl
head observability/metrics/metrics.jsonl  
head observability/logs/operator.jsonl
```

### Performance Testing
```bash
# Measure observability overhead
go test ./test/integration/observability_test.go -v -run TestPerformanceImpact
```

## Configuration

### OTEL SDK Configuration
Located in `internal/telemetry/otel.go`:
- **Trace Sampling**: Currently set to sample all traces
- **Export Intervals**: Metrics exported every 15 seconds
- **Batch Sizes**: Traces batched up to 100 spans
- **Resource Attributes**: Service identification

### Logger Configuration  
Located in `internal/telemetry/logger.go`:
- **Format**: JSON structured logging
- **Outputs**: File + console in development
- **Fields**: Automatic service/version/component fields
- **Context**: Automatic trace and correlation ID inclusion

### Metrics Configuration
Located in `internal/telemetry/metrics.go`:
- **Naming**: OpenTelemetry semantic conventions
- **Labels**: Resource names, namespaces, operations
- **Types**: Counters, gauges, histograms as appropriate
- **Cardinality**: Bounded to prevent memory issues

## Production Considerations

### Security
- **No Secrets**: Telemetry never includes sensitive data
- **File Permissions**: Secure access to telemetry files
- **Log Rotation**: Automatic rotation to prevent disk space issues
- **Audit Compliance**: Structured logs support audit requirements

### Reliability
- **Error Isolation**: Telemetry errors don't affect operator
- **Graceful Degradation**: Falls back to console logging
- **Resource Monitoring**: Self-monitoring of telemetry overhead
- **Circuit Breakers**: Automatic disable on repeated failures

### Scalability
- **Bounded Resources**: Memory limits on telemetry buffers
- **Efficient Export**: Batched and compressed exports
- **Sampling**: Production sampling rates to manage volume
- **Retention**: Configurable data retention policies

## Troubleshooting

### Common Issues

**No telemetry files created:**
- Check directory permissions
- Verify `observability/` directory exists
- Check for startup errors in logs

**High memory usage:**
- Review sampling rates
- Check export intervals
- Monitor batch sizes

**Missing correlation IDs:**
- Ensure middleware wraps all operations
- Check context propagation
- Verify correlation ID generation

**Performance impact:**
- Measure overhead with performance tests
- Adjust sampling rates
- Review export frequencies

### Debug Commands
```bash
# Check telemetry file formats
jq '.' observability/traces/traces.jsonl | head -20
jq '.' observability/metrics/metrics.jsonl | head -20  
jq '.' observability/logs/operator.jsonl | head -20

# Monitor file sizes
watch -n 1 'ls -lh observability/*/*.jsonl'

# Count telemetry entries
wc -l observability/*/*.jsonl
```

## Future Enhancements

### Planned Features
- **Dashboard Templates**: Grafana dashboard configs
- **Alert Rules**: Prometheus alerting rules  
- **Export Options**: Additional export formats
- **Compression**: Compressed file outputs
- **Retention**: Automatic file cleanup

### Integration Options
- **Prometheus**: Native Prometheus metrics export
- **Jaeger**: Direct Jaeger trace export
- **ElasticSearch**: Log shipping to ELK stack
- **Cloud Providers**: AWS CloudWatch, GCP Monitoring

---

**Built for Production Observability** 🚀

This observability framework provides enterprise-grade visibility into Roost-Keeper operations, enabling rapid debugging, performance optimization, and reliable production monitoring.
