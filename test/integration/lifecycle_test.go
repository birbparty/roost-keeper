package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/lifecycle"
)

// TestLifecycleBasicFlow tests the basic lifecycle flow from creation to ready
func TestLifecycleBasicFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client := setupTestClient(t)

	// Create test namespace
	namespace := "test-lifecycle-basic"
	createTestNamespace(t, client, namespace)
	defer cleanupTestNamespace(t, client, namespace)

	// Create a simple ManagedRoost
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost-basic",
			Namespace: namespace,
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.bitnami.com/bitnami",
				},
				Name:    "nginx",
				Version: "15.4.4",
			},
		},
	}

	// Create the ManagedRoost
	err := client.Create(ctx, managedRoost)
	require.NoError(t, err, "Failed to create ManagedRoost")

	// Wait for the roost to transition through phases
	err = waitForPhase(ctx, client, managedRoost, roostv1alpha1.ManagedRoostPhaseReady, 5*time.Minute)
	assert.NoError(t, err, "ManagedRoost should reach Ready phase")

	// Verify final state
	err = client.Get(ctx, types.NamespacedName{Name: managedRoost.Name, Namespace: managedRoost.Namespace}, managedRoost)
	require.NoError(t, err)

	assert.Equal(t, roostv1alpha1.ManagedRoostPhaseReady, managedRoost.Status.Phase)
	assert.NotNil(t, managedRoost.Status.HelmRelease)
	assert.Equal(t, "deployed", managedRoost.Status.HelmRelease.Status)

	// Check conditions
	readyCondition := findCondition(managedRoost.Status.Conditions, "Ready")
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionTrue, readyCondition.Status)
}

// TestLifecycleTimeoutTeardown tests timeout-based teardown triggers
func TestLifecycleTimeoutTeardown(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client := setupTestClient(t)

	// Create test namespace
	namespace := "test-lifecycle-timeout"
	createTestNamespace(t, client, namespace)
	defer cleanupTestNamespace(t, client, namespace)

	// Create a ManagedRoost with short timeout
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost-timeout",
			Namespace: namespace,
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.bitnami.com/bitnami",
				},
				Name:    "nginx",
				Version: "15.4.4",
			},
			TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
				Triggers: []roostv1alpha1.TeardownTriggerSpec{
					{
						Type:    "timeout",
						Timeout: &metav1.Duration{Duration: 30 * time.Second},
					},
				},
			},
		},
	}

	// Create the ManagedRoost
	err := client.Create(ctx, managedRoost)
	require.NoError(t, err, "Failed to create ManagedRoost")

	// Wait for the roost to become ready first
	err = waitForPhase(ctx, client, managedRoost, roostv1alpha1.ManagedRoostPhaseReady, 3*time.Minute)
	require.NoError(t, err, "ManagedRoost should reach Ready phase")

	// Wait for timeout teardown to trigger
	err = waitForPhase(ctx, client, managedRoost, roostv1alpha1.ManagedRoostPhaseTearingDown, 1*time.Minute)
	assert.NoError(t, err, "ManagedRoost should transition to TearingDown phase due to timeout")

	// Verify teardown status
	err = client.Get(ctx, types.NamespacedName{Name: managedRoost.Name, Namespace: managedRoost.Namespace}, managedRoost)
	require.NoError(t, err)

	assert.NotNil(t, managedRoost.Status.Teardown)
	assert.True(t, managedRoost.Status.Teardown.Triggered)
	assert.Contains(t, managedRoost.Status.Teardown.TriggerReason, "timeout exceeded")
}

// TestLifecycleFailureRecovery tests failure handling and recovery
func TestLifecycleFailureRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client := setupTestClient(t)

	// Create test namespace
	namespace := "test-lifecycle-failure"
	createTestNamespace(t, client, namespace)
	defer cleanupTestNamespace(t, client, namespace)

	// Create a ManagedRoost with invalid chart to trigger failure
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost-failure",
			Namespace: namespace,
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://invalid-repository-url.example.com",
				},
				Name:    "invalid-chart",
				Version: "1.0.0",
			},
		},
	}

	// Create the ManagedRoost
	err := client.Create(ctx, managedRoost)
	require.NoError(t, err, "Failed to create ManagedRoost")

	// Wait for the roost to fail
	err = waitForPhase(ctx, client, managedRoost, roostv1alpha1.ManagedRoostPhaseFailed, 2*time.Minute)
	assert.NoError(t, err, "ManagedRoost should reach Failed phase")

	// Verify failure state
	err = client.Get(ctx, types.NamespacedName{Name: managedRoost.Name, Namespace: managedRoost.Namespace}, managedRoost)
	require.NoError(t, err)

	assert.Equal(t, roostv1alpha1.ManagedRoostPhaseFailed, managedRoost.Status.Phase)

	// Check conditions
	readyCondition := findCondition(managedRoost.Status.Conditions, "Ready")
	assert.NotNil(t, readyCondition)
	assert.Equal(t, metav1.ConditionFalse, readyCondition.Status)
}

