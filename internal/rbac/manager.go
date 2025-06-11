package rbac

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Manager provides enterprise-grade RBAC management with identity provider integration,
// policy templates, audit logging, and comprehensive validation
type Manager struct {
	client            client.Client
	scheme            *runtime.Scheme
	logger            *zap.Logger
	policyTemplates   map[string]*PolicyTemplate
	identityProviders map[string]IdentityProvider
	auditLogger       *AuditLogger
	validator         *PolicyValidator
}

// PolicyTemplate defines a reusable policy template with parameter substitution
type PolicyTemplate struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Rules       []rbacv1.PolicyRule `json:"rules"`
	Subjects    []rbacv1.Subject    `json:"subjects,omitempty"`
	Metadata    map[string]string   `json:"metadata,omitempty"`
	Parameters  []TemplateParameter `json:"parameters,omitempty"`
}

// TemplateParameter defines a parameter that can be substituted in templates
type TemplateParameter struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	DefaultValue string `json:"defaultValue,omitempty"`
	Required     bool   `json:"required"`
}

// IdentityProvider interface for integrating with external identity providers
type IdentityProvider interface {
	ValidateToken(ctx context.Context, token string) (*Identity, error)
	GetUserGroups(ctx context.Context, username string) ([]string, error)
	GetUserInfo(ctx context.Context, username string) (*UserInfo, error)
	GetProviderType() string
}

// Identity represents an authenticated identity
type Identity struct {
	Username    string                 `json:"username"`
	Groups      []string               `json:"groups"`
	Tenant      string                 `json:"tenant,omitempty"`
	Claims      map[string]interface{} `json:"claims"`
	ExpiresAt   time.Time              `json:"expiresAt"`
	ValidatedAt time.Time              `json:"validatedAt"`
}

// UserInfo contains detailed user information from identity provider
type UserInfo struct {
	Username    string            `json:"username"`
	DisplayName string            `json:"displayName"`
	Email       string            `json:"email"`
	Groups      []string          `json:"groups"`
	Attributes  map[string]string `json:"attributes"`
	Tenant      string            `json:"tenant,omitempty"`
}

// NewManager creates a new advanced RBAC manager
func NewManager(client client.Client, scheme *runtime.Scheme, logger *zap.Logger) *Manager {
	manager := &Manager{
		client:            client,
		scheme:            scheme,
		logger:            logger.With(zap.String("component", "rbac-manager")),
		policyTemplates:   make(map[string]*PolicyTemplate),
		identityProviders: make(map[string]IdentityProvider),
		auditLogger:       NewAuditLogger(logger),
		validator:         NewPolicyValidator(logger),
	}

	// Register default policy templates
	manager.registerDefaultTemplates()

	return manager
}

