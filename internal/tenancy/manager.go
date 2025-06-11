package tenancy

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

const (
	// Labels for tenant identification and management
	TenantIDLabel        = "roost-keeper.io/tenant"
	TenantIsolationLabel = "roost-keeper.io/isolation"
	TenantComponentLabel = "roost-keeper.io/component"

	// Annotations for tenant metadata
	TenantIsolationEnabledAnnotation = "roost-keeper.io/tenant-isolation-enforced"
	TenantPolicyVersionAnnotation    = "roost-keeper.io/tenant-policy-version"
	TenantCreatedByAnnotation        = "roost-keeper.io/tenant-created-by"

	// Component types for labeling tenant resources
	ComponentTenantRBAC      = "tenant-rbac"
	ComponentNetworkPolicy   = "network-policy"
	ComponentResourceQuota   = "resource-quota"
	ComponentNamespaceConfig = "namespace-config"

	// Policy version for tracking updates
	CurrentPolicyVersion = "v1"
)

// Manager coordinates multi-tenant isolation enforcement
type Manager struct {
	client     client.Client
	validator  *Validator
	rbacMgr    *RBACManager
	networkMgr *NetworkManager
	quotaMgr   *QuotaManager
	metrics    *TenancyMetrics
	logger     *zap.Logger
}

// NewManager creates a new tenancy manager with all required components
func NewManager(client client.Client, logger *zap.Logger) (*Manager, error) {
	if client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Initialize metrics
	metrics, err := NewTenancyMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tenancy metrics: %w", err)
	}

	// Initialize specialized managers
	validator := NewValidator(client, logger)
	rbacMgr := NewRBACManager(client, logger)
	networkMgr := NewNetworkManager(client, logger)
	quotaMgr := NewQuotaManager(client, logger)

	return &Manager{
		client:     client,
		validator:  validator,
		rbacMgr:    rbacMgr,
		networkMgr: networkMgr,
		quotaMgr:   quotaMgr,
		metrics:    metrics,
		logger:     logger.With(zap.String("component", "tenancy-manager")),
	}, nil
}

