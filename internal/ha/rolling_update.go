package ha

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RollingUpdateManager handles zero-downtime rolling updates
type RollingUpdateManager struct {
	client client.Client
	logger *zap.Logger
}

// UpdateStatus represents the status of a rolling update
type UpdateStatus struct {
	ID              string    `json:"id"`
	StartTime       time.Time `json:"start_time"`
	Status          string    `json:"status"`
	CurrentReplicas int32     `json:"current_replicas"`
	UpdatedReplicas int32     `json:"updated_replicas"`
	ReadyReplicas   int32     `json:"ready_replicas"`
	Progress        float64   `json:"progress"`
	Message         string    `json:"message"`
}

// UpdateConfig configures rolling update behavior
type UpdateConfig struct {
	MaxUnavailable *intstr.IntOrString `json:"max_unavailable,omitempty"`
	MaxSurge       *intstr.IntOrString `json:"max_surge,omitempty"`
	Timeout        time.Duration       `json:"timeout"`
	CheckInterval  time.Duration       `json:"check_interval"`
}

// NewRollingUpdateManager creates a new rolling update manager
func NewRollingUpdateManager(client client.Client, logger *zap.Logger) *RollingUpdateManager {
	return &RollingUpdateManager{
		client: client,
		logger: logger,
	}
}

// PerformRollingUpdate executes a rolling update with the new image
func (ru *RollingUpdateManager) PerformRollingUpdate(ctx context.Context, newImage string) (*UpdateStatus, error) {
	ru.logger.Info("Starting rolling update", zap.String("image", newImage))

	updateID := fmt.Sprintf("update-%d", time.Now().Unix())

	status := &UpdateStatus{
		ID:        updateID,
		StartTime: time.Now(),
		Status:    "starting",
		Progress:  0.0,
		Message:   "Initiating rolling update",
	}

	// Get the controller deployment
	deployment := &appsv1.Deployment{}
	err := ru.client.Get(ctx, client.ObjectKey{
		Namespace: "roost-keeper-system",
		Name:      "roost-keeper-controller-manager",
	}, deployment)

	if err != nil {
		status.Status = "failed"
		status.Message = fmt.Sprintf("Failed to get deployment: %v", err)
		return status, fmt.Errorf("failed to get controller deployment: %w", err)
	}

	// Store original image for rollback if needed
	originalImage := deployment.Spec.Template.Spec.Containers[0].Image

	// Configure rolling update strategy
	config := &UpdateConfig{
		MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
		Timeout:        10 * time.Minute,
		CheckInterval:  10 * time.Second,
	}

	// Update deployment image
	deployment.Spec.Template.Spec.Containers[0].Image = newImage

	// Ensure rolling update strategy is set
	if deployment.Spec.Strategy.Type != appsv1.RollingUpdateDeploymentStrategyType {
		deployment.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	}

	if deployment.Spec.Strategy.RollingUpdate == nil {
		deployment.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{}
	}

	deployment.Spec.Strategy.RollingUpdate.MaxUnavailable = config.MaxUnavailable
	deployment.Spec.Strategy.RollingUpdate.MaxSurge = config.MaxSurge

	// Apply the update
	if err := ru.client.Update(ctx, deployment); err != nil {
		status.Status = "failed"
		status.Message = fmt.Sprintf("Failed to update deployment: %v", err)
		return status, fmt.Errorf("failed to update deployment: %w", err)
	}

	status.Status = "in_progress"
	status.Message = "Rolling update initiated"
	status.Progress = 10.0

	ru.logger.Info("Rolling update initiated, monitoring progress",
		zap.String("update_id", updateID),
		zap.String("new_image", newImage),
		zap.String("original_image", originalImage))

	// Monitor the rollout
	if err := ru.monitorRollout(ctx, deployment, status, config); err != nil {
		ru.logger.Error("Rolling update failed, attempting rollback",
			zap.String("update_id", updateID),
			zap.Error(err))

		// Attempt rollback
		if rollbackErr := ru.rollbackUpdate(ctx, deployment, originalImage); rollbackErr != nil {
			ru.logger.Error("Rollback also failed", zap.Error(rollbackErr))
			status.Status = "failed_with_rollback_error"
			status.Message = fmt.Sprintf("Update failed: %v, Rollback failed: %v", err, rollbackErr)
		} else {
			status.Status = "rolled_back"
			status.Message = fmt.Sprintf("Update failed: %v, Successfully rolled back", err)
		}

		return status, err
	}

	status.Status = "completed"
	status.Progress = 100.0
	status.Message = "Rolling update completed successfully"

	ru.logger.Info("Rolling update completed successfully",
		zap.String("update_id", updateID),
		zap.Duration("duration", time.Since(status.StartTime)))

	return status, nil
}