// SetupRBAC orchestrates complete RBAC setup for a ManagedRoost
func (m *Manager) SetupRBAC(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	if managedRoost.Spec.Tenancy == nil || managedRoost.Spec.Tenancy.RBAC == nil || !managedRoost.Spec.Tenancy.RBAC.Enabled {
		m.logger.Debug("RBAC not enabled, skipping setup")
		return nil
	}

	rbacSpec := managedRoost.Spec.Tenancy.RBAC
	tenantID := rbacSpec.TenantID

	log := m.logger.With(
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
		zap.String("tenant", tenantID),
	)

	// Start RBAC setup span
	ctx, span := telemetry.StartControllerSpan(ctx, "rbac.setup", managedRoost.Name, managedRoost.Namespace)
	defer span.End()

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		log.Info("RBAC setup completed", zap.Duration("duration", duration))
	}()

	log.Info("Starting advanced RBAC setup")

	// Step 1: Validate RBAC configuration
	if err := m.validateRBACConfiguration(rbacSpec); err != nil {
		telemetry.RecordError(span, err)
		return fmt.Errorf("RBAC configuration validation failed: %w", err)
	}

	// Step 2: Setup identity provider if configured
	if rbacSpec.IdentityProvider != "" && rbacSpec.IdentityProviderConfig != nil {
		if err := m.setupIdentityProvider(ctx, rbacSpec); err != nil {
			telemetry.RecordError(span, err)
			return fmt.Errorf("failed to setup identity provider: %w", err)
		}
	}

	// Step 3: Create service accounts
	if err := m.createServiceAccounts(ctx, managedRoost, rbacSpec); err != nil {
		telemetry.RecordError(span, err)
		return fmt.Errorf("failed to create service accounts: %w", err)
	}

	// Step 4: Create roles from templates or explicit rules
	if err := m.createRoles(ctx, managedRoost, rbacSpec); err != nil {
		telemetry.RecordError(span, err)
		return fmt.Errorf("failed to create roles: %w", err)
	}

	// Step 5: Create role bindings
	if err := m.createRoleBindings(ctx, managedRoost, rbacSpec); err != nil {
		telemetry.RecordError(span, err)
		return fmt.Errorf("failed to create role bindings: %w", err)
	}

	// Step 6: Validate created policies if validation is enabled
	if rbacSpec.PolicyValidation != nil && rbacSpec.PolicyValidation.Enabled {
		if err := m.validateCreatedPolicies(ctx, managedRoost, rbacSpec); err != nil {
			telemetry.RecordError(span, err)
			return fmt.Errorf("policy validation failed: %w", err)
		}
	}

	// Step 7: Audit the RBAC setup
	m.auditLogger.LogRBACSetup(ctx, managedRoost, rbacSpec)

	log.Info("Advanced RBAC setup completed successfully")
	return nil
}

// validateRBACConfiguration validates the complete RBAC configuration
func (m *Manager) validateRBACConfiguration(rbacSpec *roostv1alpha1.RBACSpec) error {
	if rbacSpec.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	// Validate service accounts
	for _, sa := range rbacSpec.ServiceAccounts {
		if sa.Name == "" {
			return fmt.Errorf("service account name cannot be empty")
		}
		if len(sa.Name) > 63 {
			return fmt.Errorf("service account name %s exceeds 63 characters", sa.Name)
		}
	}

	// Validate roles
	for _, role := range rbacSpec.Roles {
		if role.Name == "" {
			return fmt.Errorf("role name cannot be empty")
		}
		if len(role.Name) > 63 {
			return fmt.Errorf("role name %s exceeds 63 characters", role.Name)
		}

		// Validate template reference if used
		if role.Template != "" {
			if _, exists := m.policyTemplates[role.Template]; !exists {
				return fmt.Errorf("policy template %s not found", role.Template)
			}
		}
	}

	// Validate role bindings
	for _, binding := range rbacSpec.RoleBindings {
		if binding.Name == "" {
			return fmt.Errorf("role binding name cannot be empty")
		}
		if len(binding.Subjects) == 0 {
			return fmt.Errorf("role binding %s must have at least one subject", binding.Name)
		}
	}

	return nil
}

// setupIdentityProvider configures the identity provider for RBAC
func (m *Manager) setupIdentityProvider(ctx context.Context, rbacSpec *roostv1alpha1.RBACSpec) error {
	providerName := rbacSpec.IdentityProvider
	config := rbacSpec.IdentityProviderConfig

	m.logger.Info("Setting up identity provider", zap.String("provider", providerName))

	var provider IdentityProvider
	var err error

	switch {
	case config.OIDC != nil:
		provider, err = NewOIDCProvider(config.OIDC, m.logger)
	case config.LDAP != nil:
		provider, err = NewLDAPProvider(config.LDAP, m.logger)
	case config.Custom != nil:
		// Custom provider support can be added here
		return fmt.Errorf("custom identity providers not yet supported")
	default:
		return fmt.Errorf("no identity provider configuration found")
	}

	if err != nil {
		return fmt.Errorf("failed to create identity provider: %w", err)
	}

	m.RegisterIdentityProvider(providerName, provider)
	return nil
}

