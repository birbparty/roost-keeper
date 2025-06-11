package teardown

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/health"
)

// TimeoutEvaluator evaluates timeout-based teardown triggers
type TimeoutEvaluator struct {
	TeardownEvaluatorBase
	logger *zap.Logger
}

// NewTimeoutEvaluator creates a new timeout evaluator
func NewTimeoutEvaluator(logger *zap.Logger) TeardownEvaluator {
	return &TimeoutEvaluator{
		TeardownEvaluatorBase: TeardownEvaluatorBase{
			Type:     "timeout",
			Priority: 100,
		},
		logger: logger.With(zap.String("evaluator", "timeout")),
	}
}

// ShouldTeardown evaluates if a roost should be torn down based on timeout
func (e *TimeoutEvaluator) ShouldTeardown(ctx context.Context, roostCtx *TeardownContext) (*TeardownDecision, error) {
	roost := roostCtx.ManagedRoost

	// Find timeout triggers
	if roost.Spec.TeardownPolicy == nil {
		return &TeardownDecision{ShouldTeardown: false}, nil
	}

	for _, trigger := range roost.Spec.TeardownPolicy.Triggers {
		if trigger.Type == "timeout" && trigger.Timeout != nil {
			// Check if timeout has been reached
			creationTime := roost.CreationTimestamp.Time
			timeoutDuration := trigger.Timeout.Duration
			expirationTime := creationTime.Add(timeoutDuration)

			if time.Now().After(expirationTime) {
				return &TeardownDecision{
					ShouldTeardown: true,
					Reason:         fmt.Sprintf("Timeout reached: %s elapsed since creation", timeoutDuration),
					Urgency:        TeardownUrgencyMedium,
					SafetyChecks:   []SafetyCheck{},
					Metadata: map[string]interface{}{
						"creation_time":   creationTime,
						"timeout":         timeoutDuration.String(),
						"expiration_time": expirationTime,
					},
					TriggerType: "timeout",
					EvaluatedAt: time.Now(),
					Score:       50,
				}, nil
			}
		}
	}

	return &TeardownDecision{ShouldTeardown: false}, nil
}

// Validate validates timeout trigger configuration
func (e *TimeoutEvaluator) Validate(triggers []roostv1alpha1.TeardownTriggerSpec) error {
	for _, trigger := range triggers {
		if trigger.Type == "timeout" && trigger.Timeout == nil {
			return fmt.Errorf("timeout trigger requires timeout configuration")
		}
	}
	return nil
}

// ResourceThresholdEvaluator evaluates resource-based teardown triggers
type ResourceThresholdEvaluator struct {
	TeardownEvaluatorBase
	client client.Client
	logger *zap.Logger
}

// NewResourceThresholdEvaluator creates a new resource threshold evaluator
func NewResourceThresholdEvaluator(client client.Client, logger *zap.Logger) TeardownEvaluator {
	return &ResourceThresholdEvaluator{
		TeardownEvaluatorBase: TeardownEvaluatorBase{
			Type:     "resource_threshold",
			Priority: 80,
		},
		client: client,
		logger: logger.With(zap.String("evaluator", "resource_threshold")),
	}
}

// ShouldTeardown evaluates if a roost should be torn down based on resource thresholds
func (e *ResourceThresholdEvaluator) ShouldTeardown(ctx context.Context, roostCtx *TeardownContext) (*TeardownDecision, error) {
	// Placeholder implementation - in reality, this would check resource usage
	// against configured thresholds
	return &TeardownDecision{ShouldTeardown: false}, nil
}

// Validate validates resource threshold trigger configuration
func (e *ResourceThresholdEvaluator) Validate(triggers []roostv1alpha1.TeardownTriggerSpec) error {
	return nil // Placeholder implementation
}

// HealthBasedEvaluator evaluates health-based teardown triggers
type HealthBasedEvaluator struct {
	TeardownEvaluatorBase
	healthChecker health.Checker
	logger        *zap.Logger
}

// NewHealthBasedEvaluator creates a new health-based evaluator
func NewHealthBasedEvaluator(healthChecker health.Checker, logger *zap.Logger) TeardownEvaluator {
	return &HealthBasedEvaluator{
		TeardownEvaluatorBase: TeardownEvaluatorBase{
			Type:     "health_based",
			Priority: 90,
		},
		healthChecker: healthChecker,
		logger:        logger.With(zap.String("evaluator", "health_based")),
	}
}

// ShouldTeardown evaluates if a roost should be torn down based on health status
func (e *HealthBasedEvaluator) ShouldTeardown(ctx context.Context, roostCtx *TeardownContext) (*TeardownDecision, error) {
	roost := roostCtx.ManagedRoost

	// Check if roost is unhealthy for an extended period
	if roost.Status.Health == "unhealthy" {
		// In a real implementation, you would check how long it's been unhealthy
		// and compare against configured thresholds
		return &TeardownDecision{
			ShouldTeardown: true,
			Reason:         "Roost has been unhealthy for extended period",
			Urgency:        TeardownUrgencyHigh,
			SafetyChecks:   []SafetyCheck{},
			Metadata: map[string]interface{}{
				"health_status": roost.Status.Health,
			},
			TriggerType: "health_based",
			EvaluatedAt: time.Now(),
			Score:       70,
		}, nil
	}

	return &TeardownDecision{ShouldTeardown: false}, nil
}

