# Event-Driven Integration Implementation

This document describes the implementation of the event-driven integration system for the Roost Keeper operator, providing comprehensive event publishing, consumption, and monitoring capabilities.

## Overview

The event-driven integration system enables the Roost Keeper operator to:

- **Publish lifecycle events** for ManagedRoost resources (created, updated, deleted, etc.)
- **Handle external trigger events** for automated operations (teardown, scaling, updates)
- **Provide reliable event delivery** with Kafka as the messaging backbone
- **Monitor event system health** with comprehensive metrics and observability
- **Support event replay** for debugging and recovery scenarios
- **Implement fault tolerance** with circuit breakers and dead letter queues

## Architecture

### Core Components

1. **Event Manager** (`internal/events/manager.go`)
   - Central orchestrator for all event operations
   - Manages producers and consumers lifecycle
   - Integrates with controller-runtime manager
   - Provides high-level API for event publishing

2. **Kafka Producer** (`internal/events/producer.go`)
   - Publishes CloudEvents to Kafka topics
   - Supports both sync and async publishing modes
   - Implements circuit breaker pattern for fault tolerance
   - Handles connection management and retries

3. **Kafka Consumer** (`internal/events/consumer.go`)
   - Consumes events from Kafka topics
   - Supports multiple consumer instances for scalability
   - Implements event handler pattern for processing

4. **Metrics System** (`internal/events/metrics.go`)
   - OpenTelemetry-based metrics collection
   - Tracks producer/consumer health, latency, errors
   - Monitors dead letter queues and circuit breakers

5. **Configuration** (`internal/events/config.go`)
   - Comprehensive configuration for all components
   - Support for different environments (dev, staging, prod)
   - Validation and default settings

## Event Types

### Lifecycle Events

Published automatically by the operator for ManagedRoost resources:

- `io.roost-keeper.roost.lifecycle.created` - New roost created
- `io.roost-keeper.roost.lifecycle.updated` - Roost configuration updated
- `io.roost-keeper.roost.lifecycle.deleted` - Roost deleted
- `io.roost-keeper.roost.phase.transitioned` - Phase change (pending → running → complete)
- `io.roost-keeper.roost.health.changed` - Health status change
- `io.roost-keeper.roost.teardown.triggered` - Teardown operation initiated

### Trigger Events

External events that trigger automated operations:

- `io.roost-keeper.trigger.teardown` - External teardown request
- `io.roost-keeper.trigger.scale` - Scaling operation request
- `io.roost-keeper.trigger.update` - Configuration update request

## Configuration

### Basic Configuration

```yaml
events:
  enabled: true
  kafka:
    brokers:
      - "kafka-broker-1:9092"
      - "kafka-broker-2:9092"
    security:
      protocol: "SASL_SSL"
      mechanism: "SCRAM-SHA-256"
      username: "roost-keeper"
      passwordRef:
        name: "kafka-credentials"
        key: "password"
```

### Advanced Configuration

```yaml
events:
  enabled: true
  kafka:
    producer:
      acks: "all"
      retries: 5
      batchSize: 16384
      compression: "gzip"
      idempotent: true
    consumer:
      groupId: "roost-keeper-consumers"
      autoOffsetReset: "earliest"
      instances: 3
  topics:
    lifecycle: "roost-keeper-lifecycle"
    triggers: "roost-keeper-triggers"
    autoCreate: true
    defaultPartitions: 3
    replicationFactor: 2
  deadLetterQueue:
    enabled: true
    maxRetries: 3
    storage:
      type: "s3"
      bucket: "roost-keeper-dlq"
  circuitBreaker:
    enabled: true
    failureThreshold: 5
    recoveryTimeout: "30s"
```

## Usage Examples

### Publishing Events

The event system is integrated into the controller and publishes events automatically:

```go
// In the controller reconcile loop
if err := r.eventManager.PublishRoostCreated(ctx, managedRoost); err != nil {
    log.Error(err, "Failed to publish roost created event")
}

// Phase transitions
if oldPhase != newPhase {
    if err := r.eventManager.PublishRoostPhaseTransition(ctx, managedRoost, oldPhase, newPhase); err != nil {
        log.Error(err, "Failed to publish phase transition event")
    }
}

// Health changes
if healthChanged {
    if err := r.eventManager.PublishRoostHealthChanged(ctx, managedRoost, healthStatus); err != nil {
        log.Error(err, "Failed to publish health change event")
    }
}
```

