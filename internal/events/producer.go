package events

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"github.com/cloudevents/sdk-go/v2/event"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// KafkaProducer implements EventProducer interface for Kafka
type KafkaProducer struct {
	config         *Config
	producer       sarama.SyncProducer
	asyncProducer  sarama.AsyncProducer
	client         sarama.Client
	k8sClient      client.Client
	kubeClient     kubernetes.Interface
	metrics        *EventMetricsImpl
	circuitBreaker CircuitBreaker
	logger         *zap.Logger
	mu             sync.RWMutex
	closed         bool
}

// NewKafkaProducer creates a new Kafka producer
func NewKafkaProducer(config *Config, k8sClient client.Client, kubeClient kubernetes.Interface, metrics *EventMetricsImpl, logger *zap.Logger) (*KafkaProducer, error) {
	producer := &KafkaProducer{
		config:     config,
		k8sClient:  k8sClient,
		kubeClient: kubeClient,
		metrics:    metrics,
		logger:     logger.With(zap.String("component", "kafka-producer")),
	}

	// Initialize circuit breaker
	if config.CircuitBreaker.Enabled {
		producer.circuitBreaker = NewCircuitBreaker(config.CircuitBreaker, logger)
	}

	// Initialize Kafka client and producers
	if err := producer.initializeKafka(); err != nil {
		return nil, fmt.Errorf("failed to initialize Kafka: %w", err)
	}

	return producer, nil
}

// initializeKafka sets up Kafka client and producers
func (p *KafkaProducer) initializeKafka() error {
	// Create Kafka configuration
	kafkaConfig, err := p.buildKafkaConfig()
	if err != nil {
		return fmt.Errorf("failed to build Kafka config: %w", err)
	}

	// Create Kafka client
	client, err := sarama.NewClient(p.config.Kafka.Brokers, kafkaConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kafka client: %w", err)
	}
	p.client = client

	// Create sync producer for critical events
	syncProducer, err := sarama.NewSyncProducerFromClient(client)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to create sync producer: %w", err)
	}
	p.producer = syncProducer

	// Create async producer for high-throughput scenarios
	asyncProducer, err := sarama.NewAsyncProducerFromClient(client)
	if err != nil {
		syncProducer.Close()
		client.Close()
		return fmt.Errorf("failed to create async producer: %w", err)
	}
	p.asyncProducer = asyncProducer

	// Start monitoring async producer
	go p.monitorAsyncProducer()

	p.logger.Info("Kafka producer initialized successfully")
	if p.metrics != nil {
		p.metrics.RecordProducerConnection(context.Background(), true)
	}

	return nil
}

// buildKafkaConfig creates Sarama configuration from our config
func (p *KafkaProducer) buildKafkaConfig() (*sarama.Config, error) {
	config := sarama.NewConfig()

	// Producer settings
	switch p.config.Kafka.Producer.Acks {
	case "0":
		config.Producer.RequiredAcks = sarama.NoResponse
	case "1":
		config.Producer.RequiredAcks = sarama.WaitForLocal
	case "all":
		config.Producer.RequiredAcks = sarama.WaitForAll
	}

	config.Producer.Retry.Max = p.config.Kafka.Producer.Retries
	config.Producer.Return.Successes = true
	config.Producer.Return.Errors = true
	config.Producer.Idempotent = p.config.Kafka.Producer.Idempotent
	config.Producer.Timeout = p.config.Kafka.Producer.RequestTimeout
	config.Producer.Flush.Frequency = p.config.Kafka.Producer.FlushFrequency

	// Batch settings
	config.Producer.Flush.Messages = int(p.config.Kafka.Producer.BatchSize / 1024) // Rough estimate
	config.Producer.Flush.Bytes = int(p.config.Kafka.Producer.BatchSize)

	// Compression
	switch p.config.Kafka.Producer.Compression {
	case "gzip":
		config.Producer.Compression = sarama.CompressionGZIP
	case "snappy":
		config.Producer.Compression = sarama.CompressionSnappy
	case "lz4":
		config.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		config.Producer.Compression = sarama.CompressionZSTD
	default:
		config.Producer.Compression = sarama.CompressionNone
	}

	// Net settings
	config.Net.DialTimeout = p.config.Kafka.Connection.DialTimeout
	config.Net.KeepAlive = p.config.Kafka.Connection.KeepAlive

	// Security settings
	if err := p.configureSecurity(config); err != nil {
		return nil, fmt.Errorf("failed to configure security: %w", err)
	}

	// Client ID
	config.ClientID = "roost-keeper-producer"

	// Version
	config.Version = sarama.V2_8_0_0

	return config, nil
}