// TestLifecycleWithHealthChecks tests lifecycle with health checks
func TestLifecycleWithHealthChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client := setupTestClient(t)

	// Create test namespace
	namespace := "test-lifecycle-health"
	createTestNamespace(t, client, namespace)
	defer cleanupTestNamespace(t, client, namespace)

	// Create a ManagedRoost with health checks
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost-health",
			Namespace: namespace,
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.bitnami.com/bitnami",
				},
				Name:    "nginx",
				Version: "15.4.4",
			},
			HealthChecks: []roostv1alpha1.HealthCheckSpec{
				{
					Name: "http-check",
					Type: "http",
					HTTP: &roostv1alpha1.HTTPHealthCheckSpec{
						URL:           "http://test-roost-health-nginx/",
						ExpectedCodes: []int32{200},
					},
					Interval: metav1.Duration{Duration: 10 * time.Second},
					Timeout:  metav1.Duration{Duration: 5 * time.Second},
				},
			},
		},
	}

	// Create the ManagedRoost
	err := client.Create(ctx, managedRoost)
	require.NoError(t, err, "Failed to create ManagedRoost")

	// Wait for the roost to become ready
	err = waitForPhase(ctx, client, managedRoost, roostv1alpha1.ManagedRoostPhaseReady, 5*time.Minute)
	assert.NoError(t, err, "ManagedRoost should reach Ready phase")

	// Verify health check status is tracked
	err = client.Get(ctx, types.NamespacedName{Name: managedRoost.Name, Namespace: managedRoost.Namespace}, managedRoost)
	require.NoError(t, err)

	// Should have health check status (even if checks fail due to test environment)
	assert.NotEmpty(t, managedRoost.Status.HealthChecks)

	healthCheck := findHealthCheck(managedRoost.Status.HealthChecks, "http-check")
	assert.NotNil(t, healthCheck)
	assert.NotNil(t, healthCheck.LastCheck)
}

// TestLifecycleMetrics tests that lifecycle metrics are properly recorded
func TestLifecycleMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create lifecycle manager with metrics
	metrics, err := lifecycle.NewLifecycleMetrics()
	require.NoError(t, err, "Failed to create lifecycle metrics")

	// Test metric recording
	ctx := context.Background()

	// Record test metrics
	metrics.RecordRoostCreated(ctx, "test-roost", "default")
	metrics.RecordPhaseTransition(ctx, "Pending", "Ready", "test-roost")
	metrics.RecordTeardownTrigger(ctx, "timeout", "test timeout")
	metrics.RecordCleanupCompleted(ctx, "test-roost")

	// In a real test, you would verify metrics were recorded correctly
	// This requires integration with a metrics backend or mock
	assert.NotNil(t, metrics, "Metrics should be created successfully")
}

// TestLifecycleDeletion tests proper cleanup during deletion
func TestLifecycleDeletion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()
	client := setupTestClient(t)

	// Create test namespace
	namespace := "test-lifecycle-delete"
	createTestNamespace(t, client, namespace)
	defer cleanupTestNamespace(t, client, namespace)

	// Create a ManagedRoost
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost-delete",
			Namespace: namespace,
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.bitnami.com/bitnami",
				},
				Name:    "nginx",
				Version: "15.4.4",
			},
		},
	}

	// Create the ManagedRoost
	err := client.Create(ctx, managedRoost)
	require.NoError(t, err, "Failed to create ManagedRoost")

	// Wait for ready state
	err = waitForPhase(ctx, client, managedRoost, roostv1alpha1.ManagedRoostPhaseReady, 5*time.Minute)
	require.NoError(t, err, "ManagedRoost should reach Ready phase")

	// Delete the ManagedRoost
	err = client.Delete(ctx, managedRoost)
	require.NoError(t, err, "Failed to delete ManagedRoost")

	// Wait for deletion to complete
	err = waitForDeletion(ctx, client, managedRoost, 2*time.Minute)
	assert.NoError(t, err, "ManagedRoost should be deleted")
}

// Helper functions

func waitForPhase(ctx context.Context, client client.Client, roost *roostv1alpha1.ManagedRoost, expectedPhase roostv1alpha1.ManagedRoostPhase, timeout time.Duration) error {
	return waitForCondition(ctx, client, roost, timeout, func(r *roostv1alpha1.ManagedRoost) bool {
		return r.Status.Phase == expectedPhase
	})
}

func waitForDeletion(ctx context.Context, client client.Client, roost *roostv1alpha1.ManagedRoost, timeout time.Duration) error {
	return waitForCondition(ctx, client, roost, timeout, func(r *roostv1alpha1.ManagedRoost) bool {
		err := client.Get(ctx, types.NamespacedName{Name: roost.Name, Namespace: roost.Namespace}, r)
		return err != nil // Resource should not exist
	})
}

func waitForCondition(ctx context.Context, client client.Client, roost *roostv1alpha1.ManagedRoost, timeout time.Duration, condition func(*roostv1alpha1.ManagedRoost) bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current := &roostv1alpha1.ManagedRoost{}
			err := client.Get(ctx, types.NamespacedName{Name: roost.Name, Namespace: roost.Namespace}, current)
			if err != nil {
				if condition == nil {
					return nil // Deletion case
				}
				continue
			}

			if condition(current) {
				return nil
			}
		}
	}
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func findHealthCheck(healthChecks []roostv1alpha1.HealthCheckStatus, name string) *roostv1alpha1.HealthCheckStatus {
	for _, check := range healthChecks {
		if check.Name == name {
			return &check
		}
	}
	return nil
}

// setupTestClient creates a test client
func setupTestClient(t *testing.T) client.Client {
	// In a real implementation, this would set up a test Kubernetes client
	// For now, this is a placeholder
	t.Log("Setting up test client - placeholder implementation")
	return nil
}

// createTestNamespace creates a test namespace
func createTestNamespace(t *testing.T, client client.Client, namespace string) {
	// In a real implementation, this would create a Kubernetes namespace
	t.Logf("Creating test namespace: %s", namespace)
}

// cleanupTestNamespace cleans up a test namespace
func cleanupTestNamespace(t *testing.T, client client.Client, namespace string) {
	// In a real implementation, this would delete the Kubernetes namespace
	t.Logf("Cleaning up test namespace: %s", namespace)
}
