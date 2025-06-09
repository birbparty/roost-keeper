package controllers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

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
			HelmChart: roostv1alpha1.HelmChartSpec{
				Repository: "https://charts.example.com",
				Chart:      "nginx",
				Version:    "1.0.0",
			},
			HealthChecks: roostv1alpha1.HealthCheckSpec{
				Enabled: true,
				HTTP: &roostv1alpha1.HTTPHealthCheck{
					Path: "/health",
					Port: 8080,
				},
			},
			TeardownPolicy: roostv1alpha1.TeardownPolicySpec{
				TriggerCondition: roostv1alpha1.TeardownTriggerManual,
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

	// Create reconciler
	reconciler := &ManagedRoostReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Logger: logger,
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
	assert.Equal(t, time.Minute*1, result.RequeueAfter)

	// Verify the ManagedRoost was updated
	var updatedRoost roostv1alpha1.ManagedRoost
	err = fakeClient.Get(ctx, req.NamespacedName, &updatedRoost)
	assert.NoError(t, err)
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
	logger := zap.New(zap.UseDevMode(true))

	// Create reconciler
	reconciler := &ManagedRoostReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Logger: logger,
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

	// Should not error and should not requeue
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
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
