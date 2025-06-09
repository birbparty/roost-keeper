package controllers

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// ManagedRoostReconciler reconciles a ManagedRoost object
type ManagedRoostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger logr.Logger
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
	logger := r.Logger.WithValues(
		"managedroost", req.NamespacedName.String(),
		"reconcile_id", generateReconcileID(),
	)

	// Add logger to context for tracing
	ctx = telemetry.ToContext(ctx, logger)

	// Start trace for reconciliation
	tracer := telemetry.Tracer()
	ctx, span := tracer.Start(ctx, "ManagedRoost.Reconcile")
	defer span.End()

	logger.Info("Starting reconciliation")

	// Fetch the ManagedRoost instance
	var managedRoost roostv1alpha1.ManagedRoost
	if err := r.Get(ctx, req.NamespacedName, &managedRoost); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("ManagedRoost resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get ManagedRoost")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully fetched ManagedRoost",
		"phase", string(managedRoost.Status.Phase),
		"generation", managedRoost.Generation,
		"observed_generation", managedRoost.Status.ObservedGeneration,
	)

	// TODO: Implement reconciliation logic
	// This will be expanded in subsequent tasks:
	// 1. Helm chart deployment
	// 2. Health check implementation
	// 3. Teardown policy enforcement

	// Update observed generation
	if managedRoost.Status.ObservedGeneration != managedRoost.Generation {
		managedRoost.Status.ObservedGeneration = managedRoost.Generation
		if err := r.Status().Update(ctx, &managedRoost); err != nil {
			logger.Error(err, "Failed to update observed generation")
			return ctrl.Result{}, err
		}
	}

	logger.Info("Reconciliation completed successfully")
	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
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