// createServiceAccounts creates all configured service accounts
func (m *Manager) createServiceAccounts(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec) error {
	for _, saSpec := range rbacSpec.ServiceAccounts {
		if err := m.createServiceAccount(ctx, managedRoost, rbacSpec, saSpec); err != nil {
			return fmt.Errorf("failed to create service account %s: %w", saSpec.Name, err)
		}
	}
	return nil
}

// createServiceAccount creates a single service account with full lifecycle management
func (m *Manager) createServiceAccount(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, saSpec roostv1alpha1.ServiceAccountSpec) error {
	automountToken := saSpec.AutomountToken

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saSpec.Name,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				"roost-keeper.io/managed-by": "roost-keeper",
				"roost-keeper.io/roost":      managedRoost.Name,
				"roost-keeper.io/tenant":     rbacSpec.TenantID,
				"roost-keeper.io/component":  "rbac",
			},
			Annotations: saSpec.Annotations,
		},
		AutomountServiceAccountToken: &automountToken,
	}

	// Add image pull secrets if specified
	for _, secretName := range saSpec.ImagePullSecrets {
		sa.ImagePullSecrets = append(sa.ImagePullSecrets, corev1.LocalObjectReference{
			Name: secretName,
		})
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, sa, m.scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := m.client.Create(ctx, sa); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		m.logger.Debug("Service account already exists", zap.String("name", sa.Name))
	} else {
		m.logger.Info("Created service account",
			zap.String("name", sa.Name),
			zap.String("namespace", sa.Namespace))
	}

	// Setup token rotation if configured
	if saSpec.Lifecycle != nil && saSpec.Lifecycle.TokenRotation != nil && saSpec.Lifecycle.TokenRotation.Enabled {
		if err := m.setupTokenRotation(ctx, sa, saSpec.Lifecycle.TokenRotation); err != nil {
			m.logger.Warn("Failed to setup token rotation",
				zap.String("serviceAccount", sa.Name),
				zap.Error(err))
		}
	}

	return nil
}

// createRoles creates all configured roles using templates or explicit rules
func (m *Manager) createRoles(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec) error {
	for _, roleSpec := range rbacSpec.Roles {
		if err := m.createRole(ctx, managedRoost, rbacSpec, roleSpec); err != nil {
			return fmt.Errorf("failed to create role %s: %w", roleSpec.Name, err)
		}
	}
	return nil
}

// createRole creates a single role from template or explicit rules
func (m *Manager) createRole(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, roleSpec roostv1alpha1.RoleSpec) error {
	var rules []rbacv1.PolicyRule

	// Generate role from template if specified
	if roleSpec.Template != "" {
		template, exists := m.policyTemplates[roleSpec.Template]
		if !exists {
			return fmt.Errorf("policy template %s not found", roleSpec.Template)
		}

		// Apply template with parameter substitution
		rules = m.applyTemplateParameters(template.Rules, managedRoost, rbacSpec, roleSpec)
	} else {
		// Use explicit rules
		rules = roleSpec.Rules
	}

	// Apply global parameter substitution
	rules = m.substituteParameters(rules, managedRoost, rbacSpec)

	// Create Role or ClusterRole based on type
	if roleSpec.Type == "ClusterRole" {
		return m.createClusterRole(ctx, managedRoost, rbacSpec, roleSpec, rules)
	}

	return m.createNamespacedRole(ctx, managedRoost, rbacSpec, roleSpec, rules)
}

// createNamespacedRole creates a namespaced Role
func (m *Manager) createNamespacedRole(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, roleSpec roostv1alpha1.RoleSpec, rules []rbacv1.PolicyRule) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleSpec.Name,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				"roost-keeper.io/managed-by": "roost-keeper",
				"roost-keeper.io/roost":      managedRoost.Name,
				"roost-keeper.io/tenant":     rbacSpec.TenantID,
				"roost-keeper.io/component":  "rbac",
			},
			Annotations: roleSpec.Annotations,
		},
		Rules: rules,
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, role, m.scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := m.client.Create(ctx, role); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		m.logger.Debug("Role already exists", zap.String("name", role.Name))
	} else {
		m.logger.Info("Created role",
			zap.String("name", role.Name),
			zap.String("namespace", role.Namespace),
			zap.Int("rules_count", len(role.Rules)))
	}

	return nil
}

