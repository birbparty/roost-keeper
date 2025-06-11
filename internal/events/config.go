package events

import (
	"fmt"
	"time"
)

// Config defines the configuration for the event system
type Config struct {
	// Enable or disable the event system
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Kafka configuration
	Kafka KafkaConfig `yaml:"kafka" json:"kafka"`

	// Topic configuration
	Topics TopicConfig `yaml:"topics" json:"topics"`

	// Dead letter queue configuration
	DeadLetterQueue DLQConfig `yaml:"deadLetterQueue" json:"deadLetterQueue"`

	// Event replay configuration
	Replay ReplayConfig `yaml:"replay" json:"replay"`

	// Schema validation configuration
	SchemaValidation SchemaValidationConfig `yaml:"schemaValidation" json:"schemaValidation"`

	// Circuit breaker configuration
	CircuitBreaker CircuitBreakerConfig `yaml:"circuitBreaker" json:"circuitBreaker"`
}

// KafkaConfig defines Kafka-specific configuration
type KafkaConfig struct {
	// Kafka broker addresses
	Brokers []string `yaml:"brokers" json:"brokers"`

	// Security configuration
	Security SecurityConfig `yaml:"security" json:"security"`

	// Producer configuration
	Producer ProducerConfig `yaml:"producer" json:"producer"`

	// Consumer configuration
	Consumer ConsumerConfig `yaml:"consumer" json:"consumer"`

	// Connection configuration
	Connection ConnectionConfig `yaml:"connection" json:"connection"`
}

// SecurityConfig defines security settings for Kafka
type SecurityConfig struct {
	// Security protocol (PLAINTEXT, SSL, SASL_PLAINTEXT, SASL_SSL)
	Protocol string `yaml:"protocol" json:"protocol"`

	// SASL mechanism (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
	Mechanism string `yaml:"mechanism" json:"mechanism"`

	// Username for SASL authentication
	Username string `yaml:"username" json:"username"`

	// Password reference for SASL authentication
	PasswordRef SecretRef `yaml:"passwordRef" json:"passwordRef"`

	// SSL/TLS configuration
	TLS TLSConfig `yaml:"tls" json:"tls"`
}

// SecretRef defines a reference to a Kubernetes secret
type SecretRef struct {
	Name string `yaml:"name" json:"name"`
	Key  string `yaml:"key" json:"key"`
}

// TLSConfig defines TLS settings
type TLSConfig struct {
	// Enable TLS
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Skip TLS verification (for development only)
	InsecureSkipVerify bool `yaml:"insecureSkipVerify" json:"insecureSkipVerify"`

	// CA certificate bundle
	CABundle []byte `yaml:"caBundle,omitempty" json:"caBundle,omitempty"`

	// Client certificate reference
	ClientCertRef *SecretRef `yaml:"clientCertRef,omitempty" json:"clientCertRef,omitempty"`

	// Client key reference
	ClientKeyRef *SecretRef `yaml:"clientKeyRef,omitempty" json:"clientKeyRef,omitempty"`
}

// ProducerConfig defines Kafka producer settings
type ProducerConfig struct {
	// Required acknowledgments (0, 1, all)
	Acks string `yaml:"acks" json:"acks"`

	// Number of retries
	Retries int `yaml:"retries" json:"retries"`

	// Batch size in bytes
	BatchSize int32 `yaml:"batchSize" json:"batchSize"`

	// Linger time before sending batch
	LingerMs int32 `yaml:"lingerMs" json:"lingerMs"`

	// Compression type (none, gzip, snappy, lz4, zstd)
	Compression string `yaml:"compression" json:"compression"`

	// Enable idempotent producer
	Idempotent bool `yaml:"idempotent" json:"idempotent"`

	// Maximum message bytes
	MaxMessageBytes int32 `yaml:"maxMessageBytes" json:"maxMessageBytes"`

	// Request timeout
	RequestTimeout time.Duration `yaml:"requestTimeout" json:"requestTimeout"`

	// Flush frequency
	FlushFrequency time.Duration `yaml:"flushFrequency" json:"flushFrequency"`
}

