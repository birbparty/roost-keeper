package tenancy

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	tenancyMeterName = "roost-keeper-tenancy"
)

// TenancyMetrics contains all metrics for tenant operations
type TenancyMetrics struct {
	// Tenant lifecycle metrics
	TenantOperationsTotal   metric.Int64Counter
	TenantOperationDuration metric.Float64Histogram
	TenantOperationErrors   metric.Int64Counter
	TenantsActive           metric.Int64UpDownCounter

	// Tenant isolation metrics
	TenantIsolationViolations metric.Int64Counter
	TenantAccessAttempts      metric.Int64Counter
	TenantAccessDenied        metric.Int64Counter

	// Resource quota metrics
	TenantResourceUsage    metric.Float64Gauge
	TenantQuotaViolations  metric.Int64Counter
	TenantQuotaUtilization metric.Float64Gauge

	// Network policy metrics
	TenantNetworkPolicies   metric.Int64UpDownCounter
	TenantNetworkViolations metric.Int64Counter

	// RBAC metrics
	TenantRBACResources metric.Int64UpDownCounter
	TenantRBACErrors    metric.Int64Counter
}

// TenantMetrics represents metrics for a specific tenant
type TenantMetrics struct {
	TenantID          string
	OperationsTotal   int64
	OperationErrors   int64
	LastOperationTime time.Time
	ResourceUsage     map[string]float64
	QuotaViolations   int64
	NetworkPolicies   int64
	AccessViolations  int64
	RBACResources     int64
}

