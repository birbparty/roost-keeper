package tenancy

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// RBACManager handles tenant-specific RBAC setup and management
type RBACManager struct {
	client client.Client
	logger *zap.Logger
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager(client client.Client, logger *zap.Logger) *RBACManager {
	return &RBACManager{
		client: client,
		logger: logger.With(zap.String("component", "rbac-manager")),
	}
}

// SetupTenantRBAC creates tenant-specific RBAC resources
func (r *RBACManager) SetupTenantRBAC(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	log := r.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Info("Setting up tenant RBAC")

	// Create tenant-specific service account
	if err := r.createTenantServiceAccount(ctx, managedRoost, tenantID); err != nil {
		return fmt.Errorf("failed to create tenant service account: %w", err)
	}

	// Create tenant-specific role
	if err := r.createTenantRole(ctx, managedRoost, tenantID); err != nil {
		return fmt.Errorf("failed to create tenant role: %w", err)
	}

	// Create role binding
	if err := r.createTenantRoleBinding(ctx, managedRoost, tenantID); err != nil {
		return fmt.Errorf("failed to create tenant role binding: %w", err)
	}

	log.Info("Tenant RBAC setup completed successfully")
	return nil
}

// CleanupTenantRBAC removes tenant-specific RBAC resources
func (r *RBACManager) CleanupTenantRBAC(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	log := r.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Info("Cleaning up tenant RBAC")

	var cleanupErrors []error

	// Cleanup role binding
	roleBindingName := r.getTenantRoleBindingName(tenantID)
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: managedRoost.Namespace,
		},
	}
	if err := r.client.Delete(ctx, roleBinding); err != nil && !errors.IsNotFound(err) {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete role binding %s: %w", roleBindingName, err))
	}

	// Cleanup role
	roleName := r.getTenantRoleName(tenantID)
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: managedRoost.Namespace,
		},
	}
	if err := r.client.Delete(ctx, role); err != nil && !errors.IsNotFound(err) {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete role %s: %w", roleName, err))
	}

	// Cleanup service account
	serviceAccountName := r.getTenantServiceAccountName(tenantID)
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: managedRoost.Namespace,
		},
	}
	if err := r.client.Delete(ctx, serviceAccount); err != nil && !errors.IsNotFound(err) {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete service account %s: %w", serviceAccountName, err))
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("RBAC cleanup failed with %d errors: %v", len(cleanupErrors), cleanupErrors)
	}

	log.Info("Tenant RBAC cleanup completed successfully")
	return nil
}

// createTenantServiceAccount creates a service account for the tenant
func (r *RBACManager) createTenantServiceAccount(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	serviceAccountName := r.getTenantServiceAccountName(tenantID)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentTenantRBAC,
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, serviceAccount, r.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := r.client.Create(ctx, serviceAccount); err != nil {
		if errors.IsAlreadyExists(err) {
			r.logger.Debug("Service account already exists", zap.String("name", serviceAccountName))
			return nil
		}
		return fmt.Errorf("failed to create service account: %w", err)
	}

	r.logger.Info("Created tenant service account", zap.String("name", serviceAccountName))
	return nil
}

// createTenantRole creates a role with minimal required permissions for the tenant
func (r *RBACManager) createTenantRole(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	roleName := r.getTenantRoleName(tenantID)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentTenantRBAC,
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Rules: r.getTenantRoleRules(managedRoost, tenantID),
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, role, r.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := r.client.Create(ctx, role); err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing role
			existingRole := &rbacv1.Role{}
			if err := r.client.Get(ctx, client.ObjectKey{Name: roleName, Namespace: managedRoost.Namespace}, existingRole); err != nil {
				return fmt.Errorf("failed to get existing role: %w", err)
			}
			existingRole.Rules = role.Rules
			if err := r.client.Update(ctx, existingRole); err != nil {
				return fmt.Errorf("failed to update existing role: %w", err)
			}
			r.logger.Debug("Updated existing tenant role", zap.String("name", roleName))
			return nil
		}
		return fmt.Errorf("failed to create role: %w", err)
	}

	r.logger.Info("Created tenant role", zap.String("name", roleName))
	return nil
}

