package teardown

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// AuditLogger handles audit logging for teardown operations
type AuditLogger struct {
	logger *zap.Logger
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *zap.Logger) *AuditLogger {
	return &AuditLogger{
		logger: logger.With(zap.String("component", "teardown-audit")),
	}
}

// LogEvaluationResult logs the result of a teardown evaluation
func (a *AuditLogger) LogEvaluationResult(ctx context.Context, roost *roostv1alpha1.ManagedRoost, decision *TeardownDecision) {
	event := &TeardownAuditEvent{
		EventID:        generateEventID(),
		EventType:      TeardownAuditEventTypeEvaluation,
		Timestamp:      time.Now(),
		RoostName:      roost.Name,
		RoostNamespace: roost.Namespace,
		RoostUID:       string(roost.UID),
		Actor:          "system",
		Message:        decision.Reason,
		Metadata: map[string]interface{}{
			"should_teardown": decision.ShouldTeardown,
			"urgency":         decision.Urgency,
			"score":           decision.Score,
			"trigger_type":    decision.TriggerType,
			"safety_checks":   len(decision.SafetyChecks),
		},
		Severity: a.getSeverityFromDecision(decision),
	}

	a.logEvent(event)
}

// LogExecutionStart logs the start of a teardown execution
func (a *AuditLogger) LogExecutionStart(ctx context.Context, roost *roostv1alpha1.ManagedRoost, execution *TeardownExecution) {
	event := &TeardownAuditEvent{
		EventID:        generateEventID(),
		EventType:      TeardownAuditEventTypeExecution,
		Timestamp:      time.Now(),
		RoostName:      roost.Name,
		RoostNamespace: roost.Namespace,
		RoostUID:       string(roost.UID),
		Actor:          "system",
		Message:        "Teardown execution started",
		Metadata: map[string]interface{}{
			"execution_id": execution.ExecutionID,
			"reason":       execution.Decision.Reason,
			"urgency":      execution.Decision.Urgency,
		},
		Severity: TeardownAuditSeverityInfo,
	}

	a.logEvent(event)
}

// LogExecutionSuccess logs a successful teardown execution
func (a *AuditLogger) LogExecutionSuccess(ctx context.Context, roost *roostv1alpha1.ManagedRoost, execution *TeardownExecution) {
	duration := time.Duration(0)
	if execution.EndTime != nil {
		duration = execution.EndTime.Sub(execution.StartTime)
	}

	event := &TeardownAuditEvent{
		EventID:        generateEventID(),
		EventType:      TeardownAuditEventTypeExecution,
		Timestamp:      time.Now(),
		RoostName:      roost.Name,
		RoostNamespace: roost.Namespace,
		RoostUID:       string(roost.UID),
		Actor:          "system",
		Message:        "Teardown execution completed successfully",
		Metadata: map[string]interface{}{
			"execution_id": execution.ExecutionID,
			"duration":     duration.String(),
			"steps":        len(execution.Steps),
		},
		Severity: TeardownAuditSeverityInfo,
	}

	a.logEvent(event)
}

// LogExecutionFailure logs a failed teardown execution
func (a *AuditLogger) LogExecutionFailure(ctx context.Context, roost *roostv1alpha1.ManagedRoost, execution *TeardownExecution, err error) {
	duration := time.Since(execution.StartTime)

	event := &TeardownAuditEvent{
		EventID:        generateEventID(),
		EventType:      TeardownAuditEventTypeExecution,
		Timestamp:      time.Now(),
		RoostName:      roost.Name,
		RoostNamespace: roost.Namespace,
		RoostUID:       string(roost.UID),
		Actor:          "system",
		Message:        "Teardown execution failed: " + err.Error(),
		Metadata: map[string]interface{}{
			"execution_id": execution.ExecutionID,
			"duration":     duration.String(),
			"error":        err.Error(),
			"steps":        len(execution.Steps),
		},
		Severity: TeardownAuditSeverityError,
	}

	a.logEvent(event)
}

