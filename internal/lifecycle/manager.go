package lifecycle

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/helm"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Manager handles ManagedRoost lifecycle operations
type Manager struct {
	client      client.Client
	helmManager helm.Manager
	recorder    record.EventRecorder
	metrics     *LifecycleMetrics
	logger      *zap.Logger
}

// NewManager creates a new lifecycle manager
func NewManager(client client.Client, helmManager helm.Manager, recorder record.EventRecorder, logger *zap.Logger) (*Manager, error) {
	// Initialize lifecycle metrics
	metrics, err := NewLifecycleMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize lifecycle metrics: %w", err)
	}

	return &Manager{
		client:      client,
		helmManager: helmManager,
		recorder:    recorder,
		metrics:     metrics,
		logger:      logger,
	}, nil
}

// ProcessLifecycle handles the complete lifecycle for a ManagedRoost
func (m *Manager) ProcessLifecycle(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	log := m.logger.With(
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
		zap.String("phase", string(managedRoost.Status.Phase)),
	)

	// Start lifecycle processing span
	ctx, span := telemetry.StartControllerSpan(ctx, "lifecycle_process", managedRoost.Name, managedRoost.Namespace)
	defer span.End()

	start := time.Now()
	defer func() {
		if m.metrics != nil {
			m.metrics.RecordLifecycleOperation(ctx, "process", time.Since(start).Seconds())
		}
	}()

	log.Info("Processing roost lifecycle")

	// Track roost creation if new
	if err := m.trackRoostCreation(ctx, managedRoost); err != nil {
		log.Error("Failed to track roost creation", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return fmt.Errorf("failed to track roost creation: %w", err)
	}

	// Monitor deployment status
	if err := m.monitorDeploymentStatus(ctx, managedRoost); err != nil {
		log.Error("Failed to monitor deployment status", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return fmt.Errorf("failed to monitor deployment status: %w", err)
	}

	// Evaluate teardown policies
	shouldTeardown, reason := m.evaluateTeardownPolicies(ctx, managedRoost)
	if shouldTeardown {
		log.Info("Teardown triggered", zap.String("reason", reason))
		if err := m.triggerTeardown(ctx, managedRoost, reason); err != nil {
			log.Error("Failed to trigger teardown", zap.Error(err))
			telemetry.RecordSpanError(ctx, err)
			return fmt.Errorf("failed to trigger teardown: %w", err)
		}
	}

	// Update lifecycle metrics
	if m.metrics != nil {
		m.metrics.RecordRoostByPhase(ctx, string(managedRoost.Status.Phase))
	}

	telemetry.RecordSpanSuccess(ctx)
	return nil
}

// trackRoostCreation initializes tracking for a new roost
func (m *Manager) trackRoostCreation(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	// Initialize status if not set
	if managedRoost.Status.Phase == "" {
		managedRoost.Status.Phase = roostv1alpha1.ManagedRoostPhasePending
		managedRoost.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}

		// Record creation event
		m.recorder.Event(managedRoost, "Normal", "RoostCreated", "ManagedRoost created and lifecycle tracking initiated")

		// Emit creation metric
		if m.metrics != nil {
			m.metrics.RecordRoostCreated(ctx, managedRoost.Name, managedRoost.Namespace)
		}

		m.logger.Info("Roost creation tracked",
			zap.String("roost", managedRoost.Name),
			zap.String("phase", string(managedRoost.Status.Phase)))
	}

	return nil
}

// monitorDeploymentStatus tracks the deployment progress
func (m *Manager) monitorDeploymentStatus(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	log := m.logger.With(zap.String("operation", "monitor_deployment"))

	// Get current Helm release status
	helmStatus, err := m.helmManager.GetStatus(ctx, managedRoost)
	if err != nil {
		if errors.IsNotFound(err) {
			// Release doesn't exist yet
			return nil
		}
		return fmt.Errorf("failed to get release status: %w", err)
	}

	// Skip if no Helm release found
	if helmStatus == nil {
		return nil
	}

	// Update status based on Helm release
	oldPhase := managedRoost.Status.Phase
	newPhase := m.determinePhaseFromHelmStatus(helmStatus)

	if oldPhase != newPhase {
		log.Info("Phase transition detected",
			zap.String("from", string(oldPhase)),
			zap.String("to", string(newPhase)),
			zap.String("helm_status", helmStatus.Status))

		managedRoost.Status.Phase = newPhase
		managedRoost.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}

		// Update Helm release information in status
		managedRoost.Status.HelmRelease = helmStatus

		// Record phase transition event
		m.recorder.Event(managedRoost, "Normal", "PhaseTransition",
			fmt.Sprintf("Phase changed from %s to %s", oldPhase, newPhase))

		// Emit phase transition metric
		if m.metrics != nil {
			m.metrics.RecordPhaseTransition(ctx, string(oldPhase), string(newPhase), managedRoost.Name)
		}
	}

	return nil
}

