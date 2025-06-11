package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/health"
	"github.com/birbparty/roost-keeper/internal/helm"
	"github.com/birbparty/roost-keeper/internal/lifecycle"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

const (
	// Finalizer for ManagedRoost cleanup
	ManagedRoostFinalizer = "roost-keeper.io/finalizer"

	// Condition types
	ConditionTypeReady       = "Ready"
	ConditionTypeProgressing = "Progressing"
	ConditionTypeHealthy     = "Healthy"
	ConditionTypeDegraded    = "Degraded"

	// Requeue intervals
	DefaultRequeueInterval = 5 * time.Minute
	FastRequeueInterval    = 30 * time.Second
	SlowRequeueInterval    = 10 * time.Minute
)

// ManagedRoostReconciler reconciles a ManagedRoost object
type ManagedRoostReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	Logger           logr.Logger
	Metrics          *telemetry.OperatorMetrics
	Recorder         record.EventRecorder
	Config           *rest.Config
	ZapLogger        *zap.Logger
	HelmManager      helm.Manager
	HealthChecker    health.Checker
	LifecycleManager *lifecycle.Manager
}

//+kubebuilder:rbac:groups=roost.birb.party,resources=managedroosts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=roost.birb.party,resources=managedroosts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=roost.birb.party,resources=managedroosts/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ManagedRoostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Generate correlation ID for request tracking
	ctx, correlationID := telemetry.WithCorrelationID(ctx)

	// Create enhanced logger with context
	logger := r.Logger.WithValues(
		"managedroost", req.NamespacedName.String(),
		"reconcile_id", generateReconcileID(),
		"correlation_id", correlationID,
	)

	// Add logger to context for tracing
	ctx = telemetry.ToContext(ctx, logger)

	// Start reconcile span for observability
	ctx, span := telemetry.StartControllerSpan(ctx, "reconcile", req.Name, req.Namespace)
	defer span.End()

	start := time.Now()
	var reconcileErr error

	defer func() {
		duration := time.Since(start)
		success := reconcileErr == nil

		if r.Metrics != nil {
			r.Metrics.RecordReconcile(ctx, duration, success, req.Name, req.Namespace)
		}

		if reconcileErr != nil {
			telemetry.RecordSpanError(ctx, reconcileErr)
		}
	}()

	logger.Info("Starting reconcile")

	// Fetch the ManagedRoost instance
	var managedRoost roostv1alpha1.ManagedRoost
	if err := r.Get(ctx, req.NamespacedName, &managedRoost); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ManagedRoost resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ManagedRoost")
		reconcileErr = err
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !managedRoost.GetDeletionTimestamp().IsZero() {
		result, err := r.reconcileDelete(ctx, &managedRoost)
		reconcileErr = err
		return result, err
	}

	// Handle creation/update
	result, err := r.reconcileNormal(ctx, &managedRoost)
	reconcileErr = err
	return result, err
}

func (r *ManagedRoostReconciler) reconcileNormal(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (ctrl.Result, error) {
	log := r.ZapLogger.With(
		zap.String("managedroost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
		zap.String("phase", string(managedRoost.Status.Phase)),
	)

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(managedRoost, ManagedRoostFinalizer) {
		controllerutil.AddFinalizer(managedRoost, ManagedRoostFinalizer)
		if err := r.Update(ctx, managedRoost); err != nil {
			log.Error("Failed to add finalizer", zap.Error(err))
			return ctrl.Result{}, err
		}
		r.Recorder.Event(managedRoost, "Normal", "FinalizerAdded", "Added finalizer for cleanup")
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize phase if not set
	if managedRoost.Status.Phase == "" {
		return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhasePending, "Initializing ManagedRoost")
	}

	// Determine current state and required action
	switch managedRoost.Status.Phase {
	case roostv1alpha1.ManagedRoostPhasePending:
		return r.handlePendingPhase(ctx, managedRoost)
	case roostv1alpha1.ManagedRoostPhaseDeploying:
		return r.handleDeployingPhase(ctx, managedRoost)
	case roostv1alpha1.ManagedRoostPhaseReady:
		return r.handleReadyPhase(ctx, managedRoost)
	case roostv1alpha1.ManagedRoostPhaseFailed:
		return r.handleFailedPhase(ctx, managedRoost)
	case roostv1alpha1.ManagedRoostPhaseTearingDown:
		return r.handleTearingDownPhase(ctx, managedRoost)
	default:
		log.Warn("Unknown phase, resetting to pending", zap.String("unknown_phase", string(managedRoost.Status.Phase)))
		return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhasePending, "Unknown phase detected, resetting")
	}
}

func (r *ManagedRoostReconciler) handlePendingPhase(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (ctrl.Result, error) {
	log := r.ZapLogger.With(zap.String("phase", "pending"))

	// Validate the ManagedRoost specification
	if err := r.validateSpec(ctx, managedRoost); err != nil {
		r.Recorder.Event(managedRoost, "Warning", "ValidationFailed", fmt.Sprintf("Spec validation failed: %v", err))
		return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseFailed, fmt.Sprintf("Validation failed: %v", err))
	}

	// Check if Helm release already exists
	exists, err := r.HelmManager.ReleaseExists(ctx, managedRoost)
	if err != nil {
		log.Error("Failed to check if release exists", zap.Error(err))
		return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseFailed, fmt.Sprintf("Failed to check release: %v", err))
	}

	if exists {
		log.Info("Helm release already exists, transitioning to ready phase")
		return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseReady, "Helm release already exists")
	}

	// Start deployment
	log.Info("Starting Helm deployment")
	return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseDeploying, "Starting Helm chart deployment")
}

