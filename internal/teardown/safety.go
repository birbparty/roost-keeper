package teardown

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// SafetyChecker performs safety checks before teardown execution
type SafetyChecker struct {
	client client.Client
	logger *zap.Logger
}

// NewSafetyChecker creates a new safety checker
func NewSafetyChecker(client client.Client, logger *zap.Logger) *SafetyChecker {
	return &SafetyChecker{
		client: client,
		logger: logger.With(zap.String("component", "teardown-safety")),
	}
}

// PerformSafetyChecks performs all safety checks for a teardown
func (s *SafetyChecker) PerformSafetyChecks(ctx context.Context, roost *roostv1alpha1.ManagedRoost, decision *TeardownDecision) ([]SafetyCheck, error) {
	s.logger.Info("Performing safety checks",
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace))

	var checks []SafetyCheck

	// Check for active connections
	connectionCheck := s.checkActiveConnections(ctx, roost)
	checks = append(checks, connectionCheck)

	// Check for persistent volumes
	volumeCheck := s.checkPersistentVolumes(ctx, roost)
	checks = append(checks, volumeCheck)

	// Check for dependent resources
	dependencyCheck := s.checkDependentResources(ctx, roost)
	checks = append(checks, dependencyCheck)

	// Check for running pods
	podCheck := s.checkRunningPods(ctx, roost)
	checks = append(checks, podCheck)

	// Check for services with external endpoints
	serviceCheck := s.checkServicesWithExternalEndpoints(ctx, roost)
	checks = append(checks, serviceCheck)

	// Check for data persistence requirements
	dataCheck := s.checkDataPersistence(ctx, roost)
	checks = append(checks, dataCheck)

	// Check for backup requirements
	backupCheck := s.checkBackupRequirements(ctx, roost)
	checks = append(checks, backupCheck)

	// Check for manual approval requirements
	approvalCheck := s.checkManualApprovalRequirements(ctx, roost)
	checks = append(checks, approvalCheck)

	s.logger.Info("Safety checks completed",
		zap.String("roost", roost.Name),
		zap.Int("total_checks", len(checks)),
		zap.Int("passed_checks", s.countPassedChecks(checks)))

	return checks, nil
}

// checkActiveConnections checks for active connections to the roost
func (s *SafetyChecker) checkActiveConnections(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "active_connections",
		CheckedAt:   time.Now(),
		CanOverride: true,
		Severity:    SafetySeverityWarning,
		Details:     make(map[string]interface{}),
	}

	// Get pods with helm release labels
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   roost.Name,
	})

	err := s.client.List(ctx, podList, &client.ListOptions{
		Namespace:     roost.Namespace,
		LabelSelector: labelSelector,
	})

	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to check for active connections: %v", err)
		check.Severity = SafetySeverityError
		return check
	}

	activeConnections := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			// Simple heuristic: assume running pods have active connections
			activeConnections++
		}
	}

	check.Details["active_connections"] = activeConnections
	check.Details["total_pods"] = len(podList.Items)

	if activeConnections > 0 {
		check.Passed = false
		check.Message = fmt.Sprintf("Found %d pods with potential active connections", activeConnections)
	} else {
		check.Passed = true
		check.Message = "No active connections detected"
	}

	return check
}

// checkPersistentVolumes checks for persistent volumes that would be affected
func (s *SafetyChecker) checkPersistentVolumes(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "persistent_volumes",
		CheckedAt:   time.Now(),
		CanOverride: false, // PVs are critical and shouldn't be overridden easily
		Severity:    SafetySeverityCritical,
		Details:     make(map[string]interface{}),
	}

	// Get PVCs with helm release labels
	pvcList := &corev1.PersistentVolumeClaimList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   roost.Name,
	})

	err := s.client.List(ctx, pvcList, &client.ListOptions{
		Namespace:     roost.Namespace,
		LabelSelector: labelSelector,
	})

	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to check persistent volumes: %v", err)
		return check
	}

	pvcNames := make([]string, 0, len(pvcList.Items))
	for _, pvc := range pvcList.Items {
		pvcNames = append(pvcNames, pvc.Name)
	}

	check.Details["pvc_count"] = len(pvcList.Items)
	check.Details["pvc_names"] = pvcNames

	if len(pvcList.Items) > 0 {
		// Check if data preservation is enabled
		if roost.Spec.TeardownPolicy != nil &&
			roost.Spec.TeardownPolicy.DataPreservation != nil &&
			roost.Spec.TeardownPolicy.DataPreservation.Enabled {
			check.Passed = true
			check.Message = fmt.Sprintf("Found %d PVCs but data preservation is enabled", len(pvcList.Items))
			check.CanOverride = true
			check.Severity = SafetySeverityWarning
		} else {
			check.Passed = false
			check.Message = fmt.Sprintf("Found %d PVCs without data preservation enabled", len(pvcList.Items))
		}
	} else {
		check.Passed = true
		check.Message = "No persistent volumes found"
	}

	return check
}

