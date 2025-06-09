package telemetry

import (
	"context"
	"runtime"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// ControllerMiddleware wraps controller operations with comprehensive observability
func ControllerMiddleware(
	next func(context.Context) error,
	operationName string,
	metrics *OperatorMetrics,
	roostName, namespace string,
) func(context.Context) error {
	return func(ctx context.Context) error {
		start := time.Now()

		// Add correlation ID to context if not present
		ctx, correlationID := WithCorrelationID(ctx)

		// Start tracing for the operation
		ctx, span := StartControllerSpan(ctx, operationName, roostName, namespace)
		defer span.End()

		// Add correlation ID to span
		WithSpanAttributes(ctx, attribute.String("correlation_id", correlationID))

		// Log operation start
		LogInfo(ctx, "Starting controller operation",
			"operation", operationName,
			"roost_name", roostName,
			"namespace", namespace,
		)

		// Execute the wrapped operation
		err := next(ctx)

		// Calculate duration
		duration := time.Since(start)
		success := err == nil

		// Record metrics if available
		if metrics != nil {
			metrics.RecordReconcile(ctx, duration, success, roostName, namespace)
		}

		// Record tracing outcome
		if err != nil {
			RecordError(span, err)
			LogError(ctx, err, "Controller operation failed",
				"operation", operationName,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			RecordSuccess(span)
			LogInfo(ctx, "Controller operation completed successfully",
				"operation", operationName,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return err
	}
}

// HelmMiddleware wraps Helm operations with observability
func HelmMiddleware(
	next func(context.Context) error,
	operation, chartName, chartVersion, namespace string,
	metrics *OperatorMetrics,
) func(context.Context) error {
	return func(ctx context.Context) error {
		start := time.Now()

		// Add correlation ID to context if not present
		ctx, correlationID := WithCorrelationID(ctx)

		// Start tracing for Helm operation
		ctx, span := StartHelmSpan(ctx, operation, chartName, chartVersion)
		defer span.End()

		// Add correlation ID and namespace to span
		WithSpanAttributes(ctx,
			attribute.String("correlation_id", correlationID),
			attribute.String("helm.namespace", namespace),
		)

		// Log operation start
		LogInfo(ctx, "Starting Helm operation",
			"operation", operation,
			"chart_name", chartName,
			"chart_version", chartVersion,
			"namespace", namespace,
		)

		// Execute the wrapped operation
		err := next(ctx)

		// Calculate duration
		duration := time.Since(start)
		success := err == nil

		// Record metrics if available
		if metrics != nil {
			metrics.RecordHelmOperation(ctx, operation, duration, success, chartName, chartVersion, namespace)
		}

		// Record tracing outcome
		if err != nil {
			RecordError(span, err)
			LogError(ctx, err, "Helm operation failed",
				"operation", operation,
				"chart_name", chartName,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			RecordSuccess(span)
			LogInfo(ctx, "Helm operation completed successfully",
				"operation", operation,
				"chart_name", chartName,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return err
	}
}

// HealthCheckMiddleware wraps health check operations with observability
func HealthCheckMiddleware(
	next func(context.Context) error,
	checkType, target string,
	metrics *OperatorMetrics,
) func(context.Context) error {
	return func(ctx context.Context) error {
		start := time.Now()

		// Add correlation ID to context if not present
		ctx, correlationID := WithCorrelationID(ctx)

		// Start tracing for health check
		ctx, span := StartHealthCheckSpan(ctx, checkType, target)
		defer span.End()

		// Add correlation ID to span
		WithSpanAttributes(ctx, attribute.String("correlation_id", correlationID))

		// Log operation start
		LogDebug(ctx, "Starting health check",
			"check_type", checkType,
			"target", target,
		)

		// Execute the wrapped operation
		err := next(ctx)

		// Calculate duration
		duration := time.Since(start)
		success := err == nil

		// Record metrics if available
		if metrics != nil {
			metrics.RecordHealthCheck(ctx, checkType, duration, success, target)
		}

		// Record tracing outcome
		if err != nil {
			RecordError(span, err)
			LogError(ctx, err, "Health check failed",
				"check_type", checkType,
				"target", target,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			RecordSuccess(span)
			LogDebug(ctx, "Health check completed successfully",
				"check_type", checkType,
				"target", target,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return err
	}
}

// KubernetesAPIMiddleware wraps Kubernetes API calls with observability
func KubernetesAPIMiddleware(
	next func(context.Context) error,
	method, resource, namespace string,
	metrics *OperatorMetrics,
) func(context.Context) error {
	return func(ctx context.Context) error {
		start := time.Now()

		// Add correlation ID to context if not present
		ctx, correlationID := WithCorrelationID(ctx)

		// Start tracing for Kubernetes API call
		ctx, span := StartKubernetesAPISpan(ctx, method, resource, namespace)
		defer span.End()

		// Add correlation ID to span
		WithSpanAttributes(ctx, attribute.String("correlation_id", correlationID))

		// Log API call start (debug level to avoid noise)
		LogDebug(ctx, "Starting Kubernetes API call",
			"method", method,
			"resource", resource,
			"namespace", namespace,
		)

		// Execute the wrapped operation
		err := next(ctx)

		// Calculate duration
		duration := time.Since(start)
		success := err == nil

		// Record metrics if available
		if metrics != nil {
			metrics.RecordKubernetesAPICall(ctx, method, resource, duration, success)
		}

		// Record tracing outcome
		if err != nil {
			RecordError(span, err)
			LogError(ctx, err, "Kubernetes API call failed",
				"method", method,
				"resource", resource,
				"duration_ms", duration.Milliseconds(),
			)
		} else {
			RecordSuccess(span)
			LogDebug(ctx, "Kubernetes API call completed successfully",
				"method", method,
				"resource", resource,
				"duration_ms", duration.Milliseconds(),
			)
		}

		return err
	}
}

// PerformanceMonitor periodically updates performance metrics
func PerformanceMonitor(ctx context.Context, metrics *OperatorMetrics, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updatePerformanceMetrics(ctx, metrics)
		}
	}
}

// updatePerformanceMetrics collects and updates performance metrics
func updatePerformanceMetrics(ctx context.Context, metrics *OperatorMetrics) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Update memory usage (allocated bytes)
	memoryBytes := int64(m.Alloc)

	// Get goroutine count
	goroutines := int64(runtime.NumGoroutine())

	// CPU usage would require more complex collection, so we'll use a placeholder
	// In a real implementation, you'd use a library like gopsutil
	cpuRatio := 0.0 // Placeholder

	// Record performance metrics
	metrics.UpdatePerformanceMetrics(ctx, memoryBytes, cpuRatio, goroutines)

	// Log performance metrics periodically (every 10th update to reduce noise)
	LogDebug(ctx, "Performance metrics updated",
		"memory_bytes", memoryBytes,
		"goroutines", goroutines,
		"cpu_ratio", cpuRatio,
	)
}

// InstrumentedReconciler wraps the entire reconciler with observability
type InstrumentedReconciler struct {
	name    string
	metrics *OperatorMetrics
}

// NewInstrumentedReconciler creates a new instrumented reconciler wrapper
func NewInstrumentedReconciler(name string, metrics *OperatorMetrics) *InstrumentedReconciler {
	return &InstrumentedReconciler{
		name:    name,
		metrics: metrics,
	}
}

// WrapReconcile wraps a reconcile function with full observability
func (ir *InstrumentedReconciler) WrapReconcile(
	reconcileFn func(context.Context, string, string) error,
) func(context.Context, string, string) error {
	return func(ctx context.Context, name, namespace string) error {
		// Use controller middleware to wrap the reconcile operation
		wrappedFn := ControllerMiddleware(
			func(ctx context.Context) error {
				return reconcileFn(ctx, name, namespace)
			},
			"reconcile",
			ir.metrics,
			name,
			namespace,
		)

		return wrappedFn(ctx)
	}
}

// ObservabilityConfig holds configuration for observability middleware
type ObservabilityConfig struct {
	EnableTracing         bool
	EnableMetrics         bool
	EnableDetailedLogging bool
	PerformanceInterval   time.Duration
	MetricsFlushInterval  time.Duration
}

// DefaultObservabilityConfig returns default observability configuration
func DefaultObservabilityConfig() *ObservabilityConfig {
	return &ObservabilityConfig{
		EnableTracing:         true,
		EnableMetrics:         true,
		EnableDetailedLogging: true,
		PerformanceInterval:   30 * time.Second,
		MetricsFlushInterval:  15 * time.Second,
	}
}

// WithTracing enables or disables tracing
func (c *ObservabilityConfig) WithTracing(enabled bool) *ObservabilityConfig {
	c.EnableTracing = enabled
	return c
}

// WithMetrics enables or disables metrics
func (c *ObservabilityConfig) WithMetrics(enabled bool) *ObservabilityConfig {
	c.EnableMetrics = enabled
	return c
}

// WithDetailedLogging enables or disables detailed logging
func (c *ObservabilityConfig) WithDetailedLogging(enabled bool) *ObservabilityConfig {
	c.EnableDetailedLogging = enabled
	return c
}
