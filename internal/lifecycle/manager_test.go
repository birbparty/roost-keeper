package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// mockHelmManager implements the helm.Manager interface for testing
type mockHelmManager struct {
	statusToReturn *roostv1alpha1.HelmReleaseStatus
	errorToReturn  error
}

func (m *mockHelmManager) Install(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return true, m.errorToReturn
}

func (m *mockHelmManager) Upgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return true, m.errorToReturn
}

func (m *mockHelmManager) Uninstall(ctx context.Context, roost *roostv1alpha1.ManagedRoost) error {
	return m.errorToReturn
}

func (m *mockHelmManager) ReleaseExists(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return m.statusToReturn != nil, m.errorToReturn
}

func (m *mockHelmManager) NeedsUpgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return false, m.errorToReturn
}

func (m *mockHelmManager) GetStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*roostv1alpha1.HelmReleaseStatus, error) {
	return m.statusToReturn, m.errorToReturn
}

func (m *mockHelmManager) Rollback(ctx context.Context, roost *roostv1alpha1.ManagedRoost, revision int) error {
	return m.errorToReturn
}

func TestNewManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := fake.NewClientBuilder().Build()
	helmManager := &mockHelmManager{}
	recorder := &record.FakeRecorder{}

	manager, err := NewManager(client, helmManager, recorder, logger)
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.metrics)
}

func TestTrackRoostCreation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := fake.NewClientBuilder().Build()
	helmManager := &mockHelmManager{}
	recorder := &record.FakeRecorder{}

	manager, err := NewManager(client, helmManager, recorder, logger)
	require.NoError(t, err)

	ctx := context.Background()
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Name:    "test-chart",
				Version: "1.0.0",
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://example.com/charts",
				},
			},
		},
	}

	err = manager.trackRoostCreation(ctx, managedRoost)
	assert.NoError(t, err)

	// Verify phase was set
	assert.Equal(t, roostv1alpha1.ManagedRoostPhasePending, managedRoost.Status.Phase)
	assert.NotNil(t, managedRoost.Status.LastUpdateTime)
}

func TestEvaluateTeardownTriggers(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := fake.NewClientBuilder().Build()
	helmManager := &mockHelmManager{}
	recorder := &record.FakeRecorder{}

	manager, err := NewManager(client, helmManager, recorder, logger)
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name           string
		managedRoost   *roostv1alpha1.ManagedRoost
		expectedResult bool
		expectedReason string
	}{
		{
			name: "timeout trigger activated",
			managedRoost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-roost",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
						Triggers: []roostv1alpha1.TeardownTriggerSpec{
							{
								Type:    "timeout",
								Timeout: &metav1.Duration{Duration: 1 * time.Hour},
							},
						},
					},
				},
			},
			expectedResult: true,
			expectedReason: "timeout exceeded",
		},
		{
			name: "timeout not yet reached",
			managedRoost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-roost",
					Namespace:         "default",
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-30 * time.Minute)},
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
						Triggers: []roostv1alpha1.TeardownTriggerSpec{
							{
								Type:    "timeout",
								Timeout: &metav1.Duration{Duration: 1 * time.Hour},
							},
						},
					},
				},
			},
			expectedResult: false,
		},
		{
			name: "webhook trigger via annotation",
			managedRoost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
					Annotations: map[string]string{
						"roost-keeper.io/teardown-trigger": "webhook-signal",
					},
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
						Triggers: []roostv1alpha1.TeardownTriggerSpec{
							{
								Type: "webhook",
								Webhook: &roostv1alpha1.WebhookTriggerSpec{
									URL: "https://example.com/webhook",
								},
							},
						},
					},
				},
			},
			expectedResult: true,
			expectedReason: "webhook triggered teardown",
		},
		{
			name: "no teardown policy",
			managedRoost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{},
			},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldTeardown, reason := manager.evaluateTeardownPolicies(ctx, tt.managedRoost)
			assert.Equal(t, tt.expectedResult, shouldTeardown)
			if tt.expectedReason != "" {
				assert.Contains(t, reason, tt.expectedReason)
			}
		})
	}
}

