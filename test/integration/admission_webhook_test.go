package integration

import (
	"context"
	"crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/webhook"
)

// generateRandomString generates a random string of specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[num.Int64()]
	}
	return string(b)
}

func TestSimplePolicyEngine_ValidateSimple(t *testing.T) {
	logger := zap.NewNop()

	// Pass nil for metrics in tests
	engine := webhook.NewSimplePolicyEngine(logger, nil)

	tests := []struct {
		name          string
		managedRoost  *roostkeeper.ManagedRoost
		expectValid   bool
		errorContains string
	}{
		{
			name: "valid managedroost",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "missing chart name",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "", // Missing
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
				},
			},
			expectValid:   false,
			errorContains: "Chart name is required",
		},
		{
			name: "missing chart version",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "", // Missing
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
				},
			},
			expectValid:   false,
			errorContains: "Chart version is required",
		},
		{
			name: "missing repository URL",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "", // Missing
						},
					},
				},
			},
			expectValid:   false,
			errorContains: "Chart repository URL is required",
		},
		{
			name: "namespace too long",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
					Namespace: generateRandomString(100), // Too long
				},
			},
			expectValid:   false,
			errorContains: "Namespace name must be 63 characters or less",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := engine.ValidateSimple(ctx, tt.managedRoost)

			require.NoError(t, err, "ValidateSimple should not return error")
			assert.Equal(t, tt.expectValid, result.Allowed, "Validation result should match expected")

			if !tt.expectValid && tt.errorContains != "" {
				assert.Contains(t, result.Message, tt.errorContains, "Error message should contain expected text")
			}
		})
	}
}

func TestSimplePolicyEngine_ValidateTenancy(t *testing.T) {
	logger := zap.NewNop()

	engine := webhook.NewSimplePolicyEngine(logger, nil)

	tests := []struct {
		name          string
		managedRoost  *roostkeeper.ManagedRoost
		expectValid   bool
		errorContains string
	}{
		{
			name: "no tenancy configuration",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid tenancy configuration",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
					Tenancy: &roostkeeper.TenancySpec{
						TenantID: "valid-tenant",
						RBAC: &roostkeeper.RBACSpec{
							Enabled:  true,
							TenantID: "valid-tenant",
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "tenant ID too long",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
					Tenancy: &roostkeeper.TenancySpec{
						TenantID: generateRandomString(300), // Too long
					},
				},
			},
			expectValid:   false,
			errorContains: "Tenant ID must be 253 characters or less",
		},
		{
			name: "RBAC without tenant ID",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
					Tenancy: &roostkeeper.TenancySpec{
						RBAC: &roostkeeper.RBACSpec{
							Enabled: true,
							// TenantID is missing
						},
					},
				},
			},
			expectValid:   false,
			errorContains: "RBAC tenant ID is required when RBAC is configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := engine.ValidateTenancy(ctx, tt.managedRoost)

			require.NoError(t, err, "ValidateTenancy should not return error")
			assert.Equal(t, tt.expectValid, result.Allowed, "Validation result should match expected")

			if !tt.expectValid && tt.errorContains != "" {
				assert.Contains(t, result.Message, tt.errorContains, "Error message should contain expected text")
			}
		})
	}
}

func TestSimplePolicyEngine_ValidateObservability(t *testing.T) {
	logger := zap.NewNop()

	engine := webhook.NewSimplePolicyEngine(logger, nil)

	tests := []struct {
		name          string
		managedRoost  *roostkeeper.ManagedRoost
		expectValid   bool
		errorContains string
	}{
		{
			name: "no observability configuration",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "valid observability configuration",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
					Observability: &roostkeeper.ObservabilitySpec{
						Metrics: &roostkeeper.MetricsSpec{
							Enabled: true,
							Interval: metav1.Duration{
								Duration: 30 * time.Second,
							},
						},
					},
				},
			},
			expectValid: true,
		},
		{
			name: "metrics interval too short",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
					Observability: &roostkeeper.ObservabilitySpec{
						Metrics: &roostkeeper.MetricsSpec{
							Enabled: true,
							Interval: metav1.Duration{
								Duration: 500 * time.Millisecond, // Too short
							},
						},
					},
				},
			},
			expectValid:   false,
			errorContains: "Metrics interval must be at least 1 second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := engine.ValidateObservability(ctx, tt.managedRoost)

			require.NoError(t, err, "ValidateObservability should not return error")
			assert.Equal(t, tt.expectValid, result.Allowed, "Validation result should match expected")

			if !tt.expectValid && tt.errorContains != "" {
				assert.Contains(t, result.Message, tt.errorContains, "Error message should contain expected text")
			}
		})
	}
}

