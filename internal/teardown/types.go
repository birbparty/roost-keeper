package teardown

import (
	"time"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// TeardownDecision represents the result of teardown policy evaluation
type TeardownDecision struct {
	// ShouldTeardown indicates if teardown should proceed
	ShouldTeardown bool `json:"shouldTeardown"`

	// Reason provides the reason for the teardown decision
	Reason string `json:"reason"`

	// Urgency indicates the urgency level of the teardown
	Urgency TeardownUrgency `json:"urgency"`

	// SafetyChecks contains the results of safety checks
	SafetyChecks []SafetyCheck `json:"safetyChecks"`

	// Metadata contains additional decision metadata
	Metadata map[string]interface{} `json:"metadata"`

	// TriggerType indicates which trigger type made the decision
	TriggerType string `json:"triggerType"`

	// EvaluatedAt is when the decision was made
	EvaluatedAt time.Time `json:"evaluatedAt"`

	// Score is a numeric score for the teardown decision (0-100)
	Score int32 `json:"score"`
}

// TeardownUrgency defines the urgency level of a teardown decision
type TeardownUrgency string

const (
	// TeardownUrgencyLow indicates low urgency teardown
	TeardownUrgencyLow TeardownUrgency = "low"
	// TeardownUrgencyMedium indicates medium urgency teardown
	TeardownUrgencyMedium TeardownUrgency = "medium"
	// TeardownUrgencyHigh indicates high urgency teardown
	TeardownUrgencyHigh TeardownUrgency = "high"
	// TeardownUrgencyCritical indicates critical urgency teardown
	TeardownUrgencyCritical TeardownUrgency = "critical"
)

// SafetyCheck represents the result of a safety check
type SafetyCheck struct {
	// Name of the safety check
	Name string `json:"name"`

	// Passed indicates if the safety check passed
	Passed bool `json:"passed"`

	// Message provides details about the safety check result
	Message string `json:"message"`

	// CanOverride indicates if this safety check can be overridden
	CanOverride bool `json:"canOverride"`

	// Severity indicates the severity level of a failed check
	Severity SafetySeverity `json:"severity"`

	// CheckedAt is when the safety check was performed
	CheckedAt time.Time `json:"checkedAt"`

	// Details contains additional check details
	Details map[string]interface{} `json:"details,omitempty"`
}

// SafetySeverity defines the severity level of a failed safety check
type SafetySeverity string

const (
	// SafetySeverityInfo indicates informational severity
	SafetySeverityInfo SafetySeverity = "info"
	// SafetySeverityWarning indicates warning severity
	SafetySeverityWarning SafetySeverity = "warning"
	// SafetySeverityError indicates error severity
	SafetySeverityError SafetySeverity = "error"
	// SafetySeverityCritical indicates critical severity
	SafetySeverityCritical SafetySeverity = "critical"
)

// TeardownContext provides context for teardown operations
type TeardownContext struct {
	// ManagedRoost is the roost being evaluated
	ManagedRoost *roostv1alpha1.ManagedRoost

	// EvaluationTime is when the evaluation started
	EvaluationTime time.Time

	// CorrelationID tracks the teardown evaluation
	CorrelationID string

	// Metadata contains additional context
	Metadata map[string]interface{}

	// DryRun indicates if this is a dry run evaluation
	DryRun bool

	// ForceOverride indicates if safety checks should be overridden
	ForceOverride bool
}

// TeardownExecution represents a teardown execution
type TeardownExecution struct {
	// ExecutionID uniquely identifies the execution
	ExecutionID string `json:"executionId"`

	// Decision is the teardown decision being executed
	Decision *TeardownDecision `json:"decision"`

	// StartTime is when execution started
	StartTime time.Time `json:"startTime"`

	// EndTime is when execution completed
	EndTime *time.Time `json:"endTime,omitempty"`

	// Status is the current execution status
	Status TeardownExecutionStatus `json:"status"`

	// Progress indicates execution progress (0-100)
	Progress int32 `json:"progress"`

	// Steps contains the execution steps
	Steps []TeardownExecutionStep `json:"steps"`

	// Error contains any execution error
	Error string `json:"error,omitempty"`

	// Metadata contains additional execution metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TeardownExecutionStatus defines the status of a teardown execution
type TeardownExecutionStatus string

const (
	// TeardownExecutionStatusPending indicates execution is pending
	TeardownExecutionStatusPending TeardownExecutionStatus = "pending"
	// TeardownExecutionStatusRunning indicates execution is running
	TeardownExecutionStatusRunning TeardownExecutionStatus = "running"
	// TeardownExecutionStatusCompleted indicates execution completed successfully
	TeardownExecutionStatusCompleted TeardownExecutionStatus = "completed"
	// TeardownExecutionStatusFailed indicates execution failed
	TeardownExecutionStatusFailed TeardownExecutionStatus = "failed"
	// TeardownExecutionStatusAborted indicates execution was aborted
	TeardownExecutionStatusAborted TeardownExecutionStatus = "aborted"
)

// TeardownExecutionStep represents a step in teardown execution
type TeardownExecutionStep struct {
	// Name of the step
	Name string `json:"name"`

	// Status of the step
	Status TeardownExecutionStatus `json:"status"`

	// StartTime is when the step started
	StartTime time.Time `json:"startTime"`

	// EndTime is when the step completed
	EndTime *time.Time `json:"endTime,omitempty"`

	// Message provides step details
	Message string `json:"message"`

	// Error contains any step error
	Error string `json:"error,omitempty"`

	// Metadata contains additional step metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TeardownAuditEvent represents an audit event for teardown operations
type TeardownAuditEvent struct {
	// EventID uniquely identifies the event
	EventID string `json:"eventId"`

	// EventType indicates the type of event
	EventType TeardownAuditEventType `json:"eventType"`

	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// RoostName is the name of the roost
	RoostName string `json:"roostName"`

	// RoostNamespace is the namespace of the roost
	RoostNamespace string `json:"roostNamespace"`

	// RoostUID is the UID of the roost
	RoostUID string `json:"roostUID"`

	// Actor is who or what triggered the event
	Actor string `json:"actor"`

	// Message provides event details
	Message string `json:"message"`

	// Metadata contains additional event metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Severity indicates the event severity
	Severity TeardownAuditSeverity `json:"severity"`
}

// TeardownAuditEventType defines the type of audit event
type TeardownAuditEventType string

const (
	// TeardownAuditEventTypeEvaluation indicates a teardown evaluation event
	TeardownAuditEventTypeEvaluation TeardownAuditEventType = "evaluation"
	// TeardownAuditEventTypeExecution indicates a teardown execution event
	TeardownAuditEventTypeExecution TeardownAuditEventType = "execution"
	// TeardownAuditEventTypeSafetyCheck indicates a safety check event
	TeardownAuditEventTypeSafetyCheck TeardownAuditEventType = "safety_check"
	// TeardownAuditEventTypeOverride indicates a safety override event
	TeardownAuditEventTypeOverride TeardownAuditEventType = "override"
	// TeardownAuditEventTypeAbort indicates a teardown abort event
	TeardownAuditEventTypeAbort TeardownAuditEventType = "abort"
)

// TeardownAuditSeverity defines the severity of an audit event
type TeardownAuditSeverity string

const (
	// TeardownAuditSeverityInfo indicates informational severity
	TeardownAuditSeverityInfo TeardownAuditSeverity = "info"
	// TeardownAuditSeverityWarning indicates warning severity
	TeardownAuditSeverityWarning TeardownAuditSeverity = "warning"
	// TeardownAuditSeverityError indicates error severity
	TeardownAuditSeverityError TeardownAuditSeverity = "error"
	// TeardownAuditSeverityCritical indicates critical severity
	TeardownAuditSeverityCritical TeardownAuditSeverity = "critical"
)

// TeardownMetrics contains metrics for teardown operations
type TeardownMetrics struct {
	// EvaluationCount is the number of evaluations performed
	EvaluationCount int64 `json:"evaluationCount"`

	// ExecutionCount is the number of executions performed
	ExecutionCount int64 `json:"executionCount"`

	// SuccessCount is the number of successful executions
	SuccessCount int64 `json:"successCount"`

	// FailureCount is the number of failed executions
	FailureCount int64 `json:"failureCount"`

	// AverageExecutionTime is the average execution time
	AverageExecutionTime time.Duration `json:"averageExecutionTime"`

	// SafetyCheckCounts tracks safety check results
	SafetyCheckCounts map[string]int64 `json:"safetyCheckCounts"`

	// TriggerTypeCounts tracks trigger type usage
	TriggerTypeCounts map[string]int64 `json:"triggerTypeCounts"`
}

// TeardownSchedule represents a scheduled teardown
type TeardownSchedule struct {
	// ScheduleID uniquely identifies the schedule
	ScheduleID string `json:"scheduleId"`

	// RoostName is the name of the roost
	RoostName string `json:"roostName"`

	// RoostNamespace is the namespace of the roost
	RoostNamespace string `json:"roostNamespace"`

	// CronExpression defines the schedule
	CronExpression string `json:"cronExpression"`

	// NextRun is when the next execution is scheduled
	NextRun time.Time `json:"nextRun"`

	// LastRun is when the last execution occurred
	LastRun *time.Time `json:"lastRun,omitempty"`

	// Enabled indicates if the schedule is active
	Enabled bool `json:"enabled"`

	// Metadata contains additional schedule metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}
