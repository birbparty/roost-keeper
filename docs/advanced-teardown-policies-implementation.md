# Advanced Teardown Policies Implementation

This document describes the implementation of advanced teardown policies for the Roost Keeper project, providing intelligent, automated teardown capabilities with comprehensive safety checks and audit trails.

## Overview

The advanced teardown policies system provides:

- **Multiple Trigger Types**: Timeout, failure count, resource threshold, schedule, health-based, and webhook triggers
- **Safety Checks**: Comprehensive safety validation before teardown execution
- **Data Preservation**: Automated backup and data preservation capabilities
- **Audit Trail**: Complete audit logging of all teardown decisions and executions
- **Metrics Collection**: Prometheus metrics for monitoring teardown operations
- **Manual Approval**: Optional manual approval workflows for critical teardowns
- **Dry Run Mode**: Safe testing of teardown policies without actual execution

## Architecture

The teardown system consists of several key components:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Teardown      │    │   Evaluators    │    │   Safety        │
│   Manager       │────│   (Plugins)     │────│   Checker       │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                                              │
         │              ┌─────────────────┐             │
         └──────────────│   Execution     │─────────────┘
                        │   Engine        │
                        └─────────────────┘
                                 │
                        ┌─────────────────┐
                        │   Audit &       │
                        │   Metrics       │
                        └─────────────────┘
```

## Core Components

### 1. Teardown Manager (`internal/teardown/manager.go`)

The central orchestrator that coordinates all teardown operations:

- **Policy Evaluation**: Evaluates teardown triggers against current roost state
- **Safety Validation**: Runs comprehensive safety checks
- **Execution Coordination**: Manages the teardown execution process
- **Metrics Collection**: Collects and reports teardown metrics
- **Configuration Management**: Handles teardown policy configuration

Key methods:
- `EvaluateTeardownPolicy()`: Evaluates if a roost should be torn down
- `ExecuteTeardown()`: Executes the teardown process
- `ValidateTeardownPolicy()`: Validates teardown policy configuration

### 2. Evaluators (`internal/teardown/evaluators.go`)

Plugin-based evaluators for different trigger types:

#### TimeoutEvaluator
- Triggers teardown after a specified time period
- Configurable timeout duration
- Calculates time elapsed since roost creation

#### FailureCountEvaluator
- Triggers teardown based on health check failure counts
- Aggregates failures across all health checks
- Configurable failure threshold

#### HealthBasedEvaluator
- Triggers teardown based on overall health status
- Monitors extended periods of unhealthy state
- Integrates with health checking system

#### ResourceThresholdEvaluator
- Triggers teardown based on resource usage thresholds
- Monitors CPU, memory, and storage usage
- Configurable threshold percentages

#### ScheduleEvaluator
- Triggers teardown based on cron-like schedules
- Supports complex scheduling patterns
- Useful for maintenance windows

#### WebhookEvaluator
- Triggers teardown based on external webhook responses
- Supports custom external decision logic
- Configurable HTTP endpoints and validation

### 3. Safety Checker (`internal/teardown/safety.go`)

Comprehensive safety validation system:

- **Active Connections**: Checks for active user or API connections
- **Persistent Volumes**: Validates data persistence requirements
- **Dependent Resources**: Identifies resources that depend on the roost
- **Running Pods**: Ensures graceful pod termination
- **External Endpoints**: Validates external service dependencies
- **Data Persistence**: Verifies backup and preservation requirements
- **Manual Approval**: Enforces manual approval workflows when required

### 4. Execution Engine (`internal/teardown/execution.go`)

Manages the actual teardown execution process:

1. **Pre-teardown Hooks**: Execute preparation scripts and notifications
2. **Graceful Shutdown**: Attempt graceful service shutdown
3. **Data Preservation**: Create backups and preserve specified resources
4. **Resource Teardown**: Remove Kubernetes resources and Helm releases
5. **Cleanup**: Final cleanup of temporary resources and credentials
6. **Post-teardown Hooks**: Execute completion notifications and reporting

### 5. Audit Logger (`internal/teardown/audit.go`)

Comprehensive audit trail system:

- **Decision Logging**: Records all teardown decisions with full context
- **Execution Logging**: Tracks execution steps and outcomes
- **Safety Check Logging**: Documents all safety check results
- **Structured Logging**: JSON-formatted logs for easy parsing
- **Retention Management**: Configurable log retention policies

### 6. Metrics Collector (`internal/teardown/metrics.go`)

Prometheus metrics collection:

- **Evaluation Metrics**: Count of policy evaluations and outcomes
- **Execution Metrics**: Duration and success/failure rates
- **Safety Check Metrics**: Safety check results and patterns
- **Trigger Type Metrics**: Breakdown by trigger type and effectiveness

## Configuration

### Basic Teardown Policy

```yaml
apiVersion: roost.birbparty.com/v1alpha1
kind: ManagedRoost
spec:
  teardownPolicy:
    triggers:
      - type: "timeout"
        timeout: "2h"
    requireManualApproval: false
    dataPreservation:
      enabled: true
    cleanup:
      gracePeriod: "30s"
      force: false