// NewTenancyMetrics creates and initializes all tenancy metrics
func NewTenancyMetrics() (*TenancyMetrics, error) {
	meter := otel.Meter(tenancyMeterName)

	// Tenant lifecycle metrics
	tenantOperationsTotal, err := meter.Int64Counter(
		"roost_keeper_tenant_operations_total",
		metric.WithDescription("Total number of tenant operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	tenantOperationDuration, err := meter.Float64Histogram(
		"roost_keeper_tenant_operation_duration_seconds",
		metric.WithDescription("Duration of tenant operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	tenantOperationErrors, err := meter.Int64Counter(
		"roost_keeper_tenant_operation_errors_total",
		metric.WithDescription("Total number of tenant operation errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	tenantsActive, err := meter.Int64UpDownCounter(
		"roost_keeper_tenants_active",
		metric.WithDescription("Number of active tenants"),
		metric.WithUnit("{tenants}"),
	)
	if err != nil {
		return nil, err
	}

	// Tenant isolation metrics
	tenantIsolationViolations, err := meter.Int64Counter(
		"roost_keeper_tenant_isolation_violations_total",
		metric.WithDescription("Total number of tenant isolation violations"),
		metric.WithUnit("{violations}"),
	)
	if err != nil {
		return nil, err
	}

	tenantAccessAttempts, err := meter.Int64Counter(
		"roost_keeper_tenant_access_attempts_total",
		metric.WithDescription("Total number of tenant access attempts"),
		metric.WithUnit("{attempts}"),
	)
	if err != nil {
		return nil, err
	}

	tenantAccessDenied, err := meter.Int64Counter(
		"roost_keeper_tenant_access_denied_total",
		metric.WithDescription("Total number of denied tenant access attempts"),
		metric.WithUnit("{denials}"),
	)
	if err != nil {
		return nil, err
	}

	// Resource quota metrics
	tenantResourceUsage, err := meter.Float64Gauge(
		"roost_keeper_tenant_resource_usage",
		metric.WithDescription("Current resource usage by tenant"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	tenantQuotaViolations, err := meter.Int64Counter(
		"roost_keeper_tenant_quota_violations_total",
		metric.WithDescription("Total number of tenant quota violations"),
		metric.WithUnit("{violations}"),
	)
	if err != nil {
		return nil, err
	}

	tenantQuotaUtilization, err := meter.Float64Gauge(
		"roost_keeper_tenant_quota_utilization",
		metric.WithDescription("Tenant quota utilization percentage"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	// Network policy metrics
	tenantNetworkPolicies, err := meter.Int64UpDownCounter(
		"roost_keeper_tenant_network_policies",
		metric.WithDescription("Number of network policies per tenant"),
		metric.WithUnit("{policies}"),
	)
	if err != nil {
		return nil, err
	}

	tenantNetworkViolations, err := meter.Int64Counter(
		"roost_keeper_tenant_network_violations_total",
		metric.WithDescription("Total number of tenant network violations"),
		metric.WithUnit("{violations}"),
	)
	if err != nil {
		return nil, err
	}

	// RBAC metrics
	tenantRBACResources, err := meter.Int64UpDownCounter(
		"roost_keeper_tenant_rbac_resources",
		metric.WithDescription("Number of RBAC resources per tenant"),
		metric.WithUnit("{resources}"),
	)
	if err != nil {
		return nil, err
	}

	tenantRBACErrors, err := meter.Int64Counter(
		"roost_keeper_tenant_rbac_errors_total",
		metric.WithDescription("Total number of tenant RBAC errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	return &TenancyMetrics{
		TenantOperationsTotal:     tenantOperationsTotal,
		TenantOperationDuration:   tenantOperationDuration,
		TenantOperationErrors:     tenantOperationErrors,
		TenantsActive:             tenantsActive,
		TenantIsolationViolations: tenantIsolationViolations,
		TenantAccessAttempts:      tenantAccessAttempts,
		TenantAccessDenied:        tenantAccessDenied,
		TenantResourceUsage:       tenantResourceUsage,
		TenantQuotaViolations:     tenantQuotaViolations,
		TenantQuotaUtilization:    tenantQuotaUtilization,
		TenantNetworkPolicies:     tenantNetworkPolicies,
		TenantNetworkViolations:   tenantNetworkViolations,
		TenantRBACResources:       tenantRBACResources,
		TenantRBACErrors:          tenantRBACErrors,
	}, nil
}

// RecordTenantOperation records a tenant operation with its outcome
func (m *TenancyMetrics) RecordTenantOperation(ctx context.Context, operation, tenantID string, duration time.Duration, success bool) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("tenant_id", tenantID),
		attribute.Bool("success", success),
	)

	m.TenantOperationsTotal.Add(ctx, 1, labels)
	m.TenantOperationDuration.Record(ctx, duration.Seconds(), labels)

	if !success {
		m.TenantOperationErrors.Add(ctx, 1, labels)
	}
}

// IncrementTenantsActive increments the count of active tenants
func (m *TenancyMetrics) IncrementTenantsActive(ctx context.Context, tenantID string) {
	labels := metric.WithAttributes(attribute.String("tenant_id", tenantID))
	m.TenantsActive.Add(ctx, 1, labels)
}

// DecrementTenantsActive decrements the count of active tenants
func (m *TenancyMetrics) DecrementTenantsActive(ctx context.Context, tenantID string) {
	labels := metric.WithAttributes(attribute.String("tenant_id", tenantID))
	m.TenantsActive.Add(ctx, -1, labels)
}

// RecordIsolationViolation records a tenant isolation violation
func (m *TenancyMetrics) RecordIsolationViolation(ctx context.Context, tenantID, violationType string) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("violation_type", violationType),
	)
	m.TenantIsolationViolations.Add(ctx, 1, labels)
}

// RecordAccessAttempt records a tenant access attempt
func (m *TenancyMetrics) RecordAccessAttempt(ctx context.Context, tenantID, resourceType string, allowed bool) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("resource_type", resourceType),
		attribute.Bool("allowed", allowed),
	)

	m.TenantAccessAttempts.Add(ctx, 1, labels)

	if !allowed {
		m.TenantAccessDenied.Add(ctx, 1, labels)
	}
}

// UpdateResourceUsage updates the resource usage metrics for a tenant
func (m *TenancyMetrics) UpdateResourceUsage(ctx context.Context, tenantID, resourceType string, usage float64) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("resource_type", resourceType),
	)
	m.TenantResourceUsage.Record(ctx, usage, labels)
}

// RecordQuotaViolation records a tenant quota violation
func (m *TenancyMetrics) RecordQuotaViolation(ctx context.Context, tenantID, resourceType string) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("resource_type", resourceType),
	)
	m.TenantQuotaViolations.Add(ctx, 1, labels)
}

