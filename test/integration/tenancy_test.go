package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/tenancy"
)

func TestRBACManager(t *testing.T) {
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
			Name: "test-rbac-" + generateTestID(),
		},
	}
	require.NoError(t, testClient.Create(ctx, testNamespace))
	defer func() {
		_ = testClient.Delete(ctx, testNamespace)
	}()

	// Create test ManagedRoost
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
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
				TenantID: "test-tenant",
				RBAC: &roostv1alpha1.RBACSpec{
					Enabled:  true,
					TenantID: "test-tenant",
					ServiceAccounts: []roostv1alpha1.ServiceAccountSpec{
						{
							Name: "test-service-account",
						},
					},
					Roles: []roostv1alpha1.RoleSpec{
						{
							Name: "test-role",
						},
					},
				},
			},
		},
	}
	require.NoError(t, testClient.Create(ctx, managedRoost))
	defer func() {
		_ = testClient.Delete(ctx, managedRoost)
	}()

	// Create RBAC manager
	rbacManager := tenancy.NewRBACManager(testClient, logger)

	t.Run("SetupTenantRBAC", func(t *testing.T) {
		tenantID := "test-tenant-001"

		err := rbacManager.SetupTenantRBAC(ctx, managedRoost, tenantID)
		require.NoError(t, err)

		// Verify Service Account is created
		var serviceAccount corev1.ServiceAccount
		err = testClient.Get(ctx, types.NamespacedName{
			Name:      "roost-keeper-tenant-" + tenantID,
			Namespace: testNamespace.Name,
		}, &serviceAccount)
		assert.NoError(t, err)
		assert.Equal(t, tenantID, serviceAccount.Labels[tenancy.TenantIDLabel])

		// Verify Role is created
		var role rbacv1.Role
		err = testClient.Get(ctx, types.NamespacedName{
			Name:      "roost-keeper-tenant-" + tenantID,
			Namespace: testNamespace.Name,
		}, &role)
		assert.NoError(t, err)
		assert.Equal(t, tenantID, role.Labels[tenancy.TenantIDLabel])

		// Verify RoleBinding is created
		var roleBinding rbacv1.RoleBinding
		err = testClient.Get(ctx, types.NamespacedName{
			Name:      "roost-keeper-tenant-" + tenantID,
			Namespace: testNamespace.Name,
		}, &roleBinding)
		assert.NoError(t, err)
		assert.Equal(t, tenantID, roleBinding.Labels[tenancy.TenantIDLabel])
	})

	t.Run("CleanupTenantRBAC", func(t *testing.T) {
		tenantID := "test-tenant-002"

		// Setup RBAC first
		err := rbacManager.SetupTenantRBAC(ctx, managedRoost, tenantID)
		require.NoError(t, err)

		// Verify resources exist
		var serviceAccount corev1.ServiceAccount
		err = testClient.Get(ctx, types.NamespacedName{
			Name:      "roost-keeper-tenant-" + tenantID,
			Namespace: testNamespace.Name,
		}, &serviceAccount)
		require.NoError(t, err)

		// Cleanup RBAC
		err = rbacManager.CleanupTenantRBAC(ctx, managedRoost, tenantID)
		assert.NoError(t, err)

		// Verify resources are deleted
		err = testClient.Get(ctx, types.NamespacedName{
			Name:      "roost-keeper-tenant-" + tenantID,
			Namespace: testNamespace.Name,
		}, &serviceAccount)
		assert.Error(t, err) // Should not find the resource
	})
}

