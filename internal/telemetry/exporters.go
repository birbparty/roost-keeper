package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// LocalOTELFileExporter exports telemetry data to local files in JSON format
// Compatible with local-otel stack for easy testing and development
type LocalOTELFileExporter struct {
	outputDir string
	mu        sync.RWMutex
}

// NewLocalOTELFileExporter creates a new file exporter for local-otel compatibility
func NewLocalOTELFileExporter(outputDir string) *LocalOTELFileExporter {
	return &LocalOTELFileExporter{
		outputDir: outputDir,
	}
}

// TraceExporter implements trace.SpanExporter for file-based trace export
type TraceExporter struct {
	*LocalOTELFileExporter
}

// NewTraceExporter creates a new trace exporter
func NewTraceExporter(outputDir string) *TraceExporter {
	return &TraceExporter{
		LocalOTELFileExporter: NewLocalOTELFileExporter(outputDir),
	}
}

// ExportSpans exports span data to JSON file
func (e *TraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, span := range spans {
		traceData := map[string]interface{}{
			"timestamp":      span.StartTime().Unix(),
			"trace_id":       span.SpanContext().TraceID().String(),
			"span_id":        span.SpanContext().SpanID().String(),
			"parent_span_id": span.Parent().SpanID().String(),
			"operation_name": span.Name(),
			"duration_ms":    span.EndTime().Sub(span.StartTime()).Milliseconds(),
			"status": map[string]interface{}{
				"code":    span.Status().Code.String(),
				"message": span.Status().Description,
			},
			"attributes": convertAttributes(span.Attributes()),
			"events":     convertEvents(span.Events()),
			"resource":   convertResource(span.Resource()),
			"service": map[string]interface{}{
				"name":    "roost-keeper",
				"version": "v0.1.0",
			},
		}

		if err := e.writeJSONL("traces", "traces.jsonl", traceData); err != nil {
			return fmt.Errorf("failed to write trace data: %w", err)
		}
	}

	return nil
}

// Shutdown implements trace.SpanExporter
func (e *TraceExporter) Shutdown(ctx context.Context) error {
	return nil
}

// MetricExporter implements metric.Exporter for file-based metric export
type MetricExporter struct {
	*LocalOTELFileExporter
}

// NewMetricExporter creates a new metric exporter
func NewMetricExporter(outputDir string) *MetricExporter {
	return &MetricExporter{
		LocalOTELFileExporter: NewLocalOTELFileExporter(outputDir),
	}
}

// Temporality implements metric.Exporter
func (e *MetricExporter) Temporality(kind sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

// Aggregation implements metric.Exporter
func (e *MetricExporter) Aggregation(kind sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(kind)
}

// Export exports metric data to JSON file
func (e *MetricExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	timestamp := time.Now().Unix()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			metricData := map[string]interface{}{
				"timestamp":   timestamp,
				"name":        m.Name,
				"description": m.Description,
				"unit":        m.Unit,
				"type":        getMetricType(m.Data),
				"resource":    convertResourceMetrics(rm.Resource),
				"scope": map[string]interface{}{
					"name":    sm.Scope.Name,
					"version": sm.Scope.Version,
				},
			}

			// Add data points based on metric type
			switch data := m.Data.(type) {
			case metricdata.Gauge[int64]:
				metricData["data_points"] = convertInt64DataPoints(data.DataPoints)
			case metricdata.Gauge[float64]:
				metricData["data_points"] = convertFloat64DataPoints(data.DataPoints)
			case metricdata.Sum[int64]:
				metricData["data_points"] = convertInt64DataPoints(data.DataPoints)
				metricData["is_monotonic"] = data.IsMonotonic
				metricData["temporality"] = data.Temporality.String()
			case metricdata.Sum[float64]:
				metricData["data_points"] = convertFloat64DataPoints(data.DataPoints)
				metricData["is_monotonic"] = data.IsMonotonic
				metricData["temporality"] = data.Temporality.String()
			case metricdata.Histogram[int64]:
				metricData["data_points"] = convertHistogramDataPoints(data.DataPoints)
				metricData["temporality"] = data.Temporality.String()
			case metricdata.Histogram[float64]:
				metricData["data_points"] = convertHistogramDataPoints(data.DataPoints)
				metricData["temporality"] = data.Temporality.String()
			}

			if err := e.writeJSONL("metrics", "metrics.jsonl", metricData); err != nil {
				return fmt.Errorf("failed to write metric data: %w", err)
			}
		}
	}

	return nil
}

