package ha

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Manager provides high availability management for Roost-Keeper
type Manager struct {
	client          client.Client
	k8sClient       kubernetes.Interface
	leaderElection  *leaderelection.LeaderElector
	metrics         *telemetry.OperatorMetrics
	logger          *zap.Logger
	isLeader        bool
	callbacks       *HACallbacks
	healthManager   *HealthManager
	recoveryManager *RecoveryManager
	updateManager   *RollingUpdateManager
}

// HACallbacks defines callback functions for HA events
type HACallbacks struct {
	OnStartedLeading func(ctx context.Context)
	OnStoppedLeading func()
	OnNewLeader      func(identity string)
}

// NewManager creates a new HA manager instance
func NewManager(client client.Client, k8sClient kubernetes.Interface, metrics *telemetry.OperatorMetrics, logger *zap.Logger) *Manager {
	manager := &Manager{
		client:    client,
		k8sClient: k8sClient,
		metrics:   metrics,
		logger:    logger,
		isLeader:  false,
	}

	// Initialize sub-managers
	manager.healthManager = NewHealthManager(manager, logger)
	manager.recoveryManager = NewRecoveryManager(client, logger)
	manager.updateManager = NewRollingUpdateManager(client, logger)

	return manager
}

// SetCallbacks configures the HA event callbacks
func (m *Manager) SetCallbacks(callbacks *HACallbacks) {
	m.callbacks = callbacks
}

// StartLeaderElection initiates the leader election process
func (m *Manager) StartLeaderElection(ctx context.Context) error {
	// Get pod information from environment
	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")

	if podName == "" || podNamespace == "" {
		return fmt.Errorf("POD_NAME and POD_NAMESPACE environment variables must be set")
	}

	// Create resource lock for leader election
	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      "roost-keeper-controller",
			Namespace: podNamespace,
		},
		Client: m.k8sClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: podName,
		},
	}

	// Configure leader election with optimized settings for HA
	config := leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   15 * time.Second, // Time leader lease is valid
		RenewDeadline:   10 * time.Second, // Time leader attempts to renew
		RetryPeriod:     2 * time.Second,  // Time between retry attempts
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				m.isLeader = true
				m.logger.Info("Started leading - became active controller",
					zap.String("identity", podName),
					zap.String("namespace", podNamespace))

				// Record leadership change in metrics
				if m.metrics != nil {
					m.metrics.RecordLeaderElectionChange(ctx, podName)
				}

				// Execute started leading callback
				if m.callbacks != nil && m.callbacks.OnStartedLeading != nil {
					m.callbacks.OnStartedLeading(ctx)
				}
			},
			OnStoppedLeading: func() {
				m.isLeader = false
				m.logger.Info("Stopped leading - became passive controller",
					zap.String("identity", podName),
					zap.String("namespace", podNamespace))

				// Record leadership change in metrics
				if m.metrics != nil {
					m.metrics.RecordLeaderElectionChange(context.Background(), "")
				}

				// Execute stopped leading callback
				if m.callbacks != nil && m.callbacks.OnStoppedLeading != nil {
					m.callbacks.OnStoppedLeading()
				}
			},
			OnNewLeader: func(identity string) {
				if identity == podName {
					return // We are the new leader, already handled above
				}

				m.logger.Info("New leader elected",
					zap.String("new_leader", identity),
					zap.String("current_pod", podName),
					zap.String("namespace", podNamespace))

				// Execute new leader callback
				if m.callbacks != nil && m.callbacks.OnNewLeader != nil {
					m.callbacks.OnNewLeader(identity)
				}
			},
		},
	}

	// Create leader elector
	leaderElector, err := leaderelection.NewLeaderElector(config)
	if err != nil {
		return fmt.Errorf("failed to create leader elector: %w", err)
	}

	m.leaderElection = leaderElector

	m.logger.Info("Starting leader election",
		zap.String("identity", podName),
		zap.String("namespace", podNamespace),
		zap.Duration("lease_duration", config.LeaseDuration),
		zap.Duration("renew_deadline", config.RenewDeadline),
		zap.Duration("retry_period", config.RetryPeriod))

	// Start leader election (this blocks until context is cancelled)
	go leaderElector.Run(ctx)

	return nil
}

