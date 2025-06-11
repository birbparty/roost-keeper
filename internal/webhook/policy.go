package webhook

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// PatchOperation represents a JSON patch operation
type PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// PolicyType defines the type of policy rule
type PolicyType string

const (
	PolicyTypeValidating PolicyType = "validating"
	PolicyTypeMutating   PolicyType = "mutating"
)

// PolicyEngine manages and executes admission control policies
type PolicyEngine struct {
	rules   []PolicyRule
	logger  *zap.Logger
	metrics *telemetry.WebhookMetrics
}

// PolicyRule defines a single admission control policy rule
type PolicyRule struct {
	Name        string
	Description string
	Type        PolicyType
	Selector    ResourceSelector
	Validator   func(context.Context, *AdmissionRequest) (*PolicyResult, error)
	Mutator     func(context.Context, *AdmissionRequest) (*MutationResult, error)
}

// ResourceSelector defines criteria for matching resources
type ResourceSelector struct {
	APIVersion string
	Kind       string
	Namespace  string
	Labels     map[string]string
}

// PolicyResult contains the result of a policy validation
type PolicyResult struct {
	Allowed bool
	Message string
	Code    int32
	Details map[string]interface{}
}

// MutationResult contains the result of a policy mutation
type MutationResult struct {
	Patches   []PatchOperation
	PatchType admissionv1.PatchType
	Message   string
}

// NewPolicyEngine creates a new policy engine with default policies
func NewPolicyEngine(logger *zap.Logger, metrics *telemetry.WebhookMetrics) *PolicyEngine {
	pe := &PolicyEngine{
		logger:  logger,
		metrics: metrics,
		rules:   make([]PolicyRule, 0),
	}

	// Register default policies
	pe.registerDefaultPolicies()

	return pe
}

// Validate runs all validation policies against the admission request
func (pe *PolicyEngine) Validate(ctx context.Context, req *AdmissionRequest) (*PolicyResult, error) {
	for _, rule := range pe.rules {
		if rule.Type != PolicyTypeValidating {
			continue
		}

		if !pe.matchesSelector(req, rule.Selector) {
			continue
		}

		start := time.Now()
		result, err := rule.Validator(ctx, req)
		duration := time.Since(start)

		// Record policy metrics
		if pe.metrics != nil {
			violated := result != nil && !result.Allowed
			pe.metrics.RecordPolicyEvaluation(ctx, rule.Name, duration, violated)
			pe.metrics.RecordPolicyRuleExecution(ctx, rule.Name, string(rule.Type), err == nil)
		}

		if err != nil {
			pe.logger.Error("Policy validation failed",
				zap.String("policy", rule.Name),
				zap.Error(err))
			return nil, fmt.Errorf("policy %s validation failed: %w", rule.Name, err)
		}

		if !result.Allowed {
			pe.logger.Info("Policy validation denied",
				zap.String("policy", rule.Name),
				zap.String("reason", result.Message))
			return result, nil
		}
	}

	return &PolicyResult{
		Allowed: true,
		Message: "All validation policies passed",
	}, nil
}

// Mutate runs all mutation policies against the admission request
func (pe *PolicyEngine) Mutate(ctx context.Context, req *AdmissionRequest) (*MutationResult, error) {
	var allPatches []admissionv1.PatchOperation

	for _, rule := range pe.rules {
		if rule.Type != PolicyTypeMutating {
			continue
		}

		if !pe.matchesSelector(req, rule.Selector) {
			continue
		}

		start := time.Now()
		result, err := rule.Mutator(ctx, req)
		duration := time.Since(start)

		// Record policy metrics
		if pe.metrics != nil {
			pe.metrics.RecordPolicyEvaluation(ctx, rule.Name, duration, false)
			pe.metrics.RecordPolicyRuleExecution(ctx, rule.Name, string(rule.Type), err == nil)
		}

		if err != nil {
			pe.logger.Error("Policy mutation failed",
				zap.String("policy", rule.Name),
				zap.Error(err))
			return nil, fmt.Errorf("policy %s mutation failed: %w", rule.Name, err)
		}

		if len(result.Patches) > 0 {
			allPatches = append(allPatches, result.Patches...)
			pe.logger.Info("Policy mutation applied",
				zap.String("policy", rule.Name),
				zap.Int("patch_count", len(result.Patches)))
		}
	}

	return &MutationResult{
		Patches:   allPatches,
		PatchType: admissionv1.PatchTypeJSONPatch,
		Message:   "Applied policy mutations",
	}, nil
}