// createClusterRole creates a ClusterRole
func (m *Manager) createClusterRole(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, roleSpec roostv1alpha1.RoleSpec, rules []rbacv1.PolicyRule) error {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-%s", managedRoost.Namespace, managedRoost.Name, roleSpec.Name),
			Labels: map[string]string{
				"roost-keeper.io/managed-by": "roost-keeper",
				"roost-keeper.io/roost":      managedRoost.Name,
				"roost-keeper.io/tenant":     rbacSpec.TenantID,
				"roost-keeper.io/component":  "rbac",
			},
			Annotations: roleSpec.Annotations,
		},
		Rules: rules,
	}

	if err := m.client.Create(ctx, clusterRole); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		m.logger.Debug("ClusterRole already exists", zap.String("name", clusterRole.Name))
	} else {
		m.logger.Info("Created cluster role",
			zap.String("name", clusterRole.Name),
			zap.Int("rules_count", len(clusterRole.Rules)))
	}

	return nil
}

// applyTemplateParameters applies template-specific parameter substitution
func (m *Manager) applyTemplateParameters(rules []rbacv1.PolicyRule, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, roleSpec roostv1alpha1.RoleSpec) []rbacv1.PolicyRule {
	substitutedRules := make([]rbacv1.PolicyRule, len(rules))

	for i, rule := range rules {
		substitutedRule := rule.DeepCopy()

		// Apply role-specific template parameters
		for paramName, paramValue := range roleSpec.TemplateParameters {
			placeholder := fmt.Sprintf("{{.%s}}", paramName)

			// Substitute in resource names
			for j, resourceName := range substitutedRule.ResourceNames {
				substitutedRule.ResourceNames[j] = strings.ReplaceAll(resourceName, placeholder, paramValue)
			}

			// Substitute in resources
			for j, resource := range substitutedRule.Resources {
				substitutedRule.Resources[j] = strings.ReplaceAll(resource, placeholder, paramValue)
			}
		}

		substitutedRules[i] = *substitutedRule
	}

	return substitutedRules
}

// substituteParameters applies global parameter substitution
func (m *Manager) substituteParameters(rules []rbacv1.PolicyRule, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec) []rbacv1.PolicyRule {
	substitutedRules := make([]rbacv1.PolicyRule, len(rules))

	// Global template parameters
	globalParams := map[string]string{
		"RoostName": managedRoost.Name,
		"Namespace": managedRoost.Namespace,
		"TenantID":  rbacSpec.TenantID,
	}

	// Add template parameters from RBAC spec
	for key, value := range rbacSpec.TemplateParameters {
		globalParams[key] = value
	}

	for i, rule := range rules {
		substitutedRule := rule.DeepCopy()

		// Apply global parameter substitution
		for paramName, paramValue := range globalParams {
			placeholder := fmt.Sprintf("{{.%s}}", paramName)

			// Substitute resource names
			for j, resourceName := range substitutedRule.ResourceNames {
				substitutedRule.ResourceNames[j] = strings.ReplaceAll(resourceName, placeholder, paramValue)
			}

			// Substitute resources
			for j, resource := range substitutedRule.Resources {
				substitutedRule.Resources[j] = strings.ReplaceAll(resource, placeholder, paramValue)
			}
		}

		substitutedRules[i] = *substitutedRule
	}

	return substitutedRules
}

// RegisterPolicyTemplate registers a new policy template
func (m *Manager) RegisterPolicyTemplate(template *PolicyTemplate) {
	m.policyTemplates[template.Name] = template
	m.logger.Info("Registered policy template", zap.String("name", template.Name))
}