```

### Advanced Multi-Trigger Policy

```yaml
spec:
  teardownPolicy:
    triggers:
      # Time-based teardown
      - type: "timeout"
        timeout: "4h"
      
      # Health-based teardown
      - type: "failure_count"
        failureCount: 10
      
      # Resource-based teardown
      - type: "resource_threshold"
        resourceThreshold:
          memory: "85%"
          cpu: "80%"
      
      # Scheduled teardown
      - type: "schedule"
        schedule: "0 2 * * *"  # Daily at 2 AM
      
      # External webhook trigger
      - type: "webhook"
        webhook:
          url: "https://api.example.com/teardown-decision"
          headers:
            Authorization: "Bearer ${TOKEN}"
          expectedResponse: "teardown"

    # Safety and approval settings
    requireManualApproval: true
    approvers:
      - "team-lead@company.com"
      - "ops-team@company.com"

    # Data preservation
    dataPreservation:
      enabled: true
      backupPolicy:
        schedule: "0 1 * * *"
        retention: "30d"
        storage:
          type: "s3"
          config:
            bucket: "roost-backups"
            region: "us-west-2"
      preserveResources:
        - "PersistentVolumeClaim"
        - "Secret"
        - "ConfigMap"

    # Cleanup configuration
    cleanup:
      gracePeriod: "60s"
      force: true
      order:
        - "Deployment"
        - "Service"
        - "Ingress"
        - "ConfigMap"
        # PVCs preserved by dataPreservation
```

## Usage Examples

### 1. Programmatic Evaluation

```go
// Create teardown manager
manager, err := teardown.NewManager(client, healthChecker, eventManager, logger, config)
if err != nil {
    return fmt.Errorf("failed to create teardown manager: %w", err)
}

// Evaluate teardown policy
decision, err := manager.EvaluateTeardownPolicy(ctx, roost)
if err != nil {
    return fmt.Errorf("failed to evaluate teardown policy: %w", err)
}

if decision.ShouldTeardown {
    // Execute teardown if policy conditions are met
    execution, err := manager.ExecuteTeardown(ctx, roost, decision)
    if err != nil {
        return fmt.Errorf("failed to execute teardown: %w", err)
    }
    
    log.Info("Teardown completed", 
        zap.String("execution_id", execution.ExecutionID),
        zap.String("reason", decision.Reason))
}
```

### 2. CLI Integration

```bash
# Evaluate teardown policy (dry run)
roost teardown evaluate my-roost --dry-run

# Execute teardown with manual confirmation
roost teardown execute my-roost --confirm

# View teardown history
roost teardown history my-roost