// ConsumerConfig defines Kafka consumer settings
type ConsumerConfig struct {
	// Consumer group ID
	GroupID string `yaml:"groupId" json:"groupId"`

	// Auto offset reset (earliest, latest, none)
	AutoOffsetReset string `yaml:"autoOffsetReset" json:"autoOffsetReset"`

	// Enable auto commit
	EnableAutoCommit bool `yaml:"enableAutoCommit" json:"enableAutoCommit"`

	// Auto commit interval
	AutoCommitInterval time.Duration `yaml:"autoCommitInterval" json:"autoCommitInterval"`

	// Session timeout
	SessionTimeout time.Duration `yaml:"sessionTimeout" json:"sessionTimeout"`

	// Heartbeat interval
	HeartbeatInterval time.Duration `yaml:"heartbeatInterval" json:"heartbeatInterval"`

	// Fetch size configuration
	FetchMin int32 `yaml:"fetchMin" json:"fetchMin"`
	FetchMax int32 `yaml:"fetchMax" json:"fetchMax"`

	// Maximum processing time
	MaxProcessingTime time.Duration `yaml:"maxProcessingTime" json:"maxProcessingTime"`

	// Number of consumer instances
	Instances int `yaml:"instances" json:"instances"`
}

// ConnectionConfig defines connection settings
type ConnectionConfig struct {
	// Connection timeout
	DialTimeout time.Duration `yaml:"dialTimeout" json:"dialTimeout"`

	// Keep-alive settings
	KeepAlive time.Duration `yaml:"keepAlive" json:"keepAlive"`

	// Maximum idle connections
	MaxIdleConns int `yaml:"maxIdleConns" json:"maxIdleConns"`

	// Maximum open connections
	MaxOpenConns int `yaml:"maxOpenConns" json:"maxOpenConns"`

	// Connection retry configuration
	RetryBackoff time.Duration `yaml:"retryBackoff" json:"retryBackoff"`
	MaxRetries   int           `yaml:"maxRetries" json:"maxRetries"`
}

// TopicConfig defines topic settings
type TopicConfig struct {
	// Lifecycle events topic
	Lifecycle string `yaml:"lifecycle" json:"lifecycle"`

	// Trigger events topic
	Triggers string `yaml:"triggers" json:"triggers"`

	// Topic creation settings
	AutoCreate        bool  `yaml:"autoCreate" json:"autoCreate"`
	DefaultPartitions int32 `yaml:"defaultPartitions" json:"defaultPartitions"`
	ReplicationFactor int16 `yaml:"replicationFactor" json:"replicationFactor"`

	// Retention settings
	RetentionMs int64 `yaml:"retentionMs" json:"retentionMs"`
}

// DLQConfig defines dead letter queue configuration
type DLQConfig struct {
	// Enable dead letter queue
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Maximum retry attempts before sending to DLQ
	MaxRetries int `yaml:"maxRetries" json:"maxRetries"`

	// Retry backoff strategy
	RetryBackoff RetryBackoffConfig `yaml:"retryBackoff" json:"retryBackoff"`

	// Storage configuration for DLQ
	Storage StorageConfig `yaml:"storage" json:"storage"`

	// DLQ processing interval
	ProcessingInterval time.Duration `yaml:"processingInterval" json:"processingInterval"`

	// Message retention in DLQ
	MessageRetention time.Duration `yaml:"messageRetention" json:"messageRetention"`
}

// RetryBackoffConfig defines retry backoff strategy
type RetryBackoffConfig struct {
	// Initial delay
	InitialDelay time.Duration `yaml:"initialDelay" json:"initialDelay"`

	// Maximum delay
	MaxDelay time.Duration `yaml:"maxDelay" json:"maxDelay"`

	// Backoff multiplier
	Multiplier float64 `yaml:"multiplier" json:"multiplier"`

	// Enable jitter
	Jitter bool `yaml:"jitter" json:"jitter"`
}

// ReplayConfig defines event replay configuration
type ReplayConfig struct {
	// Enable event replay
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Event retention period for replay
	Retention time.Duration `yaml:"retention" json:"retention"`

	// Storage configuration for replay events
	Storage StorageConfig `yaml:"storage" json:"storage"`

	// Batch size for replay operations
	BatchSize int `yaml:"batchSize" json:"batchSize"`

	// Compression for stored events
	Compression bool `yaml:"compression" json:"compression"`
}

// StorageConfig defines storage backend configuration
type StorageConfig struct {
	// Storage type (s3, gcs, azure, local)
	Type string `yaml:"type" json:"type"`

	// Bucket name for cloud storage
	Bucket string `yaml:"bucket" json:"bucket"`

	// Region for cloud storage
	Region string `yaml:"region" json:"region"`

	// Endpoint URL (for S3-compatible storage like Digital Ocean Spaces)
	Endpoint string `yaml:"endpoint" json:"endpoint"`

	// Access key reference
	AccessKeyRef SecretRef `yaml:"accessKeyRef" json:"accessKeyRef"`

	// Secret key reference
	SecretKeyRef SecretRef `yaml:"secretKeyRef" json:"secretKeyRef"`

	// Path prefix for storage
	Prefix string `yaml:"prefix" json:"prefix"`

	// Enable server-side encryption
	Encryption bool `yaml:"encryption" json:"encryption"`

	// Custom configuration options
	Options map[string]string `yaml:"options,omitempty" json:"options,omitempty"`
}

