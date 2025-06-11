package integration

import (
	"context"
	"testing"
	"time"

	"github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEventDrivenIntegration(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	// Create fake clients
	testScheme := runtime.NewScheme()
	v1alpha1.AddToScheme(testScheme)
	k8sClient := fake.NewClientBuilder().WithScheme(testScheme).Build()
	kubeClient := kubefake.NewSimpleClientset()

	t.Run("TestEventManagerCreation", func(t *testing.T) {
		config := events.DefaultConfig()
		config.Enabled = false // Disable Kafka for unit test

		manager, err := events.NewManager(config, k8sClient, kubeClient, logger)
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.False(t, manager.IsEnabled())
	})

	t.Run("TestEventManagerWithEnabledConfig", func(t *testing.T) {
		config := &events.Config{
			Enabled: true,
			Kafka: events.KafkaConfig{
				Brokers: []string{"localhost:9092"},
				Security: events.SecurityConfig{
					Protocol: "PLAINTEXT",
				},
				Producer: events.ProducerConfig{
					Acks:           "1",
					Retries:        3,
					BatchSize:      1024,
					LingerMs:       100,
					RequestTimeout: 10 * time.Second,
				},
				Consumer: events.ConsumerConfig{
					GroupID:         "test-group",
					AutoOffsetReset: "latest",
					Instances:       1,
				},
			},
			Topics: events.TopicConfig{
				Lifecycle: "test-lifecycle",
				Triggers:  "test-triggers",
			},
		}

		// This will create a manager but won't actually connect to Kafka
		// since we're using stub implementations
		manager, err := events.NewManager(config, k8sClient, kubeClient, logger)
		require.NoError(t, err)
		assert.NotNil(t, manager)
		assert.True(t, manager.IsEnabled())

		// Test metrics
		metrics := manager.GetMetrics()
		assert.NotNil(t, metrics)
	})

	t.Run("TestEventPublishing", func(t *testing.T) {
		config := events.DefaultConfig()
		config.Enabled = false // Disable for testing

		manager, err := events.NewManager(config, k8sClient, kubeClient, logger)
		require.NoError(t, err)

		// Create a test ManagedRoost
		roost := &v1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
				UID:       "test-uid",
			},
			Spec: v1alpha1.ManagedRoostSpec{
				Chart: v1alpha1.ChartSpec{
					Name:    "test-chart",
					Version: "1.0.0",
				},
			},
		}

		// Test publishing events (these should be no-ops when disabled)
		err = manager.PublishRoostCreated(ctx, roost)
		assert.NoError(t, err)

		err = manager.PublishRoostUpdated(ctx, roost, roost)
		assert.NoError(t, err)

		err = manager.PublishRoostDeleted(ctx, roost)
		assert.NoError(t, err)

		err = manager.PublishRoostPhaseTransition(ctx, roost, "pending", "running")
		assert.NoError(t, err)

		healthStatus := events.HealthStatus{
			Status:      "healthy",
			LastChecked: time.Now(),
		}
		err = manager.PublishRoostHealthChanged(ctx, roost, healthStatus)
		assert.NoError(t, err)

		err = manager.PublishRoostTeardownTriggered(ctx, roost, "test reason")
		assert.NoError(t, err)

		err = manager.PublishTriggerTeardown(ctx, roost, "manual trigger")
		assert.NoError(t, err)

		err = manager.PublishTriggerScale(ctx, roost, 3)
		assert.NoError(t, err)

		updateData := map[string]interface{}{
			"update_type": "config_change",
		}
		err = manager.PublishTriggerUpdate(ctx, roost, updateData)
		assert.NoError(t, err)
	})

	t.Run("TestConfigValidation", func(t *testing.T) {
		// Test valid config
		config := events.DefaultConfig()
		err := config.Validate()
		assert.NoError(t, err)

		// Test invalid config - empty brokers
		invalidConfig := events.DefaultConfig()
		invalidConfig.Kafka.Brokers = []string{}
		err = invalidConfig.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "at least one Kafka broker")

		// Test invalid protocol
		invalidConfig = events.DefaultConfig()
		invalidConfig.Kafka.Security.Protocol = "INVALID"
		err = invalidConfig.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid security protocol")

		// Test invalid acks setting
		invalidConfig = events.DefaultConfig()
		invalidConfig.Kafka.Producer.Acks = "invalid"
		err = invalidConfig.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid producer acks")

		// Test empty consumer group
		invalidConfig = events.DefaultConfig()
		invalidConfig.Kafka.Consumer.GroupID = ""
		err = invalidConfig.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "consumer group ID cannot be empty")
	})

	t.Run("TestEventTypes", func(t *testing.T) {
		// Test event type constants
		assert.Equal(t, "io.roost-keeper.roost.lifecycle.created", events.EventTypeRoostCreated)
		assert.Equal(t, "io.roost-keeper.roost.lifecycle.updated", events.EventTypeRoostUpdated)
		assert.Equal(t, "io.roost-keeper.roost.lifecycle.deleted", events.EventTypeRoostDeleted)
		assert.Equal(t, "io.roost-keeper.trigger.teardown", events.EventTypeTriggerTeardown)
		assert.Equal(t, "io.roost-keeper.trigger.scale", events.EventTypeTriggerScale)
		assert.Equal(t, "io.roost-keeper.trigger.update", events.EventTypeTriggerUpdate)

		// Test event source
		assert.Equal(t, "roost-keeper-operator", events.EventSource)

		// Test CloudEvents spec version
		assert.Equal(t, "1.0", events.CloudEventsSpecVersion)
	})

	t.Run("TestUtilityFunctions", func(t *testing.T) {
		// Test event ID generation
		id1 := events.GenerateEventID()
		id2 := events.GenerateEventID()
		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
		assert.NotEqual(t, id1, id2) // Should be unique

		// Test correlation ID context
		correlationID := events.GetCorrelationID(context.Background())
		assert.Empty(t, correlationID) // Should be empty for context without correlation ID

		// Test with correlation ID in context
		ctxWithCorrelation := context.WithValue(context.Background(), "correlation-id", "test-correlation-123")
		correlationID = events.GetCorrelationID(ctxWithCorrelation)
		assert.Equal(t, "test-correlation-123", correlationID)
	})

	t.Run("TestCircuitBreakerConfig", func(t *testing.T) {
		config := events.DefaultConfig()

		// Test circuit breaker configuration
		assert.True(t, config.CircuitBreaker.Enabled)
		assert.Equal(t, 5, config.CircuitBreaker.FailureThreshold)
		assert.Equal(t, 3, config.CircuitBreaker.SuccessThreshold)
		assert.Equal(t, 60*time.Second, config.CircuitBreaker.HalfOpenTimeout)
		assert.Equal(t, 30*time.Second, config.CircuitBreaker.RecoveryTimeout)
		assert.Equal(t, 10, config.CircuitBreaker.MaxHalfOpenCalls)
	})

	t.Run("TestTopicConfiguration", func(t *testing.T) {
		config := events.DefaultConfig()

		// Test topic configuration
		assert.Equal(t, events.TopicLifecycle, config.Topics.Lifecycle)
		assert.Equal(t, events.TopicTriggers, config.Topics.Triggers)
		assert.True(t, config.Topics.AutoCreate)
		assert.Equal(t, int32(3), config.Topics.DefaultPartitions)
		assert.Equal(t, int16(1), config.Topics.ReplicationFactor)
		assert.Equal(t, int64(604800000), config.Topics.RetentionMs) // 7 days
	})

	t.Run("TestStorageConfiguration", func(t *testing.T) {
		config := events.DefaultConfig()

		// Test DLQ storage configuration
		assert.True(t, config.DeadLetterQueue.Enabled)
		assert.Equal(t, "s3", config.DeadLetterQueue.Storage.Type)
		assert.Equal(t, "roost-keeper-dlq", config.DeadLetterQueue.Storage.Bucket)
		assert.Equal(t, "us-east-1", config.DeadLetterQueue.Storage.Region)
		assert.Equal(t, "dlq/", config.DeadLetterQueue.Storage.Prefix)

		// Test replay storage configuration
		assert.True(t, config.Replay.Enabled)
		assert.Equal(t, "s3", config.Replay.Storage.Type)
		assert.Equal(t, "roost-keeper-events", config.Replay.Storage.Bucket)
		assert.Equal(t, "us-east-1", config.Replay.Storage.Region)
		assert.Equal(t, "events/", config.Replay.Storage.Prefix)
	})

	t.Run("TestMetricsImplementation", func(t *testing.T) {
		metrics, err := events.NewEventMetrics()
		require.NoError(t, err)
		assert.NotNil(t, metrics)

		// Test metric recording (these should not error)
		ctx := context.Background()

		metrics.RecordEventPublished(ctx, "test-event", 0.5)
		metrics.RecordEventConsumed(ctx, "test-event", 0.3)
		metrics.RecordEventError(ctx, "test-event", "test-error")
		metrics.RecordProducerHealth(ctx, true)
		metrics.RecordConsumerHealth(ctx, true)
		metrics.RecordConsumerLag(ctx, "test-topic", 10)
		metrics.RecordDLQMessage(ctx, "test-event", "processing-error")
		metrics.RecordDLQProcessing(ctx, 1.0, true)
		metrics.RecordDLQRetry(ctx, "test-event", 1)
		metrics.UpdateDLQSize(ctx, 5)

		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now()
		metrics.RecordEventReplay(ctx, startTime, endTime, 100, 5.0, true)
		metrics.RecordSchemaValidation(ctx, "test-event", 0.1, true)
		metrics.RecordStorageOperation(ctx, "put", "s3", 0.2, true)
		metrics.UpdateStorageStats(ctx, "s3", 1000, 1024*1024)
	})
}

