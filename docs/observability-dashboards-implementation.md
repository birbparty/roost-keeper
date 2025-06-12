# Observability Dashboards Implementation

This document describes the comprehensive observability dashboard system implemented for Roost-Keeper, providing production-ready monitoring, alerting, and troubleshooting capabilities.

## Overview

The observability system consists of three specialized Grafana dashboards, comprehensive SLI/SLO definitions, intelligent alerting rules, and Slack integration for seamless incident response. This implementation follows Google's SRE best practices for monitoring distributed systems.

## Dashboard Architecture

### 1. Operational Dashboard (`01-operational.json`)
**Purpose**: Real-time operational overview for day-to-day monitoring

**Key Panels**:
- **System Health Overview**: Traffic light indicators for critical system components
- **Roost Status Distribution**: Pie chart showing roost phases (Running, Failed, Installing, etc.)
- **Error Rate Trends**: Time series graphs showing reconcile and health check error rates
- **Performance Metrics**: Response time percentiles (P50, P95, P99) for key operations
- **Resource Utilization**: CPU, memory, and queue depth monitoring
- **Active Operations**: Real-time view of in-progress operations

**Target Audience**: Platform engineers, SREs, on-call engineers
**Refresh Rate**: 10 seconds for real-time monitoring

### 2. SLI/SLO Dashboard (`02-sli-slo.json`)
**Purpose**: Service Level Objective tracking and error budget monitoring

**Key Panels**:
- **SLO Compliance Summary**: Current SLO status vs targets with traffic light indicators
- **Error Budget Tracking**: Burn rate analysis and remaining budget visualization
- **Availability Metrics**: 99.5% availability SLO with 30-day rolling window
- **Latency SLOs**: P95 creation latency (30s target) and health check latency (1s target)
- **Error Rate SLO**: Controller error rate target (<1%)
- **Budget Burn Rate Alerts**: Multi-window alerting for fast and slow burns

**Target Audience**: Engineering leadership, product teams, SREs
**Refresh Rate**: 30 seconds for trend analysis

### 3. Debug Dashboard (`03-debug.json`)
**Purpose**: Deep troubleshooting and incident investigation

**Key Panels**:
- **Correlation ID Tracking**: Request flow tracing across all components
- **Component Health Matrix**: Detailed health status by namespace and component
- **Recent Operations & Errors**: Chronological view of failed operations
- **Performance Diagnostics**: Detailed latency distribution and resource analysis
- **External Dependencies**: Kubernetes API and other service health
- **Error Pattern Analysis**: Common failure modes and troubleshooting guidance

**Target Audience**: On-call engineers, developers, incident responders
**Refresh Rate**: 5 seconds during active troubleshooting

## SLI/SLO Framework

### Service Level Indicators (SLIs)

#### 1. Roost Availability
```yaml
query: |
  sum(rate(roost_keeper_health_check_success_total[5m])) /
  sum(rate(roost_keeper_health_check_total[5m]))
threshold: 0.995  # 99.5% availability target
```

#### 2. Roost Creation Latency  
```yaml
query: |
  histogram_quantile(0.95, 
    sum(rate(roost_keeper_helm_install_duration_seconds_bucket[5m])) by (le))
threshold: 30  # 30 seconds P95 target
```

#### 3. Health Check Latency
```yaml
query: |
  histogram_quantile(0.95,
    sum(rate(roost_keeper_health_check_duration_seconds_bucket[5m])) by (le))
threshold: 1  # 1 second P95 target
```

#### 4. Controller Error Rate
```yaml
query: |
  sum(rate(roost_keeper_reconcile_errors_total[5m])) /
  sum(rate(roost_keeper_reconcile_total[5m]))
threshold: 0.01  # 1% error rate target
```

### Service Level Objectives (SLOs)

#### Availability SLO
- **Target**: 99.5% uptime over 30 days
- **Error Budget**: 0.5% (3.6 hours/month)
- **Alerting**: Page at 99.0%, ticket at 99.2%

#### Creation Latency SLO
- **Target**: 95% of roost creations complete under 30 seconds
- **Window**: 7 days rolling
- **Alerting**: Page at 60s, ticket at 45s

