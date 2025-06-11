package tenancy

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// QuotaManager handles tenant resource quota management
type QuotaManager struct {
	client client.Client
	logger *zap.Logger
}

// NewQuotaManager creates a new quota manager
func NewQuotaManager(client client.Client, logger *zap.Logger) *QuotaManager {
	return &QuotaManager{
		client: client,
		logger: logger.With(zap.String("component", "quota-manager")),
	}
}

// SetupResourceQuota creates and applies resource quotas for a tenant
func (q *QuotaManager) SetupResourceQuota(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	log := q.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Info("Setting up tenant resource quota")

	quotaSpec := managedRoost.Spec.Tenancy.ResourceQuota
	if quotaSpec == nil {
		return fmt.Errorf("resource quota spec is nil")
	}

	quotaName := fmt.Sprintf("roost-keeper-tenant-%s", tenantID)

	resourceQuota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentResourceQuota,
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: q.buildResourceList(quotaSpec),
		},
	}

	// Apply scopes if specified
	if len(quotaSpec.Scopes) > 0 {
		var scopes []corev1.ResourceQuotaScope
		for _, scopeStr := range quotaSpec.Scopes {
			scopes = append(scopes, corev1.ResourceQuotaScope(scopeStr))
		}
		resourceQuota.Spec.Scopes = scopes
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, resourceQuota, q.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := q.client.Create(ctx, resourceQuota); err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing quota
			existingQuota := &corev1.ResourceQuota{}
			if err := q.client.Get(ctx, client.ObjectKey{Name: quotaName, Namespace: managedRoost.Namespace}, existingQuota); err != nil {
				return fmt.Errorf("failed to get existing quota: %w", err)
			}
			existingQuota.Spec.Hard = resourceQuota.Spec.Hard
			existingQuota.Spec.Scopes = resourceQuota.Spec.Scopes
			if err := q.client.Update(ctx, existingQuota); err != nil {
				return fmt.Errorf("failed to update existing quota: %w", err)
			}
			log.Debug("Updated existing resource quota", zap.String("name", quotaName))
			return nil
		}
		return fmt.Errorf("failed to create resource quota: %w", err)
	}

	log.Info("Created tenant resource quota", zap.String("name", quotaName))
	return nil
}

// CleanupResourceQuota removes tenant-specific resource quotas
func (q *QuotaManager) CleanupResourceQuota(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	log := q.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Info("Cleaning up tenant resource quota")

	quotaName := fmt.Sprintf("roost-keeper-tenant-%s", tenantID)
	resourceQuota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      quotaName,
			Namespace: managedRoost.Namespace,
		},
	}

	if err := q.client.Delete(ctx, resourceQuota); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete resource quota %s: %w", quotaName, err)
	}

	log.Info("Tenant resource quota cleanup completed successfully")
	return nil
}

// ValidateQuotaSpec validates the resource quota specification
func (q *QuotaManager) ValidateQuotaSpec(quotaSpec *roostv1alpha1.ResourceQuotaSpec) error {
	if quotaSpec == nil {
		return fmt.Errorf("quota spec cannot be nil")
	}

	if quotaSpec.Hard == nil || len(quotaSpec.Hard) == 0 {
		return fmt.Errorf("quota spec must define hard limits")
	}

	// Validate each resource quantity
	for resourceName, quantityStr := range quotaSpec.Hard {
		if _, err := resource.ParseQuantity(quantityStr.String()); err != nil {
			return fmt.Errorf("invalid quantity for resource %s: %w", resourceName, err)
		}
	}

	// Validate scopes if specified
	validScopes := map[string]bool{
		"Terminating":    true,
		"NotTerminating": true,
		"BestEffort":     true,
		"NotBestEffort":  true,
		"PriorityClass":  true,
	}

	for _, scope := range quotaSpec.Scopes {
		if !validScopes[scope] {
			return fmt.Errorf("invalid resource quota scope: %s", scope)
		}
	}

	return nil
}

// GetResourceUsage retrieves current resource usage for a tenant
func (q *QuotaManager) GetResourceUsage(ctx context.Context, namespace, tenantID string) (*corev1.ResourceQuota, error) {
	quotaName := fmt.Sprintf("roost-keeper-tenant-%s", tenantID)

	var resourceQuota corev1.ResourceQuota
	if err := q.client.Get(ctx, client.ObjectKey{Name: quotaName, Namespace: namespace}, &resourceQuota); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("resource quota not found for tenant %s in namespace %s", tenantID, namespace)
		}
		return nil, fmt.Errorf("failed to get resource quota: %w", err)
	}

	return &resourceQuota, nil
}

