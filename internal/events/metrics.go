package events

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	eventMeterName = "roost-keeper-events"
)

// EventMetricsImpl implements the EventMetrics interface using OpenTelemetry
type EventMetricsImpl struct {
	// Event publishing metrics
	eventsPublishedTotal metric.Int64Counter
	eventPublishDuration metric.Float64Histogram
	eventPublishErrors   metric.Int64Counter

	// Event consumption metrics
	eventsConsumedTotal  metric.Int64Counter
	eventConsumeDuration metric.Float64Histogram
	eventConsumeErrors   metric.Int64Counter

	// Producer health metrics
	producerHealthStatus     metric.Int64UpDownCounter
	producerConnectionStatus metric.Int64UpDownCounter

	// Consumer health metrics
	consumerHealthStatus metric.Int64UpDownCounter
	consumerLag          metric.Int64UpDownCounter
	consumerGroupMembers metric.Int64UpDownCounter

	// Dead letter queue metrics
	dlqMessagesTotal      metric.Int64Counter
	dlqProcessingDuration metric.Float64Histogram
	dlqRetryAttempts      metric.Int64Counter
	dlqCurrentSize        metric.Int64UpDownCounter

	// Event replay metrics
	replayRequestsTotal metric.Int64Counter
	replayDuration      metric.Float64Histogram
	replayedEventsTotal metric.Int64Counter
	replayErrors        metric.Int64Counter

	// Schema validation metrics
	schemaValidationsTotal   metric.Int64Counter
	schemaValidationErrors   metric.Int64Counter
	schemaValidationDuration metric.Float64Histogram

	// Circuit breaker metrics
	circuitBreakerState      metric.Int64UpDownCounter
	circuitBreakerTrips      metric.Int64Counter
	circuitBreakerRecoveries metric.Int64Counter

	// Storage metrics
	storageOperationsTotal   metric.Int64Counter
	storageOperationDuration metric.Float64Histogram
	storageErrors            metric.Int64Counter
	storageObjectsCount      metric.Int64UpDownCounter
	storageSize              metric.Int64UpDownCounter
}