### Consuming Events

Event consumers can be registered to handle specific event types:

```go
// Register a handler for trigger events
consumer.Subscribe("roost-keeper-triggers", func(ctx context.Context, event *event.Event) error {
    switch event.Type() {
    case events.EventTypeTriggerTeardown:
        return handleTeardownTrigger(ctx, event)
    case events.EventTypeTriggerScale:
        return handleScaleTrigger(ctx, event)
    default:
        log.Info("Unhandled event type", "type", event.Type())
    }
    return nil
})
```

### Event Structure

All events follow the CloudEvents specification:

```json
{
  "specversion": "1.0",
  "type": "io.roost-keeper.roost.lifecycle.created",
  "source": "roost-keeper-operator",
  "id": "event-1234567890",
  "time": "2023-06-10T14:30:00Z",
  "subject": "roost/default/my-roost",
  "datacontenttype": "application/json",
  "data": {
    "roost": {
      "name": "my-roost",
      "namespace": "default",
      "uid": "550e8400-e29b-41d4-a716-446655440000"
    },
    "chart": {
      "name": "my-app",
      "version": "1.0.0",
      "repository": "https://charts.example.com"
    }
  },
  "roostkeeper-roost-name": "my-roost",
  "roostkeeper-roost-namespace": "default",
  "roostkeeper-correlationid": "req-abc123"
}
```

## Fault Tolerance

### Circuit Breaker

The system implements circuit breaker patterns to handle downstream failures:

- **Failure Threshold**: Opens circuit after 5 consecutive failures
- **Recovery Timeout**: Attempts recovery after 30 seconds
- **Half-Open State**: Allows limited calls to test recovery

### Dead Letter Queue

Failed events are automatically routed to a dead letter queue:

- **Retry Logic**: Configurable retry attempts with exponential backoff
- **Storage**: Persistent storage in S3 or other cloud storage
- **Monitoring**: Metrics and alerts for DLQ message accumulation

### Retry Mechanisms

- **Producer Retries**: Automatic retries with backoff for publish failures
- **Consumer Retries**: Configurable retry behavior for processing failures
- **Connection Recovery**: Automatic reconnection on network failures

## Monitoring and Observability

### Metrics

The system exposes comprehensive metrics:

```
# Event publishing metrics
roost_keeper_events_published_total{event_type="lifecycle.created"}
roost_keeper_event_publish_duration_seconds{event_type="lifecycle.created"}
roost_keeper_event_publish_errors_total{event_type="lifecycle.created"}

# Event consumption metrics
roost_keeper_events_consumed_total{event_type="trigger.teardown"}
roost_keeper_event_consume_duration_seconds{event_type="trigger.teardown"}
roost_keeper_consumer_lag{topic="roost-keeper-triggers"}

# Health metrics
roost_keeper_producer_health_status{component="producer"}
roost_keeper_consumer_health_status{component="consumer"}

# Dead letter queue metrics
roost_keeper_dlq_messages_total{event_type="lifecycle.created"}
roost_keeper_dlq_current_size
```

### Tracing

All event operations are traced using OpenTelemetry:

- **Span Creation**: Each event publish/consume operation creates a span
- **Context Propagation**: Correlation IDs are propagated across services
- **Error Tracking**: Failed operations are marked in traces

### Logging

Structured logging with contextual information:

```json
{
  "level": "info",
  "msg": "Event published successfully",
  "event_id": "event-1234567890",
  "event_type": "io.roost-keeper.roost.lifecycle.created",
  "topic": "roost-keeper-lifecycle",
  "partition": 2,
  "offset": 12345
}
```

## Deployment

### Kubernetes Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: roost-keeper-events-config
data:
  events-config.yaml: |
    enabled: true
    kafka:
      brokers:
        - "kafka-cluster-kafka-bootstrap:9092"
      security:
        protocol: "SASL_SSL"
        mechanism: "SCRAM-SHA-256"
        username: "roost-keeper"
        passwordRef:
          name: "kafka-credentials"
          key: "password"