// EnforceTenantIsolation orchestrates complete tenant isolation setup
func (m *Manager) EnforceTenantIsolation(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	if managedRoost.Spec.Tenancy == nil {
		m.logger.Debug("No tenancy configuration, skipping tenant isolation")
		return nil
	}

	tenantID := managedRoost.Spec.Tenancy.TenantID
	if tenantID == "" {
		return fmt.Errorf("tenant ID is required when tenancy is enabled")
	}

	// Create enhanced logger with tenant context
	log := m.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
		zap.String("operation", "enforce_isolation"),
	)

	// Start enforcement span for observability
	ctx, span := telemetry.StartControllerSpan(ctx, "tenancy.enforce_isolation", managedRoost.Name, managedRoost.Namespace)
	defer span.End()

	start := time.Now()
	var enforcementErr error

	defer func() {
		duration := time.Since(start)
		success := enforcementErr == nil

		// Record enforcement metrics
		if m.metrics != nil {
			m.metrics.RecordTenantOperation(ctx, "enforce_isolation", tenantID, duration, success)
		}

		if enforcementErr != nil {
			telemetry.RecordSpanError(ctx, enforcementErr)
			log.Error("Tenant isolation enforcement failed", zap.Error(enforcementErr), zap.Duration("duration", duration))
		} else {
			log.Info("Tenant isolation enforcement completed successfully", zap.Duration("duration", duration))
		}
	}()

	log.Info("Starting tenant isolation enforcement")

	// Step 1: Validate tenant access rights
	if err := m.validator.ValidateTenantAccess(ctx, managedRoost, tenantID); err != nil {
		enforcementErr = fmt.Errorf("tenant access validation failed: %w", err)
		return enforcementErr
	}

	// Step 2: Enforce namespace isolation if enabled
	if managedRoost.Spec.Tenancy.NamespaceIsolation != nil && managedRoost.Spec.Tenancy.NamespaceIsolation.Enabled {
		if err := m.enforceNamespaceIsolation(ctx, managedRoost, tenantID); err != nil {
			enforcementErr = fmt.Errorf("failed to enforce namespace isolation: %w", err)
			return enforcementErr
		}
	}

	// Step 3: Setup RBAC if enabled
	if managedRoost.Spec.Tenancy.RBAC != nil && managedRoost.Spec.Tenancy.RBAC.Enabled {
		if err := m.rbacMgr.SetupTenantRBAC(ctx, managedRoost, tenantID); err != nil {
			enforcementErr = fmt.Errorf("failed to setup tenant RBAC: %w", err)
			return enforcementErr
		}
	}

	// Step 4: Apply network policies if enabled
	if managedRoost.Spec.Tenancy.NetworkPolicy != nil && managedRoost.Spec.Tenancy.NetworkPolicy.Enabled {
		if err := m.networkMgr.ApplyNetworkPolicies(ctx, managedRoost, tenantID); err != nil {
			enforcementErr = fmt.Errorf("failed to apply network policies: %w", err)
			return enforcementErr
		}
	}

	// Step 5: Setup resource quotas if configured
	if managedRoost.Spec.Tenancy.ResourceQuota != nil && managedRoost.Spec.Tenancy.ResourceQuota.Enabled {
		if err := m.quotaMgr.SetupResourceQuota(ctx, managedRoost, tenantID); err != nil {
			enforcementErr = fmt.Errorf("failed to setup resource quota: %w", err)
			return enforcementErr
		}
	}

	// Step 6: Record successful tenant isolation
	if m.metrics != nil {
		m.metrics.IncrementTenantsActive(ctx, tenantID)
	}

	log.Info("Tenant isolation enforcement completed successfully")
	return nil
}

// CleanupTenantIsolation removes all tenant-specific resources during teardown
func (m *Manager) CleanupTenantIsolation(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	if managedRoost.Spec.Tenancy == nil {
		return nil
	}

	tenantID := managedRoost.Spec.Tenancy.TenantID
	if tenantID == "" {
		return nil // Nothing to clean up
	}

	log := m.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
		zap.String("operation", "cleanup_isolation"),
	)

	log.Info("Starting tenant isolation cleanup")

	var cleanupErrors []error

	// Cleanup network policies
	if err := m.networkMgr.CleanupNetworkPolicies(ctx, managedRoost, tenantID); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("network policy cleanup failed: %w", err))
	}

	// Cleanup RBAC resources
	if err := m.rbacMgr.CleanupTenantRBAC(ctx, managedRoost, tenantID); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("RBAC cleanup failed: %w", err))
	}

	// Cleanup resource quotas
	if err := m.quotaMgr.CleanupResourceQuota(ctx, managedRoost, tenantID); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("resource quota cleanup failed: %w", err))
	}

	// Record cleanup completion
	if m.metrics != nil {
		m.metrics.DecrementTenantsActive(ctx, tenantID)
	}

	if len(cleanupErrors) > 0 {
		log.Error("Some tenant isolation cleanup operations failed", zap.Int("errors", len(cleanupErrors)))
		return fmt.Errorf("tenant isolation cleanup failed with %d errors: %v", len(cleanupErrors), cleanupErrors)
	}

	log.Info("Tenant isolation cleanup completed successfully")
	return nil
}

