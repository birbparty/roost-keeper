package ha

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HealthManager manages HA-specific health checks
type HealthManager struct {
	logger       *zap.Logger
	haManager    *Manager
	healthChecks map[string]HealthCheck
}

// HealthCheck interface for individual health checks
type HealthCheck interface {
	Name() string
	Check(ctx context.Context) error
	GetTimeout() time.Duration
}

// NewHealthManager creates a new health manager for HA
func NewHealthManager(haManager *Manager, logger *zap.Logger) *HealthManager {
	hm := &HealthManager{
		logger:       logger,
		haManager:    haManager,
		healthChecks: make(map[string]HealthCheck),
	}

	// Register default HA health checks
	hm.RegisterHealthCheck(NewLeaderElectionHealthCheck(haManager))
	hm.RegisterHealthCheck(NewControllerHealthCheck(haManager.client, haManager.k8sClient))
	hm.RegisterHealthCheck(NewAPIServerHealthCheck(haManager.k8sClient))
	hm.RegisterHealthCheck(NewReplicaHealthCheck(haManager.client, haManager.k8sClient))

	return hm
}

// RegisterHealthCheck adds a new health check to the manager
func (hm *HealthManager) RegisterHealthCheck(check HealthCheck) {
	hm.healthChecks[check.Name()] = check
	hm.logger.Debug("Registered HA health check", zap.String("check", check.Name()))
}

// CheckHealth performs all registered health checks
func (hm *HealthManager) CheckHealth(ctx context.Context) error {
	for name, check := range hm.healthChecks {
		checkCtx, cancel := context.WithTimeout(ctx, check.GetTimeout())
		defer cancel()

		start := time.Now()
		if err := check.Check(checkCtx); err != nil {
			duration := time.Since(start)
			hm.logger.Error("HA health check failed",
				zap.String("check", name),
				zap.Duration("duration", duration),
				zap.Error(err))
			return fmt.Errorf("health check %s failed: %w", name, err)
		}

		duration := time.Since(start)
		hm.logger.Debug("HA health check passed",
			zap.String("check", name),
			zap.Duration("duration", duration))
	}
	return nil
}

// GetHealthStatus returns the status of all health checks
func (hm *HealthManager) GetHealthStatus(ctx context.Context) map[string]bool {
	status := make(map[string]bool)
	for name, check := range hm.healthChecks {
		checkCtx, cancel := context.WithTimeout(ctx, check.GetTimeout())
		status[name] = check.Check(checkCtx) == nil
		cancel()
	}
	return status
}

// Leader Election Health Check
type LeaderElectionHealthCheck struct {
	haManager *Manager
}

func NewLeaderElectionHealthCheck(haManager *Manager) *LeaderElectionHealthCheck {
	return &LeaderElectionHealthCheck{haManager: haManager}
}

func (le *LeaderElectionHealthCheck) Name() string {
	return "leader_election"
}

func (le *LeaderElectionHealthCheck) GetTimeout() time.Duration {
	return 5 * time.Second
}

func (le *LeaderElectionHealthCheck) Check(ctx context.Context) error {
	// Check if leader election is functioning
	if le.haManager.leaderElection == nil {
		return fmt.Errorf("leader election not initialized")
	}

	// Verify that a leader exists
	leader := le.haManager.GetLeaderIdentity()
	if leader == "" {
		return fmt.Errorf("no leader elected")
	}

	return nil
}

// Controller Health Check
type ControllerHealthCheck struct {
	client    client.Client
	k8sClient kubernetes.Interface
}

func NewControllerHealthCheck(client client.Client, k8sClient kubernetes.Interface) *ControllerHealthCheck {
	return &ControllerHealthCheck{
		client:    client,
		k8sClient: k8sClient,
	}
}

func (c *ControllerHealthCheck) Name() string {
	return "controller"
}

func (c *ControllerHealthCheck) GetTimeout() time.Duration {
	return 10 * time.Second
}

