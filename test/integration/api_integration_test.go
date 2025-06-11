package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/api"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

func TestAPIIntegration(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	require.NoError(t, roostkeeper.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	logger := zaptest.NewLogger(t)
	recorder := record.NewFakeRecorder(100)

	// Create API metrics (simplified for testing)
	apiMetrics, err := telemetry.NewAPIMetrics()
	require.NoError(t, err)

	// Create API manager
	manager := api.NewManager(client, scheme, recorder, apiMetrics, logger)

	t.Run("ValidateResource", func(t *testing.T) {
		// Test basic validation
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
			Spec: roostkeeper.ManagedRoostSpec{
				Chart: roostkeeper.ChartSpec{
					Name:    "nginx",
					Version: "1.0.0",
					Repository: roostkeeper.ChartRepositorySpec{
						URL:  "https://charts.bitnami.com/bitnami",
						Type: "http",
					},
				},
			},
		}

		ctx := context.Background()
		result, err := manager.ValidateResource(ctx, managedRoost)
		require.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)

		// Test validation with errors
		invalidRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-roost",
				Namespace: "default",
			},
			Spec: roostkeeper.ManagedRoostSpec{
				Chart: roostkeeper.ChartSpec{
					// Missing required fields
				},
			},
		}

		result, err = manager.ValidateResource(ctx, invalidRoost)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.NotEmpty(t, result.Errors)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
			Spec: roostkeeper.ManagedRoostSpec{
				Chart: roostkeeper.ChartSpec{
					Name:    "nginx",
					Version: "1.0.0",
					Repository: roostkeeper.ChartRepositorySpec{
						URL:  "https://charts.bitnami.com/bitnami",
						Type: "http",
					},
				},
			},
		}

		// Create the resource first
		ctx := context.Background()
		require.NoError(t, client.Create(ctx, managedRoost))

		// Update status
		statusUpdate := &api.StatusUpdate{
			Phase: "Ready",
			Conditions: []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "DeploymentSuccessful",
					Message: "Deployment completed successfully",
				},
			},
		}

		err := manager.UpdateStatus(ctx, managedRoost, statusUpdate)
		require.NoError(t, err)

		assert.Equal(t, roostkeeper.ManagedRoostPhase("Ready"), managedRoost.Status.Phase)
		assert.Len(t, managedRoost.Status.Conditions, 1)
		assert.Equal(t, "Ready", managedRoost.Status.Conditions[0].Type)
	})

	t.Run("TrackLifecycle", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
			Spec: roostkeeper.ManagedRoostSpec{
				Chart: roostkeeper.ChartSpec{
					Name:    "nginx",
					Version: "1.0.0",
					Repository: roostkeeper.ChartRepositorySpec{
						URL:  "https://charts.bitnami.com/bitnami",
						Type: "http",
					},
				},
			},
		}

		ctx := context.Background()

		// Track lifecycle event
		event := api.LifecycleEvent{
			Type:      api.LifecycleEventCreated,
			Message:   "ManagedRoost created",
			Details:   map[string]string{"operator": "test"},
			EventType: "Normal",
		}

		err := manager.TrackLifecycle(ctx, managedRoost, event)
		require.NoError(t, err)

		// Verify lifecycle tracking was initialized
		assert.NotNil(t, managedRoost.Status.Lifecycle)
		assert.Len(t, managedRoost.Status.Lifecycle.Events, 1)
		assert.Equal(t, "Created", managedRoost.Status.Lifecycle.Events[0].Type)
		assert.Equal(t, "ManagedRoost created", managedRoost.Status.Lifecycle.Events[0].Message)

		// Track another event
		event2 := api.LifecycleEvent{
			Type:      api.LifecycleEventInstalling,
			Message:   "Starting installation",
			EventType: "Normal",
		}

		err = manager.TrackLifecycle(ctx, managedRoost, event2)
		require.NoError(t, err)

		assert.Len(t, managedRoost.Status.Lifecycle.Events, 2)
		assert.Equal(t, int32(1), managedRoost.Status.Lifecycle.EventCounts["Created"])
		assert.Equal(t, int32(1), managedRoost.Status.Lifecycle.EventCounts["Installing"])
	})

	t.Run("CacheOperations", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
			Status: roostkeeper.ManagedRoostStatus{
				Phase: roostkeeper.ManagedRoostPhaseReady,
			},
		}

		// Test cache miss
		cached := manager.GetCachedStatus(managedRoost)
		assert.Nil(t, cached)

		// Manually set cache (simulate status update)
		manager.InvalidateCache(managedRoost) // Should not panic even if nothing is cached

		// Note: In a real integration test with more complex scenarios,
		// we would test actual caching behavior through status updates
	})
}

