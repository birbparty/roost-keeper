package events

import (
	"context"
	"fmt"
	"time"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/cloudevents/sdk-go/v2/event"
)

// EventManager defines the main interface for event operations
type EventManager interface {
	// Start initializes and starts the event system
	Start(ctx context.Context) error

	// Stop gracefully shuts down the event system
	Stop(ctx context.Context) error

	// PublishLifecycleEvent publishes a lifecycle event for a ManagedRoost
	PublishLifecycleEvent(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, eventType string, data interface{}) error

	// SubscribeToTriggers subscribes to external trigger events
	SubscribeToTriggers(ctx context.Context, handler TriggerEventHandler) error

	// ReplayEvents replays historical events for a given time range
	ReplayEvents(ctx context.Context, startTime, endTime time.Time, eventTypes []string) error

	// GetMetrics returns event system metrics
	GetMetrics() EventMetrics
}

// EventProducer handles event publishing
type EventProducer interface {
	// PublishEvent publishes a CloudEvent to the event stream
	PublishEvent(ctx context.Context, event *event.Event) error

	// Close gracefully closes the producer
	Close() error

	// Health returns the health status of the producer
	Health() error
}

// EventConsumer handles event consumption
type EventConsumer interface {
	// Subscribe subscribes to events on a topic with a handler
	Subscribe(topic string, handler EventHandler) error

	// Start begins consuming events
	Start(ctx context.Context) error

	// Stop stops consuming events
	Stop() error

	// Close gracefully closes the consumer
	Close() error

	// Health returns the health status of the consumer
	Health() error
}

// EventHandler defines the signature for event handling functions
type EventHandler func(ctx context.Context, event *event.Event) error

// TriggerEventHandler defines the signature for trigger event handling
type TriggerEventHandler func(ctx context.Context, trigger *TriggerEvent) error