// checkDependentResources checks for resources that depend on this roost
func (s *SafetyChecker) checkDependentResources(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "dependent_resources",
		CheckedAt:   time.Now(),
		CanOverride: true,
		Severity:    SafetySeverityWarning,
		Details:     make(map[string]interface{}),
	}

	// This is a simplified check - in a real implementation, you would:
	// 1. Check for other ManagedRoosts that reference this one
	// 2. Check for external resources that depend on services from this roost
	// 3. Check for network policies or security groups that reference this roost

	// For now, we'll do a basic check for services that might be referenced externally
	serviceList := &corev1.ServiceList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   roost.Name,
	})

	err := s.client.List(ctx, serviceList, &client.ListOptions{
		Namespace:     roost.Namespace,
		LabelSelector: labelSelector,
	})

	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to check dependent resources: %v", err)
		return check
	}

	externalServices := 0
	for _, svc := range serviceList.Items {
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort {
			externalServices++
		}
	}

	check.Details["total_services"] = len(serviceList.Items)
	check.Details["external_services"] = externalServices

	if externalServices > 0 {
		check.Passed = false
		check.Message = fmt.Sprintf("Found %d services with external access that may have dependencies", externalServices)
	} else {
		check.Passed = true
		check.Message = "No external dependencies detected"
	}

	return check
}

// checkRunningPods checks for running pods that would be terminated
func (s *SafetyChecker) checkRunningPods(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "running_pods",
		CheckedAt:   time.Now(),
		CanOverride: true,
		Severity:    SafetySeverityInfo,
		Details:     make(map[string]interface{}),
	}

	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   roost.Name,
	})

	err := s.client.List(ctx, podList, &client.ListOptions{
		Namespace:     roost.Namespace,
		LabelSelector: labelSelector,
	})

	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to check running pods: %v", err)
		check.Severity = SafetySeverityError
		return check
	}

	runningPods := 0
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			runningPods++
		}
	}

	check.Details["total_pods"] = len(podList.Items)
	check.Details["running_pods"] = runningPods

	if runningPods > 0 {
		check.Passed = true // This is informational - running pods are expected to be terminated
		check.Message = fmt.Sprintf("Found %d running pods that will be terminated", runningPods)
	} else {
		check.Passed = true
		check.Message = "No running pods found"
	}

	return check
}

// checkServicesWithExternalEndpoints checks for services with external endpoints
func (s *SafetyChecker) checkServicesWithExternalEndpoints(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "external_endpoints",
		CheckedAt:   time.Now(),
		CanOverride: true,
		Severity:    SafetySeverityWarning,
		Details:     make(map[string]interface{}),
	}

	serviceList := &corev1.ServiceList{}
	labelSelector := labels.SelectorFromSet(map[string]string{
		"app.kubernetes.io/managed-by": "Helm",
		"app.kubernetes.io/instance":   roost.Name,
	})

	err := s.client.List(ctx, serviceList, &client.ListOptions{
		Namespace:     roost.Namespace,
		LabelSelector: labelSelector,
	})

	if err != nil {
		check.Passed = false
		check.Message = fmt.Sprintf("Failed to check services: %v", err)
		check.Severity = SafetySeverityError
		return check
	}

	externalEndpoints := make([]string, 0)
	for _, svc := range serviceList.Items {
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			for _, ingress := range svc.Status.LoadBalancer.Ingress {
				if ingress.IP != "" {
					externalEndpoints = append(externalEndpoints, ingress.IP)
				}
				if ingress.Hostname != "" {
					externalEndpoints = append(externalEndpoints, ingress.Hostname)
				}
			}
		}
	}

	check.Details["external_endpoints"] = externalEndpoints
	check.Details["endpoint_count"] = len(externalEndpoints)

	if len(externalEndpoints) > 0 {
		check.Passed = false
		check.Message = fmt.Sprintf("Found %d external endpoints that will become unavailable", len(externalEndpoints))
	} else {
		check.Passed = true
		check.Message = "No external endpoints found"
	}

	return check
}