func TestQuotaManager(t *testing.T) {
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
			Name: "test-quota-" + generateTestID(),
		},
	}
	require.NoError(t, testClient.Create(ctx, testNamespace))
	defer func() {
		_ = testClient.Delete(ctx, testNamespace)
	}()

	// Create test ManagedRoost
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
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
				TenantID: "test-tenant",
				ResourceQuota: &roostv1alpha1.ResourceQuotaSpec{
					Enabled: true,
					Hard: map[string]intstr.IntOrString{
						"requests.cpu":    intstr.FromString("2"),
						"requests.memory": intstr.FromString("4Gi"),
						"pods":            intstr.FromInt(10),
					},
				},
			},
		},
	}
	require.NoError(t, testClient.Create(ctx, managedRoost))
	defer func() {
		_ = testClient.Delete(ctx, managedRoost)
	}()

	// Create quota manager
	quotaManager := tenancy.NewQuotaManager(testClient, logger)

	t.Run("SetupResourceQuota", func(t *testing.T) {
		tenantID := "test-tenant-001"

		err := quotaManager.SetupResourceQuota(ctx, managedRoost, tenantID)
		require.NoError(t, err)

		// Verify Resource Quota is created
		var resourceQuota corev1.ResourceQuota
		err = testClient.Get(ctx, types.NamespacedName{
			Name:      "roost-keeper-tenant-" + tenantID,
			Namespace: testNamespace.Name,
		}, &resourceQuota)
		assert.NoError(t, err)
		assert.Equal(t, tenantID, resourceQuota.Labels[tenancy.TenantIDLabel])
	})

	t.Run("GetQuotaStatus", func(t *testing.T) {
		tenantID := "test-tenant-002"

		// Setup quota first
		err := quotaManager.SetupResourceQuota(ctx, managedRoost, tenantID)
		require.NoError(t, err)

		// Get quota status
		status, err := quotaManager.GetQuotaStatus(ctx, testNamespace.Name, tenantID)
		assert.NoError(t, err)
		assert.NotNil(t, status)
		assert.Equal(t, tenantID, status.TenantID)
		assert.Equal(t, testNamespace.Name, status.Namespace)
	})
}