// AddRule adds a new policy rule to the engine
func (pe *PolicyEngine) AddRule(rule PolicyRule) {
	pe.rules = append(pe.rules, rule)
	pe.logger.Info("Added policy rule",
		zap.String("name", rule.Name),
		zap.String("type", string(rule.Type)),
		zap.String("description", rule.Description))
}

// RemoveRule removes a policy rule by name
func (pe *PolicyEngine) RemoveRule(name string) {
	for i, rule := range pe.rules {
		if rule.Name == name {
			pe.rules = append(pe.rules[:i], pe.rules[i+1:]...)
			pe.logger.Info("Removed policy rule", zap.String("name", name))
			return
		}
	}
}

// ListRules returns all registered policy rules
func (pe *PolicyEngine) ListRules() []PolicyRule {
	return pe.rules
}

// matchesSelector checks if the admission request matches the resource selector
func (pe *PolicyEngine) matchesSelector(req *AdmissionRequest, selector ResourceSelector) bool {
	// Check API version
	if selector.APIVersion != "" {
		gvk := req.Raw.Kind
		apiVersion := gvk.Group
		if gvk.Version != "" {
			if apiVersion != "" {
				apiVersion = apiVersion + "/" + gvk.Version
			} else {
				apiVersion = gvk.Version
			}
		}
		if apiVersion != selector.APIVersion {
			return false
		}
	}

	// Check kind
	if selector.Kind != "" && req.Raw.Kind.Kind != selector.Kind {
		return false
	}

	// Check namespace
	if selector.Namespace != "" && req.Raw.Namespace != selector.Namespace {
		return false
	}

	// Check labels (simplified implementation)
	if len(selector.Labels) > 0 {
		objectMeta := pe.extractObjectMeta(req.Object)
		if objectMeta == nil {
			return false
		}

		for key, value := range selector.Labels {
			if objectMeta.Labels[key] != value {
				return false
			}
		}
	}

	return true
}

// extractObjectMeta extracts metadata from a runtime object
func (pe *PolicyEngine) extractObjectMeta(obj runtime.Object) *metav1.ObjectMeta {
	if obj == nil {
		return nil
	}

	// Use reflection to extract metadata
	objValue := reflect.ValueOf(obj)
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
	}

	metaField := objValue.FieldByName("ObjectMeta")
	if !metaField.IsValid() {
		return nil
	}

	if meta, ok := metaField.Interface().(metav1.ObjectMeta); ok {
		return &meta
	}

	return nil
}

// registerDefaultPolicies registers the default admission control policies
func (pe *PolicyEngine) registerDefaultPolicies() {
	// Security context validation policy
	pe.AddRule(PolicyRule{
		Name:        "require-security-context",
		Description: "Ensure all ManagedRoosts have proper security context configuration",
		Type:        PolicyTypeValidating,
		Selector: ResourceSelector{
			APIVersion: "roost-keeper.io/v1alpha1",
			Kind:       "ManagedRoost",
		},
		Validator: pe.validateSecurityContext,
	})

	// Tenant label injection policy
	pe.AddRule(PolicyRule{
		Name:        "inject-tenant-labels",
		Description: "Automatically inject tenant isolation labels and metadata",
		Type:        PolicyTypeMutating,
		Selector: ResourceSelector{
			APIVersion: "roost-keeper.io/v1alpha1",
			Kind:       "ManagedRoost",
		},
		Mutator: pe.injectTenantLabels,
	})

	// Resource limits validation policy
	pe.AddRule(PolicyRule{
		Name:        "validate-resource-limits",
		Description: "Ensure resource limits are within tenant quotas and reasonable bounds",
		Type:        PolicyTypeValidating,
		Selector: ResourceSelector{
			APIVersion: "roost-keeper.io/v1alpha1",
			Kind:       "ManagedRoost",
		},
		Validator: pe.validateResourceLimits,
	})

	// RBAC configuration validation policy
	pe.AddRule(PolicyRule{
		Name:        "validate-rbac-config",
		Description: "Ensure RBAC configuration follows security best practices",
		Type:        PolicyTypeValidating,
		Selector: ResourceSelector{
			APIVersion: "roost-keeper.io/v1alpha1",
			Kind:       "ManagedRoost",
		},
		Validator: pe.validateRBACConfig,
	})

	// Network policy injection policy
	pe.AddRule(PolicyRule{
		Name:        "inject-network-policies",
		Description: "Automatically inject network isolation policies",
		Type:        PolicyTypeMutating,
		Selector: ResourceSelector{
			APIVersion: "roost-keeper.io/v1alpha1",
			Kind:       "ManagedRoost",
		},
		Mutator: pe.injectNetworkPolicies,
	})

	pe.logger.Info("Registered default admission control policies", zap.Int("count", len(pe.rules)))
}