// monitorRollout monitors the progress of a rolling update
func (ru *RollingUpdateManager) monitorRollout(ctx context.Context, deployment *appsv1.Deployment, status *UpdateStatus, config *UpdateConfig) error {
	timeout := time.After(config.Timeout)
	ticker := time.NewTicker(config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("rollout monitoring cancelled: %w", ctx.Err())

		case <-timeout:
			return fmt.Errorf("rollout timed out after %v", config.Timeout)

		case <-ticker.C:
			// Get current deployment status
			current := &appsv1.Deployment{}
			err := ru.client.Get(ctx, client.ObjectKey{
				Namespace: deployment.Namespace,
				Name:      deployment.Name,
			}, current)

			if err != nil {
				return fmt.Errorf("failed to get deployment status: %w", err)
			}

			// Update status
			status.CurrentReplicas = current.Status.Replicas
			status.UpdatedReplicas = current.Status.UpdatedReplicas
			status.ReadyReplicas = current.Status.ReadyReplicas

			// Calculate progress
			if current.Spec.Replicas != nil && *current.Spec.Replicas > 0 {
				status.Progress = float64(current.Status.UpdatedReplicas) / float64(*current.Spec.Replicas) * 100
			}

			ru.logger.Debug("Rollout progress",
				zap.String("update_id", status.ID),
				zap.Int32("replicas", current.Status.Replicas),
				zap.Int32("updated", current.Status.UpdatedReplicas),
				zap.Int32("ready", current.Status.ReadyReplicas),
				zap.Float64("progress", status.Progress))

			// Check rollout conditions
			for _, condition := range current.Status.Conditions {
				switch condition.Type {
				case appsv1.DeploymentProgressing:
					if condition.Status == "False" {
						return fmt.Errorf("deployment failed to progress: %s", condition.Message)
					}
					status.Message = condition.Message

				case appsv1.DeploymentAvailable:
					if condition.Status == "True" &&
						current.Status.UpdatedReplicas == *current.Spec.Replicas &&
						current.Status.ReadyReplicas == *current.Spec.Replicas {
						// Rollout completed successfully
						return nil
					}
				}
			}

			// Check for failed replicas
			if current.Status.UnavailableReplicas > 0 {
				ru.logger.Warn("Some replicas are unavailable during rollout",
					zap.Int32("unavailable", current.Status.UnavailableReplicas))
			}
		}
	}
}

// rollbackUpdate performs a rollback to the previous version
func (ru *RollingUpdateManager) rollbackUpdate(ctx context.Context, deployment *appsv1.Deployment, originalImage string) error {
	ru.logger.Info("Performing rollback", zap.String("original_image", originalImage))

	// Get current deployment
	current := &appsv1.Deployment{}
	err := ru.client.Get(ctx, client.ObjectKey{
		Namespace: deployment.Namespace,
		Name:      deployment.Name,
	}, current)

	if err != nil {
		return fmt.Errorf("failed to get deployment for rollback: %w", err)
	}

	// Rollback to original image
	current.Spec.Template.Spec.Containers[0].Image = originalImage

	if err := ru.client.Update(ctx, current); err != nil {
		return fmt.Errorf("failed to update deployment for rollback: %w", err)
	}

	// Wait for rollback to complete (with shorter timeout)
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("rollback monitoring cancelled: %w", ctx.Err())

		case <-timeout:
			return fmt.Errorf("rollback timed out")

		case <-ticker.C:
			err := ru.client.Get(ctx, client.ObjectKey{
				Namespace: current.Namespace,
				Name:      current.Name,
			}, current)

			if err != nil {
				return fmt.Errorf("failed to get deployment status during rollback: %w", err)
			}

			// Check if rollback is complete
			for _, condition := range current.Status.Conditions {
				if condition.Type == appsv1.DeploymentAvailable && condition.Status == "True" {
					if current.Status.UpdatedReplicas == *current.Spec.Replicas &&
						current.Status.ReadyReplicas == *current.Spec.Replicas {
						ru.logger.Info("Rollback completed successfully")
						return nil
					}
				}
			}
		}
	}
}

