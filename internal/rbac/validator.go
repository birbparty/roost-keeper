package rbac

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"go.uber.org/zap"
	rbacv1 "k8s.io/api/rbac/v1"
)

// PolicyValidator provides comprehensive policy validation for RBAC configurations
type PolicyValidator struct {
	logger *zap.Logger
}

// ValidationIssue represents a policy validation issue
type ValidationIssue struct {
	Severity    string `json:"severity"`    // error, warning, info
	Component   string `json:"component"`   // role, binding, etc.
	Rule        string `json:"rule"`        // validation rule name
	Message     string `json:"message"`     // human readable message
	Suggestion  string `json:"suggestion"`  // suggested fix
	ResourceRef string `json:"resourceRef"` // reference to the problematic resource
}

// ValidationResult contains the results of policy validation
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Issues []ValidationIssue `json:"issues"`
	Stats  ValidationStats   `json:"stats"`
}

// ValidationStats contains validation statistics
type ValidationStats struct {
	TotalRules      int `json:"totalRules"`
	ErrorCount      int `json:"errorCount"`
	WarningCount    int `json:"warningCount"`
	InfoCount       int `json:"infoCount"`
	SecurityScore   int `json:"securityScore"`   // 0-100
	ComplianceScore int `json:"complianceScore"` // 0-100
}

// SecurityRisk levels for policy rules
const (
	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

// NewPolicyValidator creates a new policy validator
func NewPolicyValidator(logger *zap.Logger) *PolicyValidator {
	return &PolicyValidator{
		logger: logger.With(zap.String("component", "policy-validator")),
	}
}

// ValidateRBACConfiguration validates complete RBAC configuration
func (v *PolicyValidator) ValidateRBACConfiguration(ctx context.Context, rbacSpec *roostv1alpha1.RBACSpec) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Issues: []ValidationIssue{},
		Stats:  ValidationStats{},
	}

	v.logger.Info("Starting RBAC policy validation", zap.String("tenant", rbacSpec.TenantID))

	// Validate service accounts
	v.validateServiceAccounts(rbacSpec, result)

	// Validate roles
	v.validateRoles(rbacSpec, result)

	// Validate role bindings
	v.validateRoleBindings(rbacSpec, result)

	// Check for security best practices
	v.validateSecurityBestPractices(rbacSpec, result)

	// Check for least privilege compliance
	v.validateLeastPrivilege(rbacSpec, result)

	// Check for privilege escalation risks
	v.validatePrivilegeEscalation(rbacSpec, result)

	// Calculate security and compliance scores
	v.calculateScores(result)

	// Determine overall validity
	result.Valid = result.Stats.ErrorCount == 0

	v.logger.Info("RBAC policy validation completed",
		zap.Bool("valid", result.Valid),
		zap.Int("errors", result.Stats.ErrorCount),
		zap.Int("warnings", result.Stats.WarningCount),
		zap.Int("security_score", result.Stats.SecurityScore))

	return result
}

// validateServiceAccounts validates service account configurations
func (v *PolicyValidator) validateServiceAccounts(rbacSpec *roostv1alpha1.RBACSpec, result *ValidationResult) {
	for _, sa := range rbacSpec.ServiceAccounts {
		// Validate service account name
		if !v.isValidKubernetesName(sa.Name) {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "error",
				Component:   "service_account",
				Rule:        "invalid_name",
				Message:     fmt.Sprintf("Service account name '%s' is not valid", sa.Name),
				Suggestion:  "Use lowercase alphanumeric characters and hyphens only",
				ResourceRef: sa.Name,
			})
		}

		// Check for security best practices
		if sa.AutomountToken {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "warning",
				Component:   "service_account",
				Rule:        "automount_token",
				Message:     fmt.Sprintf("Service account '%s' has automount token enabled", sa.Name),
				Suggestion:  "Disable automount token unless specifically required",
				ResourceRef: sa.Name,
			})
		}

		// Validate lifecycle configuration
		if sa.Lifecycle != nil && sa.Lifecycle.TokenRotation != nil && sa.Lifecycle.TokenRotation.Enabled {
			if sa.Lifecycle.TokenRotation.RotationInterval.Duration.Hours() < 24 {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity:    "warning",
					Component:   "service_account",
					Rule:        "short_rotation_interval",
					Message:     fmt.Sprintf("Service account '%s' has very short token rotation interval", sa.Name),
					Suggestion:  "Consider using at least 24 hour rotation interval",
					ResourceRef: sa.Name,
				})
			}
		}
	}
}

