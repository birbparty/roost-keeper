package webhook

import (
	"context"
	"time"

	"go.uber.org/zap"
	jsonpatch "gomodules.xyz/jsonpatch/v2"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// SimplePolicyEngine is a simplified policy engine that works with actual ManagedRoost fields
type SimplePolicyEngine struct {
	logger  *zap.Logger
	metrics *telemetry.WebhookMetrics
}

// SimpleValidator represents a validation function
type SimpleValidator func(context.Context, *roostkeeper.ManagedRoost) (*PolicyResult, error)

// SimpleMutator represents a mutation function
type SimpleMutator func(context.Context, *roostkeeper.ManagedRoost) ([]jsonpatch.JsonPatchOperation, error)

// NewSimplePolicyEngine creates a new simplified policy engine
func NewSimplePolicyEngine(logger *zap.Logger, metrics *telemetry.WebhookMetrics) *SimplePolicyEngine {
	return &SimplePolicyEngine{
		logger:  logger,
		metrics: metrics,
	}
}

// ValidateSimple performs basic validation on ManagedRoost resources
func (spe *SimplePolicyEngine) ValidateSimple(ctx context.Context, managedRoost *roostkeeper.ManagedRoost) (*PolicyResult, error) {
	start := time.Now()
	defer func() {
		if spe.metrics != nil {
			spe.metrics.RecordPolicyEvaluation(ctx, "simple-validation", time.Since(start), false)
		}
	}()

	// Basic validation: check required fields
	if managedRoost.Spec.Chart.Name == "" {
		return &PolicyResult{
			Allowed: false,
			Message: "Chart name is required",
			Code:    422,
			Details: map[string]interface{}{
				"field": "spec.chart.name",
			},
		}, nil
	}

	if managedRoost.Spec.Chart.Version == "" {
		return &PolicyResult{
			Allowed: false,
			Message: "Chart version is required",
			Code:    422,
			Details: map[string]interface{}{
				"field": "spec.chart.version",
			},
		}, nil
	}

	if managedRoost.Spec.Chart.Repository.URL == "" {
		return &PolicyResult{
			Allowed: false,
			Message: "Chart repository URL is required",
			Code:    422,
			Details: map[string]interface{}{
				"field": "spec.chart.repository.url",
			},
		}, nil
	}

	// Validate namespace if specified
	if managedRoost.Spec.Namespace != "" {
		// Basic namespace validation
		if len(managedRoost.Spec.Namespace) > 63 {
			return &PolicyResult{
				Allowed: false,
				Message: "Namespace name must be 63 characters or less",
				Code:    422,
				Details: map[string]interface{}{
					"field": "spec.namespace",
					"value": managedRoost.Spec.Namespace,
				},
			}, nil
		}
	}

	return &PolicyResult{
		Allowed: true,
		Message: "Validation passed",
	}, nil
}

// MutateSimple performs basic mutations on ManagedRoost resources
func (spe *SimplePolicyEngine) MutateSimple(ctx context.Context, managedRoost *roostkeeper.ManagedRoost) ([]jsonpatch.JsonPatchOperation, error) {
	start := time.Now()
	defer func() {
		if spe.metrics != nil {
			spe.metrics.RecordPolicyEvaluation(ctx, "simple-mutation", time.Since(start), false)
		}
	}()

	var patches []jsonpatch.JsonPatchOperation

	// Inject tenant labels based on tenancy configuration
	tenantID := "default"
	if managedRoost.Spec.Tenancy != nil && managedRoost.Spec.Tenancy.TenantID != "" {
		tenantID = managedRoost.Spec.Tenancy.TenantID
	}

	// Ensure labels exist
	if managedRoost.Labels == nil {
		patches = append(patches, jsonpatch.JsonPatchOperation{
			Operation: "add",
			Path:      "/metadata/labels",
			Value:     map[string]string{},
		})
	}

	// Add tenant label
	patches = append(patches, jsonpatch.JsonPatchOperation{
		Operation: "add",
		Path:      "/metadata/labels/roost-keeper.io~1tenant",
		Value:     tenantID,
	})

	// Add managed-by label
	patches = append(patches, jsonpatch.JsonPatchOperation{
		Operation: "add",
		Path:      "/metadata/labels/roost-keeper.io~1managed-by",
		Value:     "roost-keeper",
	})

	// Ensure annotations exist
	if managedRoost.Annotations == nil {
		patches = append(patches, jsonpatch.JsonPatchOperation{
			Operation: "add",
			Path:      "/metadata/annotations",
			Value:     map[string]string{},
		})
	}

	// Add creation timestamp annotation
	patches = append(patches, jsonpatch.JsonPatchOperation{
		Operation: "add",
		Path:      "/metadata/annotations/roost-keeper.io~1created-by-webhook",
		Value:     time.Now().Format(time.RFC3339),
	})

	spe.logger.Info("Applied simple mutations",
		zap.String("tenant", tenantID),
		zap.Int("patch_count", len(patches)))

	return patches, nil
}

// ValidateTenancy validates tenancy configuration
func (spe *SimplePolicyEngine) ValidateTenancy(ctx context.Context, managedRoost *roostkeeper.ManagedRoost) (*PolicyResult, error) {
	if managedRoost.Spec.Tenancy == nil {
		return &PolicyResult{
			Allowed: true,
			Message: "No tenancy configuration to validate",
		}, nil
	}

	// Validate tenant ID format
	if managedRoost.Spec.Tenancy.TenantID != "" {
		if len(managedRoost.Spec.Tenancy.TenantID) > 253 {
			return &PolicyResult{
				Allowed: false,
				Message: "Tenant ID must be 253 characters or less",
				Code:    422,
				Details: map[string]interface{}{
					"field": "spec.tenancy.tenantId",
					"value": managedRoost.Spec.Tenancy.TenantID,
				},
			}, nil
		}
	}

	// Validate RBAC configuration if present
	if managedRoost.Spec.Tenancy.RBAC != nil {
		if managedRoost.Spec.Tenancy.RBAC.TenantID == "" {
			return &PolicyResult{
				Allowed: false,
				Message: "RBAC tenant ID is required when RBAC is configured",
				Code:    422,
				Details: map[string]interface{}{
					"field": "spec.tenancy.rbac.tenantId",
				},
			}, nil
		}
	}

	return &PolicyResult{
		Allowed: true,
		Message: "Tenancy validation passed",
	}, nil
}

// ValidateObservability validates observability configuration
func (spe *SimplePolicyEngine) ValidateObservability(ctx context.Context, managedRoost *roostkeeper.ManagedRoost) (*PolicyResult, error) {
	if managedRoost.Spec.Observability == nil {
		return &PolicyResult{
			Allowed: true,
			Message: "No observability configuration to validate",
		}, nil
	}

	// Validate metrics configuration
	if managedRoost.Spec.Observability.Metrics != nil {
		if managedRoost.Spec.Observability.Metrics.Enabled {
			// Check interval is reasonable
			if managedRoost.Spec.Observability.Metrics.Interval.Duration < time.Second {
				return &PolicyResult{
					Allowed: false,
					Message: "Metrics interval must be at least 1 second",
					Code:    422,
					Details: map[string]interface{}{
						"field": "spec.observability.metrics.interval",
						"value": managedRoost.Spec.Observability.Metrics.Interval.Duration.String(),
					},
				}, nil
			}
		}
	}

	return &PolicyResult{
		Allowed: true,
		Message: "Observability validation passed",
	}, nil
}
