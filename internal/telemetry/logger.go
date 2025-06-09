package telemetry

import (
	"context"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap/zapcore"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// NewLogger creates a structured logger with OTEL integration and file output
func NewLogger() (logr.Logger, error) {
	// Create log file for local-otel compatibility
	logFile, err := os.OpenFile(
		filepath.Join("observability", "logs", "operator.jsonl"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		// Fallback to console only if file creation fails
		logger := crzap.New(crzap.UseDevMode(true))
		return logger, nil
	}

	// Configure encoder for local-otel JSON format
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create multi-writer (file + stdout for development)
	writer := zapcore.NewMultiWriteSyncer(
		zapcore.AddSync(logFile),
		zapcore.AddSync(os.Stdout),
	)

	// Convert to logr.Logger for controller-runtime compatibility
	logger := crzap.New(
		crzap.UseDevMode(true),
		crzap.Encoder(zapcore.NewJSONEncoder(encoderConfig)),
		crzap.WriteTo(writer),
		crzap.Level(zapcore.InfoLevel),
	)

	// Add default fields for service identification
	logger = logger.WithValues(
		"service", "roost-keeper",
		"version", "v0.1.0",
		"component", "operator",
	)

	return logger, nil
}

// WithTraceContext adds trace and span IDs to log fields
func WithTraceContext(ctx context.Context, logger logr.Logger) logr.Logger {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return logger
	}

	spanContext := span.SpanContext()
	return logger.WithValues(
		"trace_id", spanContext.TraceID().String(),
		"span_id", spanContext.SpanID().String(),
	)
}

// WithCorrelationID adds a correlation ID for request tracking
func WithCorrelationIDToLogger(ctx context.Context, logger logr.Logger) logr.Logger {
	if correlationID := getCorrelationID(ctx); correlationID != "" {
		return logger.WithValues("correlation_id", correlationID)
	}

	// Generate new correlation ID if none exists
	correlationID := uuid.New().String()
	return logger.WithValues("correlation_id", correlationID)
}

// WithOperatorContext adds operator-specific context fields
func WithOperatorContext(logger logr.Logger, roostName, namespace, operation string) logr.Logger {
	return logger.WithValues(
		"roost_name", roostName,
		"namespace", namespace,
		"operation", operation,
	)
}

// WithComponent adds component identification
func WithComponent(logger logr.Logger, component string) logr.Logger {
	return logger.WithValues("component", component)
}

// FromContext returns a logger from the given context or creates a new one
func FromContext(ctx context.Context) logr.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(logr.Logger); ok {
		// Enhance with trace context if available
		return WithTraceContext(ctx, logger)
	}

	// Fallback to a new logger if none in context
	logger, err := NewLogger()
	if err != nil {
		// If even that fails, return a no-op logger
		return logr.Discard()
	}

	// Enhance with trace context if available
	return WithTraceContext(ctx, logger)
}

// ToContext adds the logger to the given context
func ToContext(ctx context.Context, logger logr.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// WithCorrelationID adds a correlation ID to the context and returns both the new context and ID
func WithCorrelationID(ctx context.Context) (context.Context, string) {
	if correlationID := getCorrelationID(ctx); correlationID != "" {
		return ctx, correlationID
	}

	correlationID := uuid.New().String()
	return context.WithValue(ctx, correlationIDKey{}, correlationID), correlationID
}

// GetCorrelationID retrieves the correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	return getCorrelationID(ctx)
}

// getCorrelationID is the internal implementation
func getCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value(correlationIDKey{}).(string); ok {
		return correlationID
	}
	return ""
}

// LogError logs an error with enhanced context
func LogError(ctx context.Context, err error, msg string, keysAndValues ...interface{}) {
	logger := FromContext(ctx)

	// Add error details to log
	allValues := append([]interface{}{"error", err.Error()}, keysAndValues...)

	// Add trace context if available
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		spanContext := span.SpanContext()
		allValues = append(allValues,
			"trace_id", spanContext.TraceID().String(),
			"span_id", spanContext.SpanID().String(),
		)
	}

	// Add correlation ID if available
	if correlationID := getCorrelationID(ctx); correlationID != "" {
		allValues = append(allValues, "correlation_id", correlationID)
	}

	logger.Error(err, msg, allValues...)
}

// LogInfo logs an info message with enhanced context
func LogInfo(ctx context.Context, msg string, keysAndValues ...interface{}) {
	logger := FromContext(ctx)

	// Add trace context if available
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		spanContext := span.SpanContext()
		keysAndValues = append(keysAndValues,
			"trace_id", spanContext.TraceID().String(),
			"span_id", spanContext.SpanID().String(),
		)
	}

	// Add correlation ID if available
	if correlationID := getCorrelationID(ctx); correlationID != "" {
		keysAndValues = append(keysAndValues, "correlation_id", correlationID)
	}

	logger.Info(msg, keysAndValues...)
}

// LogDebug logs a debug message with enhanced context
func LogDebug(ctx context.Context, msg string, keysAndValues ...interface{}) {
	logger := FromContext(ctx)

	// Add trace context if available
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		spanContext := span.SpanContext()
		keysAndValues = append(keysAndValues,
			"trace_id", spanContext.TraceID().String(),
			"span_id", spanContext.SpanID().String(),
		)
	}

	// Add correlation ID if available
	if correlationID := getCorrelationID(ctx); correlationID != "" {
		keysAndValues = append(keysAndValues, "correlation_id", correlationID)
	}

	logger.V(1).Info(msg, keysAndValues...)
}

type loggerKey struct{}
type correlationIDKey struct{}
