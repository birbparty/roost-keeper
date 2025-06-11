package tenancy

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// Validator handles tenant access validation and security enforcement
type Validator struct {
	client client.Client
	logger *zap.Logger
}

// NewValidator creates a new tenant validator
func NewValidator(client client.Client, logger *zap.Logger) *Validator {
	return &Validator{
		client: client,
		logger: logger.With(zap.String("component", "tenant-validator")),
	}
}

// ValidateTenantAccess ensures the managed roost can only access resources belonging to its tenant
func (v *Validator) ValidateTenantAccess(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	log := v.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Debug("Validating tenant access")

	// Validate that the namespace is properly labeled for the tenant
	var namespace corev1.Namespace
	if err := v.client.Get(ctx, client.ObjectKey{Name: managedRoost.Namespace}, &namespace); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("namespace %s does not exist", managedRoost.Namespace)
		}
		return fmt.Errorf("failed to get namespace %s: %w", managedRoost.Namespace, err)
	}

	// Check if namespace has tenant labels
	if namespace.Labels == nil {
		return fmt.Errorf("namespace %s is not labeled for any tenant", managedRoost.Namespace)
	}

	namespaceTenant := namespace.Labels[TenantIDLabel]
	if namespaceTenant == "" {
		log.Warn("Namespace missing tenant label, allowing creation and will label during enforcement")
		return nil
	}

	// Verify tenant ownership
	if namespaceTenant != tenantID {
		log.Error("Tenant access violation detected",
			zap.String("namespace_tenant", namespaceTenant),
			zap.String("requested_tenant", tenantID),
		)
		return fmt.Errorf("namespace %s belongs to tenant %s, not %s",
			managedRoost.Namespace, namespaceTenant, tenantID)
	}

	log.Debug("Tenant access validation successful")
	return nil
}

// ValidateCrossNamespaceAccess ensures a tenant cannot access resources in other tenants' namespaces
func (v *Validator) ValidateCrossNamespaceAccess(ctx context.Context, tenantID string, targetNamespace string) error {
	if targetNamespace == "" {
		return nil // Local namespace access is always allowed
	}

	var namespace corev1.Namespace
	if err := v.client.Get(ctx, client.ObjectKey{Name: targetNamespace}, &namespace); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("target namespace %s does not exist", targetNamespace)
		}
		return fmt.Errorf("failed to get target namespace %s: %w", targetNamespace, err)
	}

	// Check if target namespace belongs to the same tenant
	if namespace.Labels != nil {
		namespaceTenant := namespace.Labels[TenantIDLabel]
		if namespaceTenant != "" && namespaceTenant != tenantID {
			v.logger.Error("Cross-tenant namespace access attempt blocked",
				zap.String("tenant", tenantID),
				zap.String("target_namespace", targetNamespace),
				zap.String("target_tenant", namespaceTenant),
			)
			return fmt.Errorf("tenant %s cannot access namespace %s belonging to tenant %s",
				tenantID, targetNamespace, namespaceTenant)
		}
	}

	return nil
}

// ValidateResourceAccess ensures a managed roost can only access resources within its tenant boundary
func (v *Validator) ValidateResourceAccess(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	if managedRoost.Spec.Tenancy == nil {
		return nil // No tenancy restrictions
	}

	tenantID := managedRoost.Spec.Tenancy.TenantID

	// Validate chart values don't reference cross-tenant resources
	if managedRoost.Spec.Chart.Values != nil {
		if err := v.validateChartValues(ctx, managedRoost.Spec.Chart.Values, tenantID); err != nil {
			return fmt.Errorf("chart values validation failed: %w", err)
		}
	}

	return nil
}

// validateChartValues inspects chart values for cross-tenant resource references
func (v *Validator) validateChartValues(ctx context.Context, values *roostv1alpha1.ChartValuesSpec, tenantID string) error {
	// Validate ConfigMap references
	for _, configMapRef := range values.ConfigMapRefs {
		if configMapRef.Namespace != "" {
			if err := v.ValidateCrossNamespaceAccess(ctx, tenantID, configMapRef.Namespace); err != nil {
				return fmt.Errorf("invalid ConfigMap reference: %w", err)
			}
		}
	}

	// Validate Secret references
	for _, secretRef := range values.SecretRefs {
		if secretRef.Namespace != "" {
			if err := v.ValidateCrossNamespaceAccess(ctx, tenantID, secretRef.Namespace); err != nil {
				return fmt.Errorf("invalid Secret reference: %w", err)
			}
		}
	}

	return nil
}

// ValidateNamespaceLabels ensures namespace labels are consistent with tenant requirements
func (v *Validator) ValidateNamespaceLabels(ctx context.Context, namespace *corev1.Namespace, expectedTenant string) error {
	if namespace.Labels == nil {
		return fmt.Errorf("namespace %s has no labels", namespace.Name)
	}

	actualTenant := namespace.Labels[TenantIDLabel]
	if actualTenant == "" {
		return fmt.Errorf("namespace %s missing tenant label %s", namespace.Name, TenantIDLabel)
	}

	if actualTenant != expectedTenant {
		return fmt.Errorf("namespace %s has tenant %s, expected %s",
			namespace.Name, actualTenant, expectedTenant)
	}

	// Verify isolation is enabled
	isolation := namespace.Labels[TenantIsolationLabel]
	if isolation != "enabled" {
		return fmt.Errorf("namespace %s does not have isolation enabled", namespace.Name)
	}

	return nil
}

// GetNamespaceTenant returns the tenant ID associated with a namespace
func (v *Validator) GetNamespaceTenant(ctx context.Context, namespaceName string) (string, error) {
	var namespace corev1.Namespace
	if err := v.client.Get(ctx, client.ObjectKey{Name: namespaceName}, &namespace); err != nil {
		return "", fmt.Errorf("failed to get namespace %s: %w", namespaceName, err)
	}

	if namespace.Labels == nil {
		return "", fmt.Errorf("namespace %s has no labels", namespaceName)
	}

	tenantID := namespace.Labels[TenantIDLabel]
	if tenantID == "" {
		return "", fmt.Errorf("namespace %s has no tenant label", namespaceName)
	}

	return tenantID, nil
}

// ValidateTenantIsolationEnabled verifies that tenant isolation is properly enabled for a namespace
func (v *Validator) ValidateTenantIsolationEnabled(ctx context.Context, namespaceName string) error {
	var namespace corev1.Namespace
	if err := v.client.Get(ctx, client.ObjectKey{Name: namespaceName}, &namespace); err != nil {
		return fmt.Errorf("failed to get namespace %s: %w", namespaceName, err)
	}

	// Check isolation annotation
	if namespace.Annotations == nil {
		return fmt.Errorf("namespace %s missing tenant isolation annotations", namespaceName)
	}

	if namespace.Annotations[TenantIsolationEnabledAnnotation] != "true" {
		return fmt.Errorf("tenant isolation not enabled for namespace %s", namespaceName)
	}

	// Check policy version
	policyVersion := namespace.Annotations[TenantPolicyVersionAnnotation]
	if policyVersion == "" {
		return fmt.Errorf("namespace %s missing tenant policy version", namespaceName)
	}

	if policyVersion != CurrentPolicyVersion {
		v.logger.Warn("Namespace has outdated tenant policy version",
			zap.String("namespace", namespaceName),
			zap.String("current_version", policyVersion),
			zap.String("expected_version", CurrentPolicyVersion),
		)
	}

	return nil
}
