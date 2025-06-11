package integration

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/events"
	"github.com/birbparty/roost-keeper/internal/health"
	"github.com/birbparty/roost-keeper/internal/teardown"
)

func TestTeardownPolicies(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Create a fake Kubernetes client
	scheme := runtime.NewScheme()
	require.NoError(t, roostv1alpha1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	t.Run("TimeoutEvaluator", func(t *testing.T) {
		// Create a roost with timeout policy
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-roost",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-3 * time.Hour)), // Created 3 hours ago
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
					Triggers: []roostv1alpha1.TeardownTriggerSpec{
						{
							Type:    "timeout",
							Timeout: &metav1.Duration{Duration: 2 * time.Hour}, // 2 hour timeout
						},
					},
				},
			},
		}

		// Create timeout evaluator
		evaluator := teardown.NewTimeoutEvaluator(logger)

		// Test evaluation
		teardownCtx := &teardown.TeardownContext{
			ManagedRoost:   roost,
			EvaluationTime: time.Now(),
			CorrelationID:  "test-correlation-123",
			Metadata:       make(map[string]interface{}),
			DryRun:         false,
			ForceOverride:  false,
		}

		decision, err := evaluator.ShouldTeardown(ctx, teardownCtx)
		require.NoError(t, err)
		assert.True(t, decision.ShouldTeardown, "Should teardown due to timeout")
		assert.Equal(t, "timeout", decision.TriggerType)
		assert.Contains(t, decision.Reason, "Timeout reached")
	})

	t.Run("FailureCountEvaluator", func(t *testing.T) {
		// Create a roost with failure count policy
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost-failures",
				Namespace: "default",
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
					Triggers: []roostv1alpha1.TeardownTriggerSpec{
						{
							Type:         "failure_count",
							FailureCount: func() *int32 { v := int32(5); return &v }(), // 5 failure threshold
						},
					},
				},
			},
			Status: roostv1alpha1.ManagedRoostStatus{
				HealthChecks: []roostv1alpha1.HealthCheckStatus{
					{
						Name:         "test-check-1",
						Status:       "unhealthy",
						FailureCount: 3,
					},
					{
						Name:         "test-check-2",
						Status:       "unhealthy",
						FailureCount: 4,
					},
				},
			},
		}

		// Create failure count evaluator
		evaluator := teardown.NewFailureCountEvaluator(logger)

		// Test evaluation
		teardownCtx := &teardown.TeardownContext{
			ManagedRoost:   roost,
			EvaluationTime: time.Now(),
			CorrelationID:  "test-correlation-456",
			Metadata:       make(map[string]interface{}),
			DryRun:         false,
			ForceOverride:  false,
		}

		decision, err := evaluator.ShouldTeardown(ctx, teardownCtx)
		require.NoError(t, err)
		assert.True(t, decision.ShouldTeardown, "Should teardown due to failure count")
		assert.Equal(t, "failure_count", decision.TriggerType)
		assert.Contains(t, decision.Reason, "Failure count threshold reached")
	})

	t.Run("TeardownManager", func(t *testing.T) {
		// Create health checker mock
		healthChecker := health.NewChecker(logger)

		// Create event manager (can be nil for testing)
		var eventManager *events.Manager

		// Create teardown manager
		config := teardown.DefaultManagerConfig()
		config.DryRunMode = true // Enable dry run for testing

		manager, err := teardown.NewManager(client, healthChecker, eventManager, logger, config)
		require.NoError(t, err)

		// Create a roost for testing - without data preservation to avoid safety check blocks
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "test-manager-roost",
				Namespace:         "default",
				CreationTimestamp: metav1.NewTime(time.Now().Add(-1 * time.Hour)),
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
					Triggers: []roostv1alpha1.TeardownTriggerSpec{
						{
							Type:    "timeout",
							Timeout: &metav1.Duration{Duration: 30 * time.Minute}, // 30 minute timeout
						},
					},
					RequireManualApproval: false,
					// No data preservation to avoid safety check failures
				},
			},
		}

		// Evaluate teardown policy
		decision, err := manager.EvaluateTeardownPolicy(ctx, roost)
		require.NoError(t, err)

		// The decision might be blocked by safety checks, but we should still get a decision
		assert.NotNil(t, decision, "Should get a teardown decision")

		// Test safety checks
		assert.NotEmpty(t, decision.SafetyChecks, "Should have safety checks")

		// Verify audit logging occurred (check that we don't get errors)
		assert.NotNil(t, decision.EvaluatedAt)
		assert.NotEmpty(t, decision.Reason)
	})

	t.Run("SafetyChecks", func(t *testing.T) {
		// Create safety checker
		safetyChecker := teardown.NewSafetyChecker(client, logger)

		// Create a roost for testing
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-safety-roost",
				Namespace: "default",
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
					DataPreservation: &roostv1alpha1.DataPreservationSpec{
						Enabled: true,
					},
				},
			},
		}

		// Create a mock decision
		decision := &teardown.TeardownDecision{
			ShouldTeardown: true,
			Reason:         "Test teardown",
			Urgency:        teardown.TeardownUrgencyMedium,
		}

		// Perform safety checks
		checks, err := safetyChecker.PerformSafetyChecks(ctx, roost, decision)
		require.NoError(t, err)
		assert.NotEmpty(t, checks, "Should have safety checks")

		// Verify we have expected safety checks
		checkNames := make(map[string]bool)
		for _, check := range checks {
			checkNames[check.Name] = true
		}

		expectedChecks := []string{
			"active_connections",
			"persistent_volumes",
			"dependent_resources",
			"running_pods",
			"external_endpoints",
			"data_persistence",
			"backup_requirements",
			"manual_approval",
		}

		for _, expectedCheck := range expectedChecks {
			assert.True(t, checkNames[expectedCheck], "Should have safety check: %s", expectedCheck)
		}
	})

	t.Run("MetricsCollection", func(t *testing.T) {
		// Create a custom registry for testing to avoid conflicts
		testRegistry := prometheus.NewRegistry()

		// Create metrics collector with custom registry
		metricsCollector, err := teardown.NewTeardownMetricsCollectorWithRegistry(testRegistry)
		require.NoError(t, err)

		// Create test roost
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-metrics-roost",
				Namespace: "default",
			},
		}

		// Record evaluation start
		metricsCollector.RecordEvaluationStart(ctx, roost)

		// Create test decision
		decision := &teardown.TeardownDecision{
			ShouldTeardown: true,
			Reason:         "Test decision",
			TriggerType:    "timeout",
			SafetyChecks: []teardown.SafetyCheck{
				{
					Name:   "test_check",
					Passed: true,
				},
			},
		}

		// Record evaluation result
		metricsCollector.RecordEvaluationResult(ctx, roost, decision)

		// Create test execution
		execution := &teardown.TeardownExecution{
			ExecutionID: "test-exec-123",
			Decision:    decision,
			StartTime:   time.Now(),
			Status:      teardown.TeardownExecutionStatusRunning,
		}

		// Record execution start
		metricsCollector.RecordExecutionStart(ctx, roost, execution)

		// Mark execution as completed
		endTime := time.Now()
		execution.EndTime = &endTime
		execution.Status = teardown.TeardownExecutionStatusCompleted

		// Record execution success
		metricsCollector.RecordExecutionSuccess(ctx, roost, execution)

		// Get metrics
		metrics := metricsCollector.GetMetrics()
		assert.Equal(t, int64(1), metrics.EvaluationCount)
		assert.Equal(t, int64(1), metrics.ExecutionCount)
		assert.Equal(t, int64(1), metrics.SuccessCount)
		assert.Equal(t, int64(0), metrics.FailureCount)
		assert.Contains(t, metrics.TriggerTypeCounts, "timeout")
		assert.Contains(t, metrics.SafetyCheckCounts, "test_check:passed")
	})
}

func TestTeardownPolicyValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("TimeoutValidation", func(t *testing.T) {
		evaluator := teardown.NewTimeoutEvaluator(logger)

		// Test valid configuration
		validTriggers := []roostv1alpha1.TeardownTriggerSpec{
			{
				Type:    "timeout",
				Timeout: &metav1.Duration{Duration: 1 * time.Hour},
			},
		}
		err := evaluator.Validate(validTriggers)
		assert.NoError(t, err)

		// Test invalid configuration
		invalidTriggers := []roostv1alpha1.TeardownTriggerSpec{
			{
				Type: "timeout",
				// Missing Timeout field
			},
		}
		err = evaluator.Validate(invalidTriggers)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout trigger requires timeout configuration")
	})

	t.Run("FailureCountValidation", func(t *testing.T) {
		evaluator := teardown.NewFailureCountEvaluator(logger)

		// Test valid configuration
		validTriggers := []roostv1alpha1.TeardownTriggerSpec{
			{
				Type:         "failure_count",
				FailureCount: func() *int32 { v := int32(5); return &v }(),
			},
		}
		err := evaluator.Validate(validTriggers)
		assert.NoError(t, err)

		// Test invalid configuration
		invalidTriggers := []roostv1alpha1.TeardownTriggerSpec{
			{
				Type: "failure_count",
				// Missing FailureCount field
			},
		}
		err = evaluator.Validate(invalidTriggers)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failure_count trigger requires failureCount configuration")
	})
}