func (r *ManagedRoostReconciler) handleDeployingPhase(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (ctrl.Result, error) {
	log := r.ZapLogger.With(zap.String("phase", "deploying"))

	// Attempt Helm installation
	installed, err := r.HelmManager.Install(ctx, managedRoost)
	if err != nil {
		log.Error("Helm installation failed", zap.Error(err))
		r.Recorder.Event(managedRoost, "Warning", "InstallationFailed", fmt.Sprintf("Helm installation failed: %v", err))
		return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseFailed, fmt.Sprintf("Installation failed: %v", err))
	}

	if !installed {
		log.Info("Helm installation still in progress")
		r.updateCondition(managedRoost, ConditionTypeProgressing, metav1.ConditionTrue, "Installing", "Helm installation in progress")
		if err := r.updateStatus(ctx, managedRoost); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: FastRequeueInterval}, nil
	}

	log.Info("Helm installation completed successfully")
	r.Recorder.Event(managedRoost, "Normal", "InstallationCompleted", "Helm chart installed successfully")
	return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseReady, "Helm installation completed")
}

func (r *ManagedRoostReconciler) handleReadyPhase(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (ctrl.Result, error) {
	log := r.ZapLogger.With(zap.String("phase", "ready"))

	// Use lifecycle manager to process lifecycle logic if available
	if r.LifecycleManager != nil {
		if err := r.LifecycleManager.ProcessLifecycle(ctx, managedRoost); err != nil {
			log.Error("Lifecycle processing failed", zap.Error(err))
			// Don't fail hard, fall back to basic processing
		}
	}

	// Check if upgrade is needed
	needsUpgrade, err := r.HelmManager.NeedsUpgrade(ctx, managedRoost)
	if err != nil {
		log.Error("Failed to check if upgrade needed", zap.Error(err))
		return ctrl.Result{RequeueAfter: DefaultRequeueInterval}, err
	}

	if needsUpgrade {
		log.Info("Upgrade needed, starting upgrade")
		r.Recorder.Event(managedRoost, "Normal", "UpgradeStarted", "Starting Helm chart upgrade")
		_, err := r.HelmManager.Upgrade(ctx, managedRoost)
		if err != nil {
			log.Error("Helm upgrade failed", zap.Error(err))
			r.Recorder.Event(managedRoost, "Warning", "UpgradeFailed", fmt.Sprintf("Helm upgrade failed: %v", err))
			return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseFailed, fmt.Sprintf("Upgrade failed: %v", err))
		}
		r.Recorder.Event(managedRoost, "Normal", "UpgradeCompleted", "Helm chart upgraded successfully")
	}

	// Perform health checks
	healthy, err := r.HealthChecker.CheckHealth(ctx, managedRoost)
	if err != nil {
		log.Error("Health check failed", zap.Error(err))
		r.updateCondition(managedRoost, ConditionTypeHealthy, metav1.ConditionFalse, "HealthCheckError", fmt.Sprintf("Health check error: %v", err))
	} else if !healthy {
		log.Info("Health check failed")
		r.updateCondition(managedRoost, ConditionTypeHealthy, metav1.ConditionFalse, "Unhealthy", "Health checks are failing")
	} else {
		r.updateCondition(managedRoost, ConditionTypeHealthy, metav1.ConditionTrue, "Healthy", "Health checks passing")
	}

	// Check teardown policies
	shouldTeardown, reason := r.shouldTeardown(ctx, managedRoost)
	if shouldTeardown {
		log.Info("Teardown policy triggered", zap.String("reason", reason))
		r.Recorder.Event(managedRoost, "Normal", "TeardownTriggered", fmt.Sprintf("Teardown triggered: %s", reason))
		return r.transitionToPhase(ctx, managedRoost, roostv1alpha1.ManagedRoostPhaseTearingDown, reason)
	}

	// Update ready status
	r.updateCondition(managedRoost, ConditionTypeReady, metav1.ConditionTrue, "Ready", "All systems operational")
	r.updateCondition(managedRoost, ConditionTypeProgressing, metav1.ConditionFalse, "Complete", "Deployment completed")

	if err := r.updateStatus(ctx, managedRoost); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: DefaultRequeueInterval}, nil
}