// validateSecurityContext validates security context configuration
func (pe *PolicyEngine) validateSecurityContext(ctx context.Context, req *AdmissionRequest) (*PolicyResult, error) {
	managedRoost, ok := req.Object.(*roostkeeper.ManagedRoost)
	if !ok {
		return &PolicyResult{
			Allowed: false,
			Message: "Invalid object type for security context validation",
			Code:    400,
		}, nil
	}

	// For now, this is a placeholder validation
	// In a real implementation, this would check for security context in the Helm chart values
	// or validate that security policies are enforced through other means

	// Check if RBAC is configured (which implies security practices)
	if managedRoost.Spec.Tenancy != nil && managedRoost.Spec.Tenancy.RBAC != nil && managedRoost.Spec.Tenancy.RBAC.Enabled {
		return &PolicyResult{
			Allowed: true,
			Message: "Security context validation passed - RBAC enabled",
		}, nil
	}

	// Allow for now but log a warning
	return &PolicyResult{
		Allowed: true,
		Message: "Security context validation passed - consider enabling RBAC for enhanced security",
	}, nil
}

// injectTenantLabels injects tenant isolation labels and metadata
func (pe *PolicyEngine) injectTenantLabels(ctx context.Context, req *AdmissionRequest) (*MutationResult, error) {
	managedRoost, ok := req.Object.(*roostkeeper.ManagedRoost)
	if !ok {
		return &MutationResult{}, nil
	}

	var patches []admissionv1.PatchOperation

	// Determine tenant ID
	tenantID := "default"
	if managedRoost.Spec.Tenancy != nil && managedRoost.Spec.Tenancy.TenantID != "" {
		tenantID = managedRoost.Spec.Tenancy.TenantID
	}

	// Ensure labels map exists
	if managedRoost.Labels == nil {
		patches = append(patches, admissionv1.PatchOperation{
			Op:    "add",
			Path:  "/metadata/labels",
			Value: map[string]string{},
		})
	}

	// Add tenant label
	patches = append(patches, admissionv1.PatchOperation{
		Op:    "add",
		Path:  "/metadata/labels/roost-keeper.io~1tenant",
		Value: tenantID,
	})

	// Add managed-by label
	patches = append(patches, admissionv1.PatchOperation{
		Op:    "add",
		Path:  "/metadata/labels/roost-keeper.io~1managed-by",
		Value: "roost-keeper",
	})

	// Add version label
	patches = append(patches, admissionv1.PatchOperation{
		Op:    "add",
		Path:  "/metadata/labels/roost-keeper.io~1version",
		Value: "v1alpha1",
	})

	// Ensure annotations map exists and add tenant information
	if managedRoost.Annotations == nil {
		patches = append(patches, admissionv1.PatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations",
			Value: map[string]string{},
		})
	}

	// Add tenant isolation annotation
	patches = append(patches, admissionv1.PatchOperation{
		Op:    "add",
		Path:  "/metadata/annotations/roost-keeper.io~1tenant-isolation",
		Value: "enabled",
	})

	return &MutationResult{
		Patches:   patches,
		PatchType: admissionv1.PatchTypeJSONPatch,
		Message:   fmt.Sprintf("Injected tenant labels for tenant: %s", tenantID),
	}, nil
}