func TestCircuitBreakerImplementation(t *testing.T) {
	logger := zaptest.NewLogger(t)

	config := events.CircuitBreakerConfig{
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 2,
		HalfOpenTimeout:  100 * time.Millisecond,
		RecoveryTimeout:  200 * time.Millisecond,
		MaxHalfOpenCalls: 5,
	}

	cb := events.NewCircuitBreaker(config, logger)
	assert.NotNil(t, cb)

	t.Run("TestCircuitBreakerStates", func(t *testing.T) {
		// Initially should be closed
		assert.Equal(t, events.CircuitClosed, cb.State())

		// Test successful execution
		err := cb.Execute(func() error { return nil })
		assert.NoError(t, err)
		assert.Equal(t, events.CircuitClosed, cb.State())

		// Test failures to open circuit
		for i := 0; i < 3; i++ {
			err := cb.Execute(func() error { return assert.AnError })
			assert.Error(t, err)
		}
		assert.Equal(t, events.CircuitOpen, cb.State())

		// Should reject calls when open
		err = cb.Execute(func() error { return nil })
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circuit breaker is open")
	})

	t.Run("TestCircuitBreakerReset", func(t *testing.T) {
		cb.Reset()
		assert.Equal(t, events.CircuitClosed, cb.State())

		// Should work normally after reset
		err := cb.Execute(func() error { return nil })
		assert.NoError(t, err)
	})
}
