package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// ValidationLevel defines the severity level of validation errors
type ValidationLevel string

const (
	ValidationLevelError   ValidationLevel = "error"
	ValidationLevelWarning ValidationLevel = "warning"
	ValidationLevelInfo    ValidationLevel = "info"
)

// ValidationError represents a validation error with context
type ValidationError struct {
	Field   string          `json:"field"`
	Message string          `json:"message"`
	Code    string          `json:"code"`
	Level   ValidationLevel `json:"level"`
}

// ValidationResult contains the results of resource validation
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
	Info     []ValidationError `json:"info,omitempty"`
}

// StatusUpdate represents an update to a ManagedRoost status
type StatusUpdate struct {
	Phase               string                           `json:"phase,omitempty"`
	Conditions          []metav1.Condition               `json:"conditions,omitempty"`
	ObservabilityStatus *roostkeeper.ObservabilityStatus `json:"observabilityStatus,omitempty"`
	HealthStatus        *string                          `json:"healthStatus,omitempty"`
}

// LifecycleEventType defines types of lifecycle events
type LifecycleEventType string

const (
	LifecycleEventCreated    LifecycleEventType = "Created"
	LifecycleEventInstalling LifecycleEventType = "Installing"
	LifecycleEventReady      LifecycleEventType = "Ready"
	LifecycleEventHealthy    LifecycleEventType = "Healthy"
	LifecycleEventUnhealthy  LifecycleEventType = "Unhealthy"
	LifecycleEventUpgrading  LifecycleEventType = "Upgrading"
	LifecycleEventTeardown   LifecycleEventType = "TeardownTriggered"
	LifecycleEventFailed     LifecycleEventType = "Failed"
	LifecycleEventDeleted    LifecycleEventType = "Deleted"
)

// LifecycleEvent represents a significant event in the resource lifecycle
type LifecycleEvent struct {
	Type      LifecycleEventType `json:"type"`
	Message   string             `json:"message"`
	Details   map[string]string  `json:"details,omitempty"`
	EventType string             `json:"eventType"` // "Normal" or "Warning"
}

// ValidationRule defines a single validation rule
type ValidationRule struct {
	Name        string                                            `json:"name"`
	Description string                                            `json:"description"`
	Validator   func(*roostkeeper.ManagedRoost) []ValidationError `json:"-"`
}
