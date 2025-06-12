# Security Hardening Implementation

## Overview

This document describes the implementation of trust-based security hardening for Roost-Keeper. The security system is designed for internal environments where users are trusted, focusing on operational security, audit logging, and accident prevention rather than threat protection from malicious actors.

## Architecture

### Trust-Based Security Model

```
┌─────────────────────────────────────────────────────────────┐
│                    Security Manager                         │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────── │
│  │ Policy Engine   │  │ Secret Manager  │  │ Audit Logger   │
│  │ (Monitor Mode)  │  │ (Doppler)       │  │ (Structured)   │
│  │ - Log violations│  │ - API Client    │  │ - Event Trail  │
│  │ - Metrics       │  │ - Sync to K8s   │  │ - Violations   │
│  │ - Alerts        │  │ - Rotation      │  │ - Access Logs  │
│  └─────────────────┘  └─────────────────┘  └─────────────── │
│               │                │                │           │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │            Configuration Validator                      │ │
│  │         (Prevent Configuration Mistakes)               │ │
│  └─────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

## Key Components

### 1. Security Manager (`internal/security/manager.go`)

The central orchestrator for all security operations:

```go
type SecurityManager struct {
    client          client.Client
    secretManager   *DopplerSecretManager
    policyEngine    *TrustBasedPolicyEngine
    auditLogger     *SecurityAuditLogger
    configValidator *ConfigurationValidator
    logger          *zap.Logger
    metrics         *telemetry.OperatorMetrics
}
```

**Key Features:**
- **Trust-Based Architecture**: Logs violations but doesn't block operations
- **Doppler Integration**: Secure secret management with automatic sync
- **Comprehensive Auditing**: Complete trail of all security events
- **Performance Monitoring**: Tracks security operation performance

### 2. Trust-Based Policy Engine

Lightweight security policies designed for trusted environments:

```go
type TrustPolicy interface {
    Name() string
    Validate(ctx context.Context, resource interface{}) *PolicyResult
    Mode() PolicyMode
}

const (
    PolicyModeMonitor PolicyMode = "monitor" // Log violations, don't block
    PolicyModeWarn    PolicyMode = "warn"    // Log warnings for attention
    PolicyModeInfo    PolicyMode = "info"    // Informational logging
)
```

**Built-in Policies:**
- **Resource Limits Policy**: Prevents resource exhaustion
- **Basic Security Policy**: Monitors container security settings
- **Configuration Policy**: Validates common configurations
- **Operational Policy**: Checks operational best practices

### 3. Doppler Secret Management

Integrated secret management using Doppler:

```go
type DopplerSecretManager struct {
    logger      *zap.Logger
    dopplerAPI  *DopplerAPIClient
    syncManager *SecretSyncManager
}
```

**Features:**
- **Automatic Sync**: Periodic secret synchronization (5-minute default)
- **Rotation Support**: Automated secret rotation capabilities
- **Audit Trail**: Complete logging of secret access patterns
- **Project Management**: Per-roost Doppler project isolation

### 4. Security Audit Logger

Comprehensive audit logging for all security events:

```go
type SecurityAuditLogger struct {
    logger *zap.Logger
}
```

**Audit Capabilities:**
- **Resource Access**: Tracks all resource creation/modification
- **Secret Access**: Logs secret retrieval and usage patterns
- **Policy Violations**: Records policy violations with context
- **Configuration Changes**: Tracks security configuration modifications

### 5. Security Webhook Handler

Integrates security policies with Kubernetes admission control:

```go
type SecurityWebhookHandler struct {
    securityManager *SecurityManager
    logger          *zap.Logger
    metrics         *telemetry.OperatorMetrics
    decoder         admission.Decoder
}
```

**Webhook Features:**
- **Trust-Based Validation**: Validates but doesn't block resources
- **Comprehensive Logging**: Logs all admission requests
- **Resource-Specific Policies**: Different validation for different resource types
- **Secret Detection**: Special handling for Doppler-managed secrets

## Security Features

### 1. Operational Security

**Focus**: Accident prevention and operational excellence
- **Resource Limits**: Warns about missing CPU/memory limits
- **Security Contexts**: Monitors privileged container usage
- **Host Access**: Tracks host network/PID namespace usage
- **Configuration Validation**: Prevents common misconfigurations

### 2. Secret Management

**Doppler Integration**:
- **Project Isolation**: Each roost gets its own Doppler project
- **Automatic Sync**: Secrets sync every 5 minutes
- **Rotation Support**: Configurable secret rotation
- **Kubernetes Integration**: Seamless secret injection

**Security Features**:
- **Access Logging**: Complete audit trail of secret access
- **Sensitive Detection**: Identifies potentially sensitive secret names
- **Lifecycle Management**: Tracks secret creation, rotation, and deletion

### 3. Audit and Compliance

**Comprehensive Logging**:
```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "info",
  "component": "security-audit",
  "event_type": "resource_access",
  "user": "admin@company.com",
  "action": "CREATE",
  "resource": "ManagedRoost/my-roost",
  "namespace": "production",
  "allowed": true,
  "reason": "Trust-based access granted"
}
```

**Audit Events**:
- Resource access and modifications
- Secret access patterns
- Policy violations (monitoring mode)
- Configuration changes
- User actions and API calls

### 4. Performance Monitoring

**Security Metrics**:
- Policy evaluation latency
- Secret sync performance
- Audit log processing time
- Security webhook response times

**Integration with Existing Telemetry**:
- Extends existing `OperatorMetrics`
- Prometheus-compatible metrics
- Grafana dashboard integration
- Alert configuration support

## Configuration

### Basic Security Configuration

```yaml
apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: secure-roost
spec:
  security:
    enabled: true
    mode: "trust-based"
    
    audit:
      enabled: true
      logLevel: "info"
      retention: "30d"
    
    secrets:
      provider: "doppler"
      sync:
        enabled: true
        interval: "5m"
        autoRotate: true
    
    policies:
      resourceLimits:
        enabled: true
        mode: "warn"
        enforce: false
      
      basicSecurity:
        enabled: true
        mode: "monitor"
        enforce: false
