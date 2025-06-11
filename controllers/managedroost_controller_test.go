package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Mock implementations for testing

// mockHelmManager implements the helm.Manager interface for testing
type mockHelmManager struct {
	installResult  bool
	installError   error
	upgradeResult  bool
	upgradeError   error
	uninstallError error
	existsResult   bool
	existsError    error
	needsUpgrade   bool
	upgradeError2  error
	status         *roostv1alpha1.HelmReleaseStatus
	statusError    error
}

func (m *mockHelmManager) Install(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return m.installResult, m.installError
}

func (m *mockHelmManager) Upgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return m.upgradeResult, m.upgradeError
}

func (m *mockHelmManager) Uninstall(ctx context.Context, roost *roostv1alpha1.ManagedRoost) error {
	return m.uninstallError
}

func (m *mockHelmManager) ReleaseExists(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return m.existsResult, m.existsError
}

func (m *mockHelmManager) NeedsUpgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return m.needsUpgrade, m.upgradeError2
}

func (m *mockHelmManager) GetStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*roostv1alpha1.HelmReleaseStatus, error) {
	return m.status, m.statusError
}

func (m *mockHelmManager) Rollback(ctx context.Context, roost *roostv1alpha1.ManagedRoost, revision int) error {
	return nil // Mock implementation always succeeds
}

// mockHealthChecker implements the health.Checker interface for testing
type mockHealthChecker struct {
	healthResult bool
	healthError  error
	statuses     []roostv1alpha1.HealthCheckStatus
	statusError  error
}

func (m *mockHealthChecker) CheckHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	return m.healthResult, m.healthError
}

func (m *mockHealthChecker) GetHealthStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost) ([]roostv1alpha1.HealthCheckStatus, error) {
	return m.statuses, m.statusError
}

// mockEventRecorder implements the record.EventRecorder interface for testing
type mockEventRecorder struct {
	events []string
}

func (m *mockEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	m.events = append(m.events, eventtype+":"+reason+":"+message)
}

func (m *mockEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, eventtype+":"+reason+":"+messageFmt)
}

func (m *mockEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, eventtype+":"+reason+":"+messageFmt)
}

func TestManagedRoostReconciler_Reconcile(t *testing.T) {
	// Create a fake client
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	// Create a sample ManagedRoost
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.example.com",
				},
				Name:    "nginx",
				Version: "1.0.0",
			},
			HealthChecks: []roostv1alpha1.HealthCheckSpec{
				{
					Name: "http-check",
					Type: "http",
					HTTP: &roostv1alpha1.HTTPHealthCheckSpec{
						URL: "http://localhost:8080/health",
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(managedRoost).
		Build()

	// Create logger
	logger, err := telemetry.NewLogger()
	assert.NoError(t, err)

	// Initialize ZAP logger for testing
	zapLogger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	// Create mock implementations for testing
	helmManager := &mockHelmManager{
		installResult: true,
		existsResult:  false, // Simulate new installation
	}
	healthChecker := &mockHealthChecker{
		healthResult: true,
	}
	eventRecorder := &mockEventRecorder{}

	reconciler := &ManagedRoostReconciler{
		Client:        fakeClient,
		Scheme:        scheme,
		Logger:        logger.WithName("controllers").WithName("ManagedRoost"),
		ZapLogger:     zapLogger,
		HelmManager:   helmManager,
		HealthChecker: healthChecker,
		Recorder:      eventRecorder,
	}

	// Test reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-roost",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	// Assertions
	assert.NoError(t, err)
	// Should requeue (either immediately or after time) for continued processing
	assert.True(t, result.Requeue || result.RequeueAfter > 0)

	// Verify the ManagedRoost was updated
	var updatedRoost roostv1alpha1.ManagedRoost
	err = fakeClient.Get(ctx, req.NamespacedName, &updatedRoost)
	assert.NoError(t, err)

	// Should have a phase set (initially will be Pending)
	assert.NotEmpty(t, updatedRoost.Status.Phase)

	// Should have observed generation updated
	assert.Equal(t, managedRoost.Generation, updatedRoost.Status.ObservedGeneration)
}

func TestManagedRoostReconciler_ReconcileNotFound(t *testing.T) {
	// Create a fake client without any objects
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Create logger
	logger, err := telemetry.NewLogger()
	assert.NoError(t, err)

	// Initialize ZAP logger for testing
	zapLogger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	// Create mock implementations
	helmManager := &mockHelmManager{}
	healthChecker := &mockHealthChecker{}
	eventRecorder := &mockEventRecorder{}

	// Create reconciler
	reconciler := &ManagedRoostReconciler{
		Client:        fakeClient,
		Scheme:        scheme,
		Logger:        logger,
		ZapLogger:     zapLogger,
		HelmManager:   helmManager,
		HealthChecker: healthChecker,
		Recorder:      eventRecorder,
	}

	// Test reconcile with non-existent resource
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)

	// Should not error when resource is not found
	assert.NoError(t, err)
	// Should not requeue since resource doesn't exist
	assert.Equal(t, ctrl.Result{}, result)
}

func TestManagedRoostReconciler_PhaseTransitions(t *testing.T) {
	// Create a fake client
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	// Create a ManagedRoost in pending phase
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.example.com",
				},
				Name:    "nginx",
				Version: "1.0.0",
			},
		},
		Status: roostv1alpha1.ManagedRoostStatus{
			Phase: roostv1alpha1.ManagedRoostPhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(managedRoost).
		Build()

	// Create logger
	logger, err := telemetry.NewLogger()
	assert.NoError(t, err)

	// Initialize ZAP logger for testing
	zapLogger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	// Create mock implementations - simulate successful deployment
	helmManager := &mockHelmManager{
		existsResult:  false, // Release doesn't exist yet
		installResult: true,  // Installation succeeds
	}
	healthChecker := &mockHealthChecker{
		healthResult: true,
	}
	eventRecorder := &mockEventRecorder{}

	reconciler := &ManagedRoostReconciler{
		Client:        fakeClient,
		Scheme:        scheme,
		Logger:        logger.WithName("controllers").WithName("ManagedRoost"),
		ZapLogger:     zapLogger,
		HelmManager:   helmManager,
		HealthChecker: healthChecker,
		Recorder:      eventRecorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-roost",
			Namespace: "default",
		},
	}

	ctx := context.Background()

	// First reconcile may add finalizer and transition to Deploying
	result1, err := reconciler.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.True(t, result1.Requeue)

	// Second reconcile should progress the deployment
	_, err = reconciler.Reconcile(ctx, req)
	assert.NoError(t, err)

	// Verify phase progression (should be either Deploying or Ready)
	var updatedRoost roostv1alpha1.ManagedRoost
	err = fakeClient.Get(ctx, req.NamespacedName, &updatedRoost)
	assert.NoError(t, err)

	// Should have progressed from Pending
	assert.NotEqual(t, roostv1alpha1.ManagedRoostPhasePending, updatedRoost.Status.Phase)
	assert.Contains(t, []roostv1alpha1.ManagedRoostPhase{
		roostv1alpha1.ManagedRoostPhaseDeploying,
		roostv1alpha1.ManagedRoostPhaseReady,
	}, updatedRoost.Status.Phase)
}