// LogSafetyCheckResult logs the result of a safety check
func (a *AuditLogger) LogSafetyCheckResult(ctx context.Context, roost *roostv1alpha1.ManagedRoost, check *SafetyCheck) {
	severity := TeardownAuditSeverityInfo
	if !check.Passed {
		switch check.Severity {
		case SafetySeverityWarning:
			severity = TeardownAuditSeverityWarning
		case SafetySeverityError:
			severity = TeardownAuditSeverityError
		case SafetySeverityCritical:
			severity = TeardownAuditSeverityCritical
		}
	}

	event := &TeardownAuditEvent{
		EventID:        generateEventID(),
		EventType:      TeardownAuditEventTypeSafetyCheck,
		Timestamp:      time.Now(),
		RoostName:      roost.Name,
		RoostNamespace: roost.Namespace,
		RoostUID:       string(roost.UID),
		Actor:          "system",
		Message:        check.Message,
		Metadata: map[string]interface{}{
			"check_name":     check.Name,
			"passed":         check.Passed,
			"can_override":   check.CanOverride,
			"check_severity": check.Severity,
		},
		Severity: severity,
	}

	a.logEvent(event)
}

// LogSafetyOverride logs a safety check override
func (a *AuditLogger) LogSafetyOverride(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkName, reason, actor string) {
	event := &TeardownAuditEvent{
		EventID:        generateEventID(),
		EventType:      TeardownAuditEventTypeOverride,
		Timestamp:      time.Now(),
		RoostName:      roost.Name,
		RoostNamespace: roost.Namespace,
		RoostUID:       string(roost.UID),
		Actor:          actor,
		Message:        "Safety check overridden: " + reason,
		Metadata: map[string]interface{}{
			"check_name":      checkName,
			"override_reason": reason,
		},
		Severity: TeardownAuditSeverityWarning,
	}

	a.logEvent(event)
}

// LogTeardownAbort logs a teardown abort
func (a *AuditLogger) LogTeardownAbort(ctx context.Context, roost *roostv1alpha1.ManagedRoost, reason, actor string) {
	event := &TeardownAuditEvent{
		EventID:        generateEventID(),
		EventType:      TeardownAuditEventTypeAbort,
		Timestamp:      time.Now(),
		RoostName:      roost.Name,
		RoostNamespace: roost.Namespace,
		RoostUID:       string(roost.UID),
		Actor:          actor,
		Message:        "Teardown aborted: " + reason,
		Metadata: map[string]interface{}{
			"abort_reason": reason,
		},
		Severity: TeardownAuditSeverityWarning,
	}

	a.logEvent(event)
}

// logEvent logs an audit event
func (a *AuditLogger) logEvent(event *TeardownAuditEvent) {
	// Convert event to JSON for structured logging
	eventData, err := json.Marshal(event)
	if err != nil {
		a.logger.Error("Failed to marshal audit event", zap.Error(err))
		return
	}

	// Log based on severity
	fields := []zap.Field{
		zap.String("event_id", event.EventID),
		zap.String("event_type", string(event.EventType)),
		zap.String("roost", event.RoostName),
		zap.String("namespace", event.RoostNamespace),
		zap.String("actor", event.Actor),
		zap.String("audit_data", string(eventData)),
	}

	switch event.Severity {
	case TeardownAuditSeverityInfo:
		a.logger.Info(event.Message, fields...)
	case TeardownAuditSeverityWarning:
		a.logger.Warn(event.Message, fields...)
	case TeardownAuditSeverityError:
		a.logger.Error(event.Message, fields...)
	case TeardownAuditSeverityCritical:
		a.logger.Error(event.Message, append(fields, zap.String("severity", "critical"))...)
	default:
		a.logger.Info(event.Message, fields...)
	}
}

// getSeverityFromDecision determines audit severity from teardown decision
func (a *AuditLogger) getSeverityFromDecision(decision *TeardownDecision) TeardownAuditSeverity {
	if !decision.ShouldTeardown {
		return TeardownAuditSeverityInfo
	}

	switch decision.Urgency {
	case TeardownUrgencyLow:
		return TeardownAuditSeverityInfo
	case TeardownUrgencyMedium:
		return TeardownAuditSeverityWarning
	case TeardownUrgencyHigh:
		return TeardownAuditSeverityError
	case TeardownUrgencyCritical:
		return TeardownAuditSeverityCritical
	default:
		return TeardownAuditSeverityInfo
	}
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return "audit-" + GenerateCorrelationID()
}
