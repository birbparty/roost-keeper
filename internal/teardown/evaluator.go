package teardown

import (
	"context"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// TeardownEvaluator defines the interface for teardown policy evaluators
type TeardownEvaluator interface {
	// ShouldTeardown evaluates if a roost should be torn down
	ShouldTeardown(ctx context.Context, roostCtx *TeardownContext) (*TeardownDecision, error)

	// GetType returns the evaluator type
	GetType() string

	// GetPriority returns the evaluator priority (higher numbers run first)
	GetPriority() int32

	// Validate validates the evaluator configuration
	Validate(triggers []roostv1alpha1.TeardownTriggerSpec) error

	// SupportsScheduling indicates if the evaluator supports scheduled evaluation
	SupportsScheduling() bool

	// SchedulingInterval returns the recommended scheduling interval
	SchedulingInterval() *int64 // seconds
}

// TeardownEvaluatorBase provides common functionality for evaluators
type TeardownEvaluatorBase struct {
	Type     string
	Priority int32
}

// GetType returns the evaluator type
func (e *TeardownEvaluatorBase) GetType() string {
	return e.Type
}

// GetPriority returns the evaluator priority
func (e *TeardownEvaluatorBase) GetPriority() int32 {
	return e.Priority
}

// SupportsScheduling returns false by default
func (e *TeardownEvaluatorBase) SupportsScheduling() bool {
	return false
}

// SchedulingInterval returns nil by default
func (e *TeardownEvaluatorBase) SchedulingInterval() *int64 {
	return nil
}

// EvaluatorRegistry manages teardown evaluators
type EvaluatorRegistry struct {
	evaluators map[string]TeardownEvaluator
}

// NewEvaluatorRegistry creates a new evaluator registry
func NewEvaluatorRegistry() *EvaluatorRegistry {
	return &EvaluatorRegistry{
		evaluators: make(map[string]TeardownEvaluator),
	}
}

// Register registers a teardown evaluator
func (r *EvaluatorRegistry) Register(evaluator TeardownEvaluator) {
	r.evaluators[evaluator.GetType()] = evaluator
}

// Get returns an evaluator by type
func (r *EvaluatorRegistry) Get(evaluatorType string) (TeardownEvaluator, bool) {
	evaluator, exists := r.evaluators[evaluatorType]
	return evaluator, exists
}

// List returns all registered evaluators
func (r *EvaluatorRegistry) List() []TeardownEvaluator {
	evaluators := make([]TeardownEvaluator, 0, len(r.evaluators))
	for _, evaluator := range r.evaluators {
		evaluators = append(evaluators, evaluator)
	}
	return evaluators
}

// GetByTrigger returns evaluators that match the given triggers
func (r *EvaluatorRegistry) GetByTrigger(triggers []roostv1alpha1.TeardownTriggerSpec) []TeardownEvaluator {
	var matchingEvaluators []TeardownEvaluator

	triggerTypes := make(map[string]bool)
	for _, trigger := range triggers {
		triggerTypes[trigger.Type] = true
	}

	for triggerType := range triggerTypes {
		if evaluator, exists := r.evaluators[triggerType]; exists {
			matchingEvaluators = append(matchingEvaluators, evaluator)
		}
	}

	return matchingEvaluators
}