func TestManagedRoostReconciler_HelmInstallFailure(t *testing.T) {
	// Create a fake client
	scheme := runtime.NewScheme()
	err := roostv1alpha1.AddToScheme(scheme)
	assert.NoError(t, err)

	// Create a ManagedRoost
	managedRoost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-roost",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL: "https://charts.example.com",
				},
				Name:    "nginx",
				Version: "1.0.0",
			},
		},
		Status: roostv1alpha1.ManagedRoostStatus{
			Phase: roostv1alpha1.ManagedRoostPhaseDeploying,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(managedRoost).
		Build()

	// Create logger
	logger, err := telemetry.NewLogger()
	assert.NoError(t, err)

	// Initialize ZAP logger for testing
	zapLogger, err := zap.NewDevelopment()
	assert.NoError(t, err)

	// Create mock implementations - simulate installation failure
	helmManager := &mockHelmManager{
		installResult: false,
		installError:  assert.AnError,
	}
	healthChecker := &mockHealthChecker{}
	eventRecorder := &mockEventRecorder{}

	reconciler := &ManagedRoostReconciler{
		Client:        fakeClient,
		Scheme:        scheme,
		Logger:        logger.WithName("controllers").WithName("ManagedRoost"),
		ZapLogger:     zapLogger,
		HelmManager:   helmManager,
		HealthChecker: healthChecker,
		Recorder:      eventRecorder,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-roost",
			Namespace: "default",
		},
	}

	ctx := context.Background()
	result, err := reconciler.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.True(t, result.Requeue)

	// Verify phase transition to Failed
	var updatedRoost roostv1alpha1.ManagedRoost
	err = fakeClient.Get(ctx, req.NamespacedName, &updatedRoost)
	assert.NoError(t, err)
	assert.Equal(t, roostv1alpha1.ManagedRoostPhaseFailed, updatedRoost.Status.Phase)

	// Verify error event was recorded
	assert.Greater(t, len(eventRecorder.events), 0)
	assert.Contains(t, eventRecorder.events[0], "Warning")
}

func TestGenerateReconcileID(t *testing.T) {
	id1 := generateReconcileID()
	time.Sleep(time.Millisecond * 10)
	id2 := generateReconcileID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
	assert.Len(t, id1, 19) // Format: "20060102-150405.000"
}
