package kubernetes

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

// PodChecker handles pod health assessment
type PodChecker struct {
	client client.Client
	logger *zap.Logger
}

// NewPodChecker creates a new pod health checker
func NewPodChecker(k8sClient client.Client, logger *zap.Logger) *PodChecker {
	return &PodChecker{
		client: k8sClient,
		logger: logger,
	}
}

// CheckPods performs comprehensive pod health checking
func (pc *PodChecker) CheckPods(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := pc.logger.With(
		zap.String("component", "pods"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting pod health check")

	// List pods matching the selector
	var pods corev1.PodList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := pc.client.List(ctx, &pods, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	log.Debug("Found pods", zap.Int("count", len(pods.Items)))

	if len(pods.Items) == 0 {
		return &ComponentHealth{
			Healthy:   false,
			Message:   "No pods found for application",
			CheckedAt: time.Now(),
			Details: map[string]interface{}{
				"total_pods": 0,
				"ready_pods": 0,
				"selector":   selector.String(),
				"namespace":  roost.Namespace,
			},
		}, nil
	}

	// Analyze pod health
	podAnalysis := pc.analyzePods(pods.Items, kubernetesSpec)

	// Determine overall health
	requiredRatio := 1.0 // Default: all pods must be ready
	if kubernetesSpec.RequiredReadyRatio != nil {
		requiredRatio = *kubernetesSpec.RequiredReadyRatio
	}

	actualRatio := float64(podAnalysis.ReadyPods) / float64(podAnalysis.TotalPods)
	healthy := actualRatio >= requiredRatio

	// Check restart count limits if configured
	if healthy && kubernetesSpec.CheckPodRestarts {
		maxRestarts := int32(5) // Default
		if kubernetesSpec.MaxPodRestarts != nil {
			maxRestarts = *kubernetesSpec.MaxPodRestarts
		}

		if podAnalysis.MaxRestartCount > maxRestarts {
			healthy = false
		}
	}

	// Build health message
	message := pc.buildPodHealthMessage(healthy, podAnalysis, requiredRatio, kubernetesSpec)

	return &ComponentHealth{
		Healthy:      healthy,
		Message:      message,
		CheckedAt:    time.Now(),
		ErrorCount:   podAnalysis.ErrorPods,
		WarningCount: podAnalysis.WarningPods,
		Details: map[string]interface{}{
			"total_pods":          podAnalysis.TotalPods,
			"ready_pods":          podAnalysis.ReadyPods,
			"pending_pods":        podAnalysis.PendingPods,
			"running_pods":        podAnalysis.RunningPods,
			"failed_pods":         podAnalysis.FailedPods,
			"succeeded_pods":      podAnalysis.SucceededPods,
			"unknown_pods":        podAnalysis.UnknownPods,
			"error_pods":          podAnalysis.ErrorPods,
			"warning_pods":        podAnalysis.WarningPods,
			"ready_ratio":         actualRatio,
			"required_ratio":      requiredRatio,
			"max_restart_count":   podAnalysis.MaxRestartCount,
			"total_restart_count": podAnalysis.TotalRestartCount,
			"unhealthy_pods":      podAnalysis.UnhealthyPods,
			"pod_details":         podAnalysis.PodDetails,
		},
	}, nil
}

// PodAnalysis contains the results of pod health analysis
type PodAnalysis struct {
	TotalPods         int          `json:"total_pods"`
	ReadyPods         int          `json:"ready_pods"`
	PendingPods       int          `json:"pending_pods"`
	RunningPods       int          `json:"running_pods"`
	FailedPods        int          `json:"failed_pods"`
	SucceededPods     int          `json:"succeeded_pods"`
	UnknownPods       int          `json:"unknown_pods"`
	ErrorPods         int          `json:"error_pods"`
	WarningPods       int          `json:"warning_pods"`
	MaxRestartCount   int32        `json:"max_restart_count"`
	TotalRestartCount int32        `json:"total_restart_count"`
	UnhealthyPods     []string     `json:"unhealthy_pods"`
	PodDetails        []PodDetails `json:"pod_details"`
}

// PodDetails contains detailed information about a specific pod
type PodDetails struct {
	Name         string            `json:"name"`
	Phase        string            `json:"phase"`
	Ready        bool              `json:"ready"`
	RestartCount int32             `json:"restart_count"`
	Reason       string            `json:"reason,omitempty"`
	Message      string            `json:"message,omitempty"`
	NodeName     string            `json:"node_name,omitempty"`
	Conditions   []PodCondition    `json:"conditions,omitempty"`
	Containers   []ContainerStatus `json:"containers,omitempty"`
}

// PodCondition represents a pod condition
type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ContainerStatus represents container status information
type ContainerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restart_count"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message,omitempty"`
}

// analyzePods performs detailed analysis of pod health
func (pc *PodChecker) analyzePods(pods []corev1.Pod, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) *PodAnalysis {
	analysis := &PodAnalysis{
		TotalPods:     len(pods),
		UnhealthyPods: []string{},
		PodDetails:    []PodDetails{},
	}

	for _, pod := range pods {
		podDetail := pc.analyzePod(pod)
		analysis.PodDetails = append(analysis.PodDetails, podDetail)

		// Count by phase
		switch pod.Status.Phase {
		case corev1.PodPending:
			analysis.PendingPods++
		case corev1.PodRunning:
			analysis.RunningPods++
		case corev1.PodSucceeded:
			analysis.SucceededPods++
		case corev1.PodFailed:
			analysis.FailedPods++
			analysis.ErrorPods++
		case corev1.PodUnknown:
			analysis.UnknownPods++
			analysis.WarningPods++
		}

		// Check readiness
		if pc.isPodReady(pod) {
			analysis.ReadyPods++
		} else {
			analysis.UnhealthyPods = append(analysis.UnhealthyPods, pod.Name)
			if pod.Status.Phase == corev1.PodRunning {
				analysis.WarningPods++
			}
		}

		// Count restarts
		restartCount := pc.getPodRestartCount(pod)
		analysis.TotalRestartCount += restartCount
		if restartCount > analysis.MaxRestartCount {
			analysis.MaxRestartCount = restartCount
		}
	}

	return analysis
}

// analyzePod provides detailed analysis of a single pod
func (pc *PodChecker) analyzePod(pod corev1.Pod) PodDetails {
	detail := PodDetails{
		Name:         pod.Name,
		Phase:        string(pod.Status.Phase),
		Ready:        pc.isPodReady(pod),
		RestartCount: pc.getPodRestartCount(pod),
		Reason:       pod.Status.Reason,
		Message:      pod.Status.Message,
		NodeName:     pod.Spec.NodeName,
	}

	// Extract conditions
	for _, condition := range pod.Status.Conditions {
		detail.Conditions = append(detail.Conditions, PodCondition{
			Type:    string(condition.Type),
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		})
	}

	// Extract container statuses
	for _, containerStatus := range pod.Status.ContainerStatuses {
		container := ContainerStatus{
			Name:         containerStatus.Name,
			Ready:        containerStatus.Ready,
			RestartCount: containerStatus.RestartCount,
		}

		// Determine container state
		if containerStatus.State.Running != nil {
			container.State = "running"
		} else if containerStatus.State.Waiting != nil {
			container.State = "waiting"
			container.Reason = containerStatus.State.Waiting.Reason
			container.Message = containerStatus.State.Waiting.Message
		} else if containerStatus.State.Terminated != nil {
			container.State = "terminated"
			container.Reason = containerStatus.State.Terminated.Reason
			container.Message = containerStatus.State.Terminated.Message
		}

		detail.Containers = append(detail.Containers, container)
	}

	return detail
}

// isPodReady checks if a pod is ready
func (pc *PodChecker) isPodReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// getPodRestartCount calculates the total restart count for a pod
func (pc *PodChecker) getPodRestartCount(pod corev1.Pod) int32 {
	var total int32
	for _, containerStatus := range pod.Status.ContainerStatuses {
		total += containerStatus.RestartCount
	}
	return total
}

// buildPodHealthMessage creates a descriptive health message for pods
func (pc *PodChecker) buildPodHealthMessage(healthy bool, analysis *PodAnalysis, requiredRatio float64, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) string {
	actualRatio := float64(analysis.ReadyPods) / float64(analysis.TotalPods)

	if healthy {
		if analysis.ReadyPods == analysis.TotalPods {
			return fmt.Sprintf("All %d pods are ready and healthy", analysis.TotalPods)
		}
		return fmt.Sprintf("Pod health acceptable: %d/%d ready (%.1f%% >= %.1f%% required)",
			analysis.ReadyPods, analysis.TotalPods, actualRatio*100, requiredRatio*100)
	}

	// Build failure message
	baseMessage := fmt.Sprintf("Pod health check failed: %d/%d ready (%.1f%% < %.1f%% required)",
		analysis.ReadyPods, analysis.TotalPods, actualRatio*100, requiredRatio*100)

	// Add specific issues
	issues := []string{}

	if analysis.FailedPods > 0 {
		issues = append(issues, fmt.Sprintf("%d failed", analysis.FailedPods))
	}

	if analysis.PendingPods > 0 {
		issues = append(issues, fmt.Sprintf("%d pending", analysis.PendingPods))
	}

	if kubernetesSpec.CheckPodRestarts && kubernetesSpec.MaxPodRestarts != nil {
		if analysis.MaxRestartCount > *kubernetesSpec.MaxPodRestarts {
			issues = append(issues, fmt.Sprintf("max restarts exceeded (%d > %d)", analysis.MaxRestartCount, *kubernetesSpec.MaxPodRestarts))
		}
	}

	if len(issues) > 0 {
		baseMessage += fmt.Sprintf(" - Issues: %v", issues)
	}

	return baseMessage
}