#### Health Check Latency SLO
- **Target**: 95% of health checks complete under 1 second
- **Window**: 7 days rolling
- **Alerting**: Page at 5s, ticket at 2s

#### Error Rate SLO
- **Target**: Less than 1% controller errors over 7 days
- **Error Budget**: 4% additional errors allowed
- **Alerting**: Page at 5%, ticket at 2%

## Alerting Strategy

### Multi-Window, Multi-Burn-Rate Alerting

Our alerting system implements Google's recommended multi-window, multi-burn-rate approach:

#### Fast Burn Alerts (Critical)
- **Trigger**: 2% error budget consumed in 1 hour
- **Math**: 14.4x normal burn rate
- **Action**: Page on-call engineer immediately
- **Channel**: #roost-keeper-oncall

#### Slow Burn Alerts (Warning)  
- **Trigger**: 10% error budget consumed in 6 hours
- **Math**: 6x normal burn rate
- **Action**: Create ticket for investigation
- **Channel**: #roost-keeper-slo

### Alert Hierarchy

#### Critical Alerts
1. **RoostKeeperOperatorDown**: Operator not responding (30s)
2. **RoostAvailabilitySLOCriticalBreach**: Fast availability burn (2m)
3. **RoostControllerErrorRateSLOCriticalBreach**: Error rate >5% (2m)

#### Warning Alerts
1. **RoostAvailabilitySLOWarningBreach**: Slow availability burn (15m)
2. **RoostCreationLatencySLOBreach**: P95 latency >60s (3m)
3. **RoostHealthCheckLatencySLOBreach**: P95 latency >5s (1m)
4. **RoostKeeperHighQueueDepth**: Queue >50 items (5m)
5. **RoostKeeperHighMemoryUsage**: Memory >90% (3m)

### Alert Inhibition Rules

Smart alert suppression prevents notification spam:
- Operator down alerts suppress resource usage alerts
- Critical SLO alerts suppress warning SLO alerts for same service
- Infrastructure alerts suppress application alerts when infrastructure fails

## Slack Integration

### Channel Strategy

#### #roost-keeper-oncall
- **Purpose**: Critical alerts requiring immediate attention
- **SLA**: Acknowledge within 5 minutes
- **Escalation**: PagerDuty integration for persistent alerts

#### #roost-keeper-slo  
- **Purpose**: SLO breach notifications and error budget tracking
- **SLA**: Investigate within 2 hours
- **Content**: Error budget status, burn rate analysis

#### #roost-keeper-platform
- **Purpose**: Warning-level operational alerts
- **SLA**: Investigate within 24 hours
- **Content**: Performance degradation, resource usage

### Notification Features

- **Rich Context**: Each alert includes current metrics, dashboard links, runbooks
- **Action Buttons**: Direct links to debug dashboard, runbooks, acknowledgment
- **Correlation IDs**: Automatic request tracing for faster debugging
- **Template System**: Consistent formatting with emoji indicators

## Deployment Guide

### Prerequisites

1. **Prometheus**: Collecting Roost-Keeper metrics
2. **Grafana**: Version 9.0+ with Prometheus data source
3. **Alertmanager**: For alert routing and Slack integration
4. **Slack App**: With incoming webhook permissions

### Step 1: Deploy Dashboards

```bash
# Import dashboards to Grafana
curl -X POST \
  http://grafana:3000/api/dashboards/db \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -d @observability/dashboards/01-operational.json

curl -X POST \
  http://grafana:3000/api/dashboards/db \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -d @observability/dashboards/02-sli-slo.json

curl -X POST \
  http://grafana:3000/api/dashboards/db \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -d @observability/dashboards/03-debug.json
```

### Step 2: Configure SLI/SLO Definitions

```bash
# Deploy SLI/SLO ConfigMap
kubectl apply -f observability/sli-slo/definitions.yaml
```

### Step 3: Deploy Alerting Rules

```bash
# Deploy Prometheus rules
kubectl apply -f observability/alerts/alerting-rules.yaml
```

### Step 4: Setup Slack Integration