// RegisterIdentityProvider registers a new identity provider
func (m *Manager) RegisterIdentityProvider(name string, provider IdentityProvider) {
	m.identityProviders[name] = provider
	m.logger.Info("Registered identity provider",
		zap.String("name", name),
		zap.String("type", provider.GetProviderType()))
}

// registerDefaultTemplates registers built-in policy templates
func (m *Manager) registerDefaultTemplates() {
	templates := []*PolicyTemplate{
		{
			Name:        "roost-operator",
			Description: "Full permissions for roost operators",
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"roost-keeper.io"},
					Resources: []string{"managedroosts"},
					Verbs:     []string{"*"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments", "replicasets"},
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch", "delete"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services", "configmaps", "secrets"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		},
		{
			Name:        "roost-viewer",
			Description: "Read-only access to roosts",
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"roost-keeper.io"},
					Resources: []string{"managedroosts"},
					Verbs:     []string{"get", "list", "watch"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services"},
					Verbs:     []string{"get", "list", "watch"},
				},
			},
		},
		{
			Name:        "tenant-admin",
			Description: "Tenant administrator with full permissions within tenant scope",
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{"roost-keeper.io"},
					Resources:     []string{"managedroosts"},
					Verbs:         []string{"*"},
					ResourceNames: []string{"{{.RoostName}}"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"*"},
					Verbs:     []string{"*"},
				},
			},
		},
		{
			Name:        "tenant-developer",
			Description: "Developer access within tenant scope",
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups:     []string{"roost-keeper.io"},
					Resources:     []string{"managedroosts"},
					Verbs:         []string{"get", "list", "watch", "update", "patch"},
					ResourceNames: []string{"{{.RoostName}}"},
				},
				{
					APIGroups: []string{""},
					Resources: []string{"pods", "services", "configmaps"},
					Verbs:     []string{"get", "list", "watch", "create", "update", "patch"},
				},
				{
					APIGroups: []string{"apps"},
					Resources: []string{"deployments"},
					Verbs:     []string{"get", "list", "watch", "update", "patch"},
				},
			},
		},
	}

	for _, template := range templates {
		m.RegisterPolicyTemplate(template)
	}
}

// createRoleBindings creates all configured role bindings
func (m *Manager) createRoleBindings(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec) error {
	for _, bindingSpec := range rbacSpec.RoleBindings {
		if err := m.createRoleBinding(ctx, managedRoost, rbacSpec, bindingSpec); err != nil {
			return fmt.Errorf("failed to create role binding %s: %w", bindingSpec.Name, err)
		}
	}
	return nil
}

// createRoleBinding creates a single role binding
func (m *Manager) createRoleBinding(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, bindingSpec roostv1alpha1.RoleBindingSpec) error {
	// Resolve subjects
	subjects, err := m.resolveSubjects(ctx, bindingSpec.Subjects, managedRoost, rbacSpec)
	if err != nil {
		return fmt.Errorf("failed to resolve subjects for binding %s: %w", bindingSpec.Name, err)
	}

	// Create RoleBinding or ClusterRoleBinding based on type
	if bindingSpec.Type == "ClusterRoleBinding" {
		return m.createClusterRoleBinding(ctx, managedRoost, rbacSpec, bindingSpec, subjects)
	}

	return m.createNamespacedRoleBinding(ctx, managedRoost, rbacSpec, bindingSpec, subjects)
}

