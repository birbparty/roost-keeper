package teardown

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// TeardownMetricsCollector collects metrics for teardown operations
type TeardownMetricsCollector struct {
	// Prometheus metrics
	evaluationsTotal  prometheus.Counter
	executionsTotal   prometheus.Counter
	executionDuration prometheus.Histogram
	safetyChecksTotal prometheus.CounterVec
	triggerTypesTotal prometheus.CounterVec
	activeExecutions  prometheus.Gauge

	// Internal state
	mu      sync.RWMutex
	metrics TeardownMetrics
}

// NewTeardownMetricsCollector creates a new metrics collector
func NewTeardownMetricsCollector() (*TeardownMetricsCollector, error) {
	return NewTeardownMetricsCollectorWithRegistry(prometheus.DefaultRegisterer)
}

// NewTeardownMetricsCollectorWithRegistry creates a new metrics collector with a custom registry
func NewTeardownMetricsCollectorWithRegistry(registerer prometheus.Registerer) (*TeardownMetricsCollector, error) {
	factory := promauto.With(registerer)

	collector := &TeardownMetricsCollector{
		evaluationsTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "teardown_evaluations_total",
			Help: "Total number of teardown policy evaluations",
		}),
		executionsTotal: factory.NewCounter(prometheus.CounterOpts{
			Name: "teardown_executions_total",
			Help: "Total number of teardown executions",
		}),
		executionDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "teardown_execution_duration_seconds",
			Help:    "Duration of teardown executions in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600, 1200},
		}),
		safetyChecksTotal: *factory.NewCounterVec(prometheus.CounterOpts{
			Name: "teardown_safety_checks_total",
			Help: "Total number of safety checks performed",
		}, []string{"check_name", "result"}),
		triggerTypesTotal: *factory.NewCounterVec(prometheus.CounterOpts{
			Name: "teardown_trigger_types_total",
			Help: "Total number of teardown triggers by type",
		}, []string{"trigger_type", "result"}),
		activeExecutions: factory.NewGauge(prometheus.GaugeOpts{
			Name: "teardown_active_executions",
			Help: "Number of currently active teardown executions",
		}),
		metrics: TeardownMetrics{
			SafetyCheckCounts: make(map[string]int64),
			TriggerTypeCounts: make(map[string]int64),
		},
	}

	return collector, nil
}

// RecordEvaluationStart records the start of a teardown evaluation
func (m *TeardownMetricsCollector) RecordEvaluationStart(ctx context.Context, roost *roostv1alpha1.ManagedRoost) {
	m.evaluationsTotal.Inc()

	m.mu.Lock()
	m.metrics.EvaluationCount++
	m.mu.Unlock()
}

// RecordEvaluationResult records the result of a teardown evaluation
func (m *TeardownMetricsCollector) RecordEvaluationResult(ctx context.Context, roost *roostv1alpha1.ManagedRoost, decision *TeardownDecision) {
	// Record trigger type
	m.triggerTypesTotal.WithLabelValues(decision.TriggerType, "evaluated").Inc()

	m.mu.Lock()
	m.metrics.TriggerTypeCounts[decision.TriggerType]++
	m.mu.Unlock()

	if decision.ShouldTeardown {
		m.triggerTypesTotal.WithLabelValues(decision.TriggerType, "triggered").Inc()
	}

	// Record safety checks
	for _, check := range decision.SafetyChecks {
		result := "passed"
		if !check.Passed {
			result = "failed"
		}
		m.safetyChecksTotal.WithLabelValues(check.Name, result).Inc()

		m.mu.Lock()
		m.metrics.SafetyCheckCounts[check.Name+":"+result]++
		m.mu.Unlock()
	}
}

// RecordExecutionStart records the start of a teardown execution
func (m *TeardownMetricsCollector) RecordExecutionStart(ctx context.Context, roost *roostv1alpha1.ManagedRoost, execution *TeardownExecution) {
	m.executionsTotal.Inc()
	m.activeExecutions.Inc()

	m.mu.Lock()
	m.metrics.ExecutionCount++
	m.mu.Unlock()
}

// RecordExecutionSuccess records a successful teardown execution
func (m *TeardownMetricsCollector) RecordExecutionSuccess(ctx context.Context, roost *roostv1alpha1.ManagedRoost, execution *TeardownExecution) {
	m.activeExecutions.Dec()

	if execution.EndTime != nil {
		duration := execution.EndTime.Sub(execution.StartTime)
		m.executionDuration.Observe(duration.Seconds())

		m.mu.Lock()
		m.metrics.SuccessCount++
		m.updateAverageExecutionTime(duration)
		m.mu.Unlock()
	}
}

// RecordExecutionFailure records a failed teardown execution
func (m *TeardownMetricsCollector) RecordExecutionFailure(ctx context.Context, roost *roostv1alpha1.ManagedRoost, execution *TeardownExecution) {
	m.activeExecutions.Dec()

	duration := time.Since(execution.StartTime)
	m.executionDuration.Observe(duration.Seconds())

	m.mu.Lock()
	m.metrics.FailureCount++
	m.updateAverageExecutionTime(duration)
	m.mu.Unlock()
}

// GetMetrics returns current metrics snapshot
func (m *TeardownMetricsCollector) GetMetrics() TeardownMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy to avoid race conditions
	metrics := TeardownMetrics{
		EvaluationCount:      m.metrics.EvaluationCount,
		ExecutionCount:       m.metrics.ExecutionCount,
		SuccessCount:         m.metrics.SuccessCount,
		FailureCount:         m.metrics.FailureCount,
		AverageExecutionTime: m.metrics.AverageExecutionTime,
		SafetyCheckCounts:    make(map[string]int64),
		TriggerTypeCounts:    make(map[string]int64),
	}

	for k, v := range m.metrics.SafetyCheckCounts {
		metrics.SafetyCheckCounts[k] = v
	}

	for k, v := range m.metrics.TriggerTypeCounts {
		metrics.TriggerTypeCounts[k] = v
	}

	return metrics
}

// updateAverageExecutionTime updates the running average execution time
func (m *TeardownMetricsCollector) updateAverageExecutionTime(newDuration time.Duration) {
	totalExecutions := m.metrics.SuccessCount + m.metrics.FailureCount
	if totalExecutions == 0 {
		m.metrics.AverageExecutionTime = newDuration
		return
	}

	// Calculate running average
	currentAvg := m.metrics.AverageExecutionTime
	m.metrics.AverageExecutionTime = time.Duration(
		(int64(currentAvg)*int64(totalExecutions-1) + int64(newDuration)) / int64(totalExecutions),
	)
}

// Reset resets all metrics (useful for testing)
func (m *TeardownMetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.metrics = TeardownMetrics{
		SafetyCheckCounts: make(map[string]int64),
		TriggerTypeCounts: make(map[string]int64),
	}

	// Note: Prometheus metrics cannot be reset easily, but internal metrics are reset
}