```bash
# Configure Slack webhook
./scripts/setup-slack-webhook.sh https://hooks.slack.com/services/YOUR/WEBHOOK/URL

# Deploy Alertmanager configuration
kubectl apply -f observability/alerts/slack-notifications.yaml
```

### Step 5: Verification

```bash
# Run integration tests
go test ./test/integration/observability_dashboards_test.go -v

# Check dashboard accessibility
curl http://grafana:3000/d/roost-keeper-operational
curl http://grafana:3000/d/roost-keeper-sli-slo  
curl http://grafana:3000/d/roost-keeper-debug
```

## On-Call Workflows

### High Error Rate Incident

1. **Alert**: RoostControllerErrorRateSLOCriticalBreach fired
2. **Triage**: Check operational dashboard error rate panel
3. **Investigate**: Switch to debug dashboard
   - Examine recent reconcile errors table
   - Look for correlation ID patterns
   - Check resource usage metrics
4. **Diagnose**: Common causes:
   - Kubernetes API server issues
   - Resource constraints (CPU, memory)
   - Invalid roost configurations
   - Network connectivity problems
5. **Resolve**: Apply appropriate fix based on root cause
6. **Monitor**: Verify error rate returns to baseline

### Availability Degradation

1. **Alert**: RoostAvailabilitySLOCriticalBreach fired
2. **Triage**: Check SLI/SLO dashboard availability panel
3. **Investigate**: Switch to debug dashboard
   - Review health check failures table
   - Examine external dependencies section
   - Check component health matrix
4. **Diagnose**: Common causes:
   - Health check endpoint failures
   - Network partitions
   - Service mesh issues
   - DNS resolution problems
5. **Resolve**: Address underlying service issues
6. **Monitor**: Track error budget recovery

### Performance Issues

1. **Alert**: RoostCreationLatencySLOBreach fired
2. **Triage**: Check operational dashboard latency trends
3. **Investigate**: Switch to debug dashboard
   - Examine response time percentiles
   - Check queue depths
   - Review resource utilization
4. **Diagnose**: Common causes:
   - High reconcile queue depth
   - Slow Helm operations
   - Resource contention
   - External API latency
5. **Resolve**: Optimize bottlenecks
6. **Monitor**: Verify latency improvements

## Metrics Reference

### Core Metrics Used in Dashboards

#### Reconciliation Metrics
- `roost_keeper_reconcile_total`: Total reconcile operations
- `roost_keeper_reconcile_errors_total`: Failed reconcile operations  
- `roost_keeper_reconcile_duration_seconds`: Reconcile latency histogram
- `roost_keeper_reconcile_queue_length`: Current queue depth

#### Health Check Metrics
- `roost_keeper_health_check_total`: Total health checks
- `roost_keeper_health_check_success_total`: Successful health checks
- `roost_keeper_health_check_errors_total`: Failed health checks
- `roost_keeper_health_check_duration_seconds`: Health check latency histogram

#### Helm Operation Metrics
- `roost_keeper_helm_install_total`: Helm installations
- `roost_keeper_helm_install_errors_total`: Failed installations
- `roost_keeper_helm_install_duration_seconds`: Install latency histogram
- `roost_keeper_helm_upgrade_total`: Helm upgrades
- `roost_keeper_helm_upgrade_errors_total`: Failed upgrades

#### Roost Status Metrics
- `roost_keeper_roosts_total`: Total roosts by namespace
- `roost_keeper_roosts_healthy`: Healthy roosts by namespace
- `roost_keeper_roosts_by_phase`: Roosts grouped by phase (Running, Failed, etc.)

#### System Metrics
- `roost_keeper_memory_usage_bytes`: Controller memory usage
- `roost_keeper_cpu_usage_ratio`: Controller CPU utilization
- `roost_keeper_goroutine_count`: Active goroutines

#### External Dependency Metrics
- `roost_keeper_kubernetes_api_requests_total`: K8s API requests
- `roost_keeper_kubernetes_api_errors_total`: K8s API errors
- `roost_keeper_kubernetes_api_latency_seconds`: K8s API latency

## Customization Guide

### Adding New SLIs