```

### Advanced Configuration

See `config/samples/security_managedroost.yaml` for a complete example with:
- RBAC integration with security contexts
- Network security policies
- Performance monitoring thresholds
- Observability integration

## Integration Points

### 1. RBAC System Integration

Enhances existing RBAC with security features:
- **Security Labels**: Adds security metadata to RBAC resources
- **Audit Integration**: Logs RBAC operations
- **Policy Validation**: Validates RBAC configurations

### 2. Admission Control Webhook Integration

Extends existing webhook system:
- **Security Validation**: Adds security policy validation
- **Trust-Based Mode**: Logs violations without blocking
- **Resource-Specific Logic**: Different validation for different resources

### 3. Observability Integration

Leverages existing telemetry infrastructure:
- **Metrics Extension**: Adds security metrics to existing framework
- **Structured Logging**: Integrates with existing logging system
- **Tracing Support**: Security operations are traced

### 4. Performance System Integration

Works with existing performance optimization:
- **Connection Pooling**: Reuses existing database connections
- **Caching**: Leverages existing cache infrastructure
- **Worker Pools**: Uses existing worker pool for async operations

## Deployment

### 1. Security Manager Setup

The security manager is automatically initialized when security is enabled:

```go
// In the main controller
if managedRoost.Spec.Security != nil && managedRoost.Spec.Security.Enabled {
    securityMgr := security.NewSecurityManager(client, logger, metrics)
    err := securityMgr.SetupSecurity(ctx, managedRoost.Name, managedRoost.Namespace)
    if err != nil {
        return fmt.Errorf("failed to setup security: %w", err)
    }
}
```

### 2. Webhook Registration

Security webhook handlers integrate with existing admission control:

```go
// Register security webhook
securityHandler := security.NewSecurityWebhookHandler(securityMgr, logger, metrics)
webhookServer.Register("/validate-security", &webhook.Admission{Handler: securityHandler})
```

### 3. Doppler Configuration

Doppler configuration requires API token setup:

```bash
# Set Doppler API token
export DOPPLER_API_TOKEN="your-api-token"

# Doppler projects are created automatically per roost
# Project naming: roost-{roost-name}
```

## Monitoring and Alerting

### Security Metrics

Available Prometheus metrics:

```
# Policy violations
roost_keeper_security_policy_violations_total{policy="resource-limits",severity="warning"}

# Secret operations
roost_keeper_security_secret_operations_total{operation="sync",status="success"}

# Audit events
roost_keeper_security_audit_events_total{event_type="resource_access",user="admin"}

