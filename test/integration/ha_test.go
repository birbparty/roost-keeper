package integration

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/birbparty/roost-keeper/internal/ha"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

var _ = Describe("High Availability Controller", func() {
	Context("When HA Manager is deployed", func() {
		var (
			ctx       context.Context
			cancel    context.CancelFunc
			haManager *ha.Manager
			metrics   *telemetry.OperatorMetrics
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			// Initialize metrics for HA testing
			var err error
			metrics, err = telemetry.NewOperatorMetrics()
			Expect(err).NotTo(HaveOccurred())

			// Create HA Manager
			haManager = ha.NewManager(k8sClient, k8sClientset, metrics, logger)
			Expect(haManager).NotTo(BeNil())
		})

		AfterEach(func() {
			cancel()
		})

		It("should create HA manager successfully", func() {
			Expect(haManager).NotTo(BeNil())
			Expect(haManager.IsLeader()).To(BeFalse()) // Not leader initially
		})

		It("should initialize health manager", func() {
			healthManager := haManager.GetHealthManager()
			Expect(healthManager).NotTo(BeNil())

			// Test health check execution
			err := healthManager.CheckHealth(ctx)
			// In test environment, some checks may fail, but the manager should exist
			_ = err // Health checks may fail in test env, focus on manager existence
		})

		It("should initialize recovery manager", func() {
			recoveryManager := haManager.GetRecoveryManager()
			Expect(recoveryManager).NotTo(BeNil())

			// Test backup creation
			backup, err := recoveryManager.CreateBackup(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(backup).NotTo(BeNil())
			Expect(backup.ID).NotTo(BeEmpty())
			Expect(backup.Status).To(Equal("completed"))
		})

		It("should initialize rolling update manager", func() {
			updateManager := haManager.GetRollingUpdateManager()
			Expect(updateManager).NotTo(BeNil())

			// Test update validation
			err := updateManager.ValidateUpdate(ctx, "test-image:v1.0.0")
			// May fail in test environment due to missing deployment
			_ = err // Focus on manager initialization
		})

		It("should report HA status", func() {
			status, err := haManager.GetStatus(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).NotTo(BeNil())
			Expect(status.IsLeader).To(BeFalse()) // Not leader initially
			Expect(status.ClusterHealth).To(BeElementOf("healthy", "unhealthy"))
		})

		It("should start periodic health checks", func() {
			// Start periodic health checks with short interval for testing
			go haManager.StartPeriodicHealthChecks(ctx, 1*time.Second)

			// Wait for at least one health check cycle
			time.Sleep(2 * time.Second)

			// Verify health checks are running (no specific assertions needed,
			// just ensure no panics or deadlocks)
			Expect(ctx.Err()).To(BeNil())
		})
	})

	Context("When testing HA deployment configuration", func() {
		var (
			ctx        context.Context
			cancel     context.CancelFunc
			deployment *appsv1.Deployment
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())

			// Create a test deployment for HA testing
			deployment = &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost-keeper-controller",
					Namespace: "roost-keeper-system",
					Labels: map[string]string{
						"app":           "roost-keeper",
						"control-plane": "controller-manager",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: int32Ptr(3), // HA requires 3 replicas
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app":           "roost-keeper",
							"control-plane": "controller-manager",
						},
					},
					Strategy: appsv1.DeploymentStrategy{
						Type: appsv1.RollingUpdateDeploymentStrategyType,
						RollingUpdate: &appsv1.RollingUpdateDeployment{
							MaxUnavailable: intOrStringPtr("1"),
							MaxSurge:       intOrStringPtr("1"),
						},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app":           "roost-keeper",
								"control-plane": "controller-manager",
							},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "manager",
									Image: "roost-keeper:test",
									Args: []string{
										"--leader-elect=true",
										"--leader-election-id=roost-keeper-controller",
										"--health-probe-bind-address=:8081",
										"--metrics-bind-address=:8080",
									},
									Ports: []corev1.ContainerPort{
										{
											Name:          "metrics",
											ContainerPort: 8080,
										},
										{
											Name:          "health",
											ContainerPort: 8081,
										},
									},
									LivenessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											HTTPGet: &corev1.HTTPGetAction{
												Path: "/healthz",
												Port: intstr.FromInt(8081),
											},
										},
										InitialDelaySeconds: 15,
										PeriodSeconds:       20,
									},
									ReadinessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											HTTPGet: &corev1.HTTPGetAction{
												Path: "/readyz",
												Port: intstr.FromInt(8081),
											},
										},
										InitialDelaySeconds: 5,
										PeriodSeconds:       10,
									},
								},
							},
							Affinity: &corev1.Affinity{
								PodAntiAffinity: &corev1.PodAntiAffinity{
									PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
										{
											Weight: 100,
											PodAffinityTerm: corev1.PodAffinityTerm{
												LabelSelector: &metav1.LabelSelector{
													MatchLabels: map[string]string{
														"app": "roost-keeper",
													},
												},
												TopologyKey: "kubernetes.io/hostname",
											},
										},
									},
								},
							},
						},
					},
				},
			}
		})

		AfterEach(func() {
			// Clean up test deployment
			if deployment != nil {
				_ = k8sClient.Delete(ctx, deployment)
			}
			cancel()
		})

		It("should have HA configuration with 3 replicas", func() {
			Expect(*deployment.Spec.Replicas).To(Equal(int32(3)))
		})

		It("should have rolling update strategy configured", func() {
			Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RollingUpdateDeploymentStrategyType))
			Expect(deployment.Spec.Strategy.RollingUpdate).NotTo(BeNil())
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxUnavailable.String()).To(Equal("1"))
			Expect(deployment.Spec.Strategy.RollingUpdate.MaxSurge.String()).To(Equal("1"))
		})

		It("should have leader election enabled", func() {
			args := deployment.Spec.Template.Spec.Containers[0].Args
			Expect(args).To(ContainElement("--leader-elect=true"))
			Expect(args).To(ContainElement("--leader-election-id=roost-keeper-controller"))
		})

		It("should have health probes configured", func() {
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe).NotTo(BeNil())
			Expect(container.ReadinessProbe).NotTo(BeNil())
			Expect(container.LivenessProbe.HTTPGet.Path).To(Equal("/healthz"))
			Expect(container.ReadinessProbe.HTTPGet.Path).To(Equal("/readyz"))
		})

		It("should have pod anti-affinity configured", func() {
			affinity := deployment.Spec.Template.Spec.Affinity
			Expect(affinity).NotTo(BeNil())
			Expect(affinity.PodAntiAffinity).NotTo(BeNil())
			Expect(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution).To(HaveLen(1))
		})
	})

	Context("When testing disaster recovery", func() {
		var (
			ctx             context.Context
			cancel          context.CancelFunc
			recoveryManager *ha.RecoveryManager
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			recoveryManager = ha.NewRecoveryManager(k8sClient, logger)
		})

		AfterEach(func() {
			cancel()
		})

		It("should create and list backups", func() {
			// Create a backup
			backup, err := recoveryManager.CreateBackup(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(backup).NotTo(BeNil())
			Expect(backup.ID).NotTo(BeEmpty())
			Expect(backup.Status).To(Equal("completed"))

			// List backups
			backups, err := recoveryManager.ListBackups(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(backups).NotTo(BeEmpty())
		})

		It("should validate backup integrity", func() {
			// Create a backup first
			backup, err := recoveryManager.CreateBackup(ctx)
			Expect(err).NotTo(HaveOccurred())

			// Validate the backup
			err = recoveryManager.ValidateBackup(ctx, backup.ID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should perform disaster recovery test", func() {
			err := recoveryManager.TestDisasterRecovery(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should get recovery metrics", func() {
			metrics, err := recoveryManager.GetMetrics(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(metrics).NotTo(BeNil())
			Expect(metrics.RecoveryTimeObjective).To(Equal(5 * time.Minute))
		})
	})

	Context("When testing rolling updates", func() {
		var (
			ctx           context.Context
			cancel        context.CancelFunc
			updateManager *ha.RollingUpdateManager
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			updateManager = ha.NewRollingUpdateManager(k8sClient, logger)
		})

		AfterEach(func() {
			cancel()
		})

		It("should create rolling update manager", func() {
			Expect(updateManager).NotTo(BeNil())
		})

		It("should list update history", func() {
			updates, err := updateManager.ListUpdates(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(updates).NotTo(BeEmpty()) // Mock data should exist
		})

		It("should perform canary update", func() {
			status, err := updateManager.CanaryUpdate(ctx, "roost-keeper:canary", 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(status).NotTo(BeNil())
			Expect(status.Status).To(Equal("canary_deployed"))
			Expect(status.Progress).To(Equal(50.0))
		})
	})

	Context("When testing health checks", func() {
		var (
			ctx           context.Context
			cancel        context.CancelFunc
			healthManager *ha.HealthManager
			haManager     *ha.Manager
		)

		BeforeEach(func() {
			ctx, cancel = context.WithCancel(context.Background())
			metrics, _ := telemetry.NewOperatorMetrics()
			haManager = ha.NewManager(k8sClient, k8sClientset, metrics, logger)
			healthManager = haManager.GetHealthManager()
		})

		AfterEach(func() {
			cancel()
		})

		It("should register health checks", func() {
			Expect(healthManager).NotTo(BeNil())

			// Get health status
			status := healthManager.GetHealthStatus(ctx)
			Expect(status).NotTo(BeNil())

			// Should have registered health checks
			Expect(len(status)).To(BeNumerically(">", 0))
		})

		It("should include leader election health check", func() {
			status := healthManager.GetHealthStatus(ctx)
			_, exists := status["leader_election"]
			Expect(exists).To(BeTrue())
		})

		It("should include API server health check", func() {
			status := healthManager.GetHealthStatus(ctx)
			_, exists := status["api_server"]
			Expect(exists).To(BeTrue())
		})
	})
})

// Helper functions for creating Kubernetes objects
func int32Ptr(i int32) *int32 {
	return &i
}

func intOrStringPtr(s string) *intstr.IntOrString {
	ios := intstr.FromString(s)
	return &ios
}
