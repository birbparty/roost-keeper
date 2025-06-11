package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/health/kubernetes"
)

func TestKubernetesHealthChecker_Basic(t *testing.T) {
	logger := zap.NewNop()
	scheme := runtime.NewScheme()

	// Add required schemes
	err := corev1.AddToScheme(scheme)
	require.NoError(t, err)
	err = appsv1.AddToScheme(scheme)
	require.NoError(t, err)
	err = roostv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	tests := []struct {
		name           string
		resources      []client.Object
		kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec
		expectedHealth bool
		description    string
	}{
		{
			name: "healthy pods and deployment",
			resources: []client.Object{
				createTestPod("test-pod-1", "test-app", "default", corev1.PodRunning, true),
				createTestPod("test-pod-2", "test-app", "default", corev1.PodRunning, true),
				createTestDeployment("test-deployment", "test-app", "default", 2, 2, 2, 2),
				createTestService("test-service", "test-app", "default"),
			},
			kubernetesSpec: roostv1alpha1.KubernetesHealthCheckSpec{
				CheckPods:        true,
				CheckDeployments: true,
				CheckServices:    true,
			},
			expectedHealth: true,
			description:    "All resources healthy",
		},
		{
			name: "unhealthy pods - not ready",
			resources: []client.Object{
				createTestPod("test-pod-1", "test-app", "default", corev1.PodRunning, false),
				createTestPod("test-pod-2", "test-app", "default", corev1.PodRunning, true),
				createTestDeployment("test-deployment", "test-app", "default", 2, 1, 1, 1),
			},
			kubernetesSpec: roostv1alpha1.KubernetesHealthCheckSpec{
				CheckPods:        true,
				CheckDeployments: true,
			},
			expectedHealth: false,
			description:    "One pod not ready",
		},
		{
			name: "healthy with required ratio",
			resources: []client.Object{
				createTestPod("test-pod-1", "test-app", "default", corev1.PodRunning, true),
				createTestPod("test-pod-2", "test-app", "default", corev1.PodRunning, false),
				createTestPod("test-pod-3", "test-app", "default", corev1.PodRunning, true),
			},
			kubernetesSpec: roostv1alpha1.KubernetesHealthCheckSpec{
				CheckPods:          true,
				RequiredReadyRatio: &[]float64{0.6}[0], // 60% ready is OK
			},
			expectedHealth: true,
			description:    "2/3 pods ready meets 60% threshold",
		},
		{
			name: "unhealthy with required ratio",
			resources: []client.Object{
				createTestPod("test-pod-1", "test-app", "default", corev1.PodRunning, true),
				createTestPod("test-pod-2", "test-app", "default", corev1.PodRunning, false),
				createTestPod("test-pod-3", "test-app", "default", corev1.PodRunning, false),
			},
			kubernetesSpec: roostv1alpha1.KubernetesHealthCheckSpec{
				CheckPods:          true,
				RequiredReadyRatio: &[]float64{0.8}[0], // 80% ready required
			},
			expectedHealth: false,
			description:    "1/3 pods ready fails 80% threshold",
		},
		{
			name:      "no resources found - healthy",
			resources: []client.Object{
				// Empty - no matching resources
			},
			kubernetesSpec: roostv1alpha1.KubernetesHealthCheckSpec{
				CheckPods:        true,
				CheckDeployments: true,
				CheckServices:    true,
			},
			expectedHealth: true,
			description:    "No resources found is not an error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with test resources
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.resources...).
				Build()

			// Create checker
			checker := kubernetes.NewKubernetesChecker(fakeClient, logger)

			// Create test roost
			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Name: "test-chart",
					},
				},
			}

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name: "test-check",
				Type: "kubernetes",
			}

			// Execute health check
			ctx := context.Background()
			result, err := checker.CheckHealth(ctx, roost, checkSpec, tt.kubernetesSpec)

			// Verify results
			require.NoError(t, err, "Health check should not return error")
			assert.Equal(t, tt.expectedHealth, result.Healthy, tt.description)
			assert.NotEmpty(t, result.Message, "Health result should have a message")
			assert.Greater(t, result.ExecutionTime, time.Duration(0), "Execution time should be recorded")
		})
	}
}

func TestKubernetesHealthChecker_PodRestarts(t *testing.T) {
	logger := zap.NewNop()
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	require.NoError(t, err)
	err = roostv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	tests := []struct {
		name           string
		pods           []client.Object
		maxRestarts    int32
		expectedHealth bool
		description    string
	}{
		{
			name: "low restart count - healthy",
			pods: []client.Object{
				createTestPodWithRestarts("test-pod-1", "test-app", "default", 2),
				createTestPodWithRestarts("test-pod-2", "test-app", "default", 1),
			},
			maxRestarts:    5,
			expectedHealth: true,
			description:    "Restart counts below threshold",
		},
		{
			name: "high restart count - unhealthy",
			pods: []client.Object{
				createTestPodWithRestarts("test-pod-1", "test-app", "default", 2),
				createTestPodWithRestarts("test-pod-2", "test-app", "default", 8),
			},
			maxRestarts:    5,
			expectedHealth: false,
			description:    "One pod exceeds restart threshold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.pods...).
				Build()

			checker := kubernetes.NewKubernetesChecker(fakeClient, logger)

			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-app",
					Namespace: "default",
				},
			}

			kubernetesSpec := roostv1alpha1.KubernetesHealthCheckSpec{
				CheckPods:        true,
				CheckPodRestarts: true,
				MaxPodRestarts:   &tt.maxRestarts,
			}

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name: "test-restart-check",
				Type: "kubernetes",
			}

			ctx := context.Background()
			result, err := checker.CheckHealth(ctx, roost, checkSpec, kubernetesSpec)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedHealth, result.Healthy, tt.description)
		})
	}
}

