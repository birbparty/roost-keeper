package events

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/birbparty/roost-keeper/api/v1alpha1"
)

// Manager coordinates all event-driven integration components
type Manager struct {
	config     *Config
	producer   EventProducer
	consumers  []EventConsumer
	metrics    *EventMetricsImpl
	k8sClient  client.Client
	kubeClient kubernetes.Interface
	logger     *zap.Logger
	started    bool
	mu         sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewManager creates a new event manager
func NewManager(config *Config, k8sClient client.Client, kubeClient kubernetes.Interface, logger *zap.Logger) (*Manager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid event configuration: %w", err)
	}

	// Initialize metrics
	metrics, err := NewEventMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize event metrics: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		config:     config,
		metrics:    metrics,
		k8sClient:  k8sClient,
		kubeClient: kubeClient,
		logger:     logger.With(zap.String("component", "event-manager")),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Initialize producer if enabled
	if config.Enabled {
		if err := manager.initializeProducer(); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize producer: %w", err)
		}

		// Initialize consumers if enabled
		if err := manager.initializeConsumers(); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to initialize consumers: %w", err)
		}
	}

	return manager, nil
}

// AddToManager adds the event manager to the controller-runtime manager
func (m *Manager) AddToManager(mgr manager.Manager) error {
	return mgr.Add(m)
}

// Start starts the event manager
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return nil
	}

	if !m.config.Enabled {
		m.logger.Info("Event system is disabled")
		return nil
	}

	m.logger.Info("Starting event manager")

	// Start consumers
	for i, consumer := range m.consumers {
		go func(idx int, c EventConsumer) {
			if err := c.Start(m.ctx); err != nil {
				m.logger.Error("Consumer failed to start",
					zap.Int("consumer_index", idx),
					zap.Error(err))
			}
		}(i, consumer)
	}

	// Start health monitoring
	go m.monitorHealth(ctx)

	m.started = true
	m.logger.Info("Event manager started successfully")

	// Block until context is cancelled
	<-ctx.Done()
	return nil
}

// Stop stops the event manager
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	m.logger.Info("Stopping event manager")

	// Cancel context to stop consumers
	m.cancel()

	var errors []error

	// Stop consumers
	for i, consumer := range m.consumers {
		if err := consumer.Stop(); err != nil {
			errors = append(errors, fmt.Errorf("failed to stop consumer %d: %w", i, err))
		}
	}

	// Close producer
	if m.producer != nil {
		if err := m.producer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close producer: %w", err))
		}
	}

	m.started = false

	if len(errors) > 0 {
		return fmt.Errorf("errors stopping event manager: %v", errors)
	}

	m.logger.Info("Event manager stopped successfully")
	return nil
}

// PublishRoostCreated publishes a roost created event
func (m *Manager) PublishRoostCreated(ctx context.Context, roost *v1alpha1.ManagedRoost) error {
	return m.publishLifecycleEvent(ctx, EventTypeRoostCreated, roost, nil)
}

// PublishRoostUpdated publishes a roost updated event
func (m *Manager) PublishRoostUpdated(ctx context.Context, roost *v1alpha1.ManagedRoost, oldRoost *v1alpha1.ManagedRoost) error {
	data := map[string]interface{}{
		"previous": oldRoost,
	}
	return m.publishLifecycleEvent(ctx, EventTypeRoostUpdated, roost, data)
}

// PublishRoostDeleted publishes a roost deleted event
func (m *Manager) PublishRoostDeleted(ctx context.Context, roost *v1alpha1.ManagedRoost) error {
	return m.publishLifecycleEvent(ctx, EventTypeRoostDeleted, roost, nil)
}

// PublishRoostPhaseTransition publishes a phase transition event
func (m *Manager) PublishRoostPhaseTransition(ctx context.Context, roost *v1alpha1.ManagedRoost, oldPhase, newPhase string) error {
	data := map[string]interface{}{
		"oldPhase": oldPhase,
		"newPhase": newPhase,
	}
	return m.publishLifecycleEvent(ctx, EventTypeRoostPhaseTransition, roost, data)
}

// PublishRoostHealthChanged publishes a health status change event
func (m *Manager) PublishRoostHealthChanged(ctx context.Context, roost *v1alpha1.ManagedRoost, healthStatus HealthStatus) error {
	data := map[string]interface{}{
		"healthStatus": healthStatus,
	}
	return m.publishLifecycleEvent(ctx, EventTypeRoostHealthChanged, roost, data)
}

// PublishRoostTeardownTriggered publishes a teardown triggered event
func (m *Manager) PublishRoostTeardownTriggered(ctx context.Context, roost *v1alpha1.ManagedRoost, reason string) error {
	data := map[string]interface{}{
		"reason": reason,
	}
	return m.publishLifecycleEvent(ctx, EventTypeRoostTeardownTriggered, roost, data)
}

// PublishTriggerTeardown publishes a teardown trigger event
func (m *Manager) PublishTriggerTeardown(ctx context.Context, roost *v1alpha1.ManagedRoost, reason string) error {
	data := map[string]interface{}{
		"reason": reason,
	}
	return m.publishTriggerEvent(ctx, EventTypeTriggerTeardown, roost, data)
}