// configureSecurity sets up security configuration
func (p *KafkaProducer) configureSecurity(config *sarama.Config) error {
	switch p.config.Kafka.Security.Protocol {
	case "PLAINTEXT":
		// No additional configuration needed
	case "SSL":
		config.Net.TLS.Enable = true
		if p.config.Kafka.Security.TLS.InsecureSkipVerify {
			config.Net.TLS.Config = &tls.Config{InsecureSkipVerify: true}
		}
	case "SASL_PLAINTEXT":
		if err := p.configureSASL(config); err != nil {
			return err
		}
	case "SASL_SSL":
		config.Net.TLS.Enable = true
		if p.config.Kafka.Security.TLS.InsecureSkipVerify {
			config.Net.TLS.Config = &tls.Config{InsecureSkipVerify: true}
		}
		if err := p.configureSASL(config); err != nil {
			return err
		}
	}

	return nil
}

// configureSASL sets up SASL authentication
func (p *KafkaProducer) configureSASL(config *sarama.Config) error {
	config.Net.SASL.Enable = true
	config.Net.SASL.User = p.config.Kafka.Security.Username

	// Get password from Kubernetes secret
	password, err := p.getSecretValue(p.config.Kafka.Security.PasswordRef)
	if err != nil {
		return fmt.Errorf("failed to get SASL password: %w", err)
	}
	config.Net.SASL.Password = password

	switch p.config.Kafka.Security.Mechanism {
	case "PLAIN":
		config.Net.SASL.Mechanism = sarama.SASLTypePlaintext
	case "SCRAM-SHA-256":
		config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
	case "SCRAM-SHA-512":
		config.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
	default:
		return fmt.Errorf("unsupported SASL mechanism: %s", p.config.Kafka.Security.Mechanism)
	}

	return nil
}

// getSecretValue retrieves a value from a Kubernetes secret
func (p *KafkaProducer) getSecretValue(secretRef SecretRef) (string, error) {
	ctx := context.Background()
	secret, err := p.kubeClient.CoreV1().Secrets("default").Get(ctx, secretRef.Name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s: %w", secretRef.Name, err)
	}

	value, exists := secret.Data[secretRef.Key]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s", secretRef.Key, secretRef.Name)
	}

	return string(value), nil
}

// PublishEvent publishes a CloudEvent to Kafka
func (p *KafkaProducer) PublishEvent(ctx context.Context, event *event.Event) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return fmt.Errorf("producer is closed")
	}
	p.mu.RUnlock()

	start := time.Now()
	var err error

	// Use circuit breaker if enabled
	if p.circuitBreaker != nil {
		err = p.circuitBreaker.Execute(func() error {
			return p.publishEvent(ctx, event)
		})
	} else {
		err = p.publishEvent(ctx, event)
	}

	// Record metrics
	if p.metrics != nil {
		duration := time.Since(start).Seconds()
		if err != nil {
			p.metrics.RecordEventError(ctx, event.Type(), ErrorTypeProducer)
		} else {
			p.metrics.RecordEventPublished(ctx, event.Type(), duration)
		}
	}

	return err
}

// publishEvent performs the actual event publishing
func (p *KafkaProducer) publishEvent(ctx context.Context, event *event.Event) error {
	// Add tracing information
	ctx, span := telemetry.StartGenericSpan(ctx, "event_publish", "kafka-producer")
	defer span.End()
	telemetry.AddSpanAttributes(span,
		attribute.String("event.id", event.ID()),
		attribute.String("event.type", event.Type()),
		attribute.String("event.source", event.Source()),
	)

	// Determine topic based on event type
	topic := p.getTopicForEvent(event)

	// Serialize the CloudEvent
	data, err := event.MarshalJSON()
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return fmt.Errorf("failed to serialize CloudEvent: %w", err)
	}

	// Create Kafka message
	message := &sarama.ProducerMessage{
		Topic: topic,
		Key:   sarama.StringEncoder(p.getPartitionKey(event)),
		Value: sarama.ByteEncoder(data),
		Headers: []sarama.RecordHeader{
			{Key: []byte("ce-specversion"), Value: []byte(event.SpecVersion())},
			{Key: []byte("ce-type"), Value: []byte(event.Type())},
			{Key: []byte("ce-source"), Value: []byte(event.Source())},
			{Key: []byte("ce-id"), Value: []byte(event.ID())},
			{Key: []byte("content-type"), Value: []byte("application/cloudevents+json")},
		},
	}

	// Add custom headers if present
	if correlationID := event.Extensions()[ExtensionCorrelationID]; correlationID != nil {
		if corrID, ok := correlationID.(string); ok {
			message.Headers = append(message.Headers, sarama.RecordHeader{
				Key:   []byte("ce-" + ExtensionCorrelationID),
				Value: []byte(corrID),
			})
		}
	}

	// Use sync producer for critical events
	if p.isCriticalEvent(event) {
		partition, offset, err := p.producer.SendMessage(message)
		if err != nil {
			telemetry.RecordSpanError(ctx, err)
			return fmt.Errorf("failed to send message to Kafka: %w", err)
		}

		p.logger.Debug("Event published successfully",
			zap.String("event_id", event.ID()),
			zap.String("event_type", event.Type()),
			zap.String("topic", topic),
			zap.Int32("partition", partition),
			zap.Int64("offset", offset))
	} else {
		// Use async producer for non-critical events
		select {
		case p.asyncProducer.Input() <- message:
			p.logger.Debug("Event queued for async publishing",
				zap.String("event_id", event.ID()),
				zap.String("event_type", event.Type()),
				zap.String("topic", topic))
		case <-ctx.Done():
			telemetry.RecordSpanError(ctx, ctx.Err())
			return ctx.Err()
		}
	}

	telemetry.RecordSpanSuccess(ctx)
	return nil
}

