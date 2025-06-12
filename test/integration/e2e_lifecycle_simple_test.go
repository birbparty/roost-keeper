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

var _ = Describe("Simplified End-to-End Roost Lifecycle", func() {
	var (
		ctx       context.Context
		testInfra *TestInfrastructure
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		testInfra = GetTestInfra()

		var err error
		namespace, err = testInfra.CreateTestNamespace(ctx, "e2e-simple")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if namespace != "" {
			err := testInfra.CleanupNamespace(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Basic Roost Lifecycle", func() {
		It("should successfully create and deploy a basic roost", func() {
			By("Creating a basic ManagedRoost")
			roostName := GenerateRandomName("simple-roost")
			managedRoost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roostName,
					Namespace: namespace,
					Labels: map[string]string{
						"test.roost-keeper.io/type": "simple",
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
							Inline: `replicaCount: 1`,
						},
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "http-check",
							Type: "http",
							HTTP: &roostv1alpha1.HTTPHealthCheckSpec{
								URL:           fmt.Sprintf("http://%s-nginx.%s.svc.cluster.local", roostName, namespace),
								Method:        "GET",
								ExpectedCodes: []int32{200},
							},
							Interval: metav1.Duration{Duration: 30 * time.Second},
							Timeout:  metav1.Duration{Duration: 10 * time.Second},
						},
					},
					TeardownPolicy: &roostv1alpha1.TeardownPolicySpec{
						Triggers: []roostv1alpha1.TeardownTriggerSpec{
							{
								Type:    "timeout",
								Timeout: &metav1.Duration{Duration: 2 * time.Hour},
							},
						},
					},
				},
			}

			err := GetTestClient().Create(ctx, managedRoost)
			Expect(err).NotTo(HaveOccurred())

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
			}, 10*time.Minute, 15*time.Second).Should(Equal("Ready"))

			By("Verifying roost status")
			var finalRoost roostv1alpha1.ManagedRoost
			err = GetTestClient().Get(ctx, client.ObjectKey{
				Name:      roostName,
				Namespace: namespace,
			}, &finalRoost)
			Expect(err).NotTo(HaveOccurred())

			// Verify basic status
			Expect(finalRoost.Status.Phase).To(Equal(roostv1alpha1.ManagedRoostPhaseReady))
			Expect(finalRoost.Status.ObservedGeneration).To(Equal(finalRoost.Generation))

			// Verify helm release status if available
			if finalRoost.Status.HelmRelease != nil {
				Expect(finalRoost.Status.HelmRelease.Name).To(ContainSubstring(roostName))
				Expect(finalRoost.Status.HelmRelease.Status).To(Equal("deployed"))
			}

			By("Verifying telemetry data collection")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_roost_created_total")
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())

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

		It("should handle invalid chart configurations", func() {
			By("Creating a ManagedRoost with invalid chart")
			roostName := GenerateRandomName("invalid-roost")
			managedRoost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roostName,
					Namespace: namespace,
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL: "https://invalid-repo.example.com",
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
			}, 5*time.Minute, 15*time.Second).Should(Equal("Failed"))

			By("Verifying failure telemetry")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_roost_failed_total")
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())
		})

		It("should validate health checks", func() {
			if !IsUsingK3s() {
				Skip("Skipping health check validation in envtest mode")
			}

			By("Creating a roost with health checks")
			roostName := GenerateRandomName("health-roost")
			managedRoost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      roostName,
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
							Name: "tcp-check",
							Type: "tcp",
							TCP: &roostv1alpha1.TCPHealthCheckSpec{
								Host: fmt.Sprintf("%s-nginx.%s.svc.cluster.local", roostName, namespace),
								Port: 80,
							},
							Interval: metav1.Duration{Duration: 15 * time.Second},
							Timeout:  metav1.Duration{Duration: 5 * time.Second},
						},
					},
				},
			}

			err := GetTestClient().Create(ctx, managedRoost)
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for roost to be ready")
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
			}, 8*time.Minute, 15*time.Second).Should(Equal("Ready"))

			By("Verifying health checks are reported")
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
			}, 3*time.Minute, 15*time.Second).Should(BeTrue())

			By("Verifying health check telemetry")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_health_check_total")
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())
		})
	})

	Context("Performance Testing", func() {
		It("should create multiple roosts efficiently", func() {
			By("Creating multiple roosts concurrently")
			roostCount := 5
			roostNames := make([]string, roostCount)

			for i := 0; i < roostCount; i++ {
				roostNames[i] = GenerateRandomName(fmt.Sprintf("perf-roost-%d", i))
				managedRoost := &roostv1alpha1.ManagedRoost{
					ObjectMeta: metav1.ObjectMeta{
						Name:      roostNames[i],
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

				err := GetTestClient().Create(ctx, managedRoost)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Waiting for all roosts to be ready")
			for _, roostName := range roostNames {
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
				}, 10*time.Minute, 20*time.Second).Should(Equal("Ready"))
			}

			By("Verifying performance telemetry")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				value, err := telemetryValidator.GetMetricValue("roost_keeper_roost_created_total")
				return err == nil && value >= float64(roostCount)
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())
		})
	})
})