# Get teardown metrics
roost teardown metrics
```

### 3. Webhook Integration

```go
// In your controller
func (r *ManagedRoostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... fetch roost ...
    
    // Evaluate teardown policy during reconciliation
    if r.teardownManager != nil {
        decision, err := r.teardownManager.EvaluateTeardownPolicy(ctx, roost)
        if err != nil {
            return ctrl.Result{RequeueAfter: time.Minute}, err
        }
        
        if decision.ShouldTeardown && !decision.RequiresManualApproval {
            _, err := r.teardownManager.ExecuteTeardown(ctx, roost, decision)
            if err != nil {
                return ctrl.Result{RequeueAfter: time.Minute}, err
            }
            // Roost will be deleted, no need to requeue
            return ctrl.Result{}, nil
        }
    }
    
    // ... continue normal reconciliation ...
}
```

## Safety Features

### Pre-Execution Validation

1. **Configuration Validation**: Ensures teardown policy is properly configured
2. **Resource Dependencies**: Identifies dependent resources that might be affected
3. **Data Persistence**: Validates backup requirements and data preservation settings
4. **Active Connections**: Checks for active user sessions or API connections
5. **External Dependencies**: Validates external service dependencies

### Execution Safety

1. **Graceful Shutdown**: Attempts graceful termination before force deletion
2. **Rollback Capability**: Maintains rollback information where possible
3. **Step-by-Step Execution**: Breaks teardown into recoverable steps
4. **Error Handling**: Comprehensive error handling with recovery options
5. **Timeout Protection**: Prevents indefinite hanging during teardown

### Post-Execution Audit

1. **Complete Audit Trail**: Every action is logged with full context
2. **Metrics Collection**: Performance and success metrics are recorded
3. **Notification System**: Stakeholders are notified of teardown completion
4. **Cleanup Verification**: Ensures all resources are properly cleaned up

## Monitoring and Observability

### Prometheus Metrics

```
# Teardown evaluations
teardown_evaluations_total
teardown_executions_total
teardown_execution_duration_seconds
teardown_active_executions

# Safety checks
teardown_safety_checks_total{check_name, result}

# Trigger types
teardown_trigger_types_total{trigger_type, result}
```

### Log Structure

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "component": "teardown-manager",
  "roost_name": "my-roost",
  "roost_namespace": "default",
  "correlation_id": "abc123",
  "event_type": "teardown_decision",
  "decision": {
    "should_teardown": true,
    "reason": "Timeout reached: 2h elapsed since creation",
    "trigger_type": "timeout",
    "urgency": "medium",
    "safety_checks_passed": 8,
    "safety_checks_failed": 0
  }
}
```

## Testing

### Unit Tests

- Individual evaluator testing
- Safety checker validation
- Metrics collection verification
- Configuration validation

### Integration Tests

- End-to-end teardown workflows
- Multi-trigger scenarios
- Safety check integration
- Error handling and recovery

### Load Tests

- High-volume policy evaluation
- Concurrent teardown execution
- Metrics collection performance
- Resource cleanup efficiency

## Best Practices

### Configuration

1. **Start Conservative**: Begin with longer timeouts and manual approval
2. **Layer Safety Checks**: Use multiple complementary trigger types
3. **Test Thoroughly**: Use dry-run mode extensively during setup
4. **Monitor Metrics**: Set up alerting on teardown metrics
5. **Document Policies**: Maintain clear documentation of teardown policies

### Operations

1. **Regular Review**: Periodically review and update teardown policies
2. **Incident Response**: Have procedures for teardown failures
3. **Backup Validation**: Regularly test data preservation and restoration
4. **Access Control**: Restrict teardown configuration to authorized users
5. **Audit Compliance**: Maintain audit trails for compliance requirements

## Future Enhancements

### Planned Features

1. **Machine Learning Integration**: Predictive teardown based on usage patterns
2. **Cost Optimization**: Integration with cloud cost management APIs
3. **Advanced Scheduling**: More sophisticated cron expressions and timezone support
4. **Custom Safety Checks**: Plugin architecture for custom safety validators
5. **Notification Integrations**: Slack, Teams, and other notification channels

### Extensibility

The teardown system is designed for extensibility:

- **Custom Evaluators**: Implement the `TeardownEvaluator` interface
- **Custom Safety Checks**: Add new safety validation logic
- **Custom Metrics**: Extend metrics collection for specific needs
- **Custom Audit Formats**: Implement alternative audit output formats

## Conclusion

The advanced teardown policies implementation provides a robust, safe, and auditable system for automatically managing the lifecycle of Kubernetes workloads deployed through the Roost Keeper system. With comprehensive safety checks, multiple trigger types, and extensive observability, teams can confidently automate their teardown processes while maintaining full control and visibility.