// getTopicForEvent determines the Kafka topic for an event
func (p *KafkaProducer) getTopicForEvent(event *event.Event) string {
	eventType := event.Type()

	switch {
	case isLifecycleEvent(eventType):
		return p.config.Topics.Lifecycle
	case isTriggerEvent(eventType):
		return p.config.Topics.Triggers
	default:
		return p.config.Topics.Lifecycle // Default fallback
	}
}

// getPartitionKey determines the partition key for an event
func (p *KafkaProducer) getPartitionKey(event *event.Event) string {
	// Use subject as partition key for roost events to ensure ordering
	if subject := event.Subject(); subject != "" {
		return subject
	}

	// Fallback to source
	return event.Source()
}

// isCriticalEvent determines if an event requires sync publishing
func (p *KafkaProducer) isCriticalEvent(event *event.Event) bool {
	eventType := event.Type()
	criticalEvents := map[string]bool{
		EventTypeRoostCreated:           true,
		EventTypeRoostDeleted:           true,
		EventTypeRoostTeardownTriggered: true,
		EventTypeTriggerTeardown:        true,
	}

	return criticalEvents[eventType]
}

// monitorAsyncProducer monitors async producer success/error channels
func (p *KafkaProducer) monitorAsyncProducer() {
	for {
		select {
		case success := <-p.asyncProducer.Successes():
			if success != nil {
				p.logger.Debug("Async event published successfully",
					zap.String("topic", success.Topic),
					zap.Int32("partition", success.Partition),
					zap.Int64("offset", success.Offset))
			}
		case err := <-p.asyncProducer.Errors():
			if err != nil {
				p.logger.Error("Async event publishing failed",
					zap.Error(err.Err),
					zap.String("topic", err.Msg.Topic),
					zap.Any("key", err.Msg.Key),
				)

				if p.metrics != nil {
					// Try to extract event type from headers
					eventType := "unknown"
					for _, header := range err.Msg.Headers {
						if string(header.Key) == "ce-type" {
							eventType = string(header.Value)
							break
						}
					}
					p.metrics.RecordEventError(context.Background(), eventType, ErrorTypeProducer)
				}
			}
		}

		// Check if producer is closed
		p.mu.RLock()
		if p.closed {
			p.mu.RUnlock()
			return
		}
		p.mu.RUnlock()
	}
}

// Health returns the health status of the producer
func (p *KafkaProducer) Health() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return fmt.Errorf("producer is closed")
	}

	if p.client == nil {
		return fmt.Errorf("kafka client is not initialized")
	}

	// Test connection by checking brokers
	brokers := p.client.Brokers()
	if len(brokers) == 0 {
		if p.metrics != nil {
			p.metrics.RecordProducerHealth(context.Background(), false)
		}
		return fmt.Errorf("no kafka brokers available")
	}

	// Check if at least one broker is connected
	connected := false
	for _, broker := range brokers {
		if isConnected, _ := broker.Connected(); isConnected {
			connected = true
			break
		}
	}

	if !connected {
		if p.metrics != nil {
			p.metrics.RecordProducerHealth(context.Background(), false)
		}
		return fmt.Errorf("no kafka brokers connected")
	}

	if p.metrics != nil {
		p.metrics.RecordProducerHealth(context.Background(), true)
	}

	return nil
}

// Close closes the producer
func (p *KafkaProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.logger.Info("Closing Kafka producer")

	var errors []error

	// Close async producer
	if p.asyncProducer != nil {
		if err := p.asyncProducer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close async producer: %w", err))
		}
	}

	// Close sync producer
	if p.producer != nil {
		if err := p.producer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close sync producer: %w", err))
		}
	}

	// Close client
	if p.client != nil {
		if err := p.client.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close client: %w", err))
		}
	}

	p.closed = true

	if p.metrics != nil {
		p.metrics.RecordProducerConnection(context.Background(), false)
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing producer: %v", errors)
	}

	p.logger.Info("Kafka producer closed successfully")
	return nil
}

// Helper functions
func isLifecycleEvent(eventType string) bool {
	lifecycleEvents := map[string]bool{
		EventTypeRoostCreated:           true,
		EventTypeRoostUpdated:           true,
		EventTypeRoostDeleted:           true,
		EventTypeRoostPhaseTransition:   true,
		EventTypeRoostHealthChanged:     true,
		EventTypeRoostTeardownTriggered: true,
	}
	return lifecycleEvents[eventType]
}

func isTriggerEvent(eventType string) bool {
	triggerEvents := map[string]bool{
		EventTypeTriggerTeardown: true,
		EventTypeTriggerScale:    true,
		EventTypeTriggerUpdate:   true,
	}
	return triggerEvents[eventType]
}