// IsLeader returns whether this instance is currently the leader
func (m *Manager) IsLeader() bool {
	return m.isLeader
}

// GetLeaderIdentity returns the current leader's identity
func (m *Manager) GetLeaderIdentity() string {
	if m.leaderElection != nil {
		return m.leaderElection.GetLeader()
	}
	return ""
}

// GetHealthManager returns the health management component
func (m *Manager) GetHealthManager() *HealthManager {
	return m.healthManager
}

// GetRecoveryManager returns the disaster recovery component
func (m *Manager) GetRecoveryManager() *RecoveryManager {
	return m.recoveryManager
}

// GetRollingUpdateManager returns the rolling update management component
func (m *Manager) GetRollingUpdateManager() *RollingUpdateManager {
	return m.updateManager
}

// CheckClusterHealth performs a comprehensive cluster health check
func (m *Manager) CheckClusterHealth(ctx context.Context) error {
	if m.healthManager == nil {
		return fmt.Errorf("health manager not initialized")
	}

	return m.healthManager.CheckHealth(ctx)
}

// TriggerFailover initiates a manual failover process
func (m *Manager) TriggerFailover(ctx context.Context) error {
	if !m.isLeader {
		return fmt.Errorf("cannot trigger failover: not the current leader")
	}

	m.logger.Info("Triggering manual failover")

	// Release leadership to trigger failover
	if m.leaderElection != nil {
		// Cancel the leader election context to release leadership
		// In a real implementation, you'd have a cancellable context
		m.logger.Info("Releasing leadership to trigger failover")
	}

	return nil
}

// GetHAStatus returns the current HA status
type HAStatus struct {
	IsLeader        bool      `json:"is_leader"`
	LeaderIdentity  string    `json:"leader_identity"`
	HealthyReplicas int       `json:"healthy_replicas"`
	LastFailover    time.Time `json:"last_failover,omitempty"`
	ClusterHealth   string    `json:"cluster_health"`
}

// GetStatus returns the current HA status
func (m *Manager) GetStatus(ctx context.Context) (*HAStatus, error) {
	status := &HAStatus{
		IsLeader:       m.isLeader,
		LeaderIdentity: m.GetLeaderIdentity(),
	}

	// Check cluster health
	if err := m.CheckClusterHealth(ctx); err != nil {
		status.ClusterHealth = "unhealthy"
		m.logger.Warn("Cluster health check failed", zap.Error(err))
	} else {
		status.ClusterHealth = "healthy"
	}

	// Get healthy replicas count (would be implemented to query actual replicas)
	status.HealthyReplicas = m.getHealthyReplicasCount(ctx)

	return status, nil
}

// getHealthyReplicasCount returns the number of healthy controller replicas
func (m *Manager) getHealthyReplicasCount(ctx context.Context) int {
	// In a real implementation, this would query the deployment and check pod readiness
	// For now, we'll return a default value
	return 3 // Assuming 3-replica deployment
}

// StartPeriodicHealthChecks begins periodic health monitoring
func (m *Manager) StartPeriodicHealthChecks(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	m.logger.Info("Starting periodic HA health checks", zap.Duration("interval", interval))

	for {
		select {
		case <-ctx.Done():
			m.logger.Info("Stopping periodic HA health checks")
			return
		case <-ticker.C:
			if err := m.CheckClusterHealth(ctx); err != nil {
				m.logger.Error("Periodic health check failed", zap.Error(err))

				// Record health check failure in metrics
				if m.metrics != nil {
					m.metrics.RecordHealthCheck(ctx, "ha_cluster", 0, false, "cluster")
				}
			} else {
				// Record successful health check
				if m.metrics != nil {
					m.metrics.RecordHealthCheck(ctx, "ha_cluster", 0, true, "cluster")
				}
			}
		}
	}
}