func TestNetworkManager(t *testing.T) {
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
			Name: "test-network-" + generateTestID(),
		},
	}
	require.NoError(t, testClient.Create(ctx, testNamespace))
	defer func() {
		_ = testClient.Delete(ctx, testNamespace)
	}()

	// Create test ManagedRoost
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
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
				TenantID: "test-tenant",
				NetworkPolicy: &roostv1alpha1.NetworkPolicySpec{
					Enabled: true,
					Ingress: []roostv1alpha1.NetworkPolicyRule{
						{
							From: []roostv1alpha1.NetworkPolicyPeer{
								{
									PodSelector: map[string]string{
										"app": "allowed",
									},
								},
							},
							Ports: []roostv1alpha1.NetworkPolicyPort{
								{
									Port: &intstr.IntOrString{
										Type:   intstr.Int,
										IntVal: 8080,
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(t, testClient.Create(ctx, managedRoost))
	defer func() {
		_ = testClient.Delete(ctx, managedRoost)
	}()

	// Create network manager
	networkManager := tenancy.NewNetworkManager(testClient, logger)

	t.Run("ApplyNetworkPolicies", func(t *testing.T) {
		tenantID := "test-tenant-001"

		err := networkManager.ApplyNetworkPolicies(ctx, managedRoost, tenantID)
		require.NoError(t, err)

		// Verify Network Policies are created
		var networkPolicies networkingv1.NetworkPolicyList
		err = testClient.List(ctx, &networkPolicies,
			client.InNamespace(testNamespace.Name),
			client.MatchingLabels{tenancy.TenantIDLabel: tenantID},
		)
		assert.NoError(t, err)
		assert.Greater(t, len(networkPolicies.Items), 0)

		// Should have at least deny-all, isolation, dns, and custom policies
		policyTypes := make(map[string]bool)
		for _, policy := range networkPolicies.Items {
			policyType := policy.Labels["policy-type"]
			policyTypes[policyType] = true
		}

		assert.True(t, policyTypes["default-deny"])
		assert.True(t, policyTypes["tenant-isolation"])
		assert.True(t, policyTypes["dns-access"])
		assert.True(t, policyTypes["custom-ingress"])
	})

	t.Run("ValidateNetworkPolicyConfiguration", func(t *testing.T) {
		// Valid network policy spec
		validSpec := &roostv1alpha1.NetworkPolicySpec{
			Ingress: []roostv1alpha1.NetworkPolicyRule{
				{
					From: []roostv1alpha1.NetworkPolicyPeer{
						{
							PodSelector: map[string]string{"app": "test"},
						},
					},
					Ports: []roostv1alpha1.NetworkPolicyPort{
						{
							Protocol: "TCP",
							Port: &intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: 8080,
							},
						},
					},
				},
			},
		}

		err := networkManager.ValidateNetworkPolicyConfiguration(validSpec)
		assert.NoError(t, err)

		// Invalid network policy spec (empty peer)
		invalidSpec := &roostv1alpha1.NetworkPolicySpec{
			Ingress: []roostv1alpha1.NetworkPolicyRule{
				{
					From: []roostv1alpha1.NetworkPolicyPeer{
						{}, // Empty peer - should be invalid
					},
				},
			},
		}

		err = networkManager.ValidateNetworkPolicyConfiguration(invalidSpec)
		assert.Error(t, err)
	})

	t.Run("CleanupNetworkPolicies", func(t *testing.T) {
		tenantID := "test-tenant-002"

		// Setup network policies first
		err := networkManager.ApplyNetworkPolicies(ctx, managedRoost, tenantID)
		require.NoError(t, err)

		// Verify policies exist
		var networkPolicies networkingv1.NetworkPolicyList
		err = testClient.List(ctx, &networkPolicies,
			client.InNamespace(testNamespace.Name),
			client.MatchingLabels{tenancy.TenantIDLabel: tenantID},
		)
		require.NoError(t, err)
		require.Greater(t, len(networkPolicies.Items), 0)

		// Cleanup network policies
		err = networkManager.CleanupNetworkPolicies(ctx, managedRoost, tenantID)
		assert.NoError(t, err)

		// Verify policies are deleted
		err = testClient.List(ctx, &networkPolicies,
			client.InNamespace(testNamespace.Name),
			client.MatchingLabels{tenancy.TenantIDLabel: tenantID},
		)
		assert.NoError(t, err)
		assert.Equal(t, 0, len(networkPolicies.Items))
	})
}

func TestTenancyValidator(t *testing.T) {
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

	validator := tenancy.NewValidator(testClient, logger)

	t.Run("ValidateTenantAccess", func(t *testing.T) {
		// Create test namespace
		testNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-validator-" + generateTestID(),
				Labels: map[string]string{
					tenancy.TenantIDLabel: "test-tenant",
				},
			},
		}
		require.NoError(t, testClient.Create(ctx, testNamespace))
		defer func() {
			_ = testClient.Delete(ctx, testNamespace)
		}()

		// Create test ManagedRoost
		managedRoost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
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
					TenantID: "test-tenant",
				},
			},
		}

		// Test valid tenant access
		err := validator.ValidateTenantAccess(ctx, managedRoost, "test-tenant")
		assert.NoError(t, err)

		// Test invalid tenant access
		err = validator.ValidateTenantAccess(ctx, managedRoost, "wrong-tenant")
		assert.Error(t, err)
	})

	t.Run("ValidateCrossNamespaceAccess", func(t *testing.T) {
		// Create test namespace for tenant A
		testNamespaceA := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-tenant-a-" + generateTestID(),
				Labels: map[string]string{
					tenancy.TenantIDLabel: "tenant-a",
				},
			},
		}
		require.NoError(t, testClient.Create(ctx, testNamespaceA))
		defer func() {
			_ = testClient.Delete(ctx, testNamespaceA)
		}()

		// Test same tenant access (should succeed)
		err := validator.ValidateCrossNamespaceAccess(ctx, "tenant-a", testNamespaceA.Name)
		assert.NoError(t, err)

		// Test different tenant access (should fail)
		err = validator.ValidateCrossNamespaceAccess(ctx, "tenant-b", testNamespaceA.Name)
		assert.Error(t, err)
	})

	t.Run("GetNamespaceTenant", func(t *testing.T) {
		// Create test namespace
		testNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-get-tenant-" + generateTestID(),
				Labels: map[string]string{
					tenancy.TenantIDLabel: "test-tenant-id",
				},
			},
		}
		require.NoError(t, testClient.Create(ctx, testNamespace))
		defer func() {
			_ = testClient.Delete(ctx, testNamespace)
		}()

		// Test getting tenant ID
		tenantID, err := validator.GetNamespaceTenant(ctx, testNamespace.Name)
		assert.NoError(t, err)
		assert.Equal(t, "test-tenant-id", tenantID)
	})
}

func generateTestID() string {
	return time.Now().Format("20060102-150405")
}