func TestSimplePolicyEngine_MutateSimple(t *testing.T) {
	logger := zap.NewNop()

	engine := webhook.NewSimplePolicyEngine(logger, nil)

	tests := []struct {
		name           string
		managedRoost   *roostkeeper.ManagedRoost
		expectedTenant string
		expectPatches  bool
	}{
		{
			name: "inject default tenant labels",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
				},
			},
			expectedTenant: "default",
			expectPatches:  true,
		},
		{
			name: "inject custom tenant labels",
			managedRoost: &roostkeeper.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostkeeper.ManagedRoostSpec{
					Chart: roostkeeper.ChartSpec{
						Name:    "nginx",
						Version: "1.0.0",
						Repository: roostkeeper.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
					},
					Tenancy: &roostkeeper.TenancySpec{
						TenantID: "custom-tenant",
					},
				},
			},
			expectedTenant: "custom-tenant",
			expectPatches:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			patches, err := engine.MutateSimple(ctx, tt.managedRoost)

			require.NoError(t, err, "MutateSimple should not return error")

			if tt.expectPatches {
				assert.Greater(t, len(patches), 0, "Should generate patches")

				// Verify expected patches are present
				foundTenantPatch := false
				foundManagedByPatch := false
				foundTimestampPatch := false

				for _, patch := range patches {
					if patch.Path == "/metadata/labels/roost-keeper.io~1tenant" {
						foundTenantPatch = true
						assert.Equal(t, tt.expectedTenant, patch.Value, "Tenant label should match expected")
					}
					if patch.Path == "/metadata/labels/roost-keeper.io~1managed-by" {
						foundManagedByPatch = true
						assert.Equal(t, "roost-keeper", patch.Value, "Managed-by label should be correct")
					}
					if patch.Path == "/metadata/annotations/roost-keeper.io~1created-by-webhook" {
						foundTimestampPatch = true
						// Verify it's a valid timestamp
						_, err := time.Parse(time.RFC3339, patch.Value.(string))
						assert.NoError(t, err, "Timestamp should be valid RFC3339 format")
					}
				}

				assert.True(t, foundTenantPatch, "Should find tenant label patch")
				assert.True(t, foundManagedByPatch, "Should find managed-by label patch")
				assert.True(t, foundTimestampPatch, "Should find timestamp annotation patch")
			} else {
				assert.Equal(t, 0, len(patches), "Should not generate patches")
			}
		})
	}
}

func TestAdmissionController_Basic(t *testing.T) {
	logger := zap.NewNop()
	scheme := runtime.NewScheme()

	// Add required schemes
	err := roostkeeper.AddToScheme(scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create admission controller
	controller := webhook.NewAdmissionController(fakeClient, logger)

	// Verify controller was created successfully
	assert.NotNil(t, controller, "Admission controller should be created")
	assert.NotNil(t, controller.GetCertificateManager(), "Certificate manager should be available")
}

func TestCertificateManager_Basic(t *testing.T) {
	logger := zap.NewNop()
	scheme := runtime.NewScheme()

	// Add required schemes
	err := roostkeeper.AddToScheme(scheme)
	require.NoError(t, err)

	// Create fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create certificate manager
	certManager := webhook.NewCertificateManager(fakeClient, logger)

	// Verify certificate manager was created successfully
	assert.NotNil(t, certManager, "Certificate manager should be created")

	// Test configuration methods
	certManager.SetCertificateSecret("test-secret")
	certManager.SetServiceName("test-service")
	certManager.SetNamespace("test-namespace")

	// These should not panic or error
	assert.NotPanics(t, func() {
		certManager.SetCertificateSecret("another-secret")
		certManager.SetServiceName("another-service")
		certManager.SetNamespace("another-namespace")
	}, "Configuration methods should not panic")
}