// validateRoles validates role configurations
func (v *PolicyValidator) validateRoles(rbacSpec *roostv1alpha1.RBACSpec, result *ValidationResult) {
	for _, role := range rbacSpec.Roles {
		// Validate role name
		if !v.isValidKubernetesName(role.Name) {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "error",
				Component:   "role",
				Rule:        "invalid_name",
				Message:     fmt.Sprintf("Role name '%s' is not valid", role.Name),
				Suggestion:  "Use lowercase alphanumeric characters and hyphens only",
				ResourceRef: role.Name,
			})
		}

		// Validate rules if explicitly defined
		if len(role.Rules) > 0 {
			v.validatePolicyRules(role.Rules, role.Name, result)
		}

		// Check for overly broad permissions
		v.checkBroadPermissions(role.Rules, role.Name, result)

		result.Stats.TotalRules += len(role.Rules)
	}
}

// validateRoleBindings validates role binding configurations
func (v *PolicyValidator) validateRoleBindings(rbacSpec *roostv1alpha1.RBACSpec, result *ValidationResult) {
	for _, binding := range rbacSpec.RoleBindings {
		// Validate binding name
		if !v.isValidKubernetesName(binding.Name) {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "error",
				Component:   "role_binding",
				Rule:        "invalid_name",
				Message:     fmt.Sprintf("Role binding name '%s' is not valid", binding.Name),
				Suggestion:  "Use lowercase alphanumeric characters and hyphens only",
				ResourceRef: binding.Name,
			})
		}

		// Validate subjects
		if len(binding.Subjects) == 0 {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "error",
				Component:   "role_binding",
				Rule:        "no_subjects",
				Message:     fmt.Sprintf("Role binding '%s' has no subjects", binding.Name),
				Suggestion:  "Add at least one subject to the role binding",
				ResourceRef: binding.Name,
			})
		}

		// Validate subject names
		for _, subject := range binding.Subjects {
			if subject.Name == "" {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity:    "error",
					Component:   "role_binding",
					Rule:        "empty_subject_name",
					Message:     fmt.Sprintf("Role binding '%s' has subject with empty name", binding.Name),
					Suggestion:  "Provide a valid name for all subjects",
					ResourceRef: binding.Name,
				})
			}
		}

		// Check for wildcard subjects (security risk)
		v.checkWildcardSubjects(binding, result)
	}
}

// validatePolicyRules validates individual policy rules
func (v *PolicyValidator) validatePolicyRules(rules []rbacv1.PolicyRule, roleName string, result *ValidationResult) {
	for i, rule := range rules {
		ruleRef := fmt.Sprintf("%s.rule[%d]", roleName, i)

		// Check for empty rules
		if len(rule.Verbs) == 0 {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "error",
				Component:   "policy_rule",
				Rule:        "no_verbs",
				Message:     fmt.Sprintf("Policy rule in role '%s' has no verbs", roleName),
				Suggestion:  "Specify at least one verb for the policy rule",
				ResourceRef: ruleRef,
			})
		}

		if len(rule.Resources) == 0 && len(rule.NonResourceURLs) == 0 {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "error",
				Component:   "policy_rule",
				Rule:        "no_resources",
				Message:     fmt.Sprintf("Policy rule in role '%s' has no resources or URLs", roleName),
				Suggestion:  "Specify resources or non-resource URLs for the policy rule",
				ResourceRef: ruleRef,
			})
		}

		// Check for dangerous combinations
		v.checkDangerousPermissions(rule, roleName, ruleRef, result)
	}
}