// UpdateQuotaUtilization updates the quota utilization percentage for a tenant
func (m *TenancyMetrics) UpdateQuotaUtilization(ctx context.Context, tenantID, resourceType string, utilization float64) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("resource_type", resourceType),
	)
	m.TenantQuotaUtilization.Record(ctx, utilization, labels)
}

// UpdateNetworkPolicies updates the number of network policies for a tenant
func (m *TenancyMetrics) UpdateNetworkPolicies(ctx context.Context, tenantID string, count int64) {
	labels := metric.WithAttributes(attribute.String("tenant_id", tenantID))
	m.TenantNetworkPolicies.Add(ctx, count, labels)
}

// RecordNetworkViolation records a tenant network violation
func (m *TenancyMetrics) RecordNetworkViolation(ctx context.Context, tenantID, violationType string) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("violation_type", violationType),
	)
	m.TenantNetworkViolations.Add(ctx, 1, labels)
}

// UpdateRBACResources updates the number of RBAC resources for a tenant
func (m *TenancyMetrics) UpdateRBACResources(ctx context.Context, tenantID string, count int64) {
	labels := metric.WithAttributes(attribute.String("tenant_id", tenantID))
	m.TenantRBACResources.Add(ctx, count, labels)
}

// RecordRBACError records a tenant RBAC error
func (m *TenancyMetrics) RecordRBACError(ctx context.Context, tenantID, errorType string) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("error_type", errorType),
	)
	m.TenantRBACErrors.Add(ctx, 1, labels)
}

// GetTenantMetrics returns metrics for a specific tenant (placeholder implementation)
func (m *TenancyMetrics) GetTenantMetrics(ctx context.Context, tenantID string) (*TenantMetrics, error) {
	// This is a placeholder implementation
	// In a real implementation, you would query the metrics backend
	// or maintain in-memory counters to return current values
	return &TenantMetrics{
		TenantID:          tenantID,
		OperationsTotal:   0,
		OperationErrors:   0,
		LastOperationTime: time.Now(),
		ResourceUsage:     make(map[string]float64),
		QuotaViolations:   0,
		NetworkPolicies:   0,
		AccessViolations:  0,
		RBACResources:     0,
	}, nil
}

// RecordTenantEvent records a general tenant event for auditing
func (m *TenancyMetrics) RecordTenantEvent(ctx context.Context, tenantID, eventType, eventSubject string) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("event_type", eventType),
		attribute.String("event_subject", eventSubject),
	)
	m.TenantOperationsTotal.Add(ctx, 1, labels)
}

// RecordTenantResourceQuotaStatus records the current status of a tenant's resource quota
func (m *TenancyMetrics) RecordTenantResourceQuotaStatus(ctx context.Context, tenantID string, resourceType string, used, limit float64) {
	utilization := 0.0
	if limit > 0 {
		utilization = (used / limit) * 100
	}

	// Record usage
	m.UpdateResourceUsage(ctx, tenantID, resourceType, used)

	// Record utilization percentage
	m.UpdateQuotaUtilization(ctx, tenantID, resourceType, utilization)

	// Record violation if over limit
	if used > limit {
		m.RecordQuotaViolation(ctx, tenantID, resourceType)
	}
}

// RecordTenantSecurityEvent records security-related events for a tenant
func (m *TenancyMetrics) RecordTenantSecurityEvent(ctx context.Context, tenantID, eventType, severity string) {
	labels := metric.WithAttributes(
		attribute.String("tenant_id", tenantID),
		attribute.String("event_type", eventType),
		attribute.String("severity", severity),
	)

	switch eventType {
	case "isolation_violation":
		m.TenantIsolationViolations.Add(ctx, 1, labels)
	case "network_violation":
		m.TenantNetworkViolations.Add(ctx, 1, labels)
	case "access_denied":
		m.TenantAccessDenied.Add(ctx, 1, labels)
	case "quota_violation":
		m.TenantQuotaViolations.Add(ctx, 1, labels)
	}
}
