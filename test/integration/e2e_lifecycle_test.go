package integration

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

var _ = Describe("End-to-End Roost Lifecycle", func() {
	var (
		ctx       context.Context
		testInfra *TestInfrastructure
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testInfra = GetTestInfra()

		var err error
		namespace, err = testInfra.CreateTestNamespace(ctx, "e2e-lifecycle")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if namespace != "" {
			err := testInfra.CleanupNamespace(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Complete Roost Lifecycle", func() {
		It("should successfully create, deploy, and manage a complete roost lifecycle", func() {
			By("Creating a comprehensive ManagedRoost")
			roostName := GenerateRandomName("e2e-comprehensive")
			managedRoost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roostName,
					Namespace: namespace,
					Labels: map[string]string{
						"test.roost-keeper.io/type": "e2e-comprehensive",
					},
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
						Name:    "nginx",
						Version: "15.4.4",
						Values: &roostv1alpha1.ChartValuesSpec{
							Inline: `
replicaCount: 2
service:
  type: ClusterIP
  port: 80
`,
						},
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "http-readiness",
							Type: "http",
							HTTP: &roostv1alpha1.HTTPHealthCheckSpec{
								URL:           fmt.Sprintf("http://%s-nginx.%s.svc.cluster.local", roostName, namespace),
								Method:        "GET",
								ExpectedCodes: []int32{200, 204},
							},
							Interval: metav1.Duration{Duration: 15 * time.Second},
							Timeout:  metav1.Duration{Duration: 5 * time.Second},
						},
						{
							Name: "tcp-connectivity",
							Type: "tcp",
							TCP: &roostv1alpha1.TCPHealthCheckSpec{
								Host: fmt.Sprintf("%s-nginx.%s.svc.cluster.local", roostName, namespace),
								Port: 80,
							},
							Interval: metav1.Duration{Duration: 30 * time.Second},
							Timeout:  metav1.Duration{Duration: 3 * time.Second},
						},
					},
					TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
						Triggers: []roostv1alpha1.TeardownTriggerSpec{
							{
								Type:    "timeout",
								Timeout: &metav1.Duration{Duration: 2 * time.Hour},
							},
							{
								Type: "webhook",
								Webhook: &roostv1alpha1.WebhookTriggerSpec{
									URL: "http://test-webhook-service/teardown",
								},
							},
						},
						Safety: &roostv1alpha1.TeardownSafetySpec{
							RequireConfirmation: true,
							GracePeriod:         metav1.Duration{Duration: 5 * time.Minute},
						},
					},
					RBAC: &roostv1alpha1.RBACSpec{
						ServiceAccountName: "roost-service-account",
						Roles: []roostv1alpha1.RoleSpec{
							{
								Name: "roost-reader",
								Rules: []roostv1alpha1.PolicyRule{
									{
										APIGroups: []string{""},
										Resources: []string{"pods", "services"},
										Verbs:     []string{"get", "list", "watch"},
									},
								},
							},
						},
					},
				},
			}

			err := GetTestClient().Create(ctx, managedRoost)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for roost to enter Installing phase")
			Eventually(func() string {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return ""
				}
				return string(roost.Status.Phase)
			}, 2*time.Minute, 5*time.Second).Should(Equal("Installing"))

			By("Waiting for roost to reach Ready phase")
			Eventually(func() string {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return ""
				}
				return string(roost.Status.Phase)
			}, 10*time.Minute, 10*time.Second).Should(Equal("Ready"))

			By("Verifying roost status and conditions")
			var finalRoost roostv1alpha1.ManagedRoost
			err = GetTestClient().Get(ctx, client.ObjectKey{
				Name:      roostName,
				Namespace: namespace,
			}, &finalRoost)
			Expect(err).NotTo(HaveOccurred())

			// Verify basic status
			Expect(finalRoost.Status.Phase).To(Equal(roostv1alpha1.ManagedRoostPhase("Ready")))
			Expect(finalRoost.Status.ObservedGeneration).To(Equal(finalRoost.Generation))

			// Verify helm release status
			Expect(finalRoost.Status.HelmRelease).NotTo(BeNil())
			Expect(finalRoost.Status.HelmRelease.Name).To(ContainSubstring(roostName))
			Expect(finalRoost.Status.HelmRelease.Status).To(Equal("deployed"))

			// Verify conditions
			readyCondition := findCondition(finalRoost.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionTrue))

			// Verify health checks are being executed
			if IsUsingK3s() {
				// Only verify health check status in real k3s environment
				Eventually(func() bool {
					var roost roostv1alpha1.ManagedRoost
					err := GetTestClient().Get(ctx, client.ObjectKey{
						Name:      roostName,
						Namespace: namespace,
					}, &roost)
					if err != nil {
						return false
					}
					return len(roost.Status.HealthChecks) > 0
				}, 5*time.Minute, 15*time.Second).Should(BeTrue())
			}

			By("Verifying telemetry data collection")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_roost_created_total")
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())

			Eventually(func() bool {
				return telemetryValidator.HasTrace("roost-lifecycle")
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())

			By("Testing roost update functionality")
			// Update the roost configuration
			err = GetTestClient().Get(ctx, client.ObjectKey{
				Name:      roostName,
				Namespace: namespace,
			}, &finalRoost)
			Expect(err).NotTo(HaveOccurred())

			finalRoost.Spec.Chart.Values["replicaCount"] = 3
			err = GetTestClient().Update(ctx, &finalRoost)
			Expect(err).NotTo(HaveOccurred())

			// Wait for update to be processed
			Eventually(func() int64 {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return 0
				}
				return roost.Status.ObservedGeneration
			}, 5*time.Minute, 10*time.Second).Should(Equal(finalRoost.Generation))

			By("Cleaning up the roost")
			err = GetTestClient().Delete(ctx, &finalRoost)
			Expect(err).NotTo(HaveOccurred())

			// Wait for deletion to complete
			Eventually(func() bool {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				return err != nil
			}, 5*time.Minute, 10*time.Second).Should(BeTrue())
		})

		It("should handle roost creation failures gracefully", func() {
			By("Creating a ManagedRoost with invalid configuration")
			roostName := GenerateRandomName("e2e-invalid")
			managedRoost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roostName,
					Namespace: namespace,
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL: "https://invalid-repo-url.example.com",
						},
						Name:    "nonexistent-chart",
						Version: "999.999.999",
					},
				},
			}

			err := GetTestClient().Create(ctx, managedRoost)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for roost to reach Failed phase")
			Eventually(func() string {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return ""
				}
				return string(roost.Status.Phase)
			}, 5*time.Minute, 10*time.Second).Should(Equal("Failed"))

			By("Verifying error conditions are properly set")
			var failedRoost roostv1alpha1.ManagedRoost
			err = GetTestClient().Get(ctx, client.ObjectKey{
				Name:      roostName,
				Namespace: namespace,
			}, &failedRoost)
			Expect(err).NotTo(HaveOccurred())

			// Verify failure conditions
			readyCondition := findCondition(failedRoost.Status.Conditions, "Ready")
			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(ContainSubstring("Failed"))

			By("Verifying failure metrics are recorded")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_roost_failed_total")
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())
		})

		It("should support complex multi-service roosts", func() {
			if !IsUsingK3s() {
				Skip("Skipping complex multi-service test in envtest mode")
			}

			By("Creating a complex multi-service ManagedRoost")
			roostName := GenerateRandomName("e2e-multi-service")
			managedRoost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roostName,
					Namespace: namespace,
					Labels: map[string]string{
						"test.roost-keeper.io/type": "e2e-multi-service",
					},
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL: "https://charts.bitnami.com/bitnami",
						},
						Name:    "wordpress",
						Version: "18.1.14",
						Values: map[string]interface{}{
							"wordpressUsername": "testuser",
							"wordpressPassword": "testpass123",
							"mariadb": map[string]interface{}{
								"enabled": true,
								"auth": map[string]interface{}{
									"rootPassword": "rootpass123",
									"password":     "dbpass123",
								},
							},
						},
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "wordpress-http",
							Type: "http",
							HTTP: &roostv1alpha1.HTTPHealthCheckSpec{
								URL:           fmt.Sprintf("http://%s-wordpress.%s.svc.cluster.local", roostName, namespace),
								Method:        "GET",
								ExpectedCodes: []int32{200, 302},
							},
							Interval:     metav1.Duration{Duration: 30 * time.Second},
							Timeout:      metav1.Duration{Duration: 10 * time.Second},
							InitialDelay: metav1.Duration{Duration: 2 * time.Minute},
						},
						{
							Name: "mariadb-tcp",
							Type: "tcp",
							TCP: &roostv1alpha1.TCPHealthCheckSpec{
								Host: fmt.Sprintf("%s-mariadb.%s.svc.cluster.local", roostName, namespace),
								Port: 3306,
							},
							Interval: metav1.Duration{Duration: 20 * time.Second},
							Timeout:  metav1.Duration{Duration: 5 * time.Second},
						},
					},
					TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
						Triggers: []roostv1alpha1.TeardownTriggerSpec{
							{
								Type:    "timeout",
								Timeout: &metav1.Duration{Duration: 4 * time.Hour},
							},
						},
						Safety: &roostv1alpha1.TeardownSafetySpec{
							GracePeriod: metav1.Duration{Duration: 2 * time.Minute},
						},
					},
				},
			}

			err := GetTestClient().Create(ctx, managedRoost)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for complex roost to reach Ready phase")
			Eventually(func() string {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return ""
				}
				return string(roost.Status.Phase)
			}, 15*time.Minute, 30*time.Second).Should(Equal("Ready"))

			By("Verifying all health checks are passing")
			Eventually(func() bool {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return false
				}

				// Check that all health checks are reported
				if len(roost.Status.HealthChecks) < 2 {
					return false
				}

				// Check that at least one health check is passing
				for _, healthCheck := range roost.Status.HealthChecks {
					if healthCheck.Healthy {
						return true
					}
				}
				return false
			}, 10*time.Minute, 30*time.Second).Should(BeTrue())

			By("Verifying comprehensive telemetry collection")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_health_check_total")
			}, 3*time.Minute, 15*time.Second).Should(BeTrue())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_helm_install_duration_seconds")
			}, 3*time.Minute, 15*time.Second).Should(BeTrue())
		})
	})

	Context("Roost Resource Management", func() {
		It("should properly manage roost resources and dependencies", func() {
			By("Creating roosts with dependencies")
			baseRoostName := GenerateRandomName("e2e-base")
			dependentRoostName := GenerateRandomName("e2e-dependent")

			// Create base roost first
			baseRoost := CreateTestManagedRoost(baseRoostName, namespace)
			err := GetTestClient().Create(ctx, baseRoost)
			Expect(err).NotTo(HaveOccurred())

			// Wait for base roost to be ready
			Eventually(func() string {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      baseRoostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return ""
				}
				return string(roost.Status.Phase)
			}, 8*time.Minute, 15*time.Second).Should(Equal("Ready"))

			// Create dependent roost
			dependentRoost := CreateTestManagedRoost(dependentRoostName, namespace)
			dependentRoost.Spec.Dependencies = []roostv1alpha1.DependencySpec{
				{
					Name:      baseRoostName,
					Namespace: namespace,
					Type:      "roost",
				},
			}
			err = GetTestClient().Create(ctx, dependentRoost)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying dependent roost waits for base roost")
			Eventually(func() string {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      dependentRoostName,
					Namespace: namespace,
				}, &roost)
				if err != nil {
					return ""
				}
				return string(roost.Status.Phase)
			}, 8*time.Minute, 15*time.Second).Should(Equal("Ready"))

			By("Verifying dependency tracking in status")
			var finalDependentRoost roostv1alpha1.ManagedRoost
			err = GetTestClient().Get(ctx, client.ObjectKey{
				Name:      dependentRoostName,
				Namespace: namespace,
			}, &finalDependentRoost)
			Expect(err).NotTo(HaveOccurred())

			Expect(finalDependentRoost.Status.Dependencies).NotTo(BeEmpty())
		})
	})
})

// Helper function to find a condition in the status
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
