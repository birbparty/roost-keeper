package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	serviceName    = "roost-keeper"
	serviceVersion = "v0.1.0"
)

// InitOTEL initializes OpenTelemetry SDK with traces and metrics
func InitOTEL(ctx context.Context) (func(context.Context) error, error) {
	// Create resource with service information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.ServiceNamespace("roost.birb.party"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Initialize trace provider
	traceShutdown, err := initTraceProvider(ctx, res)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize trace provider: %w", err)
	}

	// Initialize metric provider
	metricShutdown, err := initMetricProvider(ctx, res)
	if err != nil {
		traceShutdown(ctx)
		return nil, fmt.Errorf("failed to initialize metric provider: %w", err)
	}

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Return shutdown function
	return func(ctx context.Context) error {
		var errs []error
		if err := traceShutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown trace provider: %w", err))
		}
		if err := metricShutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to shutdown metric provider: %w", err))
		}
		if len(errs) > 0 {
			return fmt.Errorf("shutdown errors: %v", errs)
		}
		return nil
	}, nil
}

func initTraceProvider(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	// Create OTLP HTTP exporter for traces
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint("http://localhost:4318"),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}

	// Create trace provider with batch processor
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(100),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

func initMetricProvider(ctx context.Context, res *resource.Resource) (func(context.Context) error, error) {
	// Create Prometheus exporter for metrics
	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics exporter: %w", err)
	}

	// Create metric provider
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(exporter),
	)

	otel.SetMeterProvider(mp)

	return mp.Shutdown, nil
}

// Tracer returns the global tracer for roost-keeper
func Tracer() trace.Tracer {
	return otel.Tracer(serviceName)
}

// Meter returns the global meter for roost-keeper
func Meter() metric.Meter {
	return otel.Meter(serviceName)
}