1. **Define the SLI** in `observability/sli-slo/definitions.yaml`:
```yaml
new_sli_name:
  description: "Your SLI description"
  query: "your_prometheus_query"
  threshold: 0.99
```

2. **Create corresponding SLO**:
```yaml
new_sli_name_slo:
  sli: new_sli_name
  objective: 0.99
  window: "7d"
  error_budget: 0.05
```

3. **Add alerting rule** in `observability/alerts/alerting-rules.yaml`
4. **Update dashboards** to include new SLI visualization
5. **Test** with integration tests

### Modifying Alert Thresholds

1. **Update SLO definitions** with new objectives
2. **Recalculate burn rates** using the formula:
   ```
   burn_rate = (budget_consumption_rate / time_window) / error_budget_rate
   ```
3. **Update alert expressions** in alerting rules
4. **Test alerts** in staging environment

### Custom Dashboard Panels

1. **Choose appropriate visualization** (graph, stat, table)
2. **Write Prometheus query** using available metrics
3. **Configure thresholds** and alerting if needed
4. **Add to appropriate dashboard** based on audience
5. **Update cross-links** to maintain navigation

## Troubleshooting

### Dashboard Issues

#### Dashboard Not Loading
- Check Grafana data source configuration
- Verify Prometheus connectivity
- Ensure dashboard JSON is valid

#### Missing Data Points
- Verify metrics are being scraped by Prometheus
- Check metric label consistency
- Confirm time range alignment

#### Slow Query Performance  
- Optimize Prometheus queries with recording rules
- Reduce query time ranges for high-cardinality metrics
- Use appropriate aggregation functions

### Alerting Issues

#### Alerts Not Firing
- Check Prometheus rule evaluation
- Verify alert expressions syntax
- Confirm Alertmanager configuration

#### Too Many False Positives
- Increase alert duration thresholds
- Add context-aware inhibition rules
- Fine-tune SLO burn rate calculations

#### Missing Slack Notifications
- Verify webhook URL configuration
- Check Alertmanager routing rules
- Test webhook with manual alert

## Performance Considerations

### Dashboard Optimization

- **Query Caching**: Use recording rules for expensive queries
- **Time Range Limits**: Prevent excessive historical data loading
- **Panel Refresh Rates**: Balance real-time needs with system load
- **Variable Usage**: Minimize dashboard variable complexity

### Prometheus Optimization

- **Recording Rules**: Pre-compute SLI calculations
- **Retention Policy**: Balance storage costs with historical needs
- **Scrape Intervals**: Adjust based on metric update frequency
- **Label Cardinality**: Monitor and control high-cardinality metrics

## Security Considerations

### Access Control

- **Dashboard Permissions**: Role-based access to sensitive operational data
- **API Keys**: Secure Grafana API key management
- **Webhook Security**: Protect Slack webhook URLs
- **Network Policies**: Restrict Prometheus and Grafana network access

### Data Privacy

- **Metric Sanitization**: Avoid exposing sensitive data in labels
- **Log Aggregation**: Secure correlation ID handling
- **Retention Policies**: Comply with data retention requirements

## Future Enhancements

### Advanced Features

- **Anomaly Detection**: ML-based alerting for unusual patterns
- **Predictive SLOs**: Forecast error budget exhaustion
- **Multi-Cluster**: Aggregated dashboards across environments
- **Custom Runbooks**: Interactive troubleshooting guides

### Integration Opportunities

- **Incident Management**: PagerDuty/Opsgenie integration
- **Change Management**: Deployment correlation with SLO impact
- **Cost Optimization**: Resource usage vs performance analysis
- **Business Metrics**: User-facing SLIs and business impact correlation

## Conclusion

This observability dashboard system provides comprehensive monitoring, alerting, and troubleshooting capabilities for Roost-Keeper. The implementation follows SRE best practices and provides the foundation for reliable service operation.

The three-dashboard approach (Operational, SLI/SLO, Debug) ensures appropriate information is available to each audience while maintaining system performance. The comprehensive alerting system with multi-window burn rate detection enables proactive incident response before user impact occurs.

Regular review and optimization of SLOs, alert thresholds, and dashboard content will ensure the observability system continues to meet evolving operational needs.
