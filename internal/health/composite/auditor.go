package composite

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// Auditor provides audit trail functionality for health check decisions
type Auditor struct {
	logger    *zap.Logger
	events    []*AuditEvent
	mutex     sync.RWMutex
	maxEvents int
}

// NewAuditor creates a new auditor
func NewAuditor(logger *zap.Logger) *Auditor {
	return &Auditor{
		logger:    logger.With(zap.String("component", "health_auditor")),
		events:    make([]*AuditEvent, 0),
		maxEvents: 1000, // Limit to prevent memory issues
	}
}

// AuditEventType defines types of audit events
type AuditEventType string

const (
	// EvaluationStarted indicates start of health evaluation
	EvaluationStarted AuditEventType = "evaluation_started"
	// EvaluationCompleted indicates completion of health evaluation
	EvaluationCompleted AuditEventType = "evaluation_completed"
	// EvaluationStopped indicates early termination of evaluation
	EvaluationStopped AuditEventType = "evaluation_stopped"
	// CheckExecuted indicates execution of individual health check
	CheckExecuted AuditEventType = "check_executed"
	// CircuitBreakerTriggered indicates circuit breaker activation
	CircuitBreakerTriggered AuditEventType = "circuit_breaker_triggered"
	// CacheHit indicates cache hit for health check
	CacheHit AuditEventType = "cache_hit"
	// AnomalyDetected indicates anomaly detection
	AnomalyDetected AuditEventType = "anomaly_detected"
	// LogicEvaluation indicates logical expression evaluation
	LogicEvaluation AuditEventType = "logic_evaluation"
)

// AuditEvent represents an audit event in the health check process
type AuditEvent struct {
	Type      AuditEventType         `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Context   map[string]interface{} `json:"context"`
	Message   string                 `json:"message,omitempty"`
	Duration  *time.Duration         `json:"duration,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Severity  AuditSeverity          `json:"severity"`
}

// AuditSeverity defines the severity levels for audit events
type AuditSeverity string

const (
	// Info represents informational events
	Info AuditSeverity = "info"
	// Warning represents warning events
	Warning AuditSeverity = "warning"
	// Error represents error events
	Error AuditSeverity = "error"
	// Critical represents critical events
	Critical AuditSeverity = "critical"
)

// LogEvent logs an audit event
func (a *Auditor) LogEvent(event *AuditEvent) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	// Set default values if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Severity == "" {
		event.Severity = Info
	}

	// Add to events
	a.events = append(a.events, event)

	// Maintain maximum events limit
	if len(a.events) > a.maxEvents {
		// Remove oldest events
		copy(a.events, a.events[len(a.events)-a.maxEvents:])
		a.events = a.events[:a.maxEvents]
	}

	// Log to structured logger
	a.logToStructuredLogger(event)
}

// LogEvaluationStart logs the start of health evaluation
func (a *Auditor) LogEvaluationStart(roostName, namespace string, checkCount int) {
	a.LogEvent(&AuditEvent{
		Type:     EvaluationStarted,
		Severity: Info,
		Message:  "Health evaluation started",
		Context: map[string]interface{}{
			"roost":     roostName,
			"namespace": namespace,
			"checks":    checkCount,
		},
	})
}

// LogEvaluationComplete logs the completion of health evaluation
func (a *Auditor) LogEvaluationComplete(roostName string, overallHealthy bool, healthScore float64, duration time.Duration, anomalyDetected bool) {
	severity := Info
	if !overallHealthy {
		severity = Warning
	}
	if anomalyDetected {
		severity = Error
	}

	a.LogEvent(&AuditEvent{
		Type:     EvaluationCompleted,
		Severity: severity,
		Message:  "Health evaluation completed",
		Duration: &duration,
		Context: map[string]interface{}{
			"roost":            roostName,
			"overall_healthy":  overallHealthy,
			"health_score":     healthScore,
			"anomaly_detected": anomalyDetected,
		},
	})
}