func TestMonitorDeploymentStatus(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := fake.NewClientBuilder().Build()
	recorder := &record.FakeRecorder{}

	tests := []struct {
		name          string
		helmStatus    *roostv1alpha1.HelmReleaseStatus
		expectedPhase roostv1alpha1.ManagedRoostPhase
	}{
		{
			name: "deployed status",
			helmStatus: &roostv1alpha1.HelmReleaseStatus{
				Name:     "test-release",
				Status:   "deployed",
				Revision: 1,
			},
			expectedPhase: roostv1alpha1.ManagedRoostPhaseReady,
		},
		{
			name: "pending-install status",
			helmStatus: &roostv1alpha1.HelmReleaseStatus{
				Name:     "test-release",
				Status:   "pending-install",
				Revision: 1,
			},
			expectedPhase: roostv1alpha1.ManagedRoostPhaseDeploying,
		},
		{
			name: "failed status",
			helmStatus: &roostv1alpha1.HelmReleaseStatus{
				Name:     "test-release",
				Status:   "failed",
				Revision: 1,
			},
			expectedPhase: roostv1alpha1.ManagedRoostPhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			helmManager := &mockHelmManager{
				statusToReturn: tt.helmStatus,
			}

			manager, err := NewManager(client, helmManager, recorder, logger)
			require.NoError(t, err)

			ctx := context.Background()
			managedRoost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
				Status: roostv1alpha1.ManagedRoostStatus{
					Phase: roostv1alpha1.ManagedRoostPhasePending,
				},
			}

			err = manager.monitorDeploymentStatus(ctx, managedRoost)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPhase, managedRoost.Status.Phase)
		})
	}
}

func TestTriggerTeardown(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := fake.NewClientBuilder().Build()
	helmManager := &mockHelmManager{}
	recorder := &record.FakeRecorder{}

	manager, err := NewManager(client, helmManager, recorder, logger)
	require.NoError(t, err)

	ctx := context.Background()
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Status: roostv1alpha1.ManagedRoostStatus{
			Phase: roostv1alpha1.ManagedRoostPhaseReady,
		},
	}

	reason := "test teardown reason"
	err = manager.triggerTeardown(ctx, managedRoost, reason)
	assert.NoError(t, err)

	// Verify teardown was triggered
	assert.Equal(t, roostv1alpha1.ManagedRoostPhaseTearingDown, managedRoost.Status.Phase)
	assert.NotNil(t, managedRoost.Status.Teardown)
	assert.True(t, managedRoost.Status.Teardown.Triggered)
	assert.Equal(t, reason, managedRoost.Status.Teardown.TriggerReason)
	assert.NotNil(t, managedRoost.Status.Teardown.TriggerTime)
	assert.Equal(t, int32(0), managedRoost.Status.Teardown.Progress)
}

func TestCleanupResources(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := fake.NewClientBuilder().Build()
	helmManager := &mockHelmManager{}
	recorder := &record.FakeRecorder{}

	manager, err := NewManager(client, helmManager, recorder, logger)
	require.NoError(t, err)

	ctx := context.Background()
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Status: roostv1alpha1.ManagedRoostStatus{
			Phase: roostv1alpha1.ManagedRoostPhaseTearingDown,
			Teardown: &roostv1alpha1.TeardownStatus{
				Triggered: true,
				Progress:  0,
			},
		},
	}

	err = manager.CleanupResources(ctx, managedRoost)
	assert.NoError(t, err)

	// Verify cleanup completed
	assert.Equal(t, int32(100), managedRoost.Status.Teardown.Progress)
	assert.NotNil(t, managedRoost.Status.Teardown.CompletionTime)
}

func TestCountRecentFailures(t *testing.T) {
	logger := zaptest.NewLogger(t)
	client := fake.NewClientBuilder().Build()
	helmManager := &mockHelmManager{}
	recorder := &record.FakeRecorder{}

	manager, err := NewManager(client, helmManager, recorder, logger)
	require.NoError(t, err)

	managedRoost := &roostv1alpha1.ManagedRoost{
		Status: roostv1alpha1.ManagedRoostStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Ready",
					Status: metav1.ConditionFalse,
				},
				{
					Type:   "Healthy",
					Status: metav1.ConditionFalse,
				},
			},
			HealthChecks: []roostv1alpha1.HealthCheckStatus{
				{
					Name:         "test-check",
					Status:       "unhealthy",
					FailureCount: 3,
				},
			},
		},
	}

	failureCount := manager.countRecentFailures(managedRoost)
	assert.Equal(t, int32(5), failureCount) // 2 from conditions + 3 from health check
}