// CheckQuotaViolation checks if the tenant has exceeded resource quotas
func (q *QuotaManager) CheckQuotaViolation(ctx context.Context, namespace, tenantID string) (bool, map[string]string, error) {
	quota, err := q.GetResourceUsage(ctx, namespace, tenantID)
	if err != nil {
		return false, nil, err
	}

	violations := make(map[string]string)
	hasViolations := false

	for resourceName, hardLimit := range quota.Spec.Hard {
		used, exists := quota.Status.Used[resourceName]
		if !exists {
			continue
		}

		if used.Cmp(hardLimit) > 0 {
			hasViolations = true
			violations[string(resourceName)] = fmt.Sprintf("used: %s, limit: %s", used.String(), hardLimit.String())
		}
	}

	return hasViolations, violations, nil
}

// UpdateResourceQuota updates an existing resource quota
func (q *QuotaManager) UpdateResourceQuota(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	return q.SetupResourceQuota(ctx, managedRoost, tenantID)
}

// buildResourceList converts the quota spec to a Kubernetes ResourceList
func (q *QuotaManager) buildResourceList(quotaSpec *roostv1alpha1.ResourceQuotaSpec) corev1.ResourceList {
	resourceList := make(corev1.ResourceList)

	for resourceName, quantity := range quotaSpec.Hard {
		// Parse the quantity string to ensure it's valid
		if parsedQuantity, err := resource.ParseQuantity(quantity.String()); err == nil {
			resourceList[corev1.ResourceName(resourceName)] = parsedQuantity
		} else {
			q.logger.Warn("Invalid resource quantity, skipping",
				zap.String("resource", resourceName),
				zap.String("quantity", quantity.String()),
				zap.Error(err),
			)
		}
	}

	return resourceList
}

// GetQuotaStatus returns the current status of a tenant's resource quota
func (q *QuotaManager) GetQuotaStatus(ctx context.Context, namespace, tenantID string) (*QuotaStatus, error) {
	quota, err := q.GetResourceUsage(ctx, namespace, tenantID)
	if err != nil {
		return nil, err
	}

	status := &QuotaStatus{
		TenantID:  tenantID,
		Namespace: namespace,
		Hard:      make(map[string]string),
		Used:      make(map[string]string),
	}

	for resourceName, hardLimit := range quota.Spec.Hard {
		status.Hard[string(resourceName)] = hardLimit.String()

		if used, exists := quota.Status.Used[resourceName]; exists {
			status.Used[string(resourceName)] = used.String()
		} else {
			status.Used[string(resourceName)] = "0"
		}
	}

	return status, nil
}

// QuotaStatus represents the current quota status for a tenant
type QuotaStatus struct {
	TenantID  string            `json:"tenantId"`
	Namespace string            `json:"namespace"`
	Hard      map[string]string `json:"hard"`
	Used      map[string]string `json:"used"`
}

// ListTenantQuotas lists all resource quotas for tenants in a namespace
func (q *QuotaManager) ListTenantQuotas(ctx context.Context, namespace string) ([]corev1.ResourceQuota, error) {
	var quotaList corev1.ResourceQuotaList
	listOpts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{TenantComponentLabel: ComponentResourceQuota},
	}

	if err := q.client.List(ctx, &quotaList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list tenant quotas: %w", err)
	}

	return quotaList.Items, nil
}

// EnforceQuotaCompliance ensures all tenant resources are within quota limits
func (q *QuotaManager) EnforceQuotaCompliance(ctx context.Context, namespace string) error {
	quotas, err := q.ListTenantQuotas(ctx, namespace)
	if err != nil {
		return fmt.Errorf("failed to list quotas for enforcement: %w", err)
	}

	var violations []string
	for _, quota := range quotas {
		tenantID := quota.Labels[TenantIDLabel]
		if tenantID == "" {
			continue
		}

		hasViolation, violationDetails, err := q.CheckQuotaViolation(ctx, namespace, tenantID)
		if err != nil {
			q.logger.Error("Failed to check quota violation",
				zap.String("tenant", tenantID),
				zap.String("namespace", namespace),
				zap.Error(err),
			)
			continue
		}

		if hasViolation {
			violation := fmt.Sprintf("tenant %s: %v", tenantID, violationDetails)
			violations = append(violations, violation)
			q.logger.Warn("Quota violation detected",
				zap.String("tenant", tenantID),
				zap.String("namespace", namespace),
				zap.Any("violations", violationDetails),
			)
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("quota violations detected: %v", violations)
	}

	return nil
}
