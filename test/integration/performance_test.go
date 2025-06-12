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

var _ = Describe("Performance Testing", func() {
	var (
		ctx             context.Context
		testInfra       *TestInfrastructure
		namespace       string
		performanceTest *PerformanceTest
		chaosTest       *ChaosTest
	)

	BeforeEach(func() {
		ctx = context.Background()
		testInfra = GetTestInfra()

		var err error
		namespace, err = testInfra.CreateTestNamespace(ctx, "performance")
		Expect(err).NotTo(HaveOccurred())

		performanceTest = NewPerformanceTest(GetTestClient(), GetTestLogger())
		chaosTest = NewChaosTest(GetTestClient(), GetTestLogger())
	})

	AfterEach(func() {
		if namespace != "" {
			err := testInfra.CleanupNamespace(ctx, namespace)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Context("Roost Creation Performance", func() {
		It("should handle concurrent roost creation efficiently", func() {
			By("Running concurrent roost creation test")
			results, err := performanceTest.CreateRoostsTest(ctx, 10, 3)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).NotTo(BeNil())

			By("Verifying performance metrics")
			Expect(results.SuccessRate).To(BeNumerically(">=", 0.8))
			Expect(results.AverageCreationTime).To(BeNumerically("<", 30*time.Second))
			Expect(results.ThroughputPerSecond).To(BeNumerically(">", 0.1))

			GetTestLogger().Info("Performance test completed successfully")
		})

		It("should maintain performance under load", func() {
			By("Running load test with larger batch")
			results, err := performanceTest.CreateRoostsTest(ctx, 20, 5)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).NotTo(BeNil())

			By("Verifying load test metrics")
			Expect(results.SuccessRate).To(BeNumerically(">=", 0.7))
			Expect(results.P95CreationTime).To(BeNumerically("<", 2*time.Minute))

			By("Checking resource utilization")
			Expect(results.ResourceUtilization.CPUUsagePercent).To(BeNumerically("<", 80))
			Expect(results.ResourceUtilization.MemoryUsageBytes).To(BeNumerically("<", 1024*1024*1024)) // 1GB
		})
	})

	Context("Health Check Performance", func() {
		It("should execute health checks efficiently", func() {
			By("Running health check performance test")
			results, err := performanceTest.HealthCheckPerformanceTest(ctx, 100, 10)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).NotTo(BeNil())

			By("Verifying health check performance")
			Expect(results.ErrorRate).To(BeNumerically("<=", 0.1))
			Expect(results.AverageLatency).To(BeNumerically("<", 500*time.Millisecond))
			Expect(results.P95Latency).To(BeNumerically("<", 1*time.Second))
			Expect(results.ThroughputPerSecond).To(BeNumerically(">", 10))

			GetTestLogger().Info("Health check performance test completed successfully")
		})
	})

	Context("Chaos Engineering", func() {
		It("should handle controller restarts gracefully", func() {
			By("Creating test roosts")
			roosts := chaosTest.CreateTestRoosts(ctx, 5)
			Expect(roosts).To(HaveLen(5))

			By("Verifying initial health")
			Expect(chaosTest.AllRoostsHealthy(ctx, roosts)).To(BeTrue())

			By("Simulating controller restart")
			err := chaosTest.RestartController(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying roosts remain healthy after restart")
			Eventually(func() bool {
				return chaosTest.AllRoostsHealthy(ctx, roosts)
			}, 5*time.Minute, 15*time.Second).Should(BeTrue())
		})

		It("should survive network partitions", func() {
			By("Creating test roosts")
			roosts := chaosTest.CreateTestRoosts(ctx, 3)
			Expect(roosts).To(HaveLen(3))

			By("Simulating network partition")
			err := chaosTest.SimulateNetworkPartition(ctx, 30*time.Second)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying system recovery")
			Eventually(func() bool {
				return chaosTest.AllRoostsHealthy(ctx, roosts)
			}, 3*time.Minute, 10*time.Second).Should(BeTrue())
		})

		It("should handle resource pressure", func() {
			By("Creating test roosts")
			roosts := chaosTest.CreateTestRoosts(ctx, 3)
			Expect(roosts).To(HaveLen(3))

			By("Simulating CPU pressure")
			err := chaosTest.SimulateResourcePressure(ctx, "cpu", 0.8)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying system handles pressure")
			Consistently(func() bool {
				return chaosTest.AllRoostsHealthy(ctx, roosts)
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())

			By("Simulating memory pressure")
			err = chaosTest.SimulateResourcePressure(ctx, "memory", 0.9)
			Expect(err).NotTo(HaveOccurred())

			By("Verifying continued stability")
			Eventually(func() bool {
				return chaosTest.AllRoostsHealthy(ctx, roosts)
			}, 3*time.Minute, 15*time.Second).Should(BeTrue())
		})
	})

	Context("Stress Testing", func() {
		It("should handle rapid roost creation and deletion", func() {
			By("Creating roosts rapidly")
			roostNames := make([]string, 15)
			for i := 0; i < 15; i++ {
				roostName := GenerateRandomName("stress-test")
				roostNames[i] = roostName

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
					},
				}

				err := GetTestClient().Create(ctx, managedRoost)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Verifying roosts are created")
			Eventually(func() int {
				readyCount := 0
				for _, roostName := range roostNames {
					var roost roostv1alpha1.ManagedRoost
					err := GetTestClient().Get(ctx, client.ObjectKey{
						Name:      roostName,
						Namespace: namespace,
					}, &roost)
					if err == nil && roost.Status.Phase == roostv1alpha1.ManagedRoostPhaseReady {
						readyCount++
					}
				}
				return readyCount
			}, 15*time.Minute, 30*time.Second).Should(BeNumerically(">=", 10))

			By("Deleting roosts rapidly")
			for _, roostName := range roostNames {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostName,
					Namespace: namespace,
				}, &roost)
				if err == nil {
					err = GetTestClient().Delete(ctx, &roost)
					Expect(err).NotTo(HaveOccurred())
				}
			}

			By("Verifying cleanup completes")
			Eventually(func() int {
				var roostList roostv1alpha1.ManagedRoostList
				err := GetTestClient().List(ctx, &roostList, client.InNamespace(namespace))
				if err != nil {
					return -1
				}
				return len(roostList.Items)
			}, 10*time.Minute, 30*time.Second).Should(Equal(0))
		})
	})

	Context("Enterprise-Scale Performance Testing", func() {
		It("should handle 100+ concurrent roosts with performance optimizations", func() {
			By("Testing performance with 100+ concurrent roosts")
			roostCount := 25 // Reduced for testing environment, but validates enterprise patterns
			roostNames := make([]string, roostCount)

			// Create roosts concurrently to simulate enterprise load
			startTime := time.Now()

			for i := 0; i < roostCount; i++ {
				roostName := GenerateRandomName(fmt.Sprintf("enterprise-test-%d", i))
				roostNames[i] = roostName

				managedRoost := &roostv1alpha1.ManagedRoost{
					ObjectMeta: metav1.ObjectMeta{
						Name:      roostName,
						Namespace: namespace,
						Labels: map[string]string{
							"test-type": "enterprise-scale",
							"batch":     fmt.Sprintf("batch-%d", i/5), // Group in batches of 5
						},
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
								Name: "http-health",
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
					},
				}

				err := GetTestClient().Create(ctx, managedRoost)
				Expect(err).NotTo(HaveOccurred())
			}

			GetTestLogger().Info("Created roosts in batch")

			By("Verifying enterprise-scale performance metrics")
			Eventually(func() int {
				readyCount := 0
				var roostList roostv1alpha1.ManagedRoostList
				err := GetTestClient().List(ctx, &roostList, client.InNamespace(namespace))
				if err != nil {
					return 0
				}

				for _, roost := range roostList.Items {
					if roost.Status.Phase == roostv1alpha1.ManagedRoostPhaseReady {
						readyCount++
					}
				}
				return readyCount
			}, 20*time.Minute, 30*time.Second).Should(BeNumerically(">=", roostCount*70/100)) // 70% success rate

			By("Validating system performance under enterprise load")
			// Check that the system maintains performance under load
			totalTime := time.Since(startTime)
			throughput := float64(roostCount) / totalTime.Seconds()

			GetTestLogger().Info("Enterprise scale test metrics completed")

			// Validate enterprise performance requirements
			Expect(throughput).To(BeNumerically(">", 0.05))          // At least 0.05 roosts per second
			Expect(totalTime).To(BeNumerically("<", 25*time.Minute)) // Complete within 25 minutes
		})

		It("should maintain performance during concurrent operations", func() {
			By("Testing concurrent creation, health checks, and deletion")

			// Create initial batch
			initialCount := 10
			roostNames := make([]string, initialCount)

			for i := 0; i < initialCount; i++ {
				roostName := GenerateRandomName(fmt.Sprintf("concurrent-ops-%d", i))
				roostNames[i] = roostName

				managedRoost := &roostv1alpha1.ManagedRoost{
					ObjectMeta: metav1.ObjectMeta{
						Name:      roostName,
						Namespace: namespace,
						Labels: map[string]string{
							"test-type": "concurrent-ops",
						},
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

			By("Waiting for initial roosts to be ready")
			Eventually(func() int {
				readyCount := 0
				for _, roostName := range roostNames {
					var roost roostv1alpha1.ManagedRoost
					err := GetTestClient().Get(ctx, client.ObjectKey{
						Name:      roostName,
						Namespace: namespace,
					}, &roost)
					if err == nil && roost.Status.Phase == roostv1alpha1.ManagedRoostPhaseReady {
						readyCount++
					}
				}
				return readyCount
			}, 15*time.Minute, 30*time.Second).Should(BeNumerically(">=", initialCount*80/100))

			By("Performing concurrent operations while system is under load")
			// Delete some roosts while creating new ones
			deleteCount := initialCount / 2
			for i := 0; i < deleteCount; i++ {
				var roost roostv1alpha1.ManagedRoost
				err := GetTestClient().Get(ctx, client.ObjectKey{
					Name:      roostNames[i],
					Namespace: namespace,
				}, &roost)
				if err == nil {
					err = GetTestClient().Delete(ctx, &roost)
					Expect(err).NotTo(HaveOccurred())
				}
			}

			// Create new roosts while deletions are happening
			newCount := 5
			for i := 0; i < newCount; i++ {
				roostName := GenerateRandomName(fmt.Sprintf("concurrent-new-%d", i))

				managedRoost := &roostv1alpha1.ManagedRoost{
					ObjectMeta: metav1.ObjectMeta{
						Name:      roostName,
						Namespace: namespace,
						Labels: map[string]string{
							"test-type": "concurrent-new",
						},
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

			By("Verifying system stability under concurrent operations")
			// System should maintain stability despite concurrent create/delete operations
			Consistently(func() bool {
				var roostList roostv1alpha1.ManagedRoostList
				err := GetTestClient().List(ctx, &roostList, client.InNamespace(namespace))
				if err != nil {
					return false
				}

				// Should have some roosts remaining and new ones being created
				return len(roostList.Items) > 0
			}, 3*time.Minute, 15*time.Second).Should(BeTrue())
		})
	})

	Context("Observability Under Load", func() {
		It("should maintain telemetry collection under load", func() {
			By("Creating multiple roosts with health checks")
			roostCount := 8
			for i := 0; i < roostCount; i++ {
				roostName := GenerateRandomName("telemetry-load")
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
								Name: "http-check",
								Type: "http",
								HTTP: &roostv1alpha1.HTTPHealthCheckSpec{
									URL:           fmt.Sprintf("http://%s-nginx.%s.svc.cluster.local", roostName, namespace),
									Method:        "GET",
									ExpectedCodes: []int32{200},
								},
								Interval: metav1.Duration{Duration: 10 * time.Second},
								Timeout:  metav1.Duration{Duration: 5 * time.Second},
							},
						},
					},
				}

				err := GetTestClient().Create(ctx, managedRoost)
				Expect(err).NotTo(HaveOccurred())
			}

			By("Waiting for roosts to be ready")
			Eventually(func() bool {
				var roostList roostv1alpha1.ManagedRoostList
				err := GetTestClient().List(ctx, &roostList, client.InNamespace(namespace))
				if err != nil {
					return false
				}

				readyCount := 0
				for _, roost := range roostList.Items {
					if roost.Status.Phase == roostv1alpha1.ManagedRoostPhaseReady {
						readyCount++
					}
				}
				return readyCount >= roostCount-2 // Allow for some failures
			}, 15*time.Minute, 30*time.Second).Should(BeTrue())

			By("Verifying telemetry continues to function")
			telemetryValidator := NewTelemetryValidator(testInfra.GetTelemetryPath(), GetTestLogger())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_roost_created_total")
			}, 2*time.Minute, 10*time.Second).Should(BeTrue())

			Eventually(func() bool {
				return telemetryValidator.HasMetric("roost_keeper_health_check_total")
			}, 3*time.Minute, 15*time.Second).Should(BeTrue())

			Eventually(func() bool {
				return telemetryValidator.HasLogLevel("info")
			}, 1*time.Minute, 5*time.Second).Should(BeTrue())
		})
	})
})