// createNamespacedRoleBinding creates a namespaced RoleBinding
func (m *Manager) createNamespacedRoleBinding(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, bindingSpec roostv1alpha1.RoleBindingSpec, subjects []rbacv1.Subject) error {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bindingSpec.Name,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				"roost-keeper.io/managed-by": "roost-keeper",
				"roost-keeper.io/roost":      managedRoost.Name,
				"roost-keeper.io/tenant":     rbacSpec.TenantID,
				"roost-keeper.io/component":  "rbac",
			},
			Annotations: bindingSpec.Annotations,
		},
		Subjects: subjects,
		RoleRef: rbacv1.RoleRef{
			APIGroup: bindingSpec.RoleRef.APIGroup,
			Kind:     bindingSpec.RoleRef.Kind,
			Name:     bindingSpec.RoleRef.Name,
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, roleBinding, m.scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := m.client.Create(ctx, roleBinding); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		m.logger.Debug("Role binding already exists", zap.String("name", roleBinding.Name))
	} else {
		m.logger.Info("Created role binding",
			zap.String("name", roleBinding.Name),
			zap.String("namespace", roleBinding.Namespace),
			zap.Int("subjects_count", len(roleBinding.Subjects)))
	}

	return nil
}

// createClusterRoleBinding creates a ClusterRoleBinding
func (m *Manager) createClusterRoleBinding(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec, bindingSpec roostv1alpha1.RoleBindingSpec, subjects []rbacv1.Subject) error {
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%s-%s", managedRoost.Namespace, managedRoost.Name, bindingSpec.Name),
			Labels: map[string]string{
				"roost-keeper.io/managed-by": "roost-keeper",
				"roost-keeper.io/roost":      managedRoost.Name,
				"roost-keeper.io/tenant":     rbacSpec.TenantID,
				"roost-keeper.io/component":  "rbac",
			},
			Annotations: bindingSpec.Annotations,
		},
		Subjects: subjects,
		RoleRef: rbacv1.RoleRef{
			APIGroup: bindingSpec.RoleRef.APIGroup,
			Kind:     bindingSpec.RoleRef.Kind,
			Name:     bindingSpec.RoleRef.Name,
		},
	}

	if err := m.client.Create(ctx, clusterRoleBinding); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		m.logger.Debug("ClusterRoleBinding already exists", zap.String("name", clusterRoleBinding.Name))
	} else {
		m.logger.Info("Created cluster role binding",
			zap.String("name", clusterRoleBinding.Name),
			zap.Int("subjects_count", len(clusterRoleBinding.Subjects)))
	}

	return nil
}

// resolveSubjects resolves subjects and validates them against identity providers
func (m *Manager) resolveSubjects(ctx context.Context, subjectSpecs []roostv1alpha1.SubjectSpec, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec) ([]rbacv1.Subject, error) {
	var subjects []rbacv1.Subject

	for _, subjectSpec := range subjectSpecs {
		switch subjectSpec.Kind {
		case "ServiceAccount":
			namespace := subjectSpec.Namespace
			if namespace == "" {
				namespace = managedRoost.Namespace
			}
			subjects = append(subjects, rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      subjectSpec.Name,
				Namespace: namespace,
			})

		case "User":
			// Validate user against identity provider if validation is enabled
			if subjectSpec.ValidateWithProvider && rbacSpec.IdentityProvider != "" {
				if err := m.validateUser(ctx, subjectSpec.Name, rbacSpec.IdentityProvider); err != nil {
					return nil, fmt.Errorf("failed to validate user %s: %w", subjectSpec.Name, err)
				}
			}

			subjects = append(subjects, rbacv1.Subject{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     subjectSpec.Name,
			})

		case "Group":
			// Validate group against identity provider if validation is enabled
			if subjectSpec.ValidateWithProvider && rbacSpec.IdentityProvider != "" {
				if err := m.validateGroup(ctx, subjectSpec.Name, rbacSpec.IdentityProvider); err != nil {
					return nil, fmt.Errorf("failed to validate group %s: %w", subjectSpec.Name, err)
				}
			}

			subjects = append(subjects, rbacv1.Subject{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     subjectSpec.Name,
			})
		}
	}

	return subjects, nil
}

// validateUser validates a user against the configured identity provider
func (m *Manager) validateUser(ctx context.Context, username, providerName string) error {
	provider, exists := m.identityProviders[providerName]
	if !exists {
		return fmt.Errorf("identity provider %s not configured", providerName)
	}

	_, err := provider.GetUserInfo(ctx, username)
	if err != nil {
		return fmt.Errorf("user %s not found in identity provider: %w", username, err)
	}

	return nil
}

