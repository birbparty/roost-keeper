package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/otel/attribute"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// ManagedRoostReconciler reconciles a ManagedRoost object
type ManagedRoostReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Logger  logr.Logger
	Metrics *telemetry.OperatorMetrics
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

	// Wrap the entire reconciliation with observability middleware
	wrappedReconcile := telemetry.ControllerMiddleware(
		func(ctx context.Context) error {
			return r.doReconcile(ctx, req)
		},
		"reconcile",
		r.Metrics,
		req.Name,
		req.Namespace,
	)

	err := wrappedReconcile(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
}

// doReconcile performs the actual reconciliation logic
func (r *ManagedRoostReconciler) doReconcile(ctx context.Context, req ctrl.Request) error {
	// Start specific span for reconcile logic
	ctx, span := telemetry.StartControllerSpan(ctx, "reconcile.logic", req.Name, req.Namespace)
	defer span.End()

	// Log reconciliation start with enhanced context
	telemetry.LogInfo(ctx, "Starting reconciliation",
		"managedroost", req.NamespacedName.String(),
	)

	// Fetch the ManagedRoost instance with API call tracking
	var managedRoost roostv1alpha1.ManagedRoost
	err := telemetry.KubernetesAPIMiddleware(
		func(ctx context.Context) error {
			return r.Get(ctx, req.NamespacedName, &managedRoost)
		},
		"GET",
		"ManagedRoost",
		req.Namespace,
		r.Metrics,
	)(ctx)

	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			telemetry.LogInfo(ctx, "ManagedRoost resource not found, likely deleted")
			return nil
		}
		telemetry.RecordSpanError(ctx, err)
		return err
	}

	// Add resource details to span
	telemetry.WithSpanAttributes(ctx,
		attribute.Int64("roost.generation", int64(managedRoost.Generation)),
		attribute.String("roost.phase", string(managedRoost.Status.Phase)),
		attribute.Int64("roost.observed_generation", int64(managedRoost.Status.ObservedGeneration)),
	)

	telemetry.LogInfo(ctx, "Successfully fetched ManagedRoost",
		"phase", string(managedRoost.Status.Phase),
		"generation", managedRoost.Generation,
		"observed_generation", managedRoost.Status.ObservedGeneration,
	)

	// Update metrics for roost tracking
	if r.Metrics != nil {
		// Calculate generation lag
		generationLag := managedRoost.Generation - managedRoost.Status.ObservedGeneration
		r.Metrics.UpdateGenerationLag(ctx, generationLag, req.Name, req.Namespace)

		// Update roost phase metrics
		r.Metrics.UpdateRoostMetrics(ctx, 1, 0, string(managedRoost.Status.Phase), 1)
	}

	// TODO: Implement reconciliation logic
	// This will be expanded in subsequent tasks:
	// 1. Helm chart deployment
	// 2. Health check implementation
	// 3. Teardown policy enforcement

	// Update observed generation with API call tracking
	if managedRoost.Status.ObservedGeneration != managedRoost.Generation {
		telemetry.WithSpanEvent(ctx, "updating_observed_generation",
			attribute.Int64("from_generation", int64(managedRoost.Status.ObservedGeneration)),
			attribute.Int64("to_generation", int64(managedRoost.Generation)),
		)

		managedRoost.Status.ObservedGeneration = managedRoost.Generation

		err := telemetry.KubernetesAPIMiddleware(
			func(ctx context.Context) error {
				return r.Status().Update(ctx, &managedRoost)
			},
			"UPDATE",
			"ManagedRoost/status",
			req.Namespace,
			r.Metrics,
		)(ctx)

		if err != nil {
			telemetry.RecordSpanError(ctx, err)
			return err
		}

		telemetry.LogInfo(ctx, "Updated observed generation",
			"generation", managedRoost.Generation,
		)
	}

	telemetry.RecordSpanSuccess(ctx)
	telemetry.LogInfo(ctx, "Reconciliation completed successfully")
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagedRoostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&roostv1alpha1.ManagedRoost{}).
		Complete(r)
}

// generateReconcileID generates a unique ID for each reconciliation
func generateReconcileID() string {
	return time.Now().Format("20060102-150405.000")
}