// validateSecurityBestPractices checks for security best practices
func (v *PolicyValidator) validateSecurityBestPractices(rbacSpec *roostv1alpha1.RBACSpec, result *ValidationResult) {
	// Check if audit logging is enabled
	if rbacSpec.Audit == nil || !rbacSpec.Audit.Enabled {
		result.Issues = append(result.Issues, ValidationIssue{
			Severity:   "warning",
			Component:  "configuration",
			Rule:       "audit_disabled",
			Message:    "Audit logging is not enabled",
			Suggestion: "Enable audit logging for security compliance",
		})
	}

	// Check if policy validation is enabled
	if rbacSpec.PolicyValidation == nil || !rbacSpec.PolicyValidation.Enabled {
		result.Issues = append(result.Issues, ValidationIssue{
			Severity:   "info",
			Component:  "configuration",
			Rule:       "validation_disabled",
			Message:    "Policy validation is not enabled",
			Suggestion: "Enable policy validation for better security",
		})
	}

	// Check for identity provider integration
	if rbacSpec.IdentityProvider == "" {
		result.Issues = append(result.Issues, ValidationIssue{
			Severity:   "info",
			Component:  "configuration",
			Rule:       "no_identity_provider",
			Message:    "No identity provider configured",
			Suggestion: "Consider integrating with an identity provider for better user management",
		})
	}
}

// validateLeastPrivilege checks for least privilege compliance
func (v *PolicyValidator) validateLeastPrivilege(rbacSpec *roostv1alpha1.RBACSpec, result *ValidationResult) {
	for _, role := range rbacSpec.Roles {
		for i, rule := range role.Rules {
			ruleRef := fmt.Sprintf("%s.rule[%d]", role.Name, i)

			// Check for wildcard permissions
			if v.containsWildcard(rule.Verbs) {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity:    "warning",
					Component:   "policy_rule",
					Rule:        "wildcard_verbs",
					Message:     fmt.Sprintf("Role '%s' uses wildcard verbs", role.Name),
					Suggestion:  "Specify explicit verbs instead of wildcards",
					ResourceRef: ruleRef,
				})
			}

			if v.containsWildcard(rule.Resources) {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity:    "warning",
					Component:   "policy_rule",
					Rule:        "wildcard_resources",
					Message:     fmt.Sprintf("Role '%s' uses wildcard resources", role.Name),
					Suggestion:  "Specify explicit resources instead of wildcards",
					ResourceRef: ruleRef,
				})
			}

			// Check for overly broad API groups
			if v.containsWildcard(rule.APIGroups) || v.contains(rule.APIGroups, "") {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity:    "warning",
					Component:   "policy_rule",
					Rule:        "broad_api_groups",
					Message:     fmt.Sprintf("Role '%s' grants access to core API group or uses wildcards", role.Name),
					Suggestion:  "Limit to specific API groups when possible",
					ResourceRef: ruleRef,
				})
			}
		}
	}
}

// validatePrivilegeEscalation checks for privilege escalation risks
func (v *PolicyValidator) validatePrivilegeEscalation(rbacSpec *roostv1alpha1.RBACSpec, result *ValidationResult) {
	for _, role := range rbacSpec.Roles {
		for i, rule := range role.Rules {
			ruleRef := fmt.Sprintf("%s.rule[%d]", role.Name, i)

			// Check for dangerous RBAC permissions
			if v.hasRBACPermissions(rule) {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity:    "high",
					Component:   "policy_rule",
					Rule:        "rbac_permissions",
					Message:     fmt.Sprintf("Role '%s' has RBAC management permissions", role.Name),
					Suggestion:  "Limit RBAC permissions to authorized administrators only",
					ResourceRef: ruleRef,
				})
			}

			// Check for cluster-admin level permissions
			if v.hasClusterAdminPermissions(rule) {
				result.Issues = append(result.Issues, ValidationIssue{
					Severity:    "critical",
					Component:   "policy_rule",
					Rule:        "cluster_admin_permissions",
					Message:     fmt.Sprintf("Role '%s' has cluster-admin level permissions", role.Name),
					Suggestion:  "Review if such broad permissions are truly necessary",
					ResourceRef: ruleRef,
				})
			}
		}
	}
}

// Helper methods for validation

func (v *PolicyValidator) isValidKubernetesName(name string) bool {
	// Kubernetes name validation (DNS-1123 subdomain)
	nameRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	return len(name) <= 63 && nameRegex.MatchString(name)
}

func (v *PolicyValidator) containsWildcard(slice []string) bool {
	return v.contains(slice, "*")
}