func (r *ManagedRoostReconciler) handleFailedPhase(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (ctrl.Result, error) {
	log := r.ZapLogger.With(zap.String("phase", "failed"))

	// TODO: Implement retry logic or manual intervention handling
	log.Info("ManagedRoost in failed state, manual intervention may be required")

	r.updateCondition(managedRoost, ConditionTypeReady, metav1.ConditionFalse, "Failed", "ManagedRoost has failed")
	r.updateCondition(managedRoost, ConditionTypeProgressing, metav1.ConditionFalse, "Failed", "Operation failed")

	if err := r.updateStatus(ctx, managedRoost); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: SlowRequeueInterval}, nil
}

func (r *ManagedRoostReconciler) handleTearingDownPhase(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (ctrl.Result, error) {
	log := r.ZapLogger.With(zap.String("phase", "tearing_down"))

	// Perform cleanup
	if err := r.HelmManager.Uninstall(ctx, managedRoost); err != nil {
		log.Error("Failed to uninstall Helm release", zap.Error(err))
		r.Recorder.Event(managedRoost, "Warning", "UninstallFailed", fmt.Sprintf("Failed to uninstall: %v", err))
		return ctrl.Result{RequeueAfter: FastRequeueInterval}, err
	}

	log.Info("Helm release uninstalled successfully")
	r.Recorder.Event(managedRoost, "Normal", "TeardownCompleted", "ManagedRoost teardown completed")

	// Mark for deletion by removing finalizer
	controllerutil.RemoveFinalizer(managedRoost, ManagedRoostFinalizer)
	if err := r.Update(ctx, managedRoost); err != nil {
		log.Error("Failed to remove finalizer", zap.Error(err))
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ManagedRoostReconciler) reconcileDelete(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (ctrl.Result, error) {
	log := r.ZapLogger.With(
		zap.String("managedroost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
		zap.String("operation", "delete"),
	)

	log.Info("Starting deletion reconcile")

	if !controllerutil.ContainsFinalizer(managedRoost, ManagedRoostFinalizer) {
		log.Info("No finalizer present, allowing deletion")
		return ctrl.Result{}, nil
	}

	// Cleanup Helm release
	if err := r.HelmManager.Uninstall(ctx, managedRoost); err != nil {
		log.Error("Failed to uninstall Helm release", zap.Error(err))
		r.Recorder.Event(managedRoost, "Warning", "UninstallFailed", fmt.Sprintf("Failed to uninstall Helm release: %v", err))
		return ctrl.Result{RequeueAfter: FastRequeueInterval}, err
	}

	log.Info("Helm release uninstalled successfully")
	r.Recorder.Event(managedRoost, "Normal", "UninstallCompleted", "Helm release uninstalled successfully")

	// Remove finalizer to allow deletion
	controllerutil.RemoveFinalizer(managedRoost, ManagedRoostFinalizer)
	if err := r.Update(ctx, managedRoost); err != nil {
		log.Error("Failed to remove finalizer", zap.Error(err))
		return ctrl.Result{}, err
	}

	log.Info("Finalizer removed, deletion complete")
	return ctrl.Result{}, nil
}

func (r *ManagedRoostReconciler) transitionToPhase(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, newPhase roostv1alpha1.ManagedRoostPhase, reason string) (ctrl.Result, error) {
	oldPhase := managedRoost.Status.Phase
	managedRoost.Status.Phase = newPhase
	managedRoost.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}

	r.ZapLogger.Info("Phase transition",
		zap.String("from", string(oldPhase)),
		zap.String("to", string(newPhase)),
		zap.String("reason", reason),
	)

	// Update appropriate conditions based on phase
	switch newPhase {
	case roostv1alpha1.ManagedRoostPhasePending:
		r.updateCondition(managedRoost, ConditionTypeProgressing, metav1.ConditionTrue, "Pending", reason)
	case roostv1alpha1.ManagedRoostPhaseDeploying:
		r.updateCondition(managedRoost, ConditionTypeProgressing, metav1.ConditionTrue, "Deploying", reason)
	case roostv1alpha1.ManagedRoostPhaseReady:
		r.updateCondition(managedRoost, ConditionTypeReady, metav1.ConditionTrue, "Ready", reason)
		r.updateCondition(managedRoost, ConditionTypeProgressing, metav1.ConditionFalse, "Complete", reason)
	case roostv1alpha1.ManagedRoostPhaseFailed:
		r.updateCondition(managedRoost, ConditionTypeReady, metav1.ConditionFalse, "Failed", reason)
		r.updateCondition(managedRoost, ConditionTypeProgressing, metav1.ConditionFalse, "Failed", reason)
	}

	if err := r.updateStatus(ctx, managedRoost); err != nil {
		return ctrl.Result{}, err
	}

	// Emit event for phase transition
	r.Recorder.Event(managedRoost, "Normal", "PhaseTransition",
		fmt.Sprintf("Transitioned from %s to %s: %s", oldPhase, newPhase, reason))

	return ctrl.Result{Requeue: true}, nil
}

func (r *ManagedRoostReconciler) updateCondition(managedRoost *roostv1alpha1.ManagedRoost, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: managedRoost.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	meta.SetStatusCondition(&managedRoost.Status.Conditions, condition)
}

func (r *ManagedRoostReconciler) updateStatus(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	managedRoost.Status.ObservedGeneration = managedRoost.Generation

	if err := r.Status().Update(ctx, managedRoost); err != nil {
		r.ZapLogger.Error("Failed to update status",
			zap.String("name", managedRoost.Name),
			zap.String("namespace", managedRoost.Namespace),
			zap.Error(err))
		return err
	}

	return nil
}

func (r *ManagedRoostReconciler) validateSpec(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) error {
	// Validate chart specification
	if managedRoost.Spec.Chart.Name == "" {
		return fmt.Errorf("chart name is required")
	}

	if managedRoost.Spec.Chart.Repository.URL == "" {
		return fmt.Errorf("chart repository URL is required")
	}

	if managedRoost.Spec.Chart.Version == "" {
		return fmt.Errorf("chart version is required")
	}

	// Additional validation logic...
	return nil
}

func (r *ManagedRoostReconciler) shouldTeardown(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost) (bool, string) {
	if managedRoost.Spec.TeardownPolicy == nil {
		return false, ""
	}

	// Check all teardown triggers
	for _, trigger := range managedRoost.Spec.TeardownPolicy.Triggers {
		should, reason := r.evaluateTeardownTrigger(ctx, managedRoost, trigger)
		if should {
			return true, reason
		}
	}

	return false, ""
}

func (r *ManagedRoostReconciler) evaluateTeardownTrigger(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, trigger roostv1alpha1.TeardownTriggerSpec) (bool, string) {
	switch trigger.Type {
	case "timeout":
		if trigger.Timeout != nil {
			if time.Since(managedRoost.CreationTimestamp.Time) > trigger.Timeout.Duration {
				return true, fmt.Sprintf("timeout exceeded: %v", trigger.Timeout.Duration)
			}
		}
	case "failure_count":
		// Implementation for failure count trigger
	case "resource_threshold":
		// Implementation for resource threshold trigger
	case "schedule":
		// Implementation for schedule-based trigger
	}

	return false, ""
}

// SetupWithManager sets up the controller with the Manager with enterprise configuration.
func (r *ManagedRoostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	recoverPanic := true
	return ctrl.NewControllerManagedBy(mgr).
		For(&roostv1alpha1.ManagedRoost{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 10,            // Enterprise concurrency setting
			RecoverPanic:            &recoverPanic, // Enable panic recovery for stability
		}).
		Complete(r)
}

// generateReconcileID generates a unique ID for each reconciliation
func generateReconcileID() string {
	return time.Now().Format("20060102-150405.000")
}