// NewEventMetrics creates a new EventMetrics instance
func NewEventMetrics() (*EventMetricsImpl, error) {
	meter := otel.Meter(eventMeterName)

	// Event publishing metrics
	eventsPublishedTotal, err := meter.Int64Counter(
		"roost_keeper_events_published_total",
		metric.WithDescription("Total number of events published"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		return nil, err
	}

	eventPublishDuration, err := meter.Float64Histogram(
		"roost_keeper_event_publish_duration_seconds",
		metric.WithDescription("Duration of event publishing operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	eventPublishErrors, err := meter.Int64Counter(
		"roost_keeper_event_publish_errors_total",
		metric.WithDescription("Total number of event publishing errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	// Event consumption metrics
	eventsConsumedTotal, err := meter.Int64Counter(
		"roost_keeper_events_consumed_total",
		metric.WithDescription("Total number of events consumed"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		return nil, err
	}

	eventConsumeDuration, err := meter.Float64Histogram(
		"roost_keeper_event_consume_duration_seconds",
		metric.WithDescription("Duration of event consumption operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	eventConsumeErrors, err := meter.Int64Counter(
		"roost_keeper_event_consume_errors_total",
		metric.WithDescription("Total number of event consumption errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	// Producer health metrics
	producerHealthStatus, err := meter.Int64UpDownCounter(
		"roost_keeper_producer_health_status",
		metric.WithDescription("Producer health status (1=healthy, 0=unhealthy)"),
		metric.WithUnit("{status}"),
	)
	if err != nil {
		return nil, err
	}

	producerConnectionStatus, err := meter.Int64UpDownCounter(
		"roost_keeper_producer_connection_status",
		metric.WithDescription("Producer connection status (1=connected, 0=disconnected)"),
		metric.WithUnit("{status}"),
	)
	if err != nil {
		return nil, err
	}

	// Consumer health metrics
	consumerHealthStatus, err := meter.Int64UpDownCounter(
		"roost_keeper_consumer_health_status",
		metric.WithDescription("Consumer health status (1=healthy, 0=unhealthy)"),
		metric.WithUnit("{status}"),
	)
	if err != nil {
		return nil, err
	}

	consumerLag, err := meter.Int64UpDownCounter(
		"roost_keeper_consumer_lag",
		metric.WithDescription("Consumer lag in number of messages"),
		metric.WithUnit("{messages}"),
	)
	if err != nil {
		return nil, err
	}

	consumerGroupMembers, err := meter.Int64UpDownCounter(
		"roost_keeper_consumer_group_members",
		metric.WithDescription("Number of members in consumer group"),
		metric.WithUnit("{members}"),
	)
	if err != nil {
		return nil, err
	}

	// Dead letter queue metrics
	dlqMessagesTotal, err := meter.Int64Counter(
		"roost_keeper_dlq_messages_total",
		metric.WithDescription("Total number of messages sent to dead letter queue"),
		metric.WithUnit("{messages}"),
	)
	if err != nil {
		return nil, err
	}

	dlqProcessingDuration, err := meter.Float64Histogram(
		"roost_keeper_dlq_processing_duration_seconds",
		metric.WithDescription("Duration of DLQ message processing"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	dlqRetryAttempts, err := meter.Int64Counter(
		"roost_keeper_dlq_retry_attempts_total",
		metric.WithDescription("Total number of DLQ retry attempts"),
		metric.WithUnit("{attempts}"),
	)
	if err != nil {
		return nil, err
	}

	dlqCurrentSize, err := meter.Int64UpDownCounter(
		"roost_keeper_dlq_current_size",
		metric.WithDescription("Current number of messages in DLQ"),
		metric.WithUnit("{messages}"),
	)
	if err != nil {
		return nil, err
	}

	// Event replay metrics
	replayRequestsTotal, err := meter.Int64Counter(
		"roost_keeper_event_replay_requests_total",
		metric.WithDescription("Total number of event replay requests"),
		metric.WithUnit("{requests}"),
	)
	if err != nil {
		return nil, err
	}

	replayDuration, err := meter.Float64Histogram(
		"roost_keeper_event_replay_duration_seconds",
		metric.WithDescription("Duration of event replay operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	replayedEventsTotal, err := meter.Int64Counter(
		"roost_keeper_replayed_events_total",
		metric.WithDescription("Total number of events replayed"),
		metric.WithUnit("{events}"),
	)
	if err != nil {
		return nil, err
	}

	replayErrors, err := meter.Int64Counter(
		"roost_keeper_event_replay_errors_total",
		metric.WithDescription("Total number of event replay errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	// Schema validation metrics
	schemaValidationsTotal, err := meter.Int64Counter(
		"roost_keeper_schema_validations_total",
		metric.WithDescription("Total number of schema validations performed"),
		metric.WithUnit("{validations}"),
	)
	if err != nil {
		return nil, err
	}

	schemaValidationErrors, err := meter.Int64Counter(
		"roost_keeper_schema_validation_errors_total",
		metric.WithDescription("Total number of schema validation errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	schemaValidationDuration, err := meter.Float64Histogram(
		"roost_keeper_schema_validation_duration_seconds",
		metric.WithDescription("Duration of schema validation operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	// Circuit breaker metrics
	circuitBreakerState, err := meter.Int64UpDownCounter(
		"roost_keeper_circuit_breaker_state",
		metric.WithDescription("Circuit breaker state (0=closed, 1=open, 2=half-open)"),
		metric.WithUnit("{state}"),
	)
	if err != nil {
		return nil, err
	}

	circuitBreakerTrips, err := meter.Int64Counter(
		"roost_keeper_circuit_breaker_trips_total",
		metric.WithDescription("Total number of circuit breaker trips"),
		metric.WithUnit("{trips}"),
	)
	if err != nil {
		return nil, err
	}

	circuitBreakerRecoveries, err := meter.Int64Counter(
		"roost_keeper_circuit_breaker_recoveries_total",
		metric.WithDescription("Total number of circuit breaker recoveries"),
		metric.WithUnit("{recoveries}"),
	)
	if err != nil {
		return nil, err
	}

	// Storage metrics
	storageOperationsTotal, err := meter.Int64Counter(
		"roost_keeper_storage_operations_total",
		metric.WithDescription("Total number of storage operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	storageOperationDuration, err := meter.Float64Histogram(
		"roost_keeper_storage_operation_duration_seconds",
		metric.WithDescription("Duration of storage operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	storageErrors, err := meter.Int64Counter(
		"roost_keeper_storage_errors_total",
		metric.WithDescription("Total number of storage errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	storageObjectsCount, err := meter.Int64UpDownCounter(
		"roost_keeper_storage_objects_count",
		metric.WithDescription("Current number of objects in storage"),
		metric.WithUnit("{objects}"),
	)
	if err != nil {
		return nil, err
	}

	storageSize, err := meter.Int64UpDownCounter(
		"roost_keeper_storage_size_bytes",
		metric.WithDescription("Current storage size in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	return &EventMetricsImpl{
		eventsPublishedTotal:     eventsPublishedTotal,
		eventPublishDuration:     eventPublishDuration,
		eventPublishErrors:       eventPublishErrors,
		eventsConsumedTotal:      eventsConsumedTotal,
		eventConsumeDuration:     eventConsumeDuration,
		eventConsumeErrors:       eventConsumeErrors,
		producerHealthStatus:     producerHealthStatus,
		producerConnectionStatus: producerConnectionStatus,
		consumerHealthStatus:     consumerHealthStatus,
		consumerLag:              consumerLag,
		consumerGroupMembers:     consumerGroupMembers,
		dlqMessagesTotal:         dlqMessagesTotal,
		dlqProcessingDuration:    dlqProcessingDuration,
		dlqRetryAttempts:         dlqRetryAttempts,
		dlqCurrentSize:           dlqCurrentSize,
		replayRequestsTotal:      replayRequestsTotal,
		replayDuration:           replayDuration,
		replayedEventsTotal:      replayedEventsTotal,
		replayErrors:             replayErrors,
		schemaValidationsTotal:   schemaValidationsTotal,
		schemaValidationErrors:   schemaValidationErrors,
		schemaValidationDuration: schemaValidationDuration,
		circuitBreakerState:      circuitBreakerState,
		circuitBreakerTrips:      circuitBreakerTrips,
		circuitBreakerRecoveries: circuitBreakerRecoveries,
		storageOperationsTotal:   storageOperationsTotal,
		storageOperationDuration: storageOperationDuration,
		storageErrors:            storageErrors,
		storageObjectsCount:      storageObjectsCount,
		storageSize:              storageSize,
	}, nil
}

// RecordEventPublished records a published event
func (m *EventMetricsImpl) RecordEventPublished(ctx context.Context, eventType string, duration float64) {
	labels := metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("source", EventSource),
	)

	m.eventsPublishedTotal.Add(ctx, 1, labels)
	m.eventPublishDuration.Record(ctx, duration, labels)
}

// RecordEventConsumed records a consumed event
func (m *EventMetricsImpl) RecordEventConsumed(ctx context.Context, eventType string, duration float64) {
	labels := metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("source", EventSource),
	)

	m.eventsConsumedTotal.Add(ctx, 1, labels)
	m.eventConsumeDuration.Record(ctx, duration, labels)
}

// RecordEventError records an event processing error
func (m *EventMetricsImpl) RecordEventError(ctx context.Context, eventType, errorType string) {
	labels := metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("error_type", errorType),
		attribute.String("source", EventSource),
	)

	// Determine if it's a publish or consume error and record accordingly
	switch errorType {
	case ErrorTypeProducer, ErrorTypeValidation:
		m.eventPublishErrors.Add(ctx, 1, labels)
	case ErrorTypeConsumer, ErrorTypeProcessing, ErrorTypeDeserialization:
		m.eventConsumeErrors.Add(ctx, 1, labels)
	}
}

// RecordProducerHealth records producer health status
func (m *EventMetricsImpl) RecordProducerHealth(ctx context.Context, healthy bool) {
	value := int64(0)
	if healthy {
		value = 1
	}

	labels := metric.WithAttributes(
		attribute.String("component", "producer"),
	)

	m.producerHealthStatus.Add(ctx, value, labels)
}

// RecordConsumerHealth records consumer health status
func (m *EventMetricsImpl) RecordConsumerHealth(ctx context.Context, healthy bool) {
	value := int64(0)
	if healthy {
		value = 1
	}

	labels := metric.WithAttributes(
		attribute.String("component", "consumer"),
	)

	m.consumerHealthStatus.Add(ctx, value, labels)
}

// RecordConsumerLag records consumer lag
func (m *EventMetricsImpl) RecordConsumerLag(ctx context.Context, topic string, lag int64) {
	labels := metric.WithAttributes(
		attribute.String("topic", topic),
		attribute.String("consumer_group", "roost-keeper-consumers"),
	)

	m.consumerLag.Add(ctx, lag, labels)
}

// RecordDLQMessage records a message sent to dead letter queue
func (m *EventMetricsImpl) RecordDLQMessage(ctx context.Context, eventType, reason string) {
	labels := metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.String("reason", reason),
	)

	m.dlqMessagesTotal.Add(ctx, 1, labels)
}

// RecordDLQProcessing records DLQ message processing
func (m *EventMetricsImpl) RecordDLQProcessing(ctx context.Context, duration float64, success bool) {
	labels := metric.WithAttributes(
		attribute.Bool("success", success),
	)

	m.dlqProcessingDuration.Record(ctx, duration, labels)
}

// RecordDLQRetry records a DLQ retry attempt
func (m *EventMetricsImpl) RecordDLQRetry(ctx context.Context, eventType string, attempt int) {
	labels := metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.Int("attempt", attempt),
	)

	m.dlqRetryAttempts.Add(ctx, 1, labels)
}

// UpdateDLQSize updates the current DLQ size
func (m *EventMetricsImpl) UpdateDLQSize(ctx context.Context, size int64) {
	m.dlqCurrentSize.Add(ctx, size)
}

// RecordEventReplay records an event replay operation
func (m *EventMetricsImpl) RecordEventReplay(ctx context.Context, startTime, endTime time.Time, eventCount int64, duration float64, success bool) {
	labels := metric.WithAttributes(
		attribute.Bool("success", success),
		attribute.String("time_range", formatTimeRange(startTime, endTime)),
	)

	m.replayRequestsTotal.Add(ctx, 1, labels)
	m.replayDuration.Record(ctx, duration, labels)

	if success {
		m.replayedEventsTotal.Add(ctx, eventCount, labels)
	} else {
		m.replayErrors.Add(ctx, 1, labels)
	}
}

// RecordSchemaValidation records a schema validation operation
func (m *EventMetricsImpl) RecordSchemaValidation(ctx context.Context, eventType string, duration float64, success bool) {
	labels := metric.WithAttributes(
		attribute.String("event_type", eventType),
		attribute.Bool("success", success),
	)

	m.schemaValidationsTotal.Add(ctx, 1, labels)
	m.schemaValidationDuration.Record(ctx, duration, labels)

	if !success {
		m.schemaValidationErrors.Add(ctx, 1, labels)
	}
}

// RecordCircuitBreakerState records circuit breaker state changes
func (m *EventMetricsImpl) RecordCircuitBreakerState(ctx context.Context, component string, state CircuitState) {
	labels := metric.WithAttributes(
		attribute.String("component", component),
	)

	m.circuitBreakerState.Add(ctx, int64(state), labels)
}

// RecordCircuitBreakerTrip records a circuit breaker trip
func (m *EventMetricsImpl) RecordCircuitBreakerTrip(ctx context.Context, component, reason string) {
	labels := metric.WithAttributes(
		attribute.String("component", component),
		attribute.String("reason", reason),
	)

	m.circuitBreakerTrips.Add(ctx, 1, labels)
}

// RecordCircuitBreakerRecovery records a circuit breaker recovery
func (m *EventMetricsImpl) RecordCircuitBreakerRecovery(ctx context.Context, component string) {
	labels := metric.WithAttributes(
		attribute.String("component", component),
	)

	m.circuitBreakerRecoveries.Add(ctx, 1, labels)
}

// RecordStorageOperation records a storage operation
func (m *EventMetricsImpl) RecordStorageOperation(ctx context.Context, operation, storageType string, duration float64, success bool) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("storage_type", storageType),
		attribute.Bool("success", success),
	)

	m.storageOperationsTotal.Add(ctx, 1, labels)
	m.storageOperationDuration.Record(ctx, duration, labels)

	if !success {
		m.storageErrors.Add(ctx, 1, labels)
	}
}

// UpdateStorageStats updates storage statistics
func (m *EventMetricsImpl) UpdateStorageStats(ctx context.Context, storageType string, objectCount, sizeBytes int64) {
	labels := metric.WithAttributes(
		attribute.String("storage_type", storageType),
	)

	m.storageObjectsCount.Add(ctx, objectCount, labels)
	m.storageSize.Add(ctx, sizeBytes, labels)
}

// RecordProducerConnection records producer connection status
func (m *EventMetricsImpl) RecordProducerConnection(ctx context.Context, connected bool) {
	value := int64(0)
	if connected {
		value = 1
	}

	labels := metric.WithAttributes(
		attribute.String("component", "producer"),
	)

	m.producerConnectionStatus.Add(ctx, value, labels)
}

// UpdateConsumerGroupMembers updates the number of consumer group members
func (m *EventMetricsImpl) UpdateConsumerGroupMembers(ctx context.Context, groupID string, memberCount int64) {
	labels := metric.WithAttributes(
		attribute.String("consumer_group", groupID),
	)

	m.consumerGroupMembers.Add(ctx, memberCount, labels)
}

// formatTimeRange formats a time range for metrics labels
func formatTimeRange(start, end time.Time) string {
	duration := end.Sub(start)
	return duration.String()
}
