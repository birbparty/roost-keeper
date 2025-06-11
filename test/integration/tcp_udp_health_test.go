package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/health"
)

func TestTCPHealthChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	checker := health.NewChecker(logger)
	ctx := context.Background()

	tests := []struct {
		name        string
		roost       *roostv1alpha1.ManagedRoost
		expectPass  bool
		description string
	}{
		{
			name: "tcp-basic-connectivity",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-basic-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "nginx",
						Version: "18.1.6",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "google-tcp-test",
							Type: "tcp",
							TCP: &roostv1alpha1.TCPHealthCheckSpec{
								Host: "google.com",
								Port: 80,
							},
							Timeout: metav1.Duration{Duration: 10 * time.Second},
						},
					},
				},
			},
			expectPass:  true,
			description: "Basic TCP connectivity to Google should pass",
		},
		{
			name: "tcp-with-protocol-validation",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-protocol-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "nginx",
						Version: "18.1.6",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "google-http-over-tcp",
							Type: "tcp",
							TCP: &roostv1alpha1.TCPHealthCheckSpec{
								Host:             "google.com",
								Port:             80,
								SendData:         "GET / HTTP/1.1\r\nHost: google.com\r\n\r\n",
								ExpectedResponse: "HTTP/1.1",
							},
							Timeout: metav1.Duration{Duration: 15 * time.Second},
						},
					},
				},
			},
			expectPass:  true,
			description: "TCP with HTTP protocol validation should pass",
		},
		{
			name: "tcp-with-connection-pooling",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-pooling-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "nginx",
						Version: "18.1.6",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "google-tcp-pooled",
							Type: "tcp",
							TCP: &roostv1alpha1.TCPHealthCheckSpec{
								Host:          "google.com",
								Port:          80,
								EnablePooling: true,
							},
							Timeout: metav1.Duration{Duration: 10 * time.Second},
						},
					},
				},
			},
			expectPass:  true,
			description: "TCP with connection pooling should pass",
		},
		{
			name: "tcp-invalid-host",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tcp-invalid-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "nginx",
						Version: "18.1.6",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "invalid-host-test",
							Type: "tcp",
							TCP: &roostv1alpha1.TCPHealthCheckSpec{
								Host: "nonexistent-host-12345.invalid",
								Port: 80,
							},
							Timeout: metav1.Duration{Duration: 5 * time.Second},
						},
					},
				},
			},
			expectPass:  false,
			description: "TCP to invalid host should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			healthy, err := checker.CheckHealth(ctx, tt.roost)

			if tt.expectPass {
				assert.NoError(t, err, "Health check should not return error")
				assert.True(t, healthy, "Health check should pass")
			} else {
				// For failing tests, we expect either an error or unhealthy status
				if err == nil {
					assert.False(t, healthy, "Health check should fail")
				}
				// If there's an error, that's also acceptable for failing tests
			}

			// Test detailed status
			statuses, err := checker.GetHealthStatus(ctx, tt.roost)
			require.NoError(t, err, "Getting health status should not error")
			require.Len(t, statuses, 1, "Should have one health check status")

			status := statuses[0]
			assert.Equal(t, tt.roost.Spec.HealthChecks[0].Name, status.Name)
			assert.NotNil(t, status.LastCheck)

			if tt.expectPass {
				assert.Equal(t, "healthy", status.Status)
				assert.Equal(t, int32(0), status.FailureCount)
			} else {
				assert.Equal(t, "unhealthy", status.Status)
			}
		})
	}
}

func TestUDPHealthChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	checker := health.NewChecker(logger)
	ctx := context.Background()

	tests := []struct {
		name        string
		roost       *roostv1alpha1.ManagedRoost
		expectPass  bool
		description string
	}{
		{
			name: "udp-dns-query",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udp-dns-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "coredns",
						Version: "1.31.0",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "google-dns-udp",
							Type: "udp",
							UDP: &roostv1alpha1.UDPHealthCheckSpec{
								Host:             "8.8.8.8",
								Port:             53,
								SendData:         "\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x06google\x03com\x00\x00\x01\x00\x01",
								ExpectedResponse: "\x12\x34", // Query ID should match
								ReadTimeout:      &metav1.Duration{Duration: 3 * time.Second},
								Retries:          3,
							},
							Timeout: metav1.Duration{Duration: 10 * time.Second},
						},
					},
				},
			},
			expectPass:  true,
			description: "UDP DNS query to Google should pass",
		},
		{
			name: "udp-basic-send",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udp-basic-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "coredns",
						Version: "1.31.0",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "udp-send-only",
							Type: "udp",
							UDP: &roostv1alpha1.UDPHealthCheckSpec{
								Host:     "8.8.8.8",
								Port:     53,
								SendData: "test",
								// No ExpectedResponse - just test sending
								Retries: 2,
							},
							Timeout: metav1.Duration{Duration: 5 * time.Second},
						},
					},
				},
			},
			expectPass:  true,
			description: "UDP send-only test should pass",
		},
		{
			name: "udp-with-retries",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udp-retry-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "coredns",
						Version: "1.31.0",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "udp-with-retries",
							Type: "udp",
							UDP: &roostv1alpha1.UDPHealthCheckSpec{
								Host:             "1.1.1.1", // Cloudflare DNS
								Port:             53,
								SendData:         "\xab\xcd\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07example\x03com\x00\x00\x01\x00\x01",
								ExpectedResponse: "\xab\xcd",
								ReadTimeout:      &metav1.Duration{Duration: 2 * time.Second},
								Retries:          5,
							},
							Timeout: metav1.Duration{Duration: 15 * time.Second},
						},
					},
				},
			},
			expectPass:  true,
			description: "UDP with multiple retries should pass",
		},
		{
			name: "udp-timeout-with-expected-response",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "udp-timeout-test",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL:  "https://charts.bitnami.com/bitnami",
							Type: "http",
						},
						Name:    "coredns",
						Version: "1.31.0",
					},
					HealthChecks: []roostv1alpha1.HealthCheckSpec{
						{
							Name: "udp-timeout-test",
							Type: "udp",
							UDP: &roostv1alpha1.UDPHealthCheckSpec{
								Host:             "192.0.2.1", // RFC5737 test address - should not respond
								Port:             53,
								SendData:         "test",
								ExpectedResponse: "response", // Expect response that won't come
								ReadTimeout:      &metav1.Duration{Duration: 1 * time.Second},
								Retries:          2,
							},
							Timeout: metav1.Duration{Duration: 5 * time.Second},
						},
					},
				},
			},
			expectPass:  false,
			description: "UDP expecting response from non-responsive host should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			healthy, err := checker.CheckHealth(ctx, tt.roost)

			if tt.expectPass {
				assert.NoError(t, err, "Health check should not return error")
				assert.True(t, healthy, "Health check should pass")
			} else {
				// For failing tests, we expect either an error or unhealthy status
				if err == nil {
					assert.False(t, healthy, "Health check should fail")
				}
			}

			// Test detailed status
			statuses, err := checker.GetHealthStatus(ctx, tt.roost)
			require.NoError(t, err, "Getting health status should not error")
			require.Len(t, statuses, 1, "Should have one health check status")

			status := statuses[0]
			assert.Equal(t, tt.roost.Spec.HealthChecks[0].Name, status.Name)
			assert.NotNil(t, status.LastCheck)

			if tt.expectPass {
				assert.Equal(t, "healthy", status.Status)
				assert.Equal(t, int32(0), status.FailureCount)
			} else {
				assert.Equal(t, "unhealthy", status.Status)
			}
		})
	}
}

func TestInfraControlRegistryHealthChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	checker := health.NewChecker(logger)
	ctx := context.Background()

	// Test against the infra-control registries if available
	t.Run("infra-control-docker-registry-tcp", func(t *testing.T) {
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "infra-docker-registry-test",
				Namespace: "default",
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				Chart: roostv1alpha1.ChartSpec{
					Repository: roostv1alpha1.ChartRepositorySpec{
						URL:  "https://charts.bitnami.com/bitnami",
						Type: "http",
					},
					Name:    "nginx",
					Version: "18.1.6",
				},
				HealthChecks: []roostv1alpha1.HealthCheckSpec{
					{
						Name: "docker-registry-tcp",
						Type: "tcp",
						TCP: &roostv1alpha1.TCPHealthCheckSpec{
							Host:             "10.0.0.106",
							Port:             30000,
							SendData:         "GET /v2/ HTTP/1.1\r\nHost: 10.0.0.106:30000\r\n\r\n",
							ExpectedResponse: "HTTP/1.1 200",
							EnablePooling:    true,
						},
						Timeout: metav1.Duration{Duration: 10 * time.Second},
					},
				},
			},
		}

		healthy, err := checker.CheckHealth(ctx, roost)

		// This test might fail if infra-control is not accessible, so we'll be lenient
		if err != nil {
			t.Logf("Infra-control Docker registry not accessible: %v", err)
			t.Skip("Skipping infra-control tests - registry not accessible")
		} else {
			assert.True(t, healthy, "Docker registry TCP health check should pass")
		}
	})

	t.Run("infra-control-zot-registry-tcp", func(t *testing.T) {
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "infra-zot-registry-test",
				Namespace: "default",
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				Chart: roostv1alpha1.ChartSpec{
					Repository: roostv1alpha1.ChartRepositorySpec{
						URL:  "oci://10.0.0.106:30001/charts",
						Type: "oci",
					},
					Name:    "nginx",
					Version: "18.1.6",
				},
				HealthChecks: []roostv1alpha1.HealthCheckSpec{
					{
						Name: "zot-registry-tcp",
						Type: "tcp",
						TCP: &roostv1alpha1.TCPHealthCheckSpec{
							Host: "10.0.0.106",
							Port: 30001,
						},
						Timeout: metav1.Duration{Duration: 8 * time.Second},
					},
				},
			},
		}

		healthy, err := checker.CheckHealth(ctx, roost)

		if err != nil {
			t.Logf("Infra-control Zot registry not accessible: %v", err)
			t.Skip("Skipping infra-control tests - registry not accessible")
		} else {
			assert.True(t, healthy, "Zot registry TCP health check should pass")
		}
	})
}

func TestMixedTCPUDPHealthChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)
	checker := health.NewChecker(logger)
	ctx := context.Background()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mixed-protocol-test",
			Namespace: "default",
		},
		Spec: roostv1alpha1.ManagedRoostSpec{
			Chart: roostv1alpha1.ChartSpec{
				Repository: roostv1alpha1.ChartRepositorySpec{
					URL:  "https://charts.bitnami.com/bitnami",
					Type: "http",
				},
				Name:    "nginx",
				Version: "18.1.6",
			},
			HealthChecks: []roostv1alpha1.HealthCheckSpec{
				{
					Name: "tcp-google",
					Type: "tcp",
					TCP: &roostv1alpha1.TCPHealthCheckSpec{
						Host: "google.com",
						Port: 80,
					},
					Timeout: metav1.Duration{Duration: 10 * time.Second},
				},
				{
					Name: "udp-google-dns",
					Type: "udp",
					UDP: &roostv1alpha1.UDPHealthCheckSpec{
						Host:     "8.8.8.8",
						Port:     53,
						SendData: "test",
						Retries:  2,
					},
					Timeout: metav1.Duration{Duration: 5 * time.Second},
				},
			},
		},
	}

	t.Run("mixed-protocols", func(t *testing.T) {
		healthy, err := checker.CheckHealth(ctx, roost)
		assert.NoError(t, err, "Mixed protocol health checks should not error")
		assert.True(t, healthy, "Mixed protocol health checks should pass")

		statuses, err := checker.GetHealthStatus(ctx, roost)
		require.NoError(t, err, "Getting health status should not error")
		require.Len(t, statuses, 2, "Should have two health check statuses")

		// Verify both checks passed
		for _, status := range statuses {
			assert.Equal(t, "healthy", status.Status)
			assert.Equal(t, int32(0), status.FailureCount)
			assert.NotNil(t, status.LastCheck)
		}
	})
}

func TestHealthCheckPerformance(t *testing.T) {
	logger := zaptest.NewLogger(t)
	checker := health.NewChecker(logger)
	ctx := context.Background()

	// Test TCP connection pooling performance
	t.Run("tcp-pooling-performance", func(t *testing.T) {
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tcp-performance-test",
				Namespace: "default",
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				Chart: roostv1alpha1.ChartSpec{
					Repository: roostv1alpha1.ChartRepositorySpec{
						URL:  "https://charts.bitnami.com/bitnami",
						Type: "http",
					},
					Name:    "nginx",
					Version: "18.1.6",
				},
				HealthChecks: []roostv1alpha1.HealthCheckSpec{
					{
						Name: "tcp-pooled-performance",
						Type: "tcp",
						TCP: &roostv1alpha1.TCPHealthCheckSpec{
							Host:          "google.com",
							Port:          80,
							EnablePooling: true,
						},
						Timeout: metav1.Duration{Duration: 5 * time.Second},
					},
				},
			},
		}

		// Run multiple checks to test pooling
		start := time.Now()
		for i := 0; i < 5; i++ {
			healthy, err := checker.CheckHealth(ctx, roost)
			require.NoError(t, err, "Pooled health check should not error")
			require.True(t, healthy, "Pooled health check should pass")
		}
		duration := time.Since(start)

		t.Logf("5 pooled TCP health checks took: %v", duration)
		assert.Less(t, duration, 30*time.Second, "Pooled checks should be reasonably fast")
	})

	// Test UDP retry performance
	t.Run("udp-retry-performance", func(t *testing.T) {
		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "udp-performance-test",
				Namespace: "default",
			},
			Spec: roostv1alpha1.ManagedRoostSpec{
				Chart: roostv1alpha1.ChartSpec{
					Repository: roostv1alpha1.ChartRepositorySpec{
						URL:  "https://charts.bitnami.com/bitnami",
						Type: "http",
					},
					Name:    "coredns",
					Version: "1.31.0",
				},
				HealthChecks: []roostv1alpha1.HealthCheckSpec{
					{
						Name: "udp-retry-performance",
						Type: "udp",
						UDP: &roostv1alpha1.UDPHealthCheckSpec{
							Host:     "8.8.8.8",
							Port:     53,
							SendData: "test",
							Retries:  3,
						},
						Timeout: metav1.Duration{Duration: 8 * time.Second},
					},
				},
			},
		}

		start := time.Now()
		healthy, err := checker.CheckHealth(ctx, roost)
		duration := time.Since(start)

		require.NoError(t, err, "UDP health check should not error")
		require.True(t, healthy, "UDP health check should pass")

		t.Logf("UDP health check with retries took: %v", duration)
		assert.Less(t, duration, 10*time.Second, "UDP check should complete within timeout")
	})
}
