package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/rbac"
)

func TestAdvancedRBACManager(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Setup test client
	testClient := setupTestClient(t)
	if testClient == nil {
		t.Skip("Skipping test - no test client available (placeholder implementation)")
	}

	// Create test namespace
	testNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-rbac-advanced-" + generateTestID(),
		},
	}
	require.NoError(t, testClient.Create(ctx, testNamespace))
	defer func() {
		_ = testClient.Delete(ctx, testNamespace)
	}()

	// Create RBAC manager
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = roostv1alpha1.AddToScheme(scheme)

	rbacManager := rbac.NewManager(testClient, scheme, logger)

	t.Run("PolicyTemplateRegistration", func(t *testing.T) {
		// Test registering a custom policy template
		template := &rbac.PolicyTemplate{
			Name:        "test-template",
			Description: "Test policy template",
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{""},
					Resources: []string{"pods"},
					Verbs:     []string{"get", "list"},
				},
			},
		}

		rbacManager.RegisterPolicyTemplate(template)
		// Template registration is internal, so we test it through usage
	})

	t.Run("AdvancedRBACSetup", func(t *testing.T) {
		// Create comprehensive ManagedRoost with RBAC configuration
		managedRoost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-advanced-rbac",
				Namespace: testNamespace.Name,
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				Chart: roostv1alpha1.ChartSpec{
					Repository: roostv1alpha1.ChartRepositorySpec{
						URL: "https://charts.bitnami.com/bitnami",
					},
					Name:    "nginx",
					Version: "1.0.0",
				},
				Tenancy: &roostv1alpha1.TenancySpec{
					TenantID: "test-enterprise",
					RBAC: &roostv1alpha1.RBACSpec{
						Enabled:  true,
						TenantID: "test-enterprise",

						ServiceAccounts: []roostv1alpha1.ServiceAccountSpec{
							{
								Name: "test-operator",
								Annotations: map[string]string{
									"test.io/purpose": "testing",
								},
								AutomountToken: false,
								Lifecycle: &roostv1alpha1.ServiceAccountLifecycle{
									CleanupPolicy: "delete",
									TokenRotation: &roostv1alpha1.TokenRotationPolicy{
										Enabled:          true,
										RotationInterval: metav1.Duration{Duration: 24 * time.Hour},
										OverlapPeriod:    metav1.Duration{Duration: 2 * time.Hour},
									},
								},
							},
							{
								Name:           "test-viewer",
								AutomountToken: true,
							},
						},

						Roles: []roostv1alpha1.RoleSpec{
							{
								Name:     "test-admin",
								Template: "tenant-admin",
								Type:     "Role",
								Annotations: map[string]string{
									"test.io/security-level": "high",
								},
							},
							{
								Name:     "test-developer",
								Template: "tenant-developer",
								Type:     "Role",
							},
							{
								Name: "test-custom",
								Type: "Role",
								Rules: []rbacv1.PolicyRule{
									{
										APIGroups: []string{""},
										Resources: []string{"configmaps"},
										Verbs:     []string{"get", "list", "watch"},
									},
								},
							},
						},

						RoleBindings: []roostv1alpha1.RoleBindingSpec{
							{
								Name: "test-admin-binding",
								Type: "RoleBinding",
								Subjects: []roostv1alpha1.SubjectSpec{
									{
										Kind: "ServiceAccount",
										Name: "test-operator",
									},
								},
								RoleRef: roostv1alpha1.RoleRefSpec{
									Kind:     "Role",
									Name:     "test-admin",
									APIGroup: "rbac.authorization.k8s.io",
								},
							},
							{
								Name: "test-viewer-binding",
								Type: "RoleBinding",
								Subjects: []roostv1alpha1.SubjectSpec{
									{
										Kind: "ServiceAccount",
										Name: "test-viewer",
									},
								},
								RoleRef: roostv1alpha1.RoleRefSpec{
									Kind:     "Role",
									Name:     "test-developer",
									APIGroup: "rbac.authorization.k8s.io",
								},
							},
						},

						TemplateParameters: map[string]string{
							"Environment": "test",
							"Department":  "engineering",
						},

						Audit: &roostv1alpha1.AuditConfig{
							Enabled: true,
							Level:   "standard",
							Events: []string{
								"rbac_setup",
								"service_account_created",
								"role_created",
								"role_binding_created",
							},
						},

						PolicyValidation: &roostv1alpha1.PolicyValidationConfig{
							Enabled:                  true,
							Mode:                     "warn",
							CheckLeastPrivilege:      true,
							CheckPrivilegeEscalation: true,
						},
					},
				},
			},
		}

		require.NoError(t, testClient.Create(ctx, managedRoost))
		defer func() {
			_ = testClient.Delete(ctx, managedRoost)
		}()

		// Setup RBAC
		err := rbacManager.SetupRBAC(ctx, managedRoost)
		require.NoError(t, err)

		// Verify service accounts were created
		for _, saSpec := range managedRoost.Spec.Tenancy.RBAC.ServiceAccounts {
			var sa corev1.ServiceAccount
			err := testClient.Get(ctx, types.NamespacedName{
				Name:      saSpec.Name,
				Namespace: testNamespace.Name,
			}, &sa)
			assert.NoError(t, err, "Service account %s should exist", saSpec.Name)
			assert.Equal(t, "test-enterprise", sa.Labels["roost-keeper.io/tenant"])
			assert.Equal(t, "rbac", sa.Labels["roost-keeper.io/component"])
		}

		// Verify roles were created
		for _, roleSpec := range managedRoost.Spec.Tenancy.RBAC.Roles {
			var role rbacv1.Role
			err := testClient.Get(ctx, types.NamespacedName{
				Name:      roleSpec.Name,
				Namespace: testNamespace.Name,
			}, &role)
			assert.NoError(t, err, "Role %s should exist", roleSpec.Name)
			assert.Equal(t, "test-enterprise", role.Labels["roost-keeper.io/tenant"])
			assert.Greater(t, len(role.Rules), 0, "Role should have rules")
		}

		// Verify role bindings were created
		for _, bindingSpec := range managedRoost.Spec.Tenancy.RBAC.RoleBindings {
			var binding rbacv1.RoleBinding
			err := testClient.Get(ctx, types.NamespacedName{
				Name:      bindingSpec.Name,
				Namespace: testNamespace.Name,
			}, &binding)
			assert.NoError(t, err, "RoleBinding %s should exist", bindingSpec.Name)
			assert.Equal(t, "test-enterprise", binding.Labels["roost-keeper.io/tenant"])
			assert.Equal(t, len(bindingSpec.Subjects), len(binding.Subjects))
		}
	})

	t.Run("PolicyValidation", func(t *testing.T) {
		// Test policy validation with the validator
		rbacSpec := &roostv1alpha1.RBACSpec{
			TenantID: "test-validation",
			Enabled:  true,
			ServiceAccounts: []roostv1alpha1.ServiceAccountSpec{
				{
					Name:           "valid-sa",
					AutomountToken: false, // Security best practice
				},
				{
					Name:           "invalid-name-too-long-for-kubernetes-service-account-naming",
					AutomountToken: true,
				},
			},
			Roles: []roostv1alpha1.RoleSpec{
				{
					Name: "valid-role",
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{""},
							Resources: []string{"pods"},
							Verbs:     []string{"get", "list"},
						},
					},
				},
				{
					Name: "", // Invalid empty name
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"*"}, // Overly broad
							Resources: []string{"*"}, // Overly broad
							Verbs:     []string{"*"}, // Overly broad
						},
					},
				},
			},
			RoleBindings: []roostv1alpha1.RoleBindingSpec{
				{
					Name: "valid-binding",
					Subjects: []roostv1alpha1.SubjectSpec{
						{
							Kind: "ServiceAccount",
							Name: "valid-sa",
						},
					},
					RoleRef: roostv1alpha1.RoleRefSpec{
						Kind: "Role",
						Name: "valid-role",
					},
				},
				{
					Name:     "invalid-binding",
					Subjects: []roostv1alpha1.SubjectSpec{}, // No subjects
					RoleRef: roostv1alpha1.RoleRefSpec{
						Kind: "Role",
						Name: "valid-role",
					},
				},
			},
		}

		validator := rbac.NewPolicyValidator(logger)
		result := validator.ValidateRBACConfiguration(ctx, rbacSpec)

		assert.False(t, result.Valid, "Validation should fail for invalid configuration")
		assert.Greater(t, result.Stats.ErrorCount, 0, "Should have validation errors")
		assert.Greater(t, len(result.Issues), 0, "Should have validation issues")

		// Check that security scoring works
		assert.GreaterOrEqual(t, result.Stats.SecurityScore, 0)
		assert.LessOrEqual(t, result.Stats.SecurityScore, 100)
	})

	t.Run("IdentityProviderIntegration", func(t *testing.T) {
		// Test placeholder identity provider
		provider := &rbac.PlaceholderProvider{}

		// Test token validation
		identity, err := provider.ValidateToken(ctx, "test-token")
		assert.NoError(t, err)
		assert.NotNil(t, identity)
		assert.Equal(t, "placeholder-user", identity.Username)

		// Test user info retrieval
		userInfo, err := provider.GetUserInfo(ctx, "test-user")
		assert.NoError(t, err)
		assert.NotNil(t, userInfo)
		assert.Equal(t, "test-user", userInfo.Username)

		// Test user groups retrieval
		groups, err := provider.GetUserGroups(ctx, "test-user")
		assert.NoError(t, err)
		assert.Contains(t, groups, "placeholder-group")

		// Test provider type
		assert.NotEmpty(t, provider.GetProviderType())
	})

	t.Run("AuditLogging", func(t *testing.T) {
		// Test audit logger functionality
		auditLogger := rbac.NewAuditLogger(logger)

		// Test various audit events
		managedRoost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-audit",
				Namespace: testNamespace.Name,
			},
		}

		rbacSpec := &roostv1alpha1.RBACSpec{
			TenantID: "test-audit-tenant",
			ServiceAccounts: []roostv1alpha1.ServiceAccountSpec{
				{Name: "test-sa"},
			},
			Roles: []roostv1alpha1.RoleSpec{
				{Name: "test-role"},
			},
			RoleBindings: []roostv1alpha1.RoleBindingSpec{
				{Name: "test-binding"},
			},
		}

		// These should not error (they log internally)
		auditLogger.LogRBACSetup(ctx, managedRoost, rbacSpec)
		auditLogger.LogRBACCleanup(ctx, managedRoost, "test-audit-tenant")
		auditLogger.LogPermissionDenied(ctx, "test-user", "get", "pods")
		auditLogger.LogPermissionGranted(ctx, "test-user", "list", "services")
		auditLogger.LogPolicyValidation(ctx, "test-tenant", "passed", []string{})
		auditLogger.LogIdentityProviderAuth(ctx, "test-provider", "test-user", "success")
		auditLogger.LogServiceAccountCreation(ctx, "test-tenant", "test-sa", testNamespace.Name)
		auditLogger.LogRoleCreation(ctx, "test-tenant", "test-role", testNamespace.Name, 3)
		auditLogger.LogRoleBindingCreation(ctx, "test-tenant", "test-binding", testNamespace.Name, 1)
		auditLogger.LogTokenRotation(ctx, "test-tenant", "test-sa", true)
		auditLogger.LogPolicyTemplateUsage(ctx, "test-tenant", "tenant-admin", "test-role")

		// Test webhook configuration
		auditLogger.SetWebhookURL("https://example.com/webhook")

		// Test audit event retrieval (placeholder)
		events, err := auditLogger.GetAuditEvents(ctx, "test-tenant", time.Now().Add(-time.Hour))
		assert.NoError(t, err)
		assert.NotNil(t, events)
	})

	t.Run("TemplateParameterSubstitution", func(t *testing.T) {
		// This is tested indirectly through the RBAC setup test
		// The template substitution happens internally when roles are created from templates

		// Create a managedroost that uses template parameters
		managedRoost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-template-params",
				Namespace: testNamespace.Name,
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				Chart: roostv1alpha1.ChartSpec{
					Repository: roostv1alpha1.ChartRepositorySpec{
						URL: "https://charts.bitnami.com/bitnami",
					},
					Name:    "nginx",
					Version: "1.0.0",
				},
				Tenancy: &roostv1alpha1.TenancySpec{
					TenantID: "test-template-tenant",
					RBAC: &roostv1alpha1.RBACSpec{
						Enabled:  true,
						TenantID: "test-template-tenant",
						Roles: []roostv1alpha1.RoleSpec{
							{
								Name:     "templated-role",
								Template: "tenant-admin",
								Type:     "Role",
								TemplateParameters: map[string]string{
									"CustomResource": "test-configs",
								},
							},
						},
						TemplateParameters: map[string]string{
							"Environment": "test",
							"Department":  "engineering",
						},
					},
				},
			},
		}

		require.NoError(t, testClient.Create(ctx, managedRoost))
		defer func() {
			_ = testClient.Delete(ctx, managedRoost)
		}()

		// Setup RBAC (this will test template parameter substitution)
		err := rbacManager.SetupRBAC(ctx, managedRoost)
		assert.NoError(t, err)

		// Verify the role was created (indicating template processing worked)
		var role rbacv1.Role
		err = testClient.Get(ctx, types.NamespacedName{
			Name:      "templated-role",
			Namespace: testNamespace.Name,
		}, &role)
		assert.NoError(t, err)
		assert.Greater(t, len(role.Rules), 0)
	})
}

