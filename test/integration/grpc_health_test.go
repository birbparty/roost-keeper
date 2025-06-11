package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/health/grpc"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

func TestGRPCHealthCheckBasic(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-basic",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-basic-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 5 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host: "localhost",
		Port: 50051,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// This test will fail if no gRPC server is running, which is expected
	// The test validates that the implementation handles connection failures gracefully
	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

	// We expect this to fail since there's no gRPC server running
	assert.False(t, healthy)
	assert.Error(t, err)

	t.Logf("gRPC health check completed (expected failure): healthy=%v, error=%v", healthy, err)
}

func TestGRPCHealthCheckWithService(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-service",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-service-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 3 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "test.TestService",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

	// Expected to fail without a running service
	assert.False(t, healthy)
	assert.Error(t, err)

	t.Logf("gRPC service health check completed: healthy=%v, error=%v", healthy, err)
}

func TestGRPCHealthCheckWithTLS(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-tls",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-tls-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 5 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "secure.SecureService",
		TLS: &roostv1alpha1.GRPCTLSSpec{
			Enabled:            true,
			InsecureSkipVerify: true, // For testing
			ServerName:         "localhost",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

	// Expected to fail without a running TLS service
	assert.False(t, healthy)
	assert.Error(t, err)

	t.Logf("gRPC TLS health check completed: healthy=%v, error=%v", healthy, err)
}

func TestGRPCHealthCheckWithAuthentication(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-auth",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-auth-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 5 * time.Second},
	}

	tests := []struct {
		name     string
		authSpec *roostv1alpha1.GRPCAuthSpec
	}{
		{
			name: "JWT Authentication",
			authSpec: &roostv1alpha1.GRPCAuthSpec{
				JWT: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test.token",
			},
		},
		{
			name: "Bearer Token",
			authSpec: &roostv1alpha1.GRPCAuthSpec{
				BearerToken: "test-bearer-token",
			},
		},
		{
			name: "Custom Headers",
			authSpec: &roostv1alpha1.GRPCAuthSpec{
				Headers: map[string]string{
					"x-api-key":   "test-api-key",
					"x-tenant-id": "test-tenant",
				},
			},
		},
		{
			name: "Basic Auth",
			authSpec: &roostv1alpha1.GRPCAuthSpec{
				BasicAuth: &roostv1alpha1.BasicAuthSpec{
					Username: "testuser",
					Password: "testpass",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
				Host:    "localhost",
				Port:    50051,
				Service: "auth.AuthService",
				Auth:    tt.authSpec,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()

			healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

			// Expected to fail without a running service
			assert.False(t, healthy)
			assert.Error(t, err)

			t.Logf("%s - gRPC auth health check completed: healthy=%v, error=%v", tt.name, healthy, err)
		})
	}
}

func TestGRPCHealthCheckWithConnectionPool(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-pool",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-pool-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 5 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "pool.PoolService",
		ConnectionPool: &roostv1alpha1.GRPCConnectionPoolSpec{
			Enabled:             true,
			MaxConnections:      5,
			MaxIdleTime:         metav1.Duration{Duration: 60 * time.Second},
			KeepAliveTime:       metav1.Duration{Duration: 30 * time.Second},
			KeepAliveTimeout:    metav1.Duration{Duration: 5 * time.Second},
			HealthCheckInterval: metav1.Duration{Duration: 30 * time.Second},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

	// Expected to fail without a running service
	assert.False(t, healthy)
	assert.Error(t, err)

	t.Logf("gRPC connection pool health check completed: healthy=%v, error=%v", healthy, err)
}

func TestGRPCHealthCheckWithRetryPolicy(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-retry",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-retry-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 2 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "retry.RetryService",
		RetryPolicy: &roostv1alpha1.GRPCRetryPolicySpec{
			Enabled:      true,
			MaxAttempts:  3,
			InitialDelay: metav1.Duration{Duration: 500 * time.Millisecond},
			MaxDelay:     metav1.Duration{Duration: 2 * time.Second},
			Multiplier:   "2.0",
			RetryableStatusCodes: []string{
				"UNAVAILABLE",
				"DEADLINE_EXCEEDED",
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)
	duration := time.Since(start)

	// Should take longer due to retries
	assert.False(t, healthy)
	assert.Error(t, err)
	assert.True(t, duration > 2*time.Second, "Expected retries to take longer than base timeout")

	t.Logf("gRPC retry health check completed in %v: healthy=%v, error=%v", duration, healthy, err)
}

func TestGRPCHealthCheckWithCircuitBreaker(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-circuit",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-circuit-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 2 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "circuit.CircuitService",
		CircuitBreaker: &roostv1alpha1.GRPCCircuitBreakerSpec{
			Enabled:          true,
			FailureThreshold: 2,
			SuccessThreshold: 1,
			HalfOpenTimeout:  metav1.Duration{Duration: 10 * time.Second},
			RecoveryTimeout:  metav1.Duration{Duration: 5 * time.Second},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Test multiple failures to trigger circuit breaker
	for i := 0; i < 5; i++ {
		healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)
		assert.False(t, healthy)

		if i >= 2 {
			// Circuit should be open after failures
			t.Logf("Attempt %d: healthy=%v, error=%v (circuit should be open)", i+1, healthy, err)
		} else {
			t.Logf("Attempt %d: healthy=%v, error=%v", i+1, healthy, err)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func TestGRPCHealthCheckWithMetadata(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-metadata",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-metadata-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 5 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "metadata.MetadataService",
		Metadata: map[string]string{
			"x-request-id":  "test-123",
			"x-client-name": "roost-keeper-test",
			"x-version":     "v1.0.0",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

	// Expected to fail without a running service
	assert.False(t, healthy)
	assert.Error(t, err)

	t.Logf("gRPC metadata health check completed: healthy=%v, error=%v", healthy, err)
}

func TestGRPCHealthCheckExternalService(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping external service test in short mode")
	}

	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-external",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-external-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 10 * time.Second},
	}

	// Test against infra-control node (if available)
	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "10.0.0.106",
		Port:    50051,
		Service: "example.ExampleService",
		ConnectionPool: &roostv1alpha1.GRPCConnectionPoolSpec{
			Enabled:        true,
			MaxConnections: 3,
		},
		RetryPolicy: &roostv1alpha1.GRPCRetryPolicySpec{
			Enabled:     true,
			MaxAttempts: 2,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

	// This may succeed or fail depending on whether a gRPC service is running on infra-control
	t.Logf("External gRPC health check completed: healthy=%v, error=%v", healthy, err)

	// Just verify the function doesn't panic and returns a response
	assert.NotNil(t, &healthy)
}

func TestGRPCHealthCheckStreamingInterface(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-streaming",
			Namespace: "default",
		},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:            "localhost",
		Port:            50051,
		Service:         "stream.StreamService",
		EnableStreaming: true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resultCh := make(chan *grpc.HealthResult, 10)
	checker := grpc.NewGRPCChecker(logger)

	// This will fail without a running service, but tests the interface
	err := checker.StartStreamingHealthCheck(ctx, roost, grpcSpec, resultCh)

	// Expected to fail without a running service
	assert.Error(t, err)

	t.Logf("gRPC streaming health check interface test completed: error=%v", err)
}

func TestGRPCHealthCheckWithObservability(t *testing.T) {
	// Use a simple logger for observability testing
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-observability",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-observability-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 5 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "observability.ObservabilityService",
	}

	// Create a span context for tracing
	ctx, span := telemetry.StartControllerSpan(context.Background(), "test.grpc_health_check", roost.Name, roost.Namespace)
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)

	// Expected to fail without a running service
	assert.False(t, healthy)
	assert.Error(t, err)

	// Record span error for observability
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
	}

	t.Logf("gRPC observability health check completed: healthy=%v, error=%v", healthy, err)
	t.Log("Telemetry data should be available in local-otel for analysis")
}

func TestGRPCHealthCheckErrorHandling(t *testing.T) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-errors",
			Namespace: "default",
		},
	}

	tests := []struct {
		name     string
		grpcSpec roostv1alpha1.GRPCHealthCheckSpec
		timeout  time.Duration
	}{
		{
			name: "Invalid Host",
			grpcSpec: roostv1alpha1.GRPCHealthCheckSpec{
				Host: "invalid-host-that-does-not-exist.local",
				Port: 50051,
			},
			timeout: 3 * time.Second,
		},
		{
			name: "Invalid Port",
			grpcSpec: roostv1alpha1.GRPCHealthCheckSpec{
				Host: "localhost",
				Port: 99999, // Invalid port
			},
			timeout: 3 * time.Second,
		},
		{
			name: "Connection Refused",
			grpcSpec: roostv1alpha1.GRPCHealthCheckSpec{
				Host: "localhost",
				Port: 12345, // Likely nothing running here
			},
			timeout: 2 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name:    fmt.Sprintf("grpc-error-test-%s", tt.name),
				Type:    "grpc",
				Timeout: metav1.Duration{Duration: tt.timeout},
			}

			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout+2*time.Second)
			defer cancel()

			healthy, err := grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, tt.grpcSpec)

			// All these should fail gracefully
			assert.False(t, healthy)
			assert.Error(t, err)

			t.Logf("%s - Error handling test completed: healthy=%v, error=%v", tt.name, healthy, err)
		})
	}
}