# Performance
roost_keeper_security_operation_duration_seconds{operation="policy_evaluation"}
```

### Grafana Dashboard

Security-specific panels:
- **Trust Score**: Overall security posture
- **Policy Violations**: Real-time violation tracking
- **Secret Management**: Doppler sync status and performance
- **Audit Activity**: User activity and access patterns

### Alerting Rules

```yaml
groups:
  - name: roost-keeper-security
    rules:
      - alert: HighPolicyViolationRate
        expr: rate(roost_keeper_security_policy_violations_total[5m]) > 0.1
        labels:
          severity: warning
        annotations:
          summary: "High rate of security policy violations"
      
      - alert: SecretSyncFailure
        expr: roost_keeper_security_secret_operations_total{status="failed"} > 0
        labels:
          severity: critical
        annotations:
          summary: "Doppler secret sync failure detected"
```

## Testing

Comprehensive test coverage in `test/integration/security_test.go`:

- **Security Manager**: Basic functionality and setup
- **Policy Engine**: Policy registration and evaluation
- **Audit Logger**: Event logging and trail initialization
- **Secret Manager**: Doppler integration and sync
- **Webhook Handler**: Admission control integration

Run security tests:

```bash
go test ./test/integration -run TestSecurity -v
```

## Best Practices

### 1. Trust-Based Security

- **Monitor, Don't Block**: Log violations for visibility without disrupting operations
- **Comprehensive Auditing**: Maintain complete audit trails for compliance
- **Performance First**: Measure security operation impact and optimize accordingly

### 2. Secret Management

- **Project Isolation**: Use separate Doppler projects per roost
- **Regular Rotation**: Enable automatic secret rotation
- **Access Logging**: Monitor secret access patterns for anomalies

### 3. Policy Configuration

- **Start with Warnings**: Begin with warning-level policies before enforcement
- **Gradual Rollout**: Enable policies incrementally across environments
- **Performance Monitoring**: Track policy evaluation performance

### 4. Operational Excellence

- **Dashboard Monitoring**: Use Grafana dashboards for real-time visibility
- **Alert Configuration**: Set up alerts for security events
- **Regular Reviews**: Periodically review audit logs and policy violations

## Troubleshooting

### Common Issues

1. **Doppler Sync Failures**
   - Check API token validity
   - Verify project permissions
   - Review network connectivity

2. **Policy Evaluation Errors**
   - Check resource validation logic
   - Verify policy configuration
   - Review error logs

3. **Webhook Performance Issues**
   - Monitor webhook response times
   - Check admission controller resource limits
   - Review concurrent request handling

### Debug Commands

```bash
# Check security manager logs
kubectl logs -n roost-system deployment/roost-keeper-manager | grep "security"

# Review audit events
kubectl logs -n roost-system deployment/roost-keeper-manager | grep "security-audit"

# Monitor Doppler sync
kubectl logs -n roost-system deployment/roost-keeper-manager | grep "doppler"
```

## Security Considerations

### 1. API Token Security

- **Doppler API Token**: Store securely as Kubernetes secret
- **Rotation**: Regularly rotate API tokens
- **Permissions**: Use least-privilege Doppler service accounts

### 2. Audit Log Security

- **Log Integrity**: Ensure audit logs cannot be modified
- **Retention**: Configure appropriate log retention periods
- **Access Control**: Restrict access to audit logs

### 3. Network Security

- **TLS**: All communication uses TLS encryption
- **Network Policies**: Basic network isolation when enabled
- **Service Mesh**: Compatible with service mesh security

## Future Enhancements

### 1. Enhanced Scanning (Optional)

If trust model changes, can add:
- **Trivy Integration**: Container vulnerability scanning
- **Cluster Configuration Scanning**: Kubernetes security assessment
- **Policy as Code**: OPA/Gatekeeper integration

### 2. Advanced Secret Management

- **Multiple Providers**: Support for Vault, AWS Secrets Manager
- **Cross-Environment Sync**: Secret propagation across environments
- **Secret Discovery**: Automatic detection of hardcoded secrets

### 3. Compliance Frameworks

- **SOC2 Compliance**: Automated control validation
- **PCI-DSS Support**: Payment card industry requirements
- **Custom Frameworks**: Extensible compliance rule engine

## Conclusion

The trust-based security hardening provides comprehensive operational security for Roost-Keeper while maintaining the performance and usability expected in internal environments. The system focuses on:

- **Operational Excellence**: Preventing accidents and misconfigurations
- **Complete Visibility**: Comprehensive audit trails and monitoring
- **Performance Optimization**: Minimal impact on system performance
- **Developer Experience**: Transparent security that doesn't impede productivity

This implementation serves as a solid foundation that can be enhanced with additional security features as requirements evolve.