func TestRBACValidatorStandalone(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := rbac.NewPolicyValidator(logger)
	ctx := context.Background()

	t.Run("SecurityBestPractices", func(t *testing.T) {
		rbacSpec := &roostv1alpha1.RBACSpec{
			TenantID: "test-tenant",
			Enabled:  true,
			ServiceAccounts: []roostv1alpha1.ServiceAccountSpec{
				{
					Name:           "secure-sa",
					AutomountToken: false, // Good practice
				},
				{
					Name:           "insecure-sa",
					AutomountToken: true, // Will generate warning
				},
			},
			Audit: &roostv1alpha1.AuditConfig{
				Enabled: true, // Good practice
			},
			PolicyValidation: &roostv1alpha1.PolicyValidationConfig{
				Enabled: true, // Good practice
			},
			IdentityProvider: "corporate-oidc", // Good practice
		}

		result := validator.ValidateRBACConfiguration(ctx, rbacSpec)

		// Should be valid but have some warnings
		assert.True(t, result.Valid)
		assert.GreaterOrEqual(t, result.Stats.WarningCount, 1)   // At least one warning for automount token
		assert.GreaterOrEqual(t, result.Stats.SecurityScore, 80) // Should have good security score
	})

	t.Run("LeastPrivilegeViolations", func(t *testing.T) {
		rbacSpec := &roostv1alpha1.RBACSpec{
			TenantID: "test-tenant",
			Enabled:  true,
			Roles: []roostv1alpha1.RoleSpec{
				{
					Name: "overprivileged-role",
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"*"}, // Wildcard - bad
							Resources: []string{"*"}, // Wildcard - bad
							Verbs:     []string{"*"}, // Wildcard - bad
						},
					},
				},
			},
		}

		result := validator.ValidateRBACConfiguration(ctx, rbacSpec)

		// Should have warnings about wildcards
		assert.Greater(t, result.Stats.WarningCount, 0)

		// Check for specific wildcard warnings
		hasWildcardWarning := false
		for _, issue := range result.Issues {
			if issue.Rule == "wildcard_verbs" || issue.Rule == "wildcard_resources" {
				hasWildcardWarning = true
				break
			}
		}
		assert.True(t, hasWildcardWarning, "Should have wildcard warnings")
	})

	t.Run("PrivilegeEscalationRisks", func(t *testing.T) {
		rbacSpec := &roostv1alpha1.RBACSpec{
			TenantID: "test-tenant",
			Enabled:  true,
			Roles: []roostv1alpha1.RoleSpec{
				{
					Name: "dangerous-role",
					Rules: []rbacv1.PolicyRule{
						{
							APIGroups: []string{"rbac.authorization.k8s.io"},
							Resources: []string{"roles", "rolebindings"},
							Verbs:     []string{"*"},
						},
						{
							APIGroups: []string{""},
							Resources: []string{"nodes"},
							Verbs:     []string{"get", "list"},
						},
					},
				},
			},
		}

		result := validator.ValidateRBACConfiguration(ctx, rbacSpec)

		// Should have high severity warnings
		hasHighSeverityIssue := false
		for _, issue := range result.Issues {
			if issue.Severity == "high" || issue.Severity == "critical" {
				hasHighSeverityIssue = true
				break
			}
		}
		assert.True(t, hasHighSeverityIssue, "Should have high severity security issues")
	})
}