// GetUpdateStatus returns the current status of a rolling update
func (ru *RollingUpdateManager) GetUpdateStatus(ctx context.Context, updateID string) (*UpdateStatus, error) {
	// In a real implementation, this would query stored update status
	// For now, we'll get the current deployment status

	deployment := &appsv1.Deployment{}
	err := ru.client.Get(ctx, client.ObjectKey{
		Namespace: "roost-keeper-system",
		Name:      "roost-keeper-controller-manager",
	}, deployment)

	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}

	status := &UpdateStatus{
		ID:              updateID,
		CurrentReplicas: deployment.Status.Replicas,
		UpdatedReplicas: deployment.Status.UpdatedReplicas,
		ReadyReplicas:   deployment.Status.ReadyReplicas,
	}

	// Determine status based on deployment conditions
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			if condition.Status == "True" {
				status.Status = "completed"
				status.Progress = 100.0
			} else {
				status.Status = "in_progress"
			}
			status.Message = condition.Message
			break
		}
	}

	return status, nil
}

// ListUpdates returns a list of recent rolling updates
func (ru *RollingUpdateManager) ListUpdates(ctx context.Context) ([]*UpdateStatus, error) {
	// In a real implementation, this would query stored update history
	// For now, return a mock list
	return []*UpdateStatus{
		{
			ID:        "update-example",
			StartTime: time.Now().Add(-1 * time.Hour),
			Status:    "completed",
			Progress:  100.0,
			Message:   "Update completed successfully",
		},
	}, nil
}

// ValidateUpdate performs pre-update validation
func (ru *RollingUpdateManager) ValidateUpdate(ctx context.Context, newImage string) error {
	ru.logger.Info("Validating rolling update", zap.String("image", newImage))

	// Validate deployment exists
	deployment := &appsv1.Deployment{}
	err := ru.client.Get(ctx, client.ObjectKey{
		Namespace: "roost-keeper-system",
		Name:      "roost-keeper-controller-manager",
	}, deployment)

	if err != nil {
		return fmt.Errorf("deployment not found: %w", err)
	}

	// Check if deployment is currently stable
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			if condition.Status == "False" {
				return fmt.Errorf("deployment is not in a healthy state for update: %s", condition.Message)
			}
		}
	}

	// Validate minimum replica count for HA
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas < 2 {
		return fmt.Errorf("insufficient replicas for safe rolling update (minimum 2 required)")
	}

	// Check ready replicas
	if deployment.Status.ReadyReplicas < 2 {
		return fmt.Errorf("insufficient ready replicas for safe rolling update: %d/2", deployment.Status.ReadyReplicas)
	}

	ru.logger.Info("Rolling update validation passed")
	return nil
}

// CanaryUpdate performs a canary deployment update
func (ru *RollingUpdateManager) CanaryUpdate(ctx context.Context, newImage string, canaryReplicas int32) (*UpdateStatus, error) {
	ru.logger.Info("Starting canary update",
		zap.String("image", newImage),
		zap.Int32("canary_replicas", canaryReplicas))

	// In a real implementation, this would:
	// 1. Create a separate canary deployment with limited replicas
	// 2. Update traffic routing to send small percentage to canary
	// 3. Monitor canary health and metrics
	// 4. Provide mechanisms to promote or rollback canary

	updateID := fmt.Sprintf("canary-%d", time.Now().Unix())

	return &UpdateStatus{
		ID:        updateID,
		StartTime: time.Now(),
		Status:    "canary_deployed",
		Progress:  50.0,
		Message:   "Canary deployment created, monitoring health",
	}, nil
}