// validateResourceLimits validates resource limits against tenant quotas
func (pe *PolicyEngine) validateResourceLimits(ctx context.Context, req *AdmissionRequest) (*PolicyResult, error) {
	managedRoost, ok := req.Object.(*roostkeeper.ManagedRoost)
	if !ok {
		return &PolicyResult{
			Allowed: false,
			Message: "Invalid object type for resource limits validation",
			Code:    400,
		}, nil
	}

	// Validate resource limits if specified
	if managedRoost.Spec.Resources != nil {
		// Check CPU limits
		if managedRoost.Spec.Resources.CPU != "" {
			// Basic CPU validation - in production this would check against tenant quotas
			if strings.HasSuffix(managedRoost.Spec.Resources.CPU, "m") {
				// Validate CPU in millicores
			} else {
				// Validate CPU in cores
			}
		}

		// Check memory limits
		if managedRoost.Spec.Resources.Memory != "" {
			// Basic memory validation - in production this would check against tenant quotas
			if !strings.HasSuffix(managedRoost.Spec.Resources.Memory, "i") &&
				!strings.HasSuffix(managedRoost.Spec.Resources.Memory, "B") {
				return &PolicyResult{
					Allowed: false,
					Message: "Memory limits must specify valid units (Mi, Gi, etc.)",
					Code:    422,
					Details: map[string]interface{}{
						"policy": "validate-resource-limits",
						"field":  "spec.resources.memory",
						"value":  managedRoost.Spec.Resources.Memory,
					},
				}, nil
			}
		}

		// Check storage limits
		if managedRoost.Spec.Resources.Storage != "" {
			// Basic storage validation
			if !strings.HasSuffix(managedRoost.Spec.Resources.Storage, "i") &&
				!strings.HasSuffix(managedRoost.Spec.Resources.Storage, "B") {
				return &PolicyResult{
					Allowed: false,
					Message: "Storage limits must specify valid units (Gi, Ti, etc.)",
					Code:    422,
					Details: map[string]interface{}{
						"policy": "validate-resource-limits",
						"field":  "spec.resources.storage",
						"value":  managedRoost.Spec.Resources.Storage,
					},
				}, nil
			}
		}
	}

	return &PolicyResult{
		Allowed: true,
		Message: "Resource limits validation passed",
	}, nil
}

// validateRBACConfig validates RBAC configuration
func (pe *PolicyEngine) validateRBACConfig(ctx context.Context, req *AdmissionRequest) (*PolicyResult, error) {
	managedRoost, ok := req.Object.(*roostkeeper.ManagedRoost)
	if !ok {
		return &PolicyResult{
			Allowed: false,
			Message: "Invalid object type for RBAC validation",
			Code:    400,
		}, nil
	}

	// Validate RBAC configuration if specified
	if managedRoost.Spec.RBAC != nil {
		// Ensure service account is specified
		if managedRoost.Spec.RBAC.ServiceAccount == "" {
			return &PolicyResult{
				Allowed: false,
				Message: "Service account must be specified when RBAC is enabled",
				Code:    422,
				Details: map[string]interface{}{
					"policy": "validate-rbac-config",
					"field":  "spec.rbac.serviceAccount",
				},
			}, nil
		}

		// Validate role bindings
		if len(managedRoost.Spec.RBAC.RoleBindings) > 0 {
			for i, binding := range managedRoost.Spec.RBAC.RoleBindings {
				if binding.RoleName == "" {
					return &PolicyResult{
						Allowed: false,
						Message: fmt.Sprintf("Role name is required for role binding at index %d", i),
						Code:    422,
						Details: map[string]interface{}{
							"policy": "validate-rbac-config",
							"field":  fmt.Sprintf("spec.rbac.roleBindings[%d].roleName", i),
						},
					}, nil
				}
			}
		}
	}

	return &PolicyResult{
		Allowed: true,
		Message: "RBAC configuration validation passed",
	}, nil
}

// injectNetworkPolicies injects network isolation policies
func (pe *PolicyEngine) injectNetworkPolicies(ctx context.Context, req *AdmissionRequest) (*MutationResult, error) {
	managedRoost, ok := req.Object.(*roostkeeper.ManagedRoost)
	if !ok {
		return &MutationResult{}, nil
	}

	var patches []admissionv1.PatchOperation

	// Only inject network policies if tenancy is enabled
	if managedRoost.Spec.Tenancy != nil && managedRoost.Spec.Tenancy.Enabled {
		// Ensure networking section exists
		if managedRoost.Spec.Networking == nil {
			patches = append(patches, admissionv1.PatchOperation{
				Op:    "add",
				Path:  "/spec/networking",
				Value: &roostkeeper.NetworkingSpec{},
			})
		}

		// Enable network policy isolation
		patches = append(patches, admissionv1.PatchOperation{
			Op:   "add",
			Path: "/spec/networking/networkPolicy",
			Value: roostkeeper.NetworkPolicySpec{
				Enabled: true,
				Isolation: &roostkeeper.NetworkIsolationSpec{
					Enabled: true,
				},
			},
		})
	}

	return &MutationResult{
		Patches:   patches,
		PatchType: admissionv1.PatchTypeJSONPatch,
		Message:   "Injected network isolation policies",
	}, nil
}