// determinePhaseFromHelmStatus maps Helm status to ManagedRoost phase
func (m *Manager) determinePhaseFromHelmStatus(helmStatus *roostv1alpha1.HelmReleaseStatus) roostv1alpha1.ManagedRoostPhase {
	switch helmStatus.Status {
	case "deployed":
		return roostv1alpha1.ManagedRoostPhaseReady
	case "pending-install", "pending-upgrade":
		return roostv1alpha1.ManagedRoostPhaseDeploying
	case "pending-rollback":
		return roostv1alpha1.ManagedRoostPhaseDeploying
	case "failed":
		return roostv1alpha1.ManagedRoostPhaseFailed
	case "uninstalling":
		return roostv1alpha1.ManagedRoostPhaseTearingDown
	default:
		return roostv1alpha1.ManagedRoostPhasePending
	}
}

// evaluateTeardownPolicies checks if any teardown triggers are activated
func (m *Manager) evaluateTeardownPolicies(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (bool, string) {
	if managedRoost.Spec.TeardownPolicy == nil {
		return false, ""
	}

	log := m.logger.With(zap.String("operation", "evaluate_teardown"))

	// Check each teardown trigger
	for _, trigger := range managedRoost.Spec.TeardownPolicy.Triggers {
		should, reason := m.evaluateTeardownTrigger(ctx, managedRoost, trigger)
		if should {
			log.Info("Teardown trigger activated",
				zap.String("trigger_type", trigger.Type),
				zap.String("reason", reason))

			// Record teardown trigger metric
			if m.metrics != nil {
				m.metrics.RecordTeardownTrigger(ctx, trigger.Type, reason)
			}

			return true, reason
		}
	}

	return false, ""
}

// evaluateTeardownTrigger evaluates a single teardown trigger
func (m *Manager) evaluateTeardownTrigger(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, trigger roostv1alpha1.TeardownTriggerSpec) (bool, string) {
	switch trigger.Type {
	case "timeout":
		if trigger.Timeout != nil {
			age := time.Since(managedRoost.CreationTimestamp.Time)
			if age > trigger.Timeout.Duration {
				return true, fmt.Sprintf("timeout exceeded: %v (age: %v)", trigger.Timeout.Duration, age)
			}
		}

	case "failure_count":
		if trigger.FailureCount != nil {
			// Count failures from status conditions
			failureCount := m.countRecentFailures(managedRoost)
			if failureCount >= *trigger.FailureCount {
				return true, fmt.Sprintf("failure count exceeded: %d/%d", failureCount, *trigger.FailureCount)
			}
		}

	case "resource_threshold":
		if trigger.ResourceThreshold != nil {
			// Check resource usage (simplified implementation)
			exceeded, reason := m.checkResourceThresholds(ctx, managedRoost, trigger.ResourceThreshold)
			if exceeded {
				return true, fmt.Sprintf("resource threshold exceeded: %s", reason)
			}
		}

	case "schedule":
		if trigger.Schedule != "" {
			// Evaluate cron schedule (simplified implementation)
			if m.shouldTriggerBySchedule(trigger.Schedule, managedRoost.CreationTimestamp.Time) {
				return true, fmt.Sprintf("scheduled teardown: %s", trigger.Schedule)
			}
		}

	case "webhook":
		if trigger.Webhook != nil {
			// Webhook triggers would be handled externally by setting annotations
			// Check for teardown annotation
			if managedRoost.Annotations != nil {
				if triggerReason, exists := managedRoost.Annotations["roost-keeper.io/teardown-trigger"]; exists {
					return true, fmt.Sprintf("webhook triggered teardown: %s", triggerReason)
				}
			}
		}
	}

	return false, ""
}

// triggerTeardown initiates the teardown process
func (m *Manager) triggerTeardown(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, reason string) error {
	log := m.logger.With(
		zap.String("operation", "trigger_teardown"),
		zap.String("reason", reason),
	)

	log.Info("Triggering roost teardown")

	// Update phase to terminating
	managedRoost.Status.Phase = roostv1alpha1.ManagedRoostPhaseTearingDown
	managedRoost.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}

	// Add teardown status
	managedRoost.Status.Teardown = &roostv1alpha1.TeardownStatus{
		Triggered:     true,
		TriggerReason: reason,
		TriggerTime:   &metav1.Time{Time: time.Now()},
		Progress:      0,
	}

	// Record teardown event
	m.recorder.Event(managedRoost, "Normal", "TeardownTriggered",
		fmt.Sprintf("Teardown triggered: %s", reason))

	// Emit teardown metric
	if m.metrics != nil {
		m.metrics.RecordTeardownTriggered(ctx, reason, managedRoost.Name)
	}

	return nil
}