// ForceFlush implements metric.Exporter
func (e *MetricExporter) ForceFlush(ctx context.Context) error {
	return nil
}

// Shutdown implements metric.Exporter
func (e *MetricExporter) Shutdown(ctx context.Context) error {
	return nil
}

// writeJSONL writes data as JSON Lines to the specified file
func (e *LocalOTELFileExporter) writeJSONL(subdir, filename string, data interface{}) error {
	dir := filepath.Join(e.outputDir, subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	file, err := os.OpenFile(
		filepath.Join(dir, filename),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND,
		0644,
	)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	line, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if _, err := fmt.Fprintf(file, "%s\n", line); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	return nil
}

// Helper functions for data conversion

func convertAttributes(attrs []attribute.KeyValue) map[string]interface{} {
	result := make(map[string]interface{})
	for _, attr := range attrs {
		result[string(attr.Key)] = attr.Value.AsInterface()
	}
	return result
}

func convertEvents(events []sdktrace.Event) []map[string]interface{} {
	result := make([]map[string]interface{}, len(events))
	for i, event := range events {
		result[i] = map[string]interface{}{
			"timestamp":  event.Time.Unix(),
			"name":       event.Name,
			"attributes": convertAttributes(event.Attributes),
		}
	}
	return result
}

func convertResource(res *resource.Resource) map[string]interface{} {
	result := make(map[string]interface{})
	for _, attr := range res.Attributes() {
		result[string(attr.Key)] = attr.Value.AsInterface()
	}
	return result
}

func convertResourceMetrics(res *resource.Resource) map[string]interface{} {
	result := make(map[string]interface{})
	for _, attr := range res.Attributes() {
		result[string(attr.Key)] = attr.Value.AsInterface()
	}
	return result
}

func getMetricType(data metricdata.Aggregation) string {
	switch data.(type) {
	case metricdata.Gauge[int64], metricdata.Gauge[float64]:
		return "gauge"
	case metricdata.Sum[int64], metricdata.Sum[float64]:
		return "counter"
	case metricdata.Histogram[int64], metricdata.Histogram[float64]:
		return "histogram"
	default:
		return "unknown"
	}
}

func convertInt64DataPoints(points []metricdata.DataPoint[int64]) []map[string]interface{} {
	result := make([]map[string]interface{}, len(points))
	for i, dp := range points {
		result[i] = map[string]interface{}{
			"timestamp":  dp.Time.Unix(),
			"value":      dp.Value,
			"attributes": convertAttributeSet(dp.Attributes),
		}
	}
	return result
}

func convertFloat64DataPoints(points []metricdata.DataPoint[float64]) []map[string]interface{} {
	result := make([]map[string]interface{}, len(points))
	for i, dp := range points {
		result[i] = map[string]interface{}{
			"timestamp":  dp.Time.Unix(),
			"value":      dp.Value,
			"attributes": convertAttributeSet(dp.Attributes),
		}
	}
	return result
}

func convertHistogramDataPoints[T int64 | float64](points []metricdata.HistogramDataPoint[T]) []map[string]interface{} {
	result := make([]map[string]interface{}, len(points))
	for i, dp := range points {
		result[i] = map[string]interface{}{
			"timestamp":     dp.Time.Unix(),
			"count":         dp.Count,
			"sum":           dp.Sum,
			"min":           dp.Min,
			"max":           dp.Max,
			"bucket_counts": dp.BucketCounts,
			"bounds":        dp.Bounds,
			"attributes":    convertAttributeSet(dp.Attributes),
		}
	}
	return result
}

func convertAttributeSet(attrs attribute.Set) map[string]interface{} {
	result := make(map[string]interface{})
	iter := attrs.Iter()
	for iter.Next() {
		attr := iter.Attribute()
		result[string(attr.Key)] = attr.Value.AsInterface()
	}
	return result
}
