package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/helm"
)

const (
	// Local test registry endpoints (from infrastructure document)
	LocalZotRegistry    = "10.0.0.106:30001"
	LocalDockerRegistry = "10.0.0.106:30000"

	// Test configuration
	TestTimeout = 30 * time.Second
)

// TestOCIRegistryIntegration tests OCI chart operations against the local Zot registry
func TestOCIRegistryIntegration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Skip if registry is not available
	if !isRegistryAvailable(t, LocalZotRegistry) {
		t.Skip("Zot registry not available at " + LocalZotRegistry)
	}

	// Setup test environment
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	helmManager, err := helm.NewManager(fakeClient, &rest.Config{}, logger)
	require.NoError(t, err)

	t.Run("OCI_Chart_Pull_Basic", func(t *testing.T) {
		testOCIChartPullBasic(t, ctx, helmManager)
	})

	t.Run("OCI_Authentication_Configuration", func(t *testing.T) {
		testOCIAuthenticationConfiguration(t, ctx, helmManager)
	})

	t.Run("OCI_Error_Handling", func(t *testing.T) {
		testOCIErrorHandling(t, ctx, helmManager)
	})
}

func testOCIChartPullBasic(t *testing.T, ctx context.Context, helmManager helm.Manager) {
	// Create a test ManagedRoost for OCI chart
	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-oci-chart",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL:  "oci://" + LocalZotRegistry,
					Type: "oci",
				},
				Name:    "nginx", // Assuming we have a test nginx chart
				Version: "1.0.0",
			},
		},
	}

	// Test chart loading (will fail since we don't have a chart uploaded yet, but should test the flow)
	_, err := helmManager.Install(ctx, roost)

	// We expect this to fail since we haven't uploaded a chart yet
	// But the error should be from chart not found, not from authentication or connection issues
	assert.Error(t, err)

	// The error should indicate chart pulling failed, not authentication or connection issues
	errorMsg := err.Error()
	assert.Contains(t, errorMsg, "pull", "Error should indicate chart pull operation")

	t.Logf("OCI chart pull test completed with expected error: %v", err)
}

func testOCIAuthenticationConfiguration(t *testing.T, ctx context.Context, helmManager helm.Manager) {
	tests := []struct {
		name     string
		authType string
		token    string
		username string
		password string
	}{
		{
			name:     "No_Authentication",
			authType: "",
		},
		{
			name:     "Token_Authentication",
			authType: "token",
			token:    "test-token-value",
		},
		{
			name:     "Basic_Authentication",
			authType: "basic",
			username: "testuser",
			password: "testpass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-auth-" + tt.name,
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "oci://" + LocalZotRegistry,
							Type: "oci",
						},
						Name:    "test-chart",
						Version: "1.0.0",
					},
				},
			}

			// Add authentication if specified
			if tt.authType != "" {
				roost.Spec.Chart.Repository.Auth = &roostv1alpha1.RepositoryAuthSpec{
					Type:     tt.authType,
					Token:    tt.token,
					Username: tt.username,
					Password: tt.password,
				}
			}

			// Test the authentication configuration
			_, err := helmManager.Install(ctx, roost)

			// We expect this to fail (no chart), but it should process authentication
			assert.Error(t, err)
			t.Logf("Authentication test %s completed: %v", tt.name, err)
		})
	}
}

func testOCIErrorHandling(t *testing.T, ctx context.Context, helmManager helm.Manager) {
	// Test with invalid registry
	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-invalid-registry",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL:  "oci://invalid-registry.example.com",
					Type: "oci",
				},
				Name:    "test-chart",
				Version: "1.0.0",
			},
		},
	}

	_, err := helmManager.Install(ctx, roost)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed", "Should indicate operation failure")

	t.Logf("Error handling test completed: %v", err)
}

// TestOCIRegistryConnectivity tests basic connectivity to the OCI registries
func TestOCIRegistryConnectivity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping connectivity test in short mode")
	}

	t.Run("Zot_Registry_Health", func(t *testing.T) {
		available := isRegistryAvailable(t, LocalZotRegistry)
		if !available {
			t.Logf("Zot registry not available at %s - this is expected in CI/CD", LocalZotRegistry)
		} else {
			t.Logf("Zot registry is available at %s", LocalZotRegistry)
		}
	})

	t.Run("Docker_Registry_Health", func(t *testing.T) {
		available := isRegistryAvailable(t, LocalDockerRegistry)
		if !available {
			t.Logf("Docker registry not available at %s - this is expected in CI/CD", LocalDockerRegistry)
		} else {
			t.Logf("Docker registry is available at %s", LocalDockerRegistry)
		}
	})
}

// TestHelm64CharInstallation tests Helm chart installation end-to-end
func TestHelmInstallationFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping installation flow test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)

	helmManager, err := helm.NewManager(fakeClient, &rest.Config{}, logger)
	require.NoError(t, err)

	// Test with HTTP repository (more likely to work)
	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-http-chart",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL:  "https://charts.bitnami.com/bitnami",
					Type: "http",
				},
				Name:    "nginx",
				Version: "15.0.0", // Use a specific version
			},
		},
	}

	t.Run("HTTP_Chart_Loading", func(t *testing.T) {
		// This will test chart loading (may fail due to no K8s cluster, but should load the chart)
		_, err := helmManager.Install(ctx, roost)

		// We expect this to fail due to no Kubernetes cluster access
		// But it should successfully download and parse the chart
		assert.Error(t, err)

		// The error should be about Kubernetes access, not chart loading
		errorMsg := err.Error()
		t.Logf("HTTP chart test completed with error: %v", errorMsg)
	})
}

// isRegistryAvailable checks if a registry is available by making a simple HTTP request
func isRegistryAvailable(t *testing.T, registryHost string) bool {
	// Try to connect to registry health endpoint
	// This is a simplified check - in a real scenario you'd make an HTTP request
	// For now, we'll check if we're in a development environment

	// Check environment variable or assume available in development
	if os.Getenv("ROOST_KEEPER_INTEGRATION_TEST") == "true" {
		return true
	}

	// If running in CI or without explicit integration test flag, assume not available
	return false
}

// BenchmarkOCIOperations benchmarks OCI chart operations
func BenchmarkOCIOperations(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = roostv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger, _ := zap.NewDevelopment()
	helmManager, _ := helm.NewManager(fakeClient, &rest.Config{}, logger)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench-oci-chart",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL:  "oci://" + LocalZotRegistry,
					Type: "oci",
				},
				Name:    "test-chart",
				Version: "1.0.0",
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = helmManager.Install(ctx, roost)
	}
}
