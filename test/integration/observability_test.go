package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"

	"github.com/birbparty/roost-keeper/internal/telemetry"
)

func TestObservabilityIntegration(t *testing.T) {
	// Setup temporary directory for telemetry output
	tempDir, err := os.MkdirTemp("", "roost-keeper-observability-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Change to temp directory for the test
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Initialize observability
	ctx := context.Background()
	provider, shutdown, err := telemetry.InitOTEL(ctx)
	require.NoError(t, err)
	defer func() {
		shutdownErr := shutdown(ctx)
		assert.NoError(t, shutdownErr)
	}()

	// Create metrics
	metrics, err := telemetry.NewOperatorMetrics()
	require.NoError(t, err)

	// Test logger initialization
	logger, err := telemetry.NewLogger()
	require.NoError(t, err)
	assert.NotNil(t, logger)

	// Test correlation ID functionality
	ctx, correlationID := telemetry.WithCorrelationID(ctx)
	assert.NotEmpty(t, correlationID)

	retrievedID := telemetry.GetCorrelationID(ctx)
	assert.Equal(t, correlationID, retrievedID)

	// Test tracing
	ctx, span := telemetry.StartControllerSpan(ctx, "test_operation", "test-roost", "test-namespace")

	// Add some attributes to the span
	telemetry.WithSpanAttributes(ctx,
		attribute.String("test.attribute", "test-value"),
		attribute.Int64("test.number", 42),
	)

	// Add an event to the span
	telemetry.WithSpanEvent(ctx, "test_event",
		attribute.String("event.type", "test"),
	)

	// Simulate some work
	time.Sleep(10 * time.Millisecond)

	// Record metrics
	metrics.RecordReconcile(ctx, 10*time.Millisecond, true, "test-roost", "test-namespace")
	metrics.RecordHelmOperation(ctx, "install", 50*time.Millisecond, true, "test-chart", "1.0.0", "test-namespace")
	metrics.RecordHealthCheck(ctx, "http", 5*time.Millisecond, true, "test-service")
	metrics.RecordKubernetesAPICall(ctx, "GET", "ManagedRoost", 15*time.Millisecond, true)

	// Update roost metrics
	metrics.UpdateRoostMetrics(ctx, 1, 1, "Running", 1)
	metrics.UpdateGenerationLag(ctx, 0, "test-roost", "test-namespace")

	// Test performance metrics
	metrics.UpdatePerformanceMetrics(ctx, 1024*1024, 0.5, 100)

	// Test leader election change
	metrics.RecordLeaderElectionChange(ctx, "operator-1")

	// Set operator start time and update uptime
	startTime := time.Now().Add(-1 * time.Hour)
	metrics.SetOperatorStartTime(ctx, startTime)
	metrics.UpdateOperatorUptime(ctx, startTime)

	// Record success and end span
	telemetry.RecordSpanSuccess(ctx)
	span.End()

	// Test logging with context
	telemetry.LogInfo(ctx, "Test log message",
		"test.field", "test-value",
		"number.field", 123,
	)

	telemetry.LogDebug(ctx, "Test debug message")

	// Test error logging
	testErr := assert.AnError
	telemetry.LogError(ctx, testErr, "Test error message",
		"error.code", "TEST_ERROR",
	)

	// Force flush to ensure all data is written
	err = provider.ForceFlush(ctx)
	require.NoError(t, err)

	// Add a small delay to ensure file writes complete
	time.Sleep(100 * time.Millisecond)

	// Verify directories were created
	assert.DirExists(t, "observability")
	assert.DirExists(t, "observability/traces")
	assert.DirExists(t, "observability/metrics")
	assert.DirExists(t, "observability/logs")

	// Verify trace file exists and contains data
	tracesFile := filepath.Join("observability", "traces", "traces.jsonl")
	if assert.FileExists(t, tracesFile) {
		traceData, err := os.ReadFile(tracesFile)
		require.NoError(t, err)

		// Verify trace content
		assert.Contains(t, string(traceData), "test_operation")
		assert.Contains(t, string(traceData), "test-roost")
		assert.Contains(t, string(traceData), "test-namespace")
		assert.Contains(t, string(traceData), correlationID)

		// Parse and validate JSON structure
		lines := splitJSONL(string(traceData))
		assert.Greater(t, len(lines), 0, "Should have at least one trace entry")

		for _, line := range lines {
			if line != "" {
				var traceEntry map[string]interface{}
				err := json.Unmarshal([]byte(line), &traceEntry)
				assert.NoError(t, err, "Trace entry should be valid JSON")

				// Verify required fields
				assert.Contains(t, traceEntry, "trace_id")
				assert.Contains(t, traceEntry, "span_id")
				assert.Contains(t, traceEntry, "operation_name")
				assert.Contains(t, traceEntry, "timestamp")
			}
		}
	}

	// Verify metric file exists and contains data
	metricsFile := filepath.Join("observability", "metrics", "metrics.jsonl")
	if assert.FileExists(t, metricsFile) {
		metricData, err := os.ReadFile(metricsFile)
		require.NoError(t, err)

		// Verify metric content
		assert.Contains(t, string(metricData), "roost_keeper_reconcile_total")
		assert.Contains(t, string(metricData), "roost_keeper_helm_install_total")

		// Parse and validate JSON structure
		lines := splitJSONL(string(metricData))
		assert.Greater(t, len(lines), 0, "Should have at least one metric entry")

		for _, line := range lines {
			if line != "" {
				var metricEntry map[string]interface{}
				err := json.Unmarshal([]byte(line), &metricEntry)
				assert.NoError(t, err, "Metric entry should be valid JSON")

				// Verify required fields
				assert.Contains(t, metricEntry, "name")
				assert.Contains(t, metricEntry, "timestamp")
				assert.Contains(t, metricEntry, "type")
			}
		}
	}

	// Verify log file exists and contains data
	logsFile := filepath.Join("observability", "logs", "operator.jsonl")
	if assert.FileExists(t, logsFile) {
		logData, err := os.ReadFile(logsFile)
		require.NoError(t, err)

		// Verify log content
		assert.Contains(t, string(logData), "Test log message")
		assert.Contains(t, string(logData), correlationID)

		// Parse and validate JSON structure
		lines := splitJSONL(string(logData))
		assert.Greater(t, len(lines), 0, "Should have at least one log entry")

		for _, line := range lines {
			if line != "" {
				var logEntry map[string]interface{}
				err := json.Unmarshal([]byte(line), &logEntry)
				assert.NoError(t, err, "Log entry should be valid JSON")

				// Verify required fields
				assert.Contains(t, logEntry, "timestamp")
				assert.Contains(t, logEntry, "level")
				assert.Contains(t, logEntry, "message")
				assert.Contains(t, logEntry, "service")
			}
		}
	}
}

func TestMiddlewareIntegration(t *testing.T) {
	// Setup temporary directory for telemetry output
	tempDir, err := os.MkdirTemp("", "roost-keeper-middleware-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Change to temp directory for the test
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Initialize observability
	ctx := context.Background()
	provider, shutdown, err := telemetry.InitOTEL(ctx)
	require.NoError(t, err)
	defer func() {
		shutdownErr := shutdown(ctx)
		assert.NoError(t, shutdownErr)
	}()

	// Create metrics
	metrics, err := telemetry.NewOperatorMetrics()
	require.NoError(t, err)

	// Test controller middleware
	testOperation := func(ctx context.Context) error {
		telemetry.LogInfo(ctx, "Test operation executed")
		time.Sleep(5 * time.Millisecond)
		return nil
	}

	wrappedOperation := telemetry.ControllerMiddleware(
		testOperation,
		"test_controller_operation",
		metrics,
		"test-roost",
		"test-namespace",
	)

	err = wrappedOperation(ctx)
	assert.NoError(t, err)

	// Test Helm middleware
	testHelmOperation := func(ctx context.Context) error {
		telemetry.LogInfo(ctx, "Test Helm operation executed")
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	wrappedHelmOperation := telemetry.HelmMiddleware(
		testHelmOperation,
		"install",
		"test-chart",
		"1.0.0",
		"test-namespace",
		metrics,
	)

	err = wrappedHelmOperation(ctx)
	assert.NoError(t, err)

	// Test health check middleware
	testHealthCheck := func(ctx context.Context) error {
		telemetry.LogDebug(ctx, "Test health check executed")
		time.Sleep(2 * time.Millisecond)
		return nil
	}

	wrappedHealthCheck := telemetry.HealthCheckMiddleware(
		testHealthCheck,
		"http",
		"test-service",
		metrics,
	)

	err = wrappedHealthCheck(ctx)
	assert.NoError(t, err)

	// Test Kubernetes API middleware
	testAPICall := func(ctx context.Context) error {
		telemetry.LogDebug(ctx, "Test API call executed")
		time.Sleep(3 * time.Millisecond)
		return nil
	}

	wrappedAPICall := telemetry.KubernetesAPIMiddleware(
		testAPICall,
		"GET",
		"ManagedRoost",
		"test-namespace",
		metrics,
	)

	err = wrappedAPICall(ctx)
	assert.NoError(t, err)

	// Force flush to ensure all data is written
	err = provider.ForceFlush(ctx)
	require.NoError(t, err)

	// Add a small delay to ensure file writes complete
	time.Sleep(100 * time.Millisecond)

	// Verify that telemetry was recorded
	tracesFile := filepath.Join("observability", "traces", "traces.jsonl")
	metricsFile := filepath.Join("observability", "metrics", "metrics.jsonl")
	logsFile := filepath.Join("observability", "logs", "operator.jsonl")

	assert.FileExists(t, tracesFile)
	assert.FileExists(t, metricsFile)
	assert.FileExists(t, logsFile)

	// Verify traces contain middleware operations
	if assert.FileExists(t, tracesFile) {
		traceData, err := os.ReadFile(tracesFile)
		require.NoError(t, err)
		assert.Contains(t, string(traceData), "test_controller_operation")
		assert.Contains(t, string(traceData), "helm.install")
		assert.Contains(t, string(traceData), "health_check.http")
		assert.Contains(t, string(traceData), "k8s.GET.ManagedRoost")
	}
}

func TestPerformanceImpact(t *testing.T) {
	// This test measures the performance impact of observability
	iterations := 1000

	// Measure baseline performance without observability
	start := time.Now()
	for i := 0; i < iterations; i++ {
		// Simulate some work
		time.Sleep(time.Microsecond)
	}
	baselineDuration := time.Since(start)

	// Setup temporary directory for telemetry output
	tempDir, err := os.MkdirTemp("", "roost-keeper-performance-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Change to temp directory for the test
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	// Initialize observability
	ctx := context.Background()
	_, shutdown, err := telemetry.InitOTEL(ctx)
	require.NoError(t, err)
	defer func() {
		shutdownErr := shutdown(ctx)
		assert.NoError(t, shutdownErr)
	}()

	metrics, err := telemetry.NewOperatorMetrics()
	require.NoError(t, err)

	// Measure performance with observability
	start = time.Now()
	for i := 0; i < iterations; i++ {
		ctx, span := telemetry.StartGenericSpan(ctx, "test_operation", "test")

		// Simulate some work
		time.Sleep(time.Microsecond)

		// Record a metric
		metrics.RecordReconcile(ctx, time.Microsecond, true, "test", "test")

		span.End()
	}
	observabilityDuration := time.Since(start)

	// Calculate overhead
	overhead := float64(observabilityDuration-baselineDuration) / float64(baselineDuration) * 100

	t.Logf("Baseline duration: %v", baselineDuration)
	t.Logf("Observability duration: %v", observabilityDuration)
	t.Logf("Overhead: %.2f%%", overhead)

	// Verify overhead is within acceptable limits (< 10% for this test)
	// In production, aim for < 5%
	assert.Less(t, overhead, 10.0, "Observability overhead should be less than 10%")
}

// Helper function to split JSONL content into lines
func splitJSONL(content string) []string {
	lines := []string{}
	current := ""

	for _, char := range content {
		if char == '\n' {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		lines = append(lines, current)
	}

	return lines
}