// RoostEventData represents the data payload for roost lifecycle events
type RoostEventData struct {
	Roost          RoostInfo         `json:"roost"`
	Chart          ChartInfo         `json:"chart,omitempty"`
	Phase          string            `json:"phase,omitempty"`
	PreviousPhase  string            `json:"previousPhase,omitempty"`
	Health         string            `json:"health,omitempty"`
	PreviousHealth string            `json:"previousHealth,omitempty"`
	HelmRelease    *HelmReleaseInfo  `json:"helmRelease,omitempty"`
	Error          string            `json:"error,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// RoostInfo contains basic roost identification
type RoostInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
	Tenant    string `json:"tenant,omitempty"`
}

// ChartInfo contains Helm chart information
type ChartInfo struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Repository string `json:"repository,omitempty"`
}

// HelmReleaseInfo contains Helm release details
type HelmReleaseInfo struct {
	Name         string     `json:"name"`
	Revision     int32      `json:"revision"`
	Status       string     `json:"status"`
	Description  string     `json:"description,omitempty"`
	LastDeployed *time.Time `json:"lastDeployed,omitempty"`
}

// TriggerEvent represents an external trigger event
type TriggerEvent struct {
	Type        string            `json:"type"`
	Target      RoostTarget       `json:"target"`
	Action      string            `json:"action"`
	Reason      string            `json:"reason,omitempty"`
	Parameters  map[string]string `json:"parameters,omitempty"`
	RequestedBy string            `json:"requestedBy,omitempty"`
}

// RoostTarget identifies the target roost for a trigger
type RoostTarget struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Selector  string `json:"selector,omitempty"` // For bulk operations
}

// EventMetrics defines metrics interface for the event system
type EventMetrics interface {
	// RecordEventPublished records a published event
	RecordEventPublished(ctx context.Context, eventType string, duration float64)

	// RecordEventConsumed records a consumed event
	RecordEventConsumed(ctx context.Context, eventType string, duration float64)

	// RecordEventError records an event processing error
	RecordEventError(ctx context.Context, eventType, errorType string)

	// RecordProducerHealth records producer health status
	RecordProducerHealth(ctx context.Context, healthy bool)

	// RecordConsumerHealth records consumer health status
	RecordConsumerHealth(ctx context.Context, healthy bool)

	// RecordConsumerLag records consumer lag
	RecordConsumerLag(ctx context.Context, topic string, lag int64)
}

// StorageProvider defines interface for event storage (S3, etc.)
type StorageProvider interface {
	// Store saves an event to storage
	Store(ctx context.Context, bucket, key string, data []byte) error

	// Retrieve gets an event from storage
	Retrieve(ctx context.Context, bucket, key string) ([]byte, error)

	// List lists events in storage with optional prefix
	List(ctx context.Context, bucket, prefix string) ([]string, error)

	// Delete removes an event from storage
	Delete(ctx context.Context, bucket, key string) error

	// Health checks storage connectivity
	Health(ctx context.Context) error
}

// DeadLetterQueue handles failed event processing
type DeadLetterQueue interface {
	// SendToDLQ sends a failed event to the dead letter queue
	SendToDLQ(ctx context.Context, originalEvent *event.Event, err error, retryCount int) error

	// ProcessDLQ processes events in the dead letter queue
	ProcessDLQ(ctx context.Context, handler EventHandler) error

	// GetDLQStats returns statistics about the DLQ
	GetDLQStats(ctx context.Context) (DLQStats, error)
}

// DLQStats contains dead letter queue statistics
type DLQStats struct {
	TotalMessages     int64            `json:"totalMessages"`
	MessagesByType    map[string]int64 `json:"messagesByType"`
	OldestMessage     *time.Time       `json:"oldestMessage,omitempty"`
	RetryableMessages int64            `json:"retryableMessages"`
}

// EventReplay handles historical event replay
type EventReplay interface {
	// StoreEvent stores an event for potential replay
	StoreEvent(ctx context.Context, event *event.Event) error

	// ReplayEvents replays events within a time range
	ReplayEvents(ctx context.Context, startTime, endTime time.Time, eventTypes []string, handler EventHandler) error

	// GetReplayStats returns replay statistics
	GetReplayStats(ctx context.Context) (ReplayStats, error)
}

// ReplayStats contains event replay statistics
type ReplayStats struct {
	TotalStoredEvents int64            `json:"totalStoredEvents"`
	EventsByType      map[string]int64 `json:"eventsByType"`
	StorageSize       int64            `json:"storageSize"`
	OldestEvent       *time.Time       `json:"oldestEvent,omitempty"`
	LatestEvent       *time.Time       `json:"latestEvent,omitempty"`
}

// SchemaValidator validates CloudEvents schemas
type SchemaValidator interface {
	// ValidateEvent validates a CloudEvent against its schema
	ValidateEvent(event *event.Event) error

	// RegisterEventType registers a new event type with its schema
	RegisterEventType(eventType string, schema []byte) error

	// GetSupportedTypes returns all supported event types
	GetSupportedTypes() []string
}

// Circuit breaker states for fault tolerance
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker provides fault tolerance for event operations
type CircuitBreaker interface {
	// Execute executes a function with circuit breaker protection
	Execute(fn func() error) error

	// State returns the current circuit state
	State() CircuitState

	// Reset manually resets the circuit breaker
	Reset()
}

// Event type constants for CloudEvents
const (
	// Roost lifecycle events
	EventTypeRoostCreated           = "io.roost-keeper.roost.lifecycle.created"
	EventTypeRoostUpdated           = "io.roost-keeper.roost.lifecycle.updated"
	EventTypeRoostDeleted           = "io.roost-keeper.roost.lifecycle.deleted"
	EventTypeRoostPhaseTransition   = "io.roost-keeper.roost.phase.transitioned"
	EventTypeRoostHealthChanged     = "io.roost-keeper.roost.health.changed"
	EventTypeRoostTeardownTriggered = "io.roost-keeper.roost.teardown.triggered"

	// External trigger events
	EventTypeTriggerTeardown = "io.roost-keeper.trigger.teardown"
	EventTypeTriggerScale    = "io.roost-keeper.trigger.scale"
	EventTypeTriggerUpdate   = "io.roost-keeper.trigger.update"
)

// CloudEvents source identifier
const EventSource = "roost-keeper-operator"

// CloudEvents specification version
const CloudEventsSpecVersion = "1.0"

// CloudEvents extension attributes
const (
	ExtensionTenant         = "roostkeeper-tenant"
	ExtensionCorrelationID  = "roostkeeper-correlationid"
	ExtensionOperation      = "roostkeeper-operation"
	ExtensionRetryCount     = "roostkeeper-retrycount"
	ExtensionRoostName      = "roostkeeper-roost-name"
	ExtensionRoostNamespace = "roostkeeper-roost-namespace"
	ExtensionRoostUID       = "roostkeeper-roost-uid"
)

// Topic names for Kafka
const (
	TopicLifecycle = "roost.lifecycle.v1"
	TopicTriggers  = "roost.triggers.v1"
)

// Error types for categorizing event errors
const (
	ErrorTypeValidation      = "validation"
	ErrorTypeProducer        = "producer"
	ErrorTypeConsumer        = "consumer"
	ErrorTypeStorage         = "storage"
	ErrorTypeDeserialization = "deserialization"
	ErrorTypeProcessing      = "processing"
)

// HealthStatus represents the health status of a roost
type HealthStatus struct {
	Status      string                 `json:"status"`
	LastChecked time.Time              `json:"lastChecked"`
	Checks      map[string]interface{} `json:"checks,omitempty"`
}

// GenerateEventID generates a unique event ID
func GenerateEventID() string {
	return fmt.Sprintf("event-%d", time.Now().UnixNano())
}

// GetCorrelationID retrieves correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value("correlation-id").(string); ok {
		return correlationID
	}
	return ""
}