func (c *ControllerHealthCheck) Check(ctx context.Context) error {
	// Check if the controller deployment is healthy
	deployment := &appsv1.Deployment{}
	err := c.client.Get(ctx, client.ObjectKey{
		Namespace: "roost-keeper-system",
		Name:      "roost-keeper-controller-manager",
	}, deployment)

	if err != nil {
		return fmt.Errorf("failed to get controller deployment: %w", err)
	}

	// Check deployment readiness
	if deployment.Status.ReadyReplicas == 0 {
		return fmt.Errorf("no ready replicas in controller deployment")
	}

	// Check if deployment is progressing normally
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			if condition.Status != "True" {
				return fmt.Errorf("deployment not progressing: %s", condition.Message)
			}
		}
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status != "True" {
				return fmt.Errorf("deployment not available: %s", condition.Message)
			}
		}
	}

	return nil
}

// API Server Health Check
type APIServerHealthCheck struct {
	k8sClient kubernetes.Interface
}

func NewAPIServerHealthCheck(k8sClient kubernetes.Interface) *APIServerHealthCheck {
	return &APIServerHealthCheck{k8sClient: k8sClient}
}

func (a *APIServerHealthCheck) Name() string {
	return "api_server"
}

func (a *APIServerHealthCheck) GetTimeout() time.Duration {
	return 5 * time.Second
}

func (a *APIServerHealthCheck) Check(ctx context.Context) error {
	// Test API server connectivity by listing namespaces
	_, err := a.k8sClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		return fmt.Errorf("failed to connect to Kubernetes API server: %w", err)
	}
	return nil
}

// Replica Health Check
type ReplicaHealthCheck struct {
	client    client.Client
	k8sClient kubernetes.Interface
}

func NewReplicaHealthCheck(client client.Client, k8sClient kubernetes.Interface) *ReplicaHealthCheck {
	return &ReplicaHealthCheck{
		client:    client,
		k8sClient: k8sClient,
	}
}

func (r *ReplicaHealthCheck) Name() string {
	return "replica_health"
}

func (r *ReplicaHealthCheck) GetTimeout() time.Duration {
	return 10 * time.Second
}

func (r *ReplicaHealthCheck) Check(ctx context.Context) error {
	// Get the controller deployment
	deployment := &appsv1.Deployment{}
	err := r.client.Get(ctx, client.ObjectKey{
		Namespace: "roost-keeper-system",
		Name:      "roost-keeper-controller-manager",
	}, deployment)

	if err != nil {
		return fmt.Errorf("failed to get controller deployment: %w", err)
	}

	// Check that we have the expected number of replicas
	expectedReplicas := *deployment.Spec.Replicas
	readyReplicas := deployment.Status.ReadyReplicas

	if readyReplicas < expectedReplicas {
		return fmt.Errorf("insufficient ready replicas: %d/%d", readyReplicas, expectedReplicas)
	}

	// For HA, we want at least 2 replicas ready
	if readyReplicas < 2 {
		return fmt.Errorf("insufficient replicas for HA: %d (minimum 2 required)", readyReplicas)
	}

	return nil
}

// Network Health Check
type NetworkHealthCheck struct {
	k8sClient kubernetes.Interface
}

func NewNetworkHealthCheck(k8sClient kubernetes.Interface) *NetworkHealthCheck {
	return &NetworkHealthCheck{k8sClient: k8sClient}
}

func (n *NetworkHealthCheck) Name() string {
	return "network"
}

func (n *NetworkHealthCheck) GetTimeout() time.Duration {
	return 5 * time.Second
}

func (n *NetworkHealthCheck) Check(ctx context.Context) error {
	// Check service connectivity
	_, err := n.k8sClient.CoreV1().Services("roost-keeper-system").Get(ctx, "roost-keeper-controller-manager-metrics-service", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to reach controller service: %w", err)
	}
	return nil
}

// Webhook Health Check
type WebhookHealthCheck struct {
	k8sClient kubernetes.Interface
}

func NewWebhookHealthCheck(k8sClient kubernetes.Interface) *WebhookHealthCheck {
	return &WebhookHealthCheck{k8sClient: k8sClient}
}

func (w *WebhookHealthCheck) Name() string {
	return "webhook"
}

func (w *WebhookHealthCheck) GetTimeout() time.Duration {
	return 5 * time.Second
}

func (w *WebhookHealthCheck) Check(ctx context.Context) error {
	// Check webhook service connectivity
	_, err := w.k8sClient.CoreV1().Services("roost-keeper-system").Get(ctx, "roost-keeper-webhook-service", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to reach webhook service: %w", err)
	}
	return nil
}