// checkDataPersistence checks if data persistence policies are in place
func (s *SafetyChecker) checkDataPersistence(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "data_persistence",
		CheckedAt:   time.Now(),
		CanOverride: true,
		Severity:    SafetySeverityWarning,
		Details:     make(map[string]interface{}),
	}

	hasDataPreservation := roost.Spec.TeardownPolicy != nil &&
		roost.Spec.TeardownPolicy.DataPreservation != nil &&
		roost.Spec.TeardownPolicy.DataPreservation.Enabled

	check.Details["data_preservation_enabled"] = hasDataPreservation

	if hasDataPreservation {
		check.Passed = true
		check.Message = "Data preservation is enabled"

		// Add details about backup policy if configured
		if roost.Spec.TeardownPolicy.DataPreservation.BackupPolicy != nil {
			backupPolicy := roost.Spec.TeardownPolicy.DataPreservation.BackupPolicy
			check.Details["backup_schedule"] = backupPolicy.Schedule
			check.Details["backup_retention"] = backupPolicy.Retention
		}
	} else {
		check.Passed = false
		check.Message = "Data preservation is not enabled - data may be lost"
	}

	return check
}

// checkBackupRequirements checks if backup requirements are met
func (s *SafetyChecker) checkBackupRequirements(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "backup_requirements",
		CheckedAt:   time.Now(),
		CanOverride: true,
		Severity:    SafetySeverityInfo,
		Details:     make(map[string]interface{}),
	}

	// Check if backup is configured and required
	if roost.Spec.TeardownPolicy != nil &&
		roost.Spec.TeardownPolicy.DataPreservation != nil &&
		roost.Spec.TeardownPolicy.DataPreservation.BackupPolicy != nil {

		backupPolicy := roost.Spec.TeardownPolicy.DataPreservation.BackupPolicy
		check.Details["backup_configured"] = true
		check.Details["backup_schedule"] = backupPolicy.Schedule

		// In a real implementation, you would check:
		// 1. If recent backups exist
		// 2. If backup jobs are running successfully
		// 3. If backup storage is accessible

		check.Passed = true
		check.Message = "Backup policy is configured"
	} else {
		check.Details["backup_configured"] = false
		check.Passed = true // Not having backup might be intentional
		check.Message = "No backup policy configured"
	}

	return check
}

// checkManualApprovalRequirements checks if manual approval is required
func (s *SafetyChecker) checkManualApprovalRequirements(ctx context.Context, roost *roostv1alpha1.ManagedRoost) SafetyCheck {
	check := SafetyCheck{
		Name:        "manual_approval",
		CheckedAt:   time.Now(),
		CanOverride: false, // Manual approval cannot be overridden
		Severity:    SafetySeverityCritical,
		Details:     make(map[string]interface{}),
	}

	requiresApproval := roost.Spec.TeardownPolicy != nil &&
		roost.Spec.TeardownPolicy.RequireManualApproval

	check.Details["requires_manual_approval"] = requiresApproval

	if requiresApproval {
		// In a real implementation, you would check:
		// 1. If manual approval has been granted
		// 2. Who granted the approval
		// 3. When the approval was granted
		// 4. If the approval is still valid

		// For now, we assume approval is required but not granted
		check.Passed = false
		check.Message = "Manual approval is required but not yet granted"
	} else {
		check.Passed = true
		check.Message = "Manual approval is not required"
	}

	return check
}

// countPassedChecks counts the number of passed safety checks
func (s *SafetyChecker) countPassedChecks(checks []SafetyCheck) int {
	count := 0
	for _, check := range checks {
		if check.Passed {
			count++
		}
	}
	return count
}
