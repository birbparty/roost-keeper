package telemetry

import (
	"context"

	"github.com/go-logr/logr"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// NewLogger creates a structured logger with OTEL integration
func NewLogger() (logr.Logger, error) {
	// Use controller-runtime's zap logger with development mode
	logger := crzap.New(crzap.UseDevMode(true))
	return logger, nil
}

// FromContext returns a logger from the given context or creates a new one
func FromContext(ctx context.Context) logr.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(logr.Logger); ok {
		return logger
	}

	// Fallback to a new logger if none in context
	logger, err := NewLogger()
	if err != nil {
		// If even that fails, return a no-op logger
		return logr.Discard()
	}
	return logger
}

// ToContext adds the logger to the given context
func ToContext(ctx context.Context, logger logr.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

type loggerKey struct{}