// SchemaValidationConfig defines schema validation settings
type SchemaValidationConfig struct {
	// Enable schema validation
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Validation mode (strict, warn, disabled)
	Mode string `yaml:"mode" json:"mode"`

	// Schema registry configuration
	Registry SchemaRegistryConfig `yaml:"registry" json:"registry"`

	// Cache settings for schemas
	Cache SchemaCacheConfig `yaml:"cache" json:"cache"`
}

// SchemaRegistryConfig defines schema registry settings
type SchemaRegistryConfig struct {
	// Registry type (internal, confluent, custom)
	Type string `yaml:"type" json:"type"`

	// Registry URL for external registries
	URL string `yaml:"url,omitempty" json:"url,omitempty"`

	// Authentication for registry access
	Auth *RegistryAuthConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// RegistryAuthConfig defines authentication for schema registry
type RegistryAuthConfig struct {
	// Username for basic auth
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	// Password reference for basic auth
	PasswordRef *SecretRef `yaml:"passwordRef,omitempty" json:"passwordRef,omitempty"`

	// API key for key-based auth
	APIKeyRef *SecretRef `yaml:"apiKeyRef,omitempty" json:"apiKeyRef,omitempty"`
}

// SchemaCacheConfig defines schema caching settings
type SchemaCacheConfig struct {
	// Enable schema caching
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Cache TTL
	TTL time.Duration `yaml:"ttl" json:"ttl"`

	// Maximum cache size
	MaxSize int `yaml:"maxSize" json:"maxSize"`
}

// CircuitBreakerConfig defines circuit breaker settings
type CircuitBreakerConfig struct {
	// Enable circuit breaker
	Enabled bool `yaml:"enabled" json:"enabled"`

	// Failure threshold to open circuit
	FailureThreshold int `yaml:"failureThreshold" json:"failureThreshold"`

	// Success threshold to close circuit from half-open
	SuccessThreshold int `yaml:"successThreshold" json:"successThreshold"`

	// Timeout for half-open state
	HalfOpenTimeout time.Duration `yaml:"halfOpenTimeout" json:"halfOpenTimeout"`

	// Recovery timeout before attempting to close circuit
	RecoveryTimeout time.Duration `yaml:"recoveryTimeout" json:"recoveryTimeout"`

	// Maximum number of half-open calls
	MaxHalfOpenCalls int `yaml:"maxHalfOpenCalls" json:"maxHalfOpenCalls"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
		Kafka: KafkaConfig{
			Brokers: []string{"localhost:9092"},
			Security: SecurityConfig{
				Protocol:  "PLAINTEXT",
				Mechanism: "PLAIN",
			},
			Producer: ProducerConfig{
				Acks:            "all",
				Retries:         5,
				BatchSize:       16384,
				LingerMs:        100,
				Compression:     "gzip",
				Idempotent:      true,
				MaxMessageBytes: 1000000,
				RequestTimeout:  30 * time.Second,
				FlushFrequency:  10 * time.Second,
			},
			Consumer: ConsumerConfig{
				GroupID:            "roost-keeper-consumers",
				AutoOffsetReset:    "earliest",
				EnableAutoCommit:   false,
				AutoCommitInterval: 1 * time.Second,
				SessionTimeout:     30 * time.Second,
				HeartbeatInterval:  3 * time.Second,
				FetchMin:           1,
				FetchMax:           1048576,
				MaxProcessingTime:  300 * time.Second,
				Instances:          1,
			},
			Connection: ConnectionConfig{
				DialTimeout:  10 * time.Second,
				KeepAlive:    30 * time.Second,
				MaxIdleConns: 10,
				MaxOpenConns: 100,
				RetryBackoff: 1 * time.Second,
				MaxRetries:   5,
			},
		},
		Topics: TopicConfig{
			Lifecycle:         TopicLifecycle,
			Triggers:          TopicTriggers,
			AutoCreate:        true,
			DefaultPartitions: 3,
			ReplicationFactor: 1,
			RetentionMs:       604800000, // 7 days
		},
		DeadLetterQueue: DLQConfig{
			Enabled:    true,
			MaxRetries: 3,
			RetryBackoff: RetryBackoffConfig{
				InitialDelay: 1 * time.Second,
				MaxDelay:     60 * time.Second,
				Multiplier:   2.0,
				Jitter:       true,
			},
			Storage: StorageConfig{
				Type:   "s3",
				Bucket: "roost-keeper-dlq",
				Region: "us-east-1",
				Prefix: "dlq/",
			},
			ProcessingInterval: 5 * time.Minute,
			MessageRetention:   7 * 24 * time.Hour,
		},
		Replay: ReplayConfig{
			Enabled:     true,
			Retention:   7 * 24 * time.Hour,
			BatchSize:   100,
			Compression: true,
			Storage: StorageConfig{
				Type:   "s3",
				Bucket: "roost-keeper-events",
				Region: "us-east-1",
				Prefix: "events/",
			},
		},
		SchemaValidation: SchemaValidationConfig{
			Enabled: true,
			Mode:    "strict",
			Registry: SchemaRegistryConfig{
				Type: "internal",
			},
			Cache: SchemaCacheConfig{
				Enabled: true,
				TTL:     1 * time.Hour,
				MaxSize: 1000,
			},
		},
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:          true,
			FailureThreshold: 5,
			SuccessThreshold: 3,
			HalfOpenTimeout:  60 * time.Second,
			RecoveryTimeout:  30 * time.Second,
			MaxHalfOpenCalls: 10,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if disabled
	}

	// Validate Kafka configuration
	if err := c.validateKafka(); err != nil {
		return fmt.Errorf("kafka config validation failed: %w", err)
	}

	// Validate topics
	if err := c.validateTopics(); err != nil {
		return fmt.Errorf("topics config validation failed: %w", err)
	}

	// Validate storage configurations
	if c.DeadLetterQueue.Enabled {
		if err := c.validateStorage(&c.DeadLetterQueue.Storage, "DLQ"); err != nil {
			return fmt.Errorf("DLQ storage config validation failed: %w", err)
		}
	}

	if c.Replay.Enabled {
		if err := c.validateStorage(&c.Replay.Storage, "replay"); err != nil {
			return fmt.Errorf("replay storage config validation failed: %w", err)
		}
	}

	return nil
}

// validateKafka validates Kafka configuration
func (c *Config) validateKafka() error {
	if len(c.Kafka.Brokers) == 0 {
		return fmt.Errorf("at least one Kafka broker must be specified")
	}

	// Validate security settings
	validProtocols := map[string]bool{
		"PLAINTEXT":      true,
		"SSL":            true,
		"SASL_PLAINTEXT": true,
		"SASL_SSL":       true,
	}
	if !validProtocols[c.Kafka.Security.Protocol] {
		return fmt.Errorf("invalid security protocol: %s", c.Kafka.Security.Protocol)
	}

	// Validate SASL mechanism if SASL is enabled
	if c.Kafka.Security.Protocol == "SASL_PLAINTEXT" || c.Kafka.Security.Protocol == "SASL_SSL" {
		validMechanisms := map[string]bool{
			"PLAIN":         true,
			"SCRAM-SHA-256": true,
			"SCRAM-SHA-512": true,
		}
		if !validMechanisms[c.Kafka.Security.Mechanism] {
			return fmt.Errorf("invalid SASL mechanism: %s", c.Kafka.Security.Mechanism)
		}
	}

	// Validate producer settings
	validAcks := map[string]bool{"0": true, "1": true, "all": true}
	if !validAcks[c.Kafka.Producer.Acks] {
		return fmt.Errorf("invalid producer acks setting: %s", c.Kafka.Producer.Acks)
	}

	// Validate consumer settings
	validOffsetReset := map[string]bool{"earliest": true, "latest": true, "none": true}
	if !validOffsetReset[c.Kafka.Consumer.AutoOffsetReset] {
		return fmt.Errorf("invalid auto offset reset: %s", c.Kafka.Consumer.AutoOffsetReset)
	}

	if c.Kafka.Consumer.GroupID == "" {
		return fmt.Errorf("consumer group ID cannot be empty")
	}

	return nil
}

// validateTopics validates topic configuration
func (c *Config) validateTopics() error {
	if c.Topics.Lifecycle == "" {
		return fmt.Errorf("lifecycle topic name cannot be empty")
	}
	if c.Topics.Triggers == "" {
		return fmt.Errorf("triggers topic name cannot be empty")
	}
	if c.Topics.DefaultPartitions <= 0 {
		return fmt.Errorf("default partitions must be greater than 0")
	}
	if c.Topics.ReplicationFactor <= 0 {
		return fmt.Errorf("replication factor must be greater than 0")
	}
	return nil
}

// validateStorage validates storage configuration
func (c *Config) validateStorage(storage *StorageConfig, context string) error {
	validTypes := map[string]bool{"s3": true, "gcs": true, "azure": true, "local": true}
	if !validTypes[storage.Type] {
		return fmt.Errorf("invalid storage type for %s: %s", context, storage.Type)
	}

	if storage.Type != "local" && storage.Bucket == "" {
		return fmt.Errorf("bucket name is required for %s storage type %s", context, storage.Type)
	}

	return nil
}