---
apiVersion: v1
kind: Secret
metadata:
  name: kafka-credentials
type: Opaque
data:
  password: <base64-encoded-password>
```

### Environment Variables

```bash
# Enable/disable event system
EVENTS_ENABLED=true

# Kafka configuration
KAFKA_BROKERS=kafka-broker-1:9092,kafka-broker-2:9092
KAFKA_SECURITY_PROTOCOL=SASL_SSL
KAFKA_SASL_MECHANISM=SCRAM-SHA-256
KAFKA_USERNAME=roost-keeper

# Topic configuration
KAFKA_TOPIC_LIFECYCLE=roost-keeper-lifecycle
KAFKA_TOPIC_TRIGGERS=roost-keeper-triggers
```

## Security Considerations

### Authentication

- **SASL/SCRAM**: Secure authentication with Kafka brokers
- **TLS Encryption**: All communications encrypted in transit
- **Secret Management**: Credentials stored in Kubernetes secrets

### Authorization

- **Topic Access Control**: Proper ACLs configured for Kafka topics
- **RBAC Integration**: Event publishing respects RBAC permissions
- **Audit Logging**: All event operations are audited

### Data Privacy

- **Event Sanitization**: Sensitive data is excluded from events
- **Retention Policies**: Configurable retention for event storage
- **Encryption at Rest**: Dead letter queue storage is encrypted

## Performance Considerations

### Scalability

- **Horizontal Scaling**: Multiple consumer instances for parallel processing
- **Partitioning**: Topic partitions for parallel event processing
- **Connection Pooling**: Efficient connection management

### Optimization

- **Batch Processing**: Events are batched for efficient publishing
- **Compression**: Configurable compression for network efficiency
- **Memory Management**: Bounded queues and memory limits

### Monitoring

- **Performance Metrics**: Latency and throughput monitoring
- **Resource Usage**: CPU and memory consumption tracking
- **Bottleneck Detection**: Identification of performance bottlenecks

## Troubleshooting

### Common Issues

1. **Connection Failures**
   - Check Kafka broker connectivity
   - Verify authentication credentials
   - Review network policies

2. **High Consumer Lag**
   - Increase consumer instances
   - Check processing performance
   - Review partition count

3. **Circuit Breaker Open**
   - Check downstream service health
   - Review error logs
   - Manually reset if needed

### Debugging Tools

1. **Event Replay**: Replay events for debugging
2. **Metrics Dashboard**: Monitor system health
3. **Trace Analysis**: Analyze event flow with distributed tracing
4. **Log Aggregation**: Centralized logging for troubleshooting

## Migration and Upgrades

### Backward Compatibility

- **Event Schema Evolution**: CloudEvents specification ensures compatibility
- **Configuration Migration**: Automated migration for configuration changes
- **Rolling Updates**: Zero-downtime upgrades with proper sequencing

### Data Migration

- **Topic Migration**: Tools for migrating between Kafka clusters
- **Event Transformation**: Support for event schema evolution
- **Rollback Procedures**: Safe rollback mechanisms for failed upgrades

## Best Practices

### Event Design

- **Idempotency**: Events should be designed to be idempotent
- **Immutability**: Events are immutable once published
- **Schema Evolution**: Use backward-compatible schema changes

### Operations

- **Monitoring**: Implement comprehensive monitoring and alerting
- **Testing**: Thorough testing of event handlers and fault scenarios
- **Documentation**: Maintain clear documentation for event schemas

### Performance

- **Batching**: Use batching for high-throughput scenarios
- **Partitioning**: Proper partitioning strategy for load distribution
- **Caching**: Cache frequently accessed configuration and metadata

## Future Enhancements

### Planned Features

1. **Schema Registry Integration**: Confluent Schema Registry support
2. **Event Sourcing**: Complete event sourcing implementation
3. **Multi-tenant Events**: Tenant-specific event isolation
4. **Advanced Analytics**: Real-time event analytics and insights

### Experimental Features

1. **Stream Processing**: Real-time event stream processing
2. **Event Mesh**: Multi-cluster event distribution
3. **ML Integration**: Machine learning on event streams
4. **GraphQL Events**: GraphQL subscription support for events