// LogEvaluationStop logs early termination of evaluation
func (a *Auditor) LogEvaluationStop(roostName, reason, failedCheck string) {
	a.LogEvent(&AuditEvent{
		Type:     EvaluationStopped,
		Severity: Warning,
		Message:  "Health evaluation stopped early",
		Context: map[string]interface{}{
			"roost":        roostName,
			"reason":       reason,
			"failed_check": failedCheck,
		},
	})
}

// LogCheckExecution logs the execution of an individual health check
func (a *Auditor) LogCheckExecution(checkName, checkType string, healthy bool, duration time.Duration, errorMsg string) {
	severity := Info
	if !healthy {
		severity = Warning
	}
	if errorMsg != "" {
		severity = Error
	}

	event := &AuditEvent{
		Type:     CheckExecuted,
		Severity: severity,
		Message:  "Health check executed",
		Duration: &duration,
		Context: map[string]interface{}{
			"check_name": checkName,
			"check_type": checkType,
			"healthy":    healthy,
		},
	}

	if errorMsg != "" {
		event.Error = errorMsg
	}

	a.LogEvent(event)
}

// LogCircuitBreakerTrigger logs circuit breaker activation
func (a *Auditor) LogCircuitBreakerTrigger(checkName, state string, failureCount int32) {
	a.LogEvent(&AuditEvent{
		Type:     CircuitBreakerTriggered,
		Severity: Warning,
		Message:  "Circuit breaker state changed",
		Context: map[string]interface{}{
			"check_name":    checkName,
			"state":         state,
			"failure_count": failureCount,
		},
	})
}

// LogCacheHit logs a cache hit for health check
func (a *Auditor) LogCacheHit(checkName string, age time.Duration) {
	a.LogEvent(&AuditEvent{
		Type:     CacheHit,
		Severity: Info,
		Message:  "Cache hit for health check",
		Context: map[string]interface{}{
			"check_name": checkName,
			"cache_age":  age.String(),
		},
	})
}

// LogAnomalyDetection logs anomaly detection
func (a *Auditor) LogAnomalyDetection(anomalyType, checkName string, severity float64, confidence float64) {
	auditSeverity := Warning
	if severity > 7.0 {
		auditSeverity = Error
	}
	if severity > 9.0 {
		auditSeverity = Critical
	}

	a.LogEvent(&AuditEvent{
		Type:     AnomalyDetected,
		Severity: auditSeverity,
		Message:  "Anomaly detected in health check pattern",
		Context: map[string]interface{}{
			"anomaly_type": anomalyType,
			"check_name":   checkName,
			"severity":     severity,
			"confidence":   confidence,
		},
	})
}

// LogLogicEvaluation logs logical expression evaluation
func (a *Auditor) LogLogicEvaluation(expression string, result bool, checkResults map[string]bool) {
	a.LogEvent(&AuditEvent{
		Type:     LogicEvaluation,
		Severity: Info,
		Message:  "Logical expression evaluated",
		Context: map[string]interface{}{
			"expression":    expression,
			"result":        result,
			"check_results": checkResults,
		},
	})
}

// GetRecentEvents returns the most recent audit events
func (a *Auditor) GetRecentEvents(limit int) []*AuditEvent {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	if limit <= 0 || limit >= len(a.events) {
		// Return a copy of all events
		result := make([]*AuditEvent, len(a.events))
		copy(result, a.events)
		return result
	}

	// Return the most recent 'limit' events
	start := len(a.events) - limit
	result := make([]*AuditEvent, limit)
	copy(result, a.events[start:])
	return result
}

// GetEventsByType returns events filtered by type
func (a *Auditor) GetEventsByType(eventType AuditEventType, limit int) []*AuditEvent {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	var filtered []*AuditEvent
	for i := len(a.events) - 1; i >= 0 && len(filtered) < limit; i-- {
		if a.events[i].Type == eventType {
			filtered = append(filtered, a.events[i])
		}
	}

	return filtered
}

// GetEventsBySeverity returns events filtered by severity
func (a *Auditor) GetEventsBySeverity(severity AuditSeverity, limit int) []*AuditEvent {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	var filtered []*AuditEvent
	for i := len(a.events) - 1; i >= 0 && len(filtered) < limit; i-- {
		if a.events[i].Severity == severity {
			filtered = append(filtered, a.events[i])
		}
	}

	return filtered
}