// ValidateTenantSpec validates the tenancy specification for completeness and security
func (m *Manager) ValidateTenantSpec(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	if managedRoost.Spec.Tenancy == nil {
		return nil // No tenancy spec to validate
	}

	tenancy := managedRoost.Spec.Tenancy

	// Validate tenant ID
	if tenancy.TenantID == "" {
		return fmt.Errorf("tenantId is required when tenancy is configured")
	}

	if len(tenancy.TenantID) > 63 {
		return fmt.Errorf("tenantId must be 63 characters or less, got %d", len(tenancy.TenantID))
	}

	// Validate tenant ID format (DNS label compatible)
	if !isDNSLabel(tenancy.TenantID) {
		return fmt.Errorf("tenantId must be a valid DNS label (lowercase alphanumeric and hyphens)")
	}

	// Validate resource quota configuration
	if tenancy.ResourceQuota != nil && tenancy.ResourceQuota.Enabled {
		if err := m.quotaMgr.ValidateQuotaSpec(tenancy.ResourceQuota); err != nil {
			return fmt.Errorf("invalid resource quota specification: %w", err)
		}
	}

	return nil
}

// enforceNamespaceIsolation ensures the namespace is properly configured for tenant isolation
func (m *Manager) enforceNamespaceIsolation(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	namespace := &corev1.Namespace{}
	if err := m.client.Get(ctx, client.ObjectKey{Name: managedRoost.Namespace}, namespace); err != nil {
		return fmt.Errorf("failed to get namespace %s: %w", managedRoost.Namespace, err)
	}

	// Initialize labels and annotations if nil
	if namespace.Labels == nil {
		namespace.Labels = make(map[string]string)
	}
	if namespace.Annotations == nil {
		namespace.Annotations = make(map[string]string)
	}

	// Set tenant identification labels
	namespace.Labels[TenantIDLabel] = tenantID
	namespace.Labels[TenantIsolationLabel] = "enabled"

	// Apply additional labels from tenant spec
	if managedRoost.Spec.Tenancy.NamespaceIsolation != nil {
		for key, value := range managedRoost.Spec.Tenancy.NamespaceIsolation.Labels {
			namespace.Labels[key] = value
		}
		for key, value := range managedRoost.Spec.Tenancy.NamespaceIsolation.Annotations {
			namespace.Annotations[key] = value
		}
	}

	// Set isolation enforcement annotations
	namespace.Annotations[TenantIsolationEnabledAnnotation] = "true"
	namespace.Annotations[TenantPolicyVersionAnnotation] = CurrentPolicyVersion
	namespace.Annotations[TenantCreatedByAnnotation] = "roost-keeper"

	// Update the namespace
	if err := m.client.Update(ctx, namespace); err != nil {
		return fmt.Errorf("failed to update namespace labels and annotations: %w", err)
	}

	return nil
}

// GetTenantsInNamespace returns all tenant IDs that have resources in the specified namespace
func (m *Manager) GetTenantsInNamespace(ctx context.Context, namespace string) ([]string, error) {
	// List all managed roosts in the namespace
	var roostList roostv1alpha1.ManagedRoostList
	if err := m.client.List(ctx, &roostList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list managed roosts in namespace %s: %w", namespace, err)
	}

	tenantSet := make(map[string]bool)
	for _, roost := range roostList.Items {
		if roost.Spec.Tenancy != nil && roost.Spec.Tenancy.TenantID != "" {
			tenantSet[roost.Spec.Tenancy.TenantID] = true
		}
	}

	var tenants []string
	for tenant := range tenantSet {
		tenants = append(tenants, tenant)
	}

	return tenants, nil
}

// GetTenantMetrics returns tenant-specific metrics if available
func (m *Manager) GetTenantMetrics(ctx context.Context, tenantID string) (*TenantMetrics, error) {
	if m.metrics == nil {
		return nil, fmt.Errorf("metrics not available")
	}

	return m.metrics.GetTenantMetrics(ctx, tenantID)
}

// isDNSLabel validates that a string is a valid DNS label (RFC 1123)
func isDNSLabel(value string) bool {
	if len(value) == 0 || len(value) > 63 {
		return false
	}

	for i, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
		if i == 0 || i == len(value)-1 {
			if r == '-' {
				return false // Cannot start or end with hyphen
			}
		}
	}

	return true
}
