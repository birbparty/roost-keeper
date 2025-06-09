package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "roost-keeper-operator"
)

// StartControllerSpan creates a span for controller operations
func StartControllerSpan(ctx context.Context, operationName string, roostName, namespace string) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)

	return tracer.Start(ctx, operationName,
		trace.WithAttributes(
			attribute.String("controller.operation", operationName),
			attribute.String("roost.name", roostName),
			attribute.String("roost.namespace", namespace),
			attribute.String("component", "controller"),
		),
	)
}

// StartHelmSpan creates a span for Helm operations
func StartHelmSpan(ctx context.Context, operation, chartName, chartVersion string) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)

	return tracer.Start(ctx, "helm."+operation,
		trace.WithAttributes(
			attribute.String("helm.operation", operation),
			attribute.String("helm.chart.name", chartName),
			attribute.String("helm.chart.version", chartVersion),
			attribute.String("component", "helm"),
		),
	)
}

// StartHealthCheckSpan creates a span for health check operations
func StartHealthCheckSpan(ctx context.Context, checkType, target string) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)

	return tracer.Start(ctx, "health_check."+checkType,
		trace.WithAttributes(
			attribute.String("health_check.type", checkType),
			attribute.String("health_check.target", target),
			attribute.String("component", "health_check"),
		),
	)
}

// StartKubernetesAPISpan creates a span for Kubernetes API operations
func StartKubernetesAPISpan(ctx context.Context, method, resource, namespace string) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)

	return tracer.Start(ctx, fmt.Sprintf("k8s.%s.%s", method, resource),
		trace.WithAttributes(
			attribute.String("k8s.method", method),
			attribute.String("k8s.resource", resource),
			attribute.String("k8s.namespace", namespace),
			attribute.String("component", "kubernetes_api"),
		),
	)
}

// StartGenericSpan creates a generic span with basic attributes
func StartGenericSpan(ctx context.Context, operationName, component string) (context.Context, trace.Span) {
	tracer := otel.Tracer(tracerName)

	return tracer.Start(ctx, operationName,
		trace.WithAttributes(
			attribute.String("operation", operationName),
			attribute.String("component", component),
		),
	)
}

// RecordError records an error in the current span
func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// RecordSuccess marks the span as successful
func RecordSuccess(span trace.Span) {
	span.SetStatus(codes.Ok, "")
}

// AddSpanAttributes adds attributes to the current span
func AddSpanAttributes(span trace.Span, attrs ...attribute.KeyValue) {
	span.SetAttributes(attrs...)
}

// AddSpanEvent adds an event to the current span
func AddSpanEvent(span trace.Span, name string, attrs ...attribute.KeyValue) {
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SpanFromContext returns the current span from context
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan returns a context with the given span
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// TraceID returns the trace ID from the current span context
func TraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// SpanID returns the span ID from the current span context
func SpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// WithSpanAttributes is a convenience function to add attributes to the current span
func WithSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// WithSpanEvent is a convenience function to add an event to the current span
func WithSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// RecordSpanError is a convenience function to record an error in the current span
func RecordSpanError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		RecordError(span, err)
	}
}

// RecordSpanSuccess is a convenience function to mark the current span as successful
func RecordSpanSuccess(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		RecordSuccess(span)
	}
}

// InstrumentedFunction wraps a function with tracing
func InstrumentedFunction(ctx context.Context, operationName, component string, fn func(context.Context) error) error {
	ctx, span := StartGenericSpan(ctx, operationName, component)
	defer span.End()

	err := fn(ctx)
	if err != nil {
		RecordError(span, err)
	} else {
		RecordSuccess(span)
	}

	return err
}

// Tracer returns the global tracer for roost-keeper
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}