func TestResourceValidator(t *testing.T) {
	logger := zaptest.NewLogger(t)
	validator := api.NewResourceValidator(logger)

	t.Run("ValidateValidResource", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
			Spec: roostkeeper.ManagedRoostSpec{
				Chart: roostkeeper.ChartSpec{
					Name:    "nginx",
					Version: "1.0.0",
					Repository: roostkeeper.ChartRepositorySpec{
						URL:  "https://charts.bitnami.com/bitnami",
						Type: "http",
					},
				},
				HealthChecks: []roostkeeper.HealthCheckSpec{
					{
						Name: "http-check",
						Type: "http",
						HTTP: &roostkeeper.HTTPHealthCheckSpec{
							URL:    "http://nginx/health",
							Method: "GET",
						},
						FailureThreshold: 3,
						Weight:           10,
					},
				},
			},
		}

		errors := validator.Validate(managedRoost)
		assert.Empty(t, errors)
	})

	t.Run("ValidateInvalidResource", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-roost",
				Namespace: "default",
			},
			Spec: roostkeeper.ManagedRoostSpec{
				Chart: roostkeeper.ChartSpec{
					// Missing required fields
					Repository: roostkeeper.ChartRepositorySpec{
						URL: "invalid-url",
					},
				},
				HealthChecks: []roostkeeper.HealthCheckSpec{
					{
						Name: "invalid-check",
						Type: "http",
						// Missing HTTP configuration
						FailureThreshold: 0,   // Invalid value
						Weight:           101, // Invalid value
					},
				},
			},
		}

		errors := validator.Validate(managedRoost)
		assert.NotEmpty(t, errors)

		// Check for specific validation errors
		hasChartNameError := false
		hasHTTPConfigError := false
		hasFailureThresholdError := false
		hasWeightError := false

		for _, err := range errors {
			switch err.Code {
			case "CHART_NAME_MISSING":
				hasChartNameError = true
			case "HTTP_CONFIG_MISSING":
				hasHTTPConfigError = true
			case "FAILURE_THRESHOLD_INVALID":
				hasFailureThresholdError = true
			case "WEIGHT_INVALID":
				hasWeightError = true
			}
		}

		assert.True(t, hasChartNameError, "Should have chart name validation error")
		assert.True(t, hasHTTPConfigError, "Should have HTTP config validation error")
		assert.True(t, hasFailureThresholdError, "Should have failure threshold validation error")
		assert.True(t, hasWeightError, "Should have weight validation error")
	})
}

func TestLifecycleTracker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	recorder := record.NewFakeRecorder(100)
	tracker := api.NewLifecycleTracker(logger, recorder)

	t.Run("TrackSingleEvent", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
		}

		ctx := context.Background()
		event := api.LifecycleEvent{
			Type:    api.LifecycleEventCreated,
			Message: "Resource created",
			Details: map[string]string{"source": "test"},
		}

		err := tracker.TrackEvent(ctx, managedRoost, event)
		require.NoError(t, err)

		// Verify lifecycle status was initialized
		assert.NotNil(t, managedRoost.Status.Lifecycle)
		assert.NotNil(t, managedRoost.Status.Lifecycle.CreatedAt)
		assert.NotNil(t, managedRoost.Status.Lifecycle.LastActivity)
		assert.Len(t, managedRoost.Status.Lifecycle.Events, 1)

		// Check event details
		eventRecord := managedRoost.Status.Lifecycle.Events[0]
		assert.Equal(t, "Created", eventRecord.Type)
		assert.Equal(t, "Resource created", eventRecord.Message)
		assert.Equal(t, "test", eventRecord.Details["source"])

		// Check event counts
		assert.Equal(t, int32(1), managedRoost.Status.Lifecycle.EventCounts["Created"])
	})

	t.Run("TrackMultipleEvents", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
		}

		ctx := context.Background()

		// Track multiple events
		events := []api.LifecycleEvent{
			{Type: api.LifecycleEventCreated, Message: "Created"},
			{Type: api.LifecycleEventInstalling, Message: "Installing"},
			{Type: api.LifecycleEventReady, Message: "Ready"},
			{Type: api.LifecycleEventHealthy, Message: "Healthy"},
		}

		for _, event := range events {
			err := tracker.TrackEvent(ctx, managedRoost, event)
			require.NoError(t, err)
		}

		// Verify all events were tracked
		assert.Len(t, managedRoost.Status.Lifecycle.Events, 4)
		assert.Equal(t, int32(1), managedRoost.Status.Lifecycle.EventCounts["Created"])
		assert.Equal(t, int32(1), managedRoost.Status.Lifecycle.EventCounts["Installing"])
		assert.Equal(t, int32(1), managedRoost.Status.Lifecycle.EventCounts["Ready"])
		assert.Equal(t, int32(1), managedRoost.Status.Lifecycle.EventCounts["Healthy"])

		// Test helper methods
		count := tracker.GetEventCount(managedRoost, api.LifecycleEventCreated)
		assert.Equal(t, int32(1), count)

		recentEvents := tracker.GetRecentEvents(managedRoost, 2)
		assert.Len(t, recentEvents, 2)
		assert.Equal(t, "Healthy", recentEvents[1].Type)
		assert.Equal(t, "Ready", recentEvents[0].Type)

		// Test recent event check
		isRecent := tracker.IsEventRecent(managedRoost, api.LifecycleEventHealthy, 1*time.Minute)
		assert.True(t, isRecent)

		isNotRecent := tracker.IsEventRecent(managedRoost, api.LifecycleEventHealthy, 1*time.Nanosecond)
		assert.False(t, isNotRecent)
	})

	t.Run("EventLimiting", func(t *testing.T) {
		managedRoost := &roostkeeper.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
		}

		ctx := context.Background()

		// Track more than 50 events (the limit)
		for i := 0; i < 60; i++ {
			event := api.LifecycleEvent{
				Type:    api.LifecycleEventHealthy,
				Message: "Health check",
			}
			err := tracker.TrackEvent(ctx, managedRoost, event)
			require.NoError(t, err)
		}

		// Verify events were limited to 50
		assert.Len(t, managedRoost.Status.Lifecycle.Events, 50)
		assert.Equal(t, int32(60), managedRoost.Status.Lifecycle.EventCounts["Healthy"])
	})
}