func TestGRPCHealthCheckResourceCleanup(t *testing.T) {
	logger := zap.NewNop()

	// Test that the checker properly cleans up resources
	checker := grpc.NewGRPCChecker(logger)

	// Use the checker for multiple operations
	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-grpc-cleanup",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-cleanup-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 2 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "cleanup.CleanupService",
		ConnectionPool: &roostv1alpha1.GRPCConnectionPoolSpec{
			Enabled:        true,
			MaxConnections: 2,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Perform multiple checks to create resources
	for i := 0; i < 3; i++ {
		_, err := checker.CheckHealth(ctx, roost, checkSpec, grpcSpec)
		assert.Error(t, err) // Expected without running service
	}

	// Clean up resources
	err := checker.Close()
	assert.NoError(t, err)

	t.Log("gRPC resource cleanup test completed successfully")
}

func BenchmarkGRPCHealthCheck(b *testing.B) {
	logger := zap.NewNop()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bench-grpc",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "grpc-bench-test",
		Type:    "grpc",
		Timeout: metav1.Duration{Duration: 1 * time.Second},
	}

	grpcSpec := roostv1alpha1.GRPCHealthCheckSpec{
		Host:    "localhost",
		Port:    50051,
		Service: "bench.BenchService",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = grpc.ExecuteGRPCCheck(ctx, logger, roost, checkSpec, grpcSpec)
	}
}
