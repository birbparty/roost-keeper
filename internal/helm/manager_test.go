package helm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

func TestHelmManager_Interface(t *testing.T) {
	// Test that HelmManager implements the Manager interface
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger, _ := zap.NewDevelopment()

	// This should compile without errors, proving our interface is correct
	var manager Manager
	helmManager, err := NewManager(fakeClient, &rest.Config{}, logger)
	assert.NoError(t, err)

	manager = helmManager
	assert.NotNil(t, manager)
}

func TestHelmManager_OCIAuthentication(t *testing.T) {
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger, _ := zap.NewDevelopment()

	helmManager, err := NewManager(fakeClient, &rest.Config{}, logger)
	assert.NoError(t, err)

	// Test OCI authentication configuration
	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL:  "oci://registry.digitalocean.com/test",
					Type: "oci",
					Auth: &roostv1alpha1.RepositoryAuthSpec{
						Type:     "token",
						Token:    "test-token",
						Username: "test-user",
						Password: "test-pass",
					},
				},
				Name:    "test-chart",
				Version: "1.0.0",
			},
		},
	}

	ctx := context.Background()

	// This should not panic and should handle the auth configuration by testing OCI chart loading
	_, err = helmManager.Install(ctx, roost)
	assert.Error(t, err) // Expected to fail since OCI registry is not accessible
	assert.Contains(t, err.Error(), "failed to connect to OCI registry")
}

func TestHelmManager_OCIChartLoading(t *testing.T) {
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger, _ := zap.NewDevelopment()

	helmManager, err := NewManager(fakeClient, &rest.Config{}, logger)
	assert.NoError(t, err)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL:  "oci://registry.digitalocean.com/test",
					Type: "oci",
				},
				Name:    "test-chart",
				Version: "1.0.0",
			},
		},
	}

	ctx := context.Background()

	// This should return the expected error since we haven't set up the test registry yet
	_, err = helmManager.Install(ctx, roost)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to OCI registry")
}

func TestHelmManager_RollbackSupport(t *testing.T) {
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger, _ := zap.NewDevelopment()

	helmManager, err := NewManager(fakeClient, &rest.Config{}, logger)
	assert.NoError(t, err)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.example.com",
				},
				Name:    "test-chart",
				Version: "1.0.0",
			},
		},
	}

	ctx := context.Background()

	// This demonstrates rollback functionality exists (will fail due to no real Helm setup)
	err = helmManager.Rollback(ctx, roost, 1)
	assert.Error(t, err) // Expected to fail without real Kubernetes setup
	assert.Contains(t, err.Error(), "Kubernetes cluster unreachable")
}

func TestHelmManager_AuthenticationTypes(t *testing.T) {
	tests := []struct {
		name     string
		authType string
		token    string
		username string
		password string
	}{
		{
			name:     "Token authentication",
			authType: "token",
			token:    "dop_v1_test_token",
		},
		{
			name:     "Basic authentication",
			authType: "basic",
			username: "testuser",
			password: "testpass",
		},
		{
			name:     "Default type with token",
			authType: "",
			token:    "default_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			err := roostv1alpha1.AddToScheme(scheme)
			assert.NoError(t, err)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			logger, _ := zap.NewDevelopment()

			helmManager, err := NewManager(fakeClient, &rest.Config{}, logger)
			assert.NoError(t, err)

			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "oci://registry.digitalocean.com/test",
							Type: "oci",
							Auth: &roostv1alpha1.RepositoryAuthSpec{
								Type:     tt.authType,
								Token:    tt.token,
								Username: tt.username,
								Password: tt.password,
							},
						},
						Name:    "test-chart",
						Version: "1.0.0",
					},
				},
			}

			ctx := context.Background()

			// Should handle all authentication types without error by testing Install
			_, err = helmManager.Install(ctx, roost)
			assert.Error(t, err) // Expected to fail since OCI registry is not accessible
			assert.Contains(t, err.Error(), "failed to connect to OCI registry")
		})
	}
}