// Validate validates health-based trigger configuration
func (e *HealthBasedEvaluator) Validate(triggers []roostv1alpha1.TeardownTriggerSpec) error {
	return nil // Placeholder implementation
}

// FailureCountEvaluator evaluates failure count-based teardown triggers
type FailureCountEvaluator struct {
	TeardownEvaluatorBase
	logger *zap.Logger
}

// NewFailureCountEvaluator creates a new failure count evaluator
func NewFailureCountEvaluator(logger *zap.Logger) TeardownEvaluator {
	return &FailureCountEvaluator{
		TeardownEvaluatorBase: TeardownEvaluatorBase{
			Type:     "failure_count",
			Priority: 70,
		},
		logger: logger.With(zap.String("evaluator", "failure_count")),
	}
}

// ShouldTeardown evaluates if a roost should be torn down based on failure count
func (e *FailureCountEvaluator) ShouldTeardown(ctx context.Context, roostCtx *TeardownContext) (*TeardownDecision, error) {
	roost := roostCtx.ManagedRoost

	// Find failure count triggers
	if roost.Spec.TeardownPolicy == nil {
		return &TeardownDecision{ShouldTeardown: false}, nil
	}

	for _, trigger := range roost.Spec.TeardownPolicy.Triggers {
		if trigger.Type == "failure_count" && trigger.FailureCount != nil {
			// Check health check statuses for failure counts
			failureCount := int32(0)
			for _, healthStatus := range roost.Status.HealthChecks {
				failureCount += healthStatus.FailureCount
			}

			if failureCount >= *trigger.FailureCount {
				return &TeardownDecision{
					ShouldTeardown: true,
					Reason:         fmt.Sprintf("Failure count threshold reached: %d >= %d", failureCount, *trigger.FailureCount),
					Urgency:        TeardownUrgencyHigh,
					SafetyChecks:   []SafetyCheck{},
					Metadata: map[string]interface{}{
						"failure_count": failureCount,
						"threshold":     *trigger.FailureCount,
					},
					TriggerType: "failure_count",
					EvaluatedAt: time.Now(),
					Score:       80,
				}, nil
			}
		}
	}

	return &TeardownDecision{ShouldTeardown: false}, nil
}

// Validate validates failure count trigger configuration
func (e *FailureCountEvaluator) Validate(triggers []roostv1alpha1.TeardownTriggerSpec) error {
	for _, trigger := range triggers {
		if trigger.Type == "failure_count" && trigger.FailureCount == nil {
			return fmt.Errorf("failure_count trigger requires failureCount configuration")
		}
	}
	return nil
}

// ScheduleEvaluator evaluates schedule-based teardown triggers
type ScheduleEvaluator struct {
	TeardownEvaluatorBase
	logger *zap.Logger
}

// NewScheduleEvaluator creates a new schedule evaluator
func NewScheduleEvaluator(logger *zap.Logger) TeardownEvaluator {
	return &ScheduleEvaluator{
		TeardownEvaluatorBase: TeardownEvaluatorBase{
			Type:     "schedule",
			Priority: 60,
		},
		logger: logger.With(zap.String("evaluator", "schedule")),
	}
}

// ShouldTeardown evaluates if a roost should be torn down based on schedule
func (e *ScheduleEvaluator) ShouldTeardown(ctx context.Context, roostCtx *TeardownContext) (*TeardownDecision, error) {
	// Placeholder implementation - would need cron parsing logic
	return &TeardownDecision{ShouldTeardown: false}, nil
}

// Validate validates schedule trigger configuration
func (e *ScheduleEvaluator) Validate(triggers []roostv1alpha1.TeardownTriggerSpec) error {
	return nil // Placeholder implementation
}

// SupportsScheduling indicates that schedule evaluator supports scheduling
func (e *ScheduleEvaluator) SupportsScheduling() bool {
	return true
}

// SchedulingInterval returns the recommended scheduling interval
func (e *ScheduleEvaluator) SchedulingInterval() *int64 {
	interval := int64(60) // 60 seconds
	return &interval
}

// WebhookEvaluator evaluates webhook-based teardown triggers
type WebhookEvaluator struct {
	TeardownEvaluatorBase
	logger *zap.Logger
}

// NewWebhookEvaluator creates a new webhook evaluator
func NewWebhookEvaluator(logger *zap.Logger) TeardownEvaluator {
	return &WebhookEvaluator{
		TeardownEvaluatorBase: TeardownEvaluatorBase{
			Type:     "webhook",
			Priority: 50,
		},
		logger: logger.With(zap.String("evaluator", "webhook")),
	}
}

// ShouldTeardown evaluates if a roost should be torn down based on webhook trigger
func (e *WebhookEvaluator) ShouldTeardown(ctx context.Context, roostCtx *TeardownContext) (*TeardownDecision, error) {
	// Placeholder implementation - would make HTTP requests to configured webhooks
	return &TeardownDecision{ShouldTeardown: false}, nil
}

// Validate validates webhook trigger configuration
func (e *WebhookEvaluator) Validate(triggers []roostv1alpha1.TeardownTriggerSpec) error {
	for _, trigger := range triggers {
		if trigger.Type == "webhook" && trigger.Webhook == nil {
			return fmt.Errorf("webhook trigger requires webhook configuration")
		}
	}
	return nil
}