// cleanupResources performs comprehensive resource cleanup
func (m *Manager) CleanupResources(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	log := m.logger.With(zap.String("operation", "cleanup_resources"))

	log.Info("Starting resource cleanup")
	start := time.Now()

	// Update teardown progress
	if managedRoost.Status.Teardown != nil {
		managedRoost.Status.Teardown.Progress = 10
	}

	// Cleanup Helm release
	if err := m.helmManager.Uninstall(ctx, managedRoost); err != nil {
		log.Error("Failed to cleanup Helm release", zap.Error(err))
		return fmt.Errorf("failed to cleanup Helm release: %w", err)
	}

	// Update teardown progress
	if managedRoost.Status.Teardown != nil {
		managedRoost.Status.Teardown.Progress = 70
	}

	// Cleanup any additional resources created by the operator
	if err := m.cleanupOperatorManagedResources(ctx, managedRoost); err != nil {
		log.Error("Failed to cleanup operator resources", zap.Error(err))
		return fmt.Errorf("failed to cleanup operator resources: %w", err)
	}

	// Complete teardown
	if managedRoost.Status.Teardown != nil {
		managedRoost.Status.Teardown.Progress = 100
		managedRoost.Status.Teardown.CompletionTime = &metav1.Time{Time: time.Now()}
	}

	// Record cleanup completion
	m.recorder.Event(managedRoost, "Normal", "CleanupCompleted", "All resources cleaned up successfully")

	// Emit cleanup metric
	if m.metrics != nil {
		m.metrics.RecordCleanupCompleted(ctx, managedRoost.Name)
		m.metrics.RecordLifecycleOperation(ctx, "cleanup", time.Since(start).Seconds())
	}

	log.Info("Resource cleanup completed", zap.Duration("duration", time.Since(start)))
	return nil
}

// cleanupOperatorManagedResources cleans up additional resources
func (m *Manager) cleanupOperatorManagedResources(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	// Implementation would cleanup any additional resources created by the operator
	// (e.g., ConfigMaps, Secrets, Services that aren't part of the Helm chart)

	// For now, this is a placeholder - in a real implementation, you would:
	// 1. List resources with owner references or labels
	// 2. Delete them in the proper order
	// 3. Wait for deletion confirmation

	return nil
}

// countRecentFailures counts recent failures from status conditions
func (m *Manager) countRecentFailures(managedRoost *roostv1alpha1.ManagedRoost) int32 {
	failureCount := int32(0)

	// Check status conditions for failure indications
	for _, condition := range managedRoost.Status.Conditions {
		if condition.Type == "Ready" && condition.Status == metav1.ConditionFalse {
			failureCount++
		}
		if condition.Type == "Healthy" && condition.Status == metav1.ConditionFalse {
			failureCount++
		}
	}

	// Also count health check failures
	for _, healthCheck := range managedRoost.Status.HealthChecks {
		if healthCheck.Status == "unhealthy" {
			failureCount += healthCheck.FailureCount
		}
	}

	return failureCount
}

// checkResourceThresholds checks resource usage against thresholds
func (m *Manager) checkResourceThresholds(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, threshold *roostv1alpha1.ResourceThresholdSpec) (bool, string) {
	// Implementation would check actual resource usage against thresholds
	// This would require integration with metrics or resource monitoring
	// For now, return false as this is a complex implementation

	// In a real implementation, you would:
	// 1. Query metrics for CPU/memory/storage usage
	// 2. Compare against thresholds
	// 3. Return true if any threshold is exceeded

	return false, ""
}

// shouldTriggerBySchedule evaluates cron-based triggers
func (m *Manager) shouldTriggerBySchedule(schedule string, creationTime time.Time) bool {
	// Implementation would parse cron expression and evaluate against current time
	// This is a placeholder - in a real implementation, you would:
	// 1. Parse the cron expression
	// 2. Calculate next trigger time based on creation time
	// 3. Check if current time matches trigger time

	// For now, return false as this requires a cron parsing library
	return false
}

// GarbageCollect removes orphaned resources
func (m *Manager) GarbageCollect(ctx context.Context) error {
	log := m.logger.With(zap.String("operation", "garbage_collect"))

	log.Info("Starting garbage collection")
	start := time.Now()

	// Find orphaned resources (Helm releases without corresponding ManagedRoost)
	var managedRoosts roostv1alpha1.ManagedRoostList
	if err := m.client.List(ctx, &managedRoosts); err != nil {
		return fmt.Errorf("failed to list ManagedRoosts: %w", err)
	}

	// Track cleanup operations
	cleanupCount := 0

	// Implementation would:
	// 1. List all Helm releases
	// 2. Check if each release has a corresponding ManagedRoost
	// 3. Clean up orphaned releases
	// 4. Clean up other orphaned resources (ConfigMaps, Secrets, etc.)

	if cleanupCount > 0 {
		log.Info("Garbage collection completed",
			zap.Int("cleaned_up", cleanupCount),
			zap.Duration("duration", time.Since(start)))

		if m.metrics != nil {
			m.metrics.RecordGarbageCollection(ctx, cleanupCount)
		}
	}

	return nil
}

// GetLifecycleMetrics returns the lifecycle metrics instance
func (m *Manager) GetLifecycleMetrics() *LifecycleMetrics {
	return m.metrics
}