// validateGroup validates a group against the configured identity provider
func (m *Manager) validateGroup(ctx context.Context, groupName, providerName string) error {
	// This is a placeholder for group validation
	// In a real implementation, this would validate group existence
	return nil
}

// validateCreatedPolicies validates the created RBAC policies
func (m *Manager) validateCreatedPolicies(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec) error {
	if m.validator == nil {
		return fmt.Errorf("policy validator not available")
	}

	validationResult := m.validator.ValidateRBACConfiguration(ctx, rbacSpec)

	// Log validation results
	m.auditLogger.LogPolicyValidation(ctx, rbacSpec.TenantID,
		map[bool]string{true: "passed", false: "failed"}[validationResult.Valid],
		func() []string {
			var issues []string
			for _, issue := range validationResult.Issues {
				if issue.Severity == "error" || issue.Severity == "critical" {
					issues = append(issues, issue.Message)
				}
			}
			return issues
		}())

	// Handle validation mode
	if rbacSpec.PolicyValidation.Mode == "strict" && !validationResult.Valid {
		return fmt.Errorf("policy validation failed with %d errors", validationResult.Stats.ErrorCount)
	}

	if rbacSpec.PolicyValidation.Mode == "warn" && len(validationResult.Issues) > 0 {
		m.logger.Warn("Policy validation issues found",
			zap.Int("total_issues", len(validationResult.Issues)),
			zap.Int("errors", validationResult.Stats.ErrorCount),
			zap.Int("warnings", validationResult.Stats.WarningCount))
	}

	return nil
}

// setupTokenRotation configures automatic token rotation for a service account
func (m *Manager) setupTokenRotation(ctx context.Context, sa *corev1.ServiceAccount, policy *roostv1alpha1.TokenRotationPolicy) error {
	// This is a placeholder for token rotation implementation
	// In a real implementation, this would set up a background process
	// to rotate service account tokens according to the policy
	m.logger.Info("Token rotation configured",
		zap.String("serviceAccount", sa.Name),
		zap.Duration("interval", policy.RotationInterval.Duration))
	return nil
}

// NewOIDCProvider creates a placeholder OIDC provider
func NewOIDCProvider(config *roostv1alpha1.OIDCConfig, logger *zap.Logger) (IdentityProvider, error) {
	return &PlaceholderProvider{
		providerType: "oidc",
		logger:       logger,
	}, nil
}

// NewLDAPProvider creates a placeholder LDAP provider
func NewLDAPProvider(config *roostv1alpha1.LDAPConfig, logger *zap.Logger) (IdentityProvider, error) {
	return &PlaceholderProvider{
		providerType: "ldap",
		logger:       logger,
	}, nil
}

// PlaceholderProvider is a placeholder identity provider implementation
type PlaceholderProvider struct {
	providerType string
	logger       *zap.Logger
}

func (p *PlaceholderProvider) ValidateToken(ctx context.Context, token string) (*Identity, error) {
	return &Identity{
		Username:    "placeholder-user",
		Groups:      []string{"placeholder-group"},
		Claims:      make(map[string]interface{}),
		ExpiresAt:   time.Now().Add(time.Hour),
		ValidatedAt: time.Now(),
	}, nil
}

func (p *PlaceholderProvider) GetUserGroups(ctx context.Context, username string) ([]string, error) {
	return []string{"placeholder-group"}, nil
}

func (p *PlaceholderProvider) GetUserInfo(ctx context.Context, username string) (*UserInfo, error) {
	return &UserInfo{
		Username:    username,
		DisplayName: username,
		Email:       username + "@example.com",
		Groups:      []string{"placeholder-group"},
		Attributes:  make(map[string]string),
	}, nil
}

func (p *PlaceholderProvider) GetProviderType() string {
	return p.providerType
}