// GetEventsSince returns events since a specific time
func (a *Auditor) GetEventsSince(since time.Time) []*AuditEvent {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	var filtered []*AuditEvent
	for _, event := range a.events {
		if event.Timestamp.After(since) {
			filtered = append(filtered, event)
		}
	}

	return filtered
}

// GetAuditSummary returns a summary of audit events
func (a *Auditor) GetAuditSummary() *AuditSummary {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	summary := &AuditSummary{
		TotalEvents:    len(a.events),
		EventCounts:    make(map[AuditEventType]int),
		SeverityCounts: make(map[AuditSeverity]int),
		TimeRange:      &AuditTimeRange{},
	}

	if len(a.events) > 0 {
		summary.TimeRange.Start = a.events[0].Timestamp
		summary.TimeRange.End = a.events[len(a.events)-1].Timestamp
	}

	for _, event := range a.events {
		summary.EventCounts[event.Type]++
		summary.SeverityCounts[event.Severity]++
	}

	return summary
}

// AuditSummary provides a summary of audit events
type AuditSummary struct {
	TotalEvents    int                    `json:"total_events"`
	EventCounts    map[AuditEventType]int `json:"event_counts"`
	SeverityCounts map[AuditSeverity]int  `json:"severity_counts"`
	TimeRange      *AuditTimeRange        `json:"time_range"`
}

// AuditTimeRange represents the time range of audit events
type AuditTimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ClearEvents clears all audit events
func (a *Auditor) ClearEvents() {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.events = make([]*AuditEvent, 0)
	a.logger.Info("Audit events cleared")
}

// SetMaxEvents sets the maximum number of events to retain
func (a *Auditor) SetMaxEvents(max int) {
	a.mutex.Lock()
	defer a.mutex.Unlock()

	a.maxEvents = max

	// Trim events if necessary
	if len(a.events) > max {
		copy(a.events, a.events[len(a.events)-max:])
		a.events = a.events[:max]
	}

	a.logger.Info("Updated maximum audit events",
		zap.Int("max_events", max),
		zap.Int("current_events", len(a.events)))
}

// logToStructuredLogger logs the event to the structured logger
func (a *Auditor) logToStructuredLogger(event *AuditEvent) {
	fields := []zap.Field{
		zap.String("audit_type", string(event.Type)),
		zap.String("severity", string(event.Severity)),
		zap.Time("timestamp", event.Timestamp),
	}

	if event.Duration != nil {
		fields = append(fields, zap.Duration("duration", *event.Duration))
	}

	if event.Error != "" {
		fields = append(fields, zap.String("audit_error", event.Error))
	}

	// Add context fields
	for key, value := range event.Context {
		fields = append(fields, zap.Any(key, value))
	}

	// Log at appropriate level based on severity
	switch event.Severity {
	case Info:
		a.logger.Info(event.Message, fields...)
	case Warning:
		a.logger.Warn(event.Message, fields...)
	case Error:
		a.logger.Error(event.Message, fields...)
	case Critical:
		a.logger.Error(event.Message, append(fields, zap.Bool("critical", true))...)
	default:
		a.logger.Info(event.Message, fields...)
	}
}

// ExportEvents exports audit events for external analysis
func (a *Auditor) ExportEvents(format string) ([]byte, error) {
	a.mutex.RLock()
	defer a.mutex.RUnlock()

	switch format {
	case "json":
		return a.exportAsJSON()
	case "csv":
		return a.exportAsCSV()
	default:
		return a.exportAsJSON()
	}
}

// exportAsJSON exports events as JSON
func (a *Auditor) exportAsJSON() ([]byte, error) {
	// This would typically use json.Marshal
	// For now, return a placeholder
	return []byte("{}"), nil
}

// exportAsCSV exports events as CSV
func (a *Auditor) exportAsCSV() ([]byte, error) {
	// This would typically create CSV format
	// For now, return a placeholder
	return []byte("timestamp,type,severity,message\n"), nil
}
