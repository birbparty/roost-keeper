package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// APIMetrics contains metrics specific to API operations
type APIMetrics struct {
	// Validation metrics
	ValidationOperations metric.Int64Counter
	ValidationFailures   metric.Int64Counter
	ValidationDuration   metric.Float64Histogram

	// Status update metrics
	StatusUpdates        metric.Int64Counter
	StatusUpdateDuration metric.Float64Histogram
	StatusUpdateErrors   metric.Int64Counter

	// Lifecycle tracking metrics
	LifecycleEvents       metric.Int64Counter
	LifecycleEventsByType metric.Int64Counter

	// Cache metrics
	APICacheHits   metric.Int64Counter
	APICacheMisses metric.Int64Counter
	APICacheSize   metric.Int64UpDownCounter

	// General API operation metrics
	APIOperations        metric.Int64Counter
	APIOperationDuration metric.Float64Histogram
	APIErrors            metric.Int64Counter
}

// NewAPIMetrics creates and initializes API-specific metrics
func NewAPIMetrics() (*APIMetrics, error) {
	meter := otel.Meter(meterName)

	// Validation metrics
	validationOperations, err := meter.Int64Counter(
		"roost_keeper_api_validation_operations_total",
		metric.WithDescription("Total number of validation operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	validationFailures, err := meter.Int64Counter(
		"roost_keeper_api_validation_failures_total",
		metric.WithDescription("Total number of validation failures"),
		metric.WithUnit("{failures}"),
	)
	if err != nil {
		return nil, err
	}

	validationDuration, err := meter.Float64Histogram(
		"roost_keeper_api_validation_duration_seconds",
		metric.WithDescription("Duration of validation operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	// Status update metrics
	statusUpdates, err := meter.Int64Counter(
		"roost_keeper_api_status_updates_total",
		metric.WithDescription("Total number of status updates"),
		metric.WithUnit("{updates}"),
	)
	if err != nil {
		return nil, err
	}

	statusUpdateDuration, err := meter.Float64Histogram(
		"roost_keeper_api_status_update_duration_seconds",
		metric.WithDescription("Duration of status update operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	statusUpdateErrors, err := meter.Int64Counter(
		"roost_keeper_api_status_update_errors_total",
		metric.WithDescription("Total number of status update errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	// Lifecycle tracking metrics
	lifecycleEvents, err := meter.Int64Counter(
		"roost_keeper_api_lifecycle_events_total",
		metric.WithDescription("Total number of lifecycle events tracked"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		return nil, err
	}

	lifecycleEventsByType, err := meter.Int64Counter(
		"roost_keeper_api_lifecycle_events_by_type_total",
		metric.WithDescription("Total number of lifecycle events by type"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		return nil, err
	}

	// Cache metrics
	apiCacheHits, err := meter.Int64Counter(
		"roost_keeper_api_cache_hits_total",
		metric.WithDescription("Total number of API cache hits"),
		metric.WithUnit("{hits}"),
	)
	if err != nil {
		return nil, err
	}

	apiCacheMisses, err := meter.Int64Counter(
		"roost_keeper_api_cache_misses_total",
		metric.WithDescription("Total number of API cache misses"),
		metric.WithUnit("{misses}"),
	)
	if err != nil {
		return nil, err
	}

	apiCacheSize, err := meter.Int64UpDownCounter(
		"roost_keeper_api_cache_size",
		metric.WithDescription("Current size of API cache"),
		metric.WithUnit("{items}"),
	)
	if err != nil {
		return nil, err
	}

	// General API operation metrics
	apiOperations, err := meter.Int64Counter(
		"roost_keeper_api_operations_total",
		metric.WithDescription("Total number of API operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	apiOperationDuration, err := meter.Float64Histogram(
		"roost_keeper_api_operation_duration_seconds",
		metric.WithDescription("Duration of API operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	apiErrors, err := meter.Int64Counter(
		"roost_keeper_api_errors_total",
		metric.WithDescription("Total number of API errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	return &APIMetrics{
		ValidationOperations:  validationOperations,
		ValidationFailures:    validationFailures,
		ValidationDuration:    validationDuration,
		StatusUpdates:         statusUpdates,
		StatusUpdateDuration:  statusUpdateDuration,
		StatusUpdateErrors:    statusUpdateErrors,
		LifecycleEvents:       lifecycleEvents,
		LifecycleEventsByType: lifecycleEventsByType,
		APICacheHits:          apiCacheHits,
		APICacheMisses:        apiCacheMisses,
		APICacheSize:          apiCacheSize,
		APIOperations:         apiOperations,
		APIOperationDuration:  apiOperationDuration,
		APIErrors:             apiErrors,
	}, nil
}

// RecordValidationOperation records a validation operation
func (m *APIMetrics) RecordValidationOperation(ctx context.Context, success bool, errorCount int) {
	labels := metric.WithAttributes(
		attribute.Bool("success", success),
	)

	m.ValidationOperations.Add(ctx, 1, labels)

	if !success {
		m.ValidationFailures.Add(ctx, int64(errorCount), labels)
	}
}

// RecordStatusUpdate records a status update operation
func (m *APIMetrics) RecordStatusUpdate(ctx context.Context, phase string) {
	labels := metric.WithAttributes(
		attribute.String("phase", phase),
	)

	m.StatusUpdates.Add(ctx, 1, labels)
}

// RecordLifecycleEvent records a lifecycle event
func (m *APIMetrics) RecordLifecycleEvent(ctx context.Context, eventType string) {
	labels := metric.WithAttributes(
		attribute.String("event_type", eventType),
	)

	m.LifecycleEvents.Add(ctx, 1)
	m.LifecycleEventsByType.Add(ctx, 1, labels)
}

// RecordCacheHit records a cache hit
func (m *APIMetrics) RecordCacheHit(ctx context.Context, cacheType string) {
	labels := metric.WithAttributes(
		attribute.String("cache_type", cacheType),
	)

	m.APICacheHits.Add(ctx, 1, labels)
}

// RecordCacheMiss records a cache miss
func (m *APIMetrics) RecordCacheMiss(ctx context.Context, cacheType string) {
	labels := metric.WithAttributes(
		attribute.String("cache_type", cacheType),
	)

	m.APICacheMisses.Add(ctx, 1, labels)
}

// RecordAPIOperation records a general API operation
func (m *APIMetrics) RecordAPIOperation(ctx context.Context, operation string, duration float64) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
	)

	m.APIOperations.Add(ctx, 1, labels)
	m.APIOperationDuration.Record(ctx, duration, labels)
}

// RecordAPIError records an API error
func (m *APIMetrics) RecordAPIError(ctx context.Context, operation string) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
	)

	m.APIErrors.Add(ctx, 1, labels)
}

// StartAPISpan starts a tracing span for API operations
func StartAPISpan(ctx context.Context, operation, resourceName, namespace string) (context.Context, trace.Span) {
	tracer := otel.Tracer(meterName)

	spanName := "api." + operation
	ctx, span := tracer.Start(ctx, spanName)

	span.SetAttributes(
		attribute.String("api.operation", operation),
		attribute.String("resource.name", resourceName),
		attribute.String("resource.namespace", namespace),
	)

	return ctx, span
}
