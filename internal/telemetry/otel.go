package telemetry

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

const (
	serviceName    = "roost-keeper"
	serviceVersion = "v0.1.0"
	outputDir      = "observability"
)

// ObservabilityProvider holds the OTEL providers and configuration
type ObservabilityProvider struct {
	TraceProvider  *sdktrace.TracerProvider
	MetricProvider *sdkmetric.MeterProvider
	Resource       *resource.Resource
	traceExporter  *TraceExporter
	metricExporter *MetricExporter
}

// InitOTEL initializes OpenTelemetry SDK with file-based exporters for local-otel compatibility
func InitOTEL(ctx context.Context) (*ObservabilityProvider, func(context.Context) error, error) {
	// Ensure output directories exist
	if err := ensureDirectories(); err != nil {
		return nil, nil, fmt.Errorf("failed to create output directories: %w", err)
	}

	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.ServiceNamespace("roost.birb.party"),
			semconv.K8SContainerName("roost-keeper-manager"),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize trace provider with file exporter
	traceExporter := NewTraceExporter(outputDir)
	traceProvider, err := initTraceProvider(res, traceExporter)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize trace provider: %w", err)
	}

	// Initialize metric provider with file exporter
	metricExporter := NewMetricExporter(outputDir)
	metricProvider, err := initMetricProvider(res, metricExporter)
	if err != nil {
		traceProvider.Shutdown(ctx)
		return nil, nil, fmt.Errorf("failed to initialize metric provider: %w", err)
	}

	// Set global providers
	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(metricProvider)

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	provider := &ObservabilityProvider{
		TraceProvider:  traceProvider,
		MetricProvider: metricProvider,
		Resource:       res,
		traceExporter:  traceExporter,
		metricExporter: metricExporter,
	}

	// Return shutdown function
	shutdown := func(ctx context.Context) error {
		var errs []error

		// Shutdown trace provider
		if err := traceProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown trace provider: %w", err))
		}

		// Shutdown metric provider
		if err := metricProvider.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown metric provider: %w", err))
		}

		// Shutdown exporters
		if err := traceExporter.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown trace exporter: %w", err))
		}

		if err := metricExporter.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown metric exporter: %w", err))
		}

		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}
		return nil
	}

	return provider, shutdown, nil
}

// ensureDirectories creates the required output directories
func ensureDirectories() error {
	dirs := []string{
		outputDir + "/traces",
		outputDir + "/metrics",
		outputDir + "/logs",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// initTraceProvider creates a trace provider with file-based exporter
func initTraceProvider(res *resource.Resource, exporter *TraceExporter) (*sdktrace.TracerProvider, error) {
	// Create trace provider with batch processor for performance
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(100),
			sdktrace.WithMaxQueueSize(1000),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Sample all traces in development
	)

	return tp, nil
}

// initMetricProvider creates a metric provider with file-based exporter
func initMetricProvider(res *resource.Resource, exporter *MetricExporter) (*sdkmetric.MeterProvider, error) {
	// Create metric provider with periodic reader
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(15*time.Second), // Export metrics every 15 seconds
		)),
	)

	return mp, nil
}

// ForceFlush forces all pending telemetry to be exported
func (p *ObservabilityProvider) ForceFlush(ctx context.Context) error {
	var errs []error

	// Force flush trace provider
	if err := p.TraceProvider.ForceFlush(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to flush trace provider: %w", err))
	}

	// Force flush metric provider
	if err := p.MetricProvider.ForceFlush(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to flush metric provider: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("flush errors: %v", errs)
	}

	return nil
}

// Meter returns the global meter for roost-keeper
func Meter() metric.Meter {
	return otel.Meter(serviceName)
}