// PublishTriggerScale publishes a scale trigger event
func (m *Manager) PublishTriggerScale(ctx context.Context, roost *v1alpha1.ManagedRoost, replicas int32) error {
	data := map[string]interface{}{
		"replicas": replicas,
	}
	return m.publishTriggerEvent(ctx, EventTypeTriggerScale, roost, data)
}

// PublishTriggerUpdate publishes an update trigger event
func (m *Manager) PublishTriggerUpdate(ctx context.Context, roost *v1alpha1.ManagedRoost, updateData map[string]interface{}) error {
	return m.publishTriggerEvent(ctx, EventTypeTriggerUpdate, roost, updateData)
}

// Health returns the health status of the event system
func (m *Manager) Health() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.config.Enabled {
		return nil // Event system is disabled, so it's healthy
	}

	if !m.started {
		return fmt.Errorf("event manager is not started")
	}

	// Check producer health
	if m.producer != nil {
		if err := m.producer.Health(); err != nil {
			return fmt.Errorf("producer unhealthy: %w", err)
		}
	}

	// Check consumer health
	for i, consumer := range m.consumers {
		if err := consumer.Health(); err != nil {
			return fmt.Errorf("consumer %d unhealthy: %w", i, err)
		}
	}

	return nil
}

// GetMetrics returns the event metrics
func (m *Manager) GetMetrics() *EventMetricsImpl {
	return m.metrics
}

// IsEnabled returns whether the event system is enabled
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}

// initializeProducer sets up the event producer
func (m *Manager) initializeProducer() error {
	producer, err := NewKafkaProducer(m.config, m.k8sClient, m.kubeClient, m.metrics, m.logger)
	if err != nil {
		return fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	m.producer = producer
	m.logger.Info("Event producer initialized")
	return nil
}

// initializeConsumers sets up event consumers
func (m *Manager) initializeConsumers() error {
	// Create consumers based on configuration
	consumerCount := m.config.Kafka.Consumer.Instances
	if consumerCount <= 0 {
		consumerCount = 1
	}

	m.consumers = make([]EventConsumer, consumerCount)

	for i := 0; i < consumerCount; i++ {
		consumer, err := NewKafkaConsumer(m.config, m.k8sClient, m.kubeClient, m.metrics, m.logger)
		if err != nil {
			return fmt.Errorf("failed to create consumer %d: %w", i, err)
		}
		m.consumers[i] = consumer
	}

	m.logger.Info("Event consumers initialized", zap.Int("count", consumerCount))
	return nil
}

// publishLifecycleEvent publishes a lifecycle event
func (m *Manager) publishLifecycleEvent(ctx context.Context, eventType string, roost *v1alpha1.ManagedRoost, data map[string]interface{}) error {
	if !m.config.Enabled || m.producer == nil {
		return nil // Event system disabled
	}

	event, err := m.createRoostEvent(eventType, roost, data)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	return m.producer.PublishEvent(ctx, event)
}

// publishTriggerEvent publishes a trigger event
func (m *Manager) publishTriggerEvent(ctx context.Context, eventType string, roost *v1alpha1.ManagedRoost, data map[string]interface{}) error {
	if !m.config.Enabled || m.producer == nil {
		return nil // Event system disabled
	}

	event, err := m.createRoostEvent(eventType, roost, data)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	return m.producer.PublishEvent(ctx, event)
}

// createRoostEvent creates a CloudEvent for a roost
func (m *Manager) createRoostEvent(eventType string, roost *v1alpha1.ManagedRoost, data map[string]interface{}) (*event.Event, error) {
	e := event.New()
	e.SetSpecVersion(CloudEventsSpecVersion)
	e.SetType(eventType)
	e.SetSource(EventSource)
	e.SetID(GenerateEventID())
	e.SetTime(time.Now())
	e.SetSubject(fmt.Sprintf("roost/%s/%s", roost.Namespace, roost.Name))

	// Set extension attributes
	e.SetExtension(ExtensionRoostName, roost.Name)
	e.SetExtension(ExtensionRoostNamespace, roost.Namespace)
	e.SetExtension(ExtensionRoostUID, string(roost.UID))

	// Add correlation ID if in context
	if correlationID := GetCorrelationID(context.Background()); correlationID != "" {
		e.SetExtension(ExtensionCorrelationID, correlationID)
	}

	// Prepare event data
	eventData := map[string]interface{}{
		"roost": roost,
	}

	// Add additional data if provided
	if data != nil {
		for k, v := range data {
			eventData[k] = v
		}
	}

	if err := e.SetData("application/json", eventData); err != nil {
		return nil, fmt.Errorf("failed to set event data: %w", err)
	}

	return &e, nil
}

// monitorHealth periodically checks system health and updates metrics
func (m *Manager) monitorHealth(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkAndRecordHealth()
		}
	}
}

// checkAndRecordHealth checks component health and records metrics
func (m *Manager) checkAndRecordHealth() {
	ctx := context.Background()

	// Check producer health
	if m.producer != nil {
		healthy := m.producer.Health() == nil
		m.metrics.RecordProducerHealth(ctx, healthy)
	}

	// Check consumer health
	for _, consumer := range m.consumers {
		healthy := consumer.Health() == nil
		m.metrics.RecordConsumerHealth(ctx, healthy)
	}
}

// NeedsLeaderElection returns false as events don't require leader election
func (m *Manager) NeedsLeaderElection() bool {
	return false
}