func TestKubernetesHealthChecker_Dependencies(t *testing.T) {
	logger := zap.NewNop()
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	require.NoError(t, err)
	err = roostv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	// Test with ConfigMap, Secret, and PVC
	resources := []client.Object{
		createTestConfigMap("test-config", "test-app", "default"),
		createTestSecret("test-secret", "test-app", "default"),
		createTestPVC("test-pvc", "test-app", "default", corev1.ClaimBound),
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(resources...).
		Build()

	checker := kubernetes.NewKubernetesChecker(fakeClient, logger)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	kubernetesSpec := roostv1alpha1.KubernetesHealthCheckSpec{
		CheckDependencies: true,
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name: "test-dependency-check",
		Type: "kubernetes",
	}

	ctx := context.Background()
	result, err := checker.CheckHealth(ctx, roost, checkSpec, kubernetesSpec)

	require.NoError(t, err)
	assert.True(t, result.Healthy, "Dependencies should be healthy")
	assert.Contains(t, result.Details, "total_dependencies")
}

func TestKubernetesHealthChecker_Cache(t *testing.T) {
	logger := zap.NewNop()
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	require.NoError(t, err)
	err = roostv1alpha1.AddToScheme(scheme)
	require.NoError(t, err)

	pod := createTestPod("test-pod", "test-app", "default", corev1.PodRunning, true)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod).
		Build()

	checker := kubernetes.NewKubernetesChecker(fakeClient, logger)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-app",
			Namespace: "default",
		},
	}

	kubernetesSpec := roostv1alpha1.KubernetesHealthCheckSpec{
		CheckPods:     true,
		EnableCaching: &[]bool{true}[0],
		CacheTTL: &metav1.Duration{
			Duration: 1 * time.Minute,
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name: "test-cache-check",
		Type: "kubernetes",
	}

	ctx := context.Background()

	// First call - should not be from cache
	result1, err := checker.CheckHealth(ctx, roost, checkSpec, kubernetesSpec)
	require.NoError(t, err)
	assert.True(t, result1.Healthy)
	assert.False(t, result1.FromCache, "First call should not be from cache")

	// Second call - should be from cache
	result2, err := checker.CheckHealth(ctx, roost, checkSpec, kubernetesSpec)
	require.NoError(t, err)
	assert.True(t, result2.Healthy)
	assert.True(t, result2.FromCache, "Second call should be from cache")
}

// Helper functions to create test resources

func createTestPod(name, appLabel, namespace string, phase corev1.PodPhase, ready bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance": appLabel,
				"app.kubernetes.io/name":     "test-chart",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx:latest",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: phase,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "test-container",
					Ready:        ready,
					RestartCount: 0,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{},
					},
				},
			},
		},
	}

	// Set pod ready condition
	condition := corev1.PodCondition{
		Type:   corev1.PodReady,
		Status: corev1.ConditionFalse,
	}
	if ready {
		condition.Status = corev1.ConditionTrue
	}
	pod.Status.Conditions = []corev1.PodCondition{condition}

	return pod
}

func createTestPodWithRestarts(name, appLabel, namespace string, restarts int32) *corev1.Pod {
	pod := createTestPod(name, appLabel, namespace, corev1.PodRunning, true)
	pod.Status.ContainerStatuses[0].RestartCount = restarts
	return pod
}

func createTestDeployment(name, appLabel, namespace string, replicas, readyReplicas, availableReplicas, updatedReplicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance": appLabel,
				"app.kubernetes.io/name":     "test-chart",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appLabel,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": appLabel,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:            replicas,
			ReadyReplicas:       readyReplicas,
			AvailableReplicas:   availableReplicas,
			UpdatedReplicas:     updatedReplicas,
			UnavailableReplicas: replicas - availableReplicas,
		},
	}
}

func createTestService(name, appLabel, namespace string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance": appLabel,
				"app.kubernetes.io/name":     "test-chart",
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": appLabel,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       80,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

func createTestConfigMap(name, appLabel, namespace string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance": appLabel,
				"app.kubernetes.io/name":     "test-chart",
			},
		},
		Data: map[string]string{
			"config.yaml": "test: value",
		},
	}
}

func createTestSecret(name, appLabel, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance": appLabel,
				"app.kubernetes.io/name":     "test-chart",
			},
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}
}

func createTestPVC(name, appLabel, namespace string, phase corev1.PersistentVolumeClaimPhase) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/instance": appLabel,
				"app.kubernetes.io/name":     "test-chart",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase: phase,
		},
	}
}