// createTenantRoleBinding creates a role binding to connect the service account to the role
func (r *RBACManager) createTenantRoleBinding(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	roleBindingName := r.getTenantRoleBindingName(tenantID)
	roleName := r.getTenantRoleName(tenantID)
	serviceAccountName := r.getTenantServiceAccountName(tenantID)

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentTenantRBAC,
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: managedRoost.Namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     roleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, roleBinding, r.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := r.client.Create(ctx, roleBinding); err != nil {
		if errors.IsAlreadyExists(err) {
			r.logger.Debug("Role binding already exists", zap.String("name", roleBindingName))
			return nil
		}
		return fmt.Errorf("failed to create role binding: %w", err)
	}

	r.logger.Info("Created tenant role binding", zap.String("name", roleBindingName))
	return nil
}

// getTenantRoleRules returns the minimal set of RBAC rules for a tenant
func (r *RBACManager) getTenantRoleRules(managedRoost *roostv1alpha1.ManagedRoost, tenantID string) []rbacv1.PolicyRule {
	rules := []rbacv1.PolicyRule{
		// Allow access to own ManagedRoost resource
		{
			APIGroups:     []string{"roost.birb.party"},
			Resources:     []string{"managedroosts"},
			Verbs:         []string{"get", "list", "watch", "update", "patch"},
			ResourceNames: []string{managedRoost.Name},
		},
		// Allow read access to ConfigMaps and Secrets in same namespace
		{
			APIGroups: []string{""},
			Resources: []string{"configmaps", "secrets"},
			Verbs:     []string{"get", "list", "watch"},
		},
		// Allow read access to pods for health checks
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		},
		// Allow read access to services for health checks
		{
			APIGroups: []string{""},
			Resources: []string{"services"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}

	// Add custom roles if specified in tenant RBAC config
	if managedRoost.Spec.Tenancy != nil && managedRoost.Spec.Tenancy.RBAC != nil {
		for _, roleSpec := range managedRoost.Spec.Tenancy.RBAC.Roles {
			// If the role has explicit rules, use them
			if len(roleSpec.Rules) > 0 {
				rules = append(rules, roleSpec.Rules...)
			} else {
				// Add a basic rule based on role name
				rules = append(rules, rbacv1.PolicyRule{
					APIGroups: []string{""},
					Resources: []string{roleSpec.Name},
					Verbs:     []string{"get", "list", "watch"},
				})
			}
		}
	}

	return rules
}

// Helper functions for generating consistent names
func (r *RBACManager) getTenantServiceAccountName(tenantID string) string {
	return fmt.Sprintf("roost-keeper-tenant-%s", tenantID)
}

func (r *RBACManager) getTenantRoleName(tenantID string) string {
	return fmt.Sprintf("roost-keeper-tenant-%s", tenantID)
}

func (r *RBACManager) getTenantRoleBindingName(tenantID string) string {
	return fmt.Sprintf("roost-keeper-tenant-%s", tenantID)
}

// GetTenantServiceAccountName returns the service account name for a tenant
func (r *RBACManager) GetTenantServiceAccountName(tenantID string) string {
	return r.getTenantServiceAccountName(tenantID)
}

// ValidateRBACConfiguration validates the RBAC configuration in the tenant spec
func (r *RBACManager) ValidateRBACConfiguration(rbacSpec *roostv1alpha1.RBACSpec) error {
	if rbacSpec == nil {
		return nil
	}

	// Validate tenant ID
	if rbacSpec.TenantID == "" {
		return fmt.Errorf("tenant ID is required when RBAC is enabled")
	}
	if len(rbacSpec.TenantID) > 63 {
		return fmt.Errorf("tenant ID must be 63 characters or less")
	}

	// Validate service account names
	for _, sa := range rbacSpec.ServiceAccounts {
		if sa.Name == "" {
			return fmt.Errorf("service account name cannot be empty")
		}
		if len(sa.Name) > 63 {
			return fmt.Errorf("service account name %s must be 63 characters or less", sa.Name)
		}
	}

	// Validate role names
	for _, role := range rbacSpec.Roles {
		if role.Name == "" {
			return fmt.Errorf("role name cannot be empty")
		}
		if len(role.Name) > 63 {
			return fmt.Errorf("role name %s must be 63 characters or less", role.Name)
		}
	}

	// Validate role binding names
	for _, roleBinding := range rbacSpec.RoleBindings {
		if roleBinding.Name == "" {
			return fmt.Errorf("role binding name cannot be empty")
		}
		if len(roleBinding.Name) > 63 {
			return fmt.Errorf("role binding name %s must be 63 characters or less", roleBinding.Name)
		}
	}

	return nil
}