func (v *PolicyValidator) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (v *PolicyValidator) checkBroadPermissions(rules []rbacv1.PolicyRule, roleName string, result *ValidationResult) {
	for i, rule := range rules {
		ruleRef := fmt.Sprintf("%s.rule[%d]", roleName, i)

		// Check for admin-level permissions on secrets
		if v.contains(rule.Resources, "secrets") && v.contains(rule.Verbs, "*") {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "high",
				Component:   "policy_rule",
				Rule:        "broad_secret_access",
				Message:     fmt.Sprintf("Role '%s' has broad access to secrets", roleName),
				Suggestion:  "Limit secret access to specific resources when possible",
				ResourceRef: ruleRef,
			})
		}
	}
}

func (v *PolicyValidator) checkWildcardSubjects(binding roostv1alpha1.RoleBindingSpec, result *ValidationResult) {
	for _, subject := range binding.Subjects {
		if strings.Contains(subject.Name, "*") {
			result.Issues = append(result.Issues, ValidationIssue{
				Severity:    "high",
				Component:   "role_binding",
				Rule:        "wildcard_subject",
				Message:     fmt.Sprintf("Role binding '%s' uses wildcard in subject name", binding.Name),
				Suggestion:  "Use explicit subject names instead of wildcards",
				ResourceRef: binding.Name,
			})
		}
	}
}

func (v *PolicyValidator) checkDangerousPermissions(rule rbacv1.PolicyRule, roleName, ruleRef string, result *ValidationResult) {
	// Check for exec permissions on pods
	if v.contains(rule.Resources, "pods") && v.contains(rule.Verbs, "create") &&
		len(rule.ResourceNames) == 0 {
		result.Issues = append(result.Issues, ValidationIssue{
			Severity:    "medium",
			Component:   "policy_rule",
			Rule:        "pod_create_access",
			Message:     fmt.Sprintf("Role '%s' can create pods without restriction", roleName),
			Suggestion:  "Consider restricting pod creation or adding resource name constraints",
			ResourceRef: ruleRef,
		})
	}

	// Check for node access
	if v.contains(rule.Resources, "nodes") {
		result.Issues = append(result.Issues, ValidationIssue{
			Severity:    "high",
			Component:   "policy_rule",
			Rule:        "node_access",
			Message:     fmt.Sprintf("Role '%s' has access to node resources", roleName),
			Suggestion:  "Node access should be limited to cluster administrators",
			ResourceRef: ruleRef,
		})
	}
}

func (v *PolicyValidator) hasRBACPermissions(rule rbacv1.PolicyRule) bool {
	rbacResources := []string{"roles", "rolebindings", "clusterroles", "clusterrolebindings"}
	for _, resource := range rbacResources {
		if v.contains(rule.Resources, resource) {
			return true
		}
	}
	return false
}

func (v *PolicyValidator) hasClusterAdminPermissions(rule rbacv1.PolicyRule) bool {
	return v.containsWildcard(rule.APIGroups) &&
		v.containsWildcard(rule.Resources) &&
		v.containsWildcard(rule.Verbs)
}

func (v *PolicyValidator) calculateScores(result *ValidationResult) {
	// Calculate security score (0-100)
	totalIssues := len(result.Issues)
	if totalIssues == 0 {
		result.Stats.SecurityScore = 100
	} else {
		// Weighted scoring based on severity
		severityWeights := map[string]int{
			"error":   10,
			"high":    7,
			"medium":  4,
			"warning": 2,
			"info":    1,
		}

		totalWeight := 0
		for _, issue := range result.Issues {
			if weight, exists := severityWeights[issue.Severity]; exists {
				totalWeight += weight
			}
		}

		// Calculate score (max penalty of 80 points)
		penalty := totalWeight * 2
		if penalty > 80 {
			penalty = 80
		}
		result.Stats.SecurityScore = 100 - penalty
	}

	// Calculate compliance score (simplified)
	result.Stats.ComplianceScore = result.Stats.SecurityScore

	// Count issues by severity
	for _, issue := range result.Issues {
		switch issue.Severity {
		case "error", "critical":
			result.Stats.ErrorCount++
		case "warning", "high", "medium":
			result.Stats.WarningCount++
		case "info":
			result.Stats.InfoCount++
		}
	}
}
