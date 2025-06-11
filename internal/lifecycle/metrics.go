package lifecycle

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// LifecycleMetrics contains lifecycle-specific metrics for the Roost-Keeper operator
type LifecycleMetrics struct {
	// Lifecycle operation metrics
	LifecycleOperations metric.Int64Counter
	LifecycleDuration   metric.Float64Histogram

	// Roost lifecycle metrics
	RoostsCreated      metric.Int64Counter
	RoostsByPhase      metric.Int64UpDownCounter
	PhaseTransitions   metric.Int64Counter
	PhaseTransitionAge metric.Float64Histogram

	// Teardown metrics
	TeardownsTriggered metric.Int64Counter
	TeardownDuration   metric.Float64Histogram
	TeardownTriggers   metric.Int64Counter
	CleanupOperations  metric.Int64Counter

	// Garbage collection metrics
	GarbageCollections metric.Int64Counter
	OrphanedResources  metric.Int64Counter
	GCDuration         metric.Float64Histogram

	// Error tracking
	LifecycleErrors metric.Int64Counter
}

// NewLifecycleMetrics creates and initializes lifecycle metrics
func NewLifecycleMetrics() (*LifecycleMetrics, error) {
	meter := otel.Meter("roost-keeper-lifecycle")

	lifecycleOps, err := meter.Int64Counter(
		"roost_keeper_lifecycle_operations_total",
		metric.WithDescription("Total lifecycle operations performed"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	lifecycleDuration, err := meter.Float64Histogram(
		"roost_keeper_lifecycle_duration_seconds",
		metric.WithDescription("Duration of lifecycle operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	roostsCreated, err := meter.Int64Counter(
		"roost_keeper_roosts_created_total",
		metric.WithDescription("Total roosts created"),
		metric.WithUnit("{roosts}"),
	)
	if err != nil {
		return nil, err
	}

	roostsByPhase, err := meter.Int64UpDownCounter(
		"roost_keeper_roosts_by_phase",
		metric.WithDescription("Number of roosts by phase"),
		metric.WithUnit("{roosts}"),
	)
	if err != nil {
		return nil, err
	}

	phaseTransitions, err := meter.Int64Counter(
		"roost_keeper_phase_transitions_total",
		metric.WithDescription("Total phase transitions"),
		metric.WithUnit("{transitions}"),
	)
	if err != nil {
		return nil, err
	}

	phaseTransitionAge, err := meter.Float64Histogram(
		"roost_keeper_phase_transition_age_seconds",
		metric.WithDescription("Age of roost when phase transitions occur"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	teardownsTriggered, err := meter.Int64Counter(
		"roost_keeper_teardowns_triggered_total",
		metric.WithDescription("Total teardowns triggered"),
		metric.WithUnit("{teardowns}"),
	)
	if err != nil {
		return nil, err
	}

	teardownDuration, err := meter.Float64Histogram(
		"roost_keeper_teardown_duration_seconds",
		metric.WithDescription("Duration of teardown operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	teardownTriggers, err := meter.Int64Counter(
		"roost_keeper_teardown_triggers_total",
		metric.WithDescription("Total teardown triggers by type"),
		metric.WithUnit("{triggers}"),
	)
	if err != nil {
		return nil, err
	}

	cleanupOps, err := meter.Int64Counter(
		"roost_keeper_cleanup_operations_total",
		metric.WithDescription("Total cleanup operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	garbageCollections, err := meter.Int64Counter(
		"roost_keeper_garbage_collections_total",
		metric.WithDescription("Total garbage collection runs"),
		metric.WithUnit("{collections}"),
	)
	if err != nil {
		return nil, err
	}

	orphanedResources, err := meter.Int64Counter(
		"roost_keeper_orphaned_resources_total",
		metric.WithDescription("Total orphaned resources found"),
		metric.WithUnit("{resources}"),
	)
	if err != nil {
		return nil, err
	}

	gcDuration, err := meter.Float64Histogram(
		"roost_keeper_gc_duration_seconds",
		metric.WithDescription("Duration of garbage collection operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	lifecycleErrors, err := meter.Int64Counter(
		"roost_keeper_lifecycle_errors_total",
		metric.WithDescription("Total lifecycle operation errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	return &LifecycleMetrics{
		LifecycleOperations: lifecycleOps,
		LifecycleDuration:   lifecycleDuration,
		RoostsCreated:       roostsCreated,
		RoostsByPhase:       roostsByPhase,
		PhaseTransitions:    phaseTransitions,
		PhaseTransitionAge:  phaseTransitionAge,
		TeardownsTriggered:  teardownsTriggered,
		TeardownDuration:    teardownDuration,
		TeardownTriggers:    teardownTriggers,
		CleanupOperations:   cleanupOps,
		GarbageCollections:  garbageCollections,
		OrphanedResources:   orphanedResources,
		GCDuration:          gcDuration,
		LifecycleErrors:     lifecycleErrors,
	}, nil
}

// RecordLifecycleOperation records a lifecycle operation with its duration
func (lm *LifecycleMetrics) RecordLifecycleOperation(ctx context.Context, operation string, duration float64) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
	)

	lm.LifecycleOperations.Add(ctx, 1, labels)
	lm.LifecycleDuration.Record(ctx, duration, labels)
}

// RecordRoostCreated records a roost creation event
func (lm *LifecycleMetrics) RecordRoostCreated(ctx context.Context, name, namespace string) {
	labels := metric.WithAttributes(
		attribute.String("name", name),
		attribute.String("namespace", namespace),
	)

	lm.RoostsCreated.Add(ctx, 1, labels)
}

// RecordRoostByPhase tracks roosts by their current phase
func (lm *LifecycleMetrics) RecordRoostByPhase(ctx context.Context, phase string) {
	labels := metric.WithAttributes(
		attribute.String("phase", phase),
	)

	lm.RoostsByPhase.Add(ctx, 1, labels)
}

// RecordPhaseTransition records a phase transition
func (lm *LifecycleMetrics) RecordPhaseTransition(ctx context.Context, fromPhase, toPhase, name string) {
	labels := metric.WithAttributes(
		attribute.String("from_phase", fromPhase),
		attribute.String("to_phase", toPhase),
		attribute.String("roost", name),
	)

	lm.PhaseTransitions.Add(ctx, 1, labels)
}

// RecordPhaseTransitionAge records the age of a roost when it transitions phases
func (lm *LifecycleMetrics) RecordPhaseTransitionAge(ctx context.Context, age time.Duration, fromPhase, toPhase string) {
	labels := metric.WithAttributes(
		attribute.String("from_phase", fromPhase),
		attribute.String("to_phase", toPhase),
	)

	lm.PhaseTransitionAge.Record(ctx, age.Seconds(), labels)
}

// RecordTeardownTriggered records when a teardown is triggered
func (lm *LifecycleMetrics) RecordTeardownTriggered(ctx context.Context, reason, roostName string) {
	labels := metric.WithAttributes(
		attribute.String("reason", reason),
		attribute.String("roost", roostName),
	)

	lm.TeardownsTriggered.Add(ctx, 1, labels)
}

// RecordTeardownTrigger records teardown triggers by type
func (lm *LifecycleMetrics) RecordTeardownTrigger(ctx context.Context, triggerType, reason string) {
	labels := metric.WithAttributes(
		attribute.String("trigger_type", triggerType),
		attribute.String("reason", reason),
	)

	lm.TeardownTriggers.Add(ctx, 1, labels)
}

// RecordTeardownDuration records the duration of a teardown operation
func (lm *LifecycleMetrics) RecordTeardownDuration(ctx context.Context, duration time.Duration, success bool) {
	labels := metric.WithAttributes(
		attribute.Bool("success", success),
	)

	lm.TeardownDuration.Record(ctx, duration.Seconds(), labels)
}

// RecordCleanupCompleted records a completed cleanup operation
func (lm *LifecycleMetrics) RecordCleanupCompleted(ctx context.Context, roostName string) {
	labels := metric.WithAttributes(
		attribute.String("roost", roostName),
	)

	lm.CleanupOperations.Add(ctx, 1, labels)
}

// RecordGarbageCollection records a garbage collection operation
func (lm *LifecycleMetrics) RecordGarbageCollection(ctx context.Context, orphanedCount int) {
	lm.GarbageCollections.Add(ctx, 1)

	if orphanedCount > 0 {
		orphanedLabels := metric.WithAttributes(
			attribute.Int("count", orphanedCount),
		)
		lm.OrphanedResources.Add(ctx, int64(orphanedCount), orphanedLabels)
	}
}

// RecordGCDuration records the duration of a garbage collection operation
func (lm *LifecycleMetrics) RecordGCDuration(ctx context.Context, duration time.Duration) {
	lm.GCDuration.Record(ctx, duration.Seconds())
}

// RecordLifecycleError records a lifecycle operation error
func (lm *LifecycleMetrics) RecordLifecycleError(ctx context.Context, operation, errorType string) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("error_type", errorType),
	)

	lm.LifecycleErrors.Add(ctx, 1, labels)
}
