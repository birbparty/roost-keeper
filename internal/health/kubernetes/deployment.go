package kubernetes

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// DeploymentChecker handles deployment, StatefulSet, and DaemonSet health assessment
type DeploymentChecker struct {
	client client.Client
	logger *zap.Logger
}

// NewDeploymentChecker creates a new deployment health checker
func NewDeploymentChecker(k8sClient client.Client, logger *zap.Logger) *DeploymentChecker {
	return &DeploymentChecker{
		client: k8sClient,
		logger: logger,
	}
}

// CheckDeployments performs comprehensive deployment health checking
func (dc *DeploymentChecker) CheckDeployments(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := dc.logger.With(
		zap.String("component", "deployments"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting deployment health check")

	// List deployments matching the selector
	var deployments appsv1.DeploymentList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := dc.client.List(ctx, &deployments, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	log.Debug("Found deployments", zap.Int("count", len(deployments.Items)))

	if len(deployments.Items) == 0 {
		return &ComponentHealth{
			Healthy:   true,
			Message:   "No deployments found (not required for health)",
			CheckedAt: time.Now(),
			Details: map[string]interface{}{
				"total_deployments": 0,
				"selector":          selector.String(),
				"namespace":         roost.Namespace,
			},
		}, nil
	}

	// Analyze deployment health
	analysis := dc.analyzeDeployments(deployments.Items)

	// Determine overall health (all deployments must be healthy)
	healthy := analysis.HealthyDeployments == analysis.TotalDeployments

	// Build health message
	message := dc.buildDeploymentHealthMessage(healthy, analysis)

	return &ComponentHealth{
		Healthy:      healthy,
		Message:      message,
		CheckedAt:    time.Now(),
		ErrorCount:   analysis.ErrorDeployments,
		WarningCount: analysis.WarningDeployments,
		Details: map[string]interface{}{
			"total_deployments":   analysis.TotalDeployments,
			"healthy_deployments": analysis.HealthyDeployments,
			"error_deployments":   analysis.ErrorDeployments,
			"warning_deployments": analysis.WarningDeployments,
			"deployment_details":  analysis.DeploymentDetails,
		},
	}, nil
}

// CheckStatefulSets performs StatefulSet health checking
func (dc *DeploymentChecker) CheckStatefulSets(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := dc.logger.With(
		zap.String("component", "statefulsets"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting StatefulSet health check")

	// List StatefulSets matching the selector
	var statefulSets appsv1.StatefulSetList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := dc.client.List(ctx, &statefulSets, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list StatefulSets: %w", err)
	}

	log.Debug("Found StatefulSets", zap.Int("count", len(statefulSets.Items)))

	if len(statefulSets.Items) == 0 {
		return &ComponentHealth{
			Healthy:   true,
			Message:   "No StatefulSets found (not required for health)",
			CheckedAt: time.Now(),
			Details: map[string]interface{}{
				"total_stateful_sets": 0,
				"selector":            selector.String(),
				"namespace":           roost.Namespace,
			},
		}, nil
	}

	// Analyze StatefulSet health
	analysis := dc.analyzeStatefulSets(statefulSets.Items)

	// Determine overall health
	healthy := analysis.HealthyStatefulSets == analysis.TotalStatefulSets

	// Build health message
	message := dc.buildStatefulSetHealthMessage(healthy, analysis)

	return &ComponentHealth{
		Healthy:      healthy,
		Message:      message,
		CheckedAt:    time.Now(),
		ErrorCount:   analysis.ErrorStatefulSets,
		WarningCount: analysis.WarningStatefulSets,
		Details: map[string]interface{}{
			"total_stateful_sets":   analysis.TotalStatefulSets,
			"healthy_stateful_sets": analysis.HealthyStatefulSets,
			"error_stateful_sets":   analysis.ErrorStatefulSets,
			"warning_stateful_sets": analysis.WarningStatefulSets,
			"stateful_set_details":  analysis.StatefulSetDetails,
		},
	}, nil
}

// CheckDaemonSets performs DaemonSet health checking
func (dc *DeploymentChecker) CheckDaemonSets(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := dc.logger.With(
		zap.String("component", "daemonsets"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting DaemonSet health check")

	// List DaemonSets matching the selector
	var daemonSets appsv1.DaemonSetList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := dc.client.List(ctx, &daemonSets, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list DaemonSets: %w", err)
	}

	log.Debug("Found DaemonSets", zap.Int("count", len(daemonSets.Items)))

	if len(daemonSets.Items) == 0 {
		return &ComponentHealth{
			Healthy:   true,
			Message:   "No DaemonSets found (not required for health)",
			CheckedAt: time.Now(),
			Details: map[string]interface{}{
				"total_daemon_sets": 0,
				"selector":          selector.String(),
				"namespace":         roost.Namespace,
			},
		}, nil
	}

	// Analyze DaemonSet health
	analysis := dc.analyzeDaemonSets(daemonSets.Items)

	// Determine overall health
	healthy := analysis.HealthyDaemonSets == analysis.TotalDaemonSets

	// Build health message
	message := dc.buildDaemonSetHealthMessage(healthy, analysis)

	return &ComponentHealth{
		Healthy:      healthy,
		Message:      message,
		CheckedAt:    time.Now(),
		ErrorCount:   analysis.ErrorDaemonSets,
		WarningCount: analysis.WarningDaemonSets,
		Details: map[string]interface{}{
			"total_daemon_sets":   analysis.TotalDaemonSets,
			"healthy_daemon_sets": analysis.HealthyDaemonSets,
			"error_daemon_sets":   analysis.ErrorDaemonSets,
			"warning_daemon_sets": analysis.WarningDaemonSets,
			"daemon_set_details":  analysis.DaemonSetDetails,
		},
	}, nil
}

// DeploymentAnalysis contains the results of deployment health analysis
type DeploymentAnalysis struct {
	TotalDeployments   int                 `json:"total_deployments"`
	HealthyDeployments int                 `json:"healthy_deployments"`
	ErrorDeployments   int                 `json:"error_deployments"`
	WarningDeployments int                 `json:"warning_deployments"`
	DeploymentDetails  []DeploymentDetails `json:"deployment_details"`
}

// StatefulSetAnalysis contains the results of StatefulSet health analysis
type StatefulSetAnalysis struct {
	TotalStatefulSets   int                  `json:"total_stateful_sets"`
	HealthyStatefulSets int                  `json:"healthy_stateful_sets"`
	ErrorStatefulSets   int                  `json:"error_stateful_sets"`
	WarningStatefulSets int                  `json:"warning_stateful_sets"`
	StatefulSetDetails  []StatefulSetDetails `json:"stateful_set_details"`
}

// DaemonSetAnalysis contains the results of DaemonSet health analysis
type DaemonSetAnalysis struct {
	TotalDaemonSets   int                `json:"total_daemon_sets"`
	HealthyDaemonSets int                `json:"healthy_daemon_sets"`
	ErrorDaemonSets   int                `json:"error_daemon_sets"`
	WarningDaemonSets int                `json:"warning_daemon_sets"`
	DaemonSetDetails  []DaemonSetDetails `json:"daemon_set_details"`
}

// DeploymentDetails contains detailed information about a specific deployment
type DeploymentDetails struct {
	Name                string `json:"name"`
	Healthy             bool   `json:"healthy"`
	DesiredReplicas     int32  `json:"desired_replicas"`
	ReadyReplicas       int32  `json:"ready_replicas"`
	AvailableReplicas   int32  `json:"available_replicas"`
	UpdatedReplicas     int32  `json:"updated_replicas"`
	UnavailableReplicas int32  `json:"unavailable_replicas"`
	Generation          int64  `json:"generation"`
	ObservedGeneration  int64  `json:"observed_generation"`
	Reason              string `json:"reason,omitempty"`
	Message             string `json:"message,omitempty"`
}

// StatefulSetDetails contains detailed information about a specific StatefulSet
type StatefulSetDetails struct {
	Name               string `json:"name"`
	Healthy            bool   `json:"healthy"`
	DesiredReplicas    int32  `json:"desired_replicas"`
	ReadyReplicas      int32  `json:"ready_replicas"`
	CurrentReplicas    int32  `json:"current_replicas"`
	UpdatedReplicas    int32  `json:"updated_replicas"`
	Generation         int64  `json:"generation"`
	ObservedGeneration int64  `json:"observed_generation"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
}

// DaemonSetDetails contains detailed information about a specific DaemonSet
type DaemonSetDetails struct {
	Name                   string `json:"name"`
	Healthy                bool   `json:"healthy"`
	DesiredNumberScheduled int32  `json:"desired_number_scheduled"`
	CurrentNumberScheduled int32  `json:"current_number_scheduled"`
	NumberMisscheduled     int32  `json:"number_misscheduled"`
	NumberReady            int32  `json:"number_ready"`
	UpdatedNumberScheduled int32  `json:"updated_number_scheduled"`
	NumberAvailable        int32  `json:"number_available"`
	NumberUnavailable      int32  `json:"number_unavailable"`
	Generation             int64  `json:"generation"`
	ObservedGeneration     int64  `json:"observed_generation"`
	Reason                 string `json:"reason,omitempty"`
	Message                string `json:"message,omitempty"`
}

// analyzeDeployments performs detailed analysis of deployment health
func (dc *DeploymentChecker) analyzeDeployments(deployments []appsv1.Deployment) *DeploymentAnalysis {
	analysis := &DeploymentAnalysis{
		TotalDeployments:  len(deployments),
		DeploymentDetails: []DeploymentDetails{},
	}

	for _, deployment := range deployments {
		details := dc.analyzeDeployment(deployment)
		analysis.DeploymentDetails = append(analysis.DeploymentDetails, details)

		if details.Healthy {
			analysis.HealthyDeployments++
		} else {
			analysis.ErrorDeployments++
		}
	}

	return analysis
}

// analyzeDeployment provides detailed analysis of a single deployment
func (dc *DeploymentChecker) analyzeDeployment(deployment appsv1.Deployment) DeploymentDetails {
	desired := int32(0)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}

	details := DeploymentDetails{
		Name:                deployment.Name,
		DesiredReplicas:     desired,
		ReadyReplicas:       deployment.Status.ReadyReplicas,
		AvailableReplicas:   deployment.Status.AvailableReplicas,
		UpdatedReplicas:     deployment.Status.UpdatedReplicas,
		UnavailableReplicas: deployment.Status.UnavailableReplicas,
		Generation:          deployment.Generation,
		ObservedGeneration:  deployment.Status.ObservedGeneration,
	}

	// Determine health based on replica status
	details.Healthy = dc.isDeploymentHealthy(deployment)

	return details
}

// isDeploymentHealthy determines if a deployment is healthy
func (dc *DeploymentChecker) isDeploymentHealthy(deployment appsv1.Deployment) bool {
	desired := int32(0)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}

	return deployment.Status.ReadyReplicas == desired &&
		deployment.Status.UpdatedReplicas == desired &&
		deployment.Status.AvailableReplicas == desired &&
		deployment.Status.UnavailableReplicas == 0 &&
		deployment.Generation == deployment.Status.ObservedGeneration
}

// analyzeStatefulSets performs detailed analysis of StatefulSet health
func (dc *DeploymentChecker) analyzeStatefulSets(statefulSets []appsv1.StatefulSet) *StatefulSetAnalysis {
	analysis := &StatefulSetAnalysis{
		TotalStatefulSets:  len(statefulSets),
		StatefulSetDetails: []StatefulSetDetails{},
	}

	for _, statefulSet := range statefulSets {
		details := dc.analyzeStatefulSet(statefulSet)
		analysis.StatefulSetDetails = append(analysis.StatefulSetDetails, details)

		if details.Healthy {
			analysis.HealthyStatefulSets++
		} else {
			analysis.ErrorStatefulSets++
		}
	}

	return analysis
}

// analyzeStatefulSet provides detailed analysis of a single StatefulSet
func (dc *DeploymentChecker) analyzeStatefulSet(statefulSet appsv1.StatefulSet) StatefulSetDetails {
	desired := int32(0)
	if statefulSet.Spec.Replicas != nil {
		desired = *statefulSet.Spec.Replicas
	}

	details := StatefulSetDetails{
		Name:               statefulSet.Name,
		DesiredReplicas:    desired,
		ReadyReplicas:      statefulSet.Status.ReadyReplicas,
		CurrentReplicas:    statefulSet.Status.CurrentReplicas,
		UpdatedReplicas:    statefulSet.Status.UpdatedReplicas,
		Generation:         statefulSet.Generation,
		ObservedGeneration: statefulSet.Status.ObservedGeneration,
	}

	// Determine health based on replica status
	details.Healthy = dc.isStatefulSetHealthy(statefulSet)

	return details
}

// isStatefulSetHealthy determines if a StatefulSet is healthy
func (dc *DeploymentChecker) isStatefulSetHealthy(statefulSet appsv1.StatefulSet) bool {
	desired := int32(0)
	if statefulSet.Spec.Replicas != nil {
		desired = *statefulSet.Spec.Replicas
	}

	return statefulSet.Status.ReadyReplicas == desired &&
		statefulSet.Status.CurrentReplicas == desired &&
		statefulSet.Generation == statefulSet.Status.ObservedGeneration
}

// analyzeDaemonSets performs detailed analysis of DaemonSet health
func (dc *DeploymentChecker) analyzeDaemonSets(daemonSets []appsv1.DaemonSet) *DaemonSetAnalysis {
	analysis := &DaemonSetAnalysis{
		TotalDaemonSets:  len(daemonSets),
		DaemonSetDetails: []DaemonSetDetails{},
	}

	for _, daemonSet := range daemonSets {
		details := dc.analyzeDaemonSet(daemonSet)
		analysis.DaemonSetDetails = append(analysis.DaemonSetDetails, details)

		if details.Healthy {
			analysis.HealthyDaemonSets++
		} else {
			analysis.ErrorDaemonSets++
		}
	}

	return analysis
}

// analyzeDaemonSet provides detailed analysis of a single DaemonSet
func (dc *DeploymentChecker) analyzeDaemonSet(daemonSet appsv1.DaemonSet) DaemonSetDetails {
	details := DaemonSetDetails{
		Name:                   daemonSet.Name,
		DesiredNumberScheduled: daemonSet.Status.DesiredNumberScheduled,
		CurrentNumberScheduled: daemonSet.Status.CurrentNumberScheduled,
		NumberMisscheduled:     daemonSet.Status.NumberMisscheduled,
		NumberReady:            daemonSet.Status.NumberReady,
		UpdatedNumberScheduled: daemonSet.Status.UpdatedNumberScheduled,
		NumberAvailable:        daemonSet.Status.NumberAvailable,
		NumberUnavailable:      daemonSet.Status.NumberUnavailable,
		Generation:             daemonSet.Generation,
		ObservedGeneration:     daemonSet.Status.ObservedGeneration,
	}

	// Determine health based on scheduling and readiness
	details.Healthy = dc.isDaemonSetHealthy(daemonSet)

	return details
}

// isDaemonSetHealthy determines if a DaemonSet is healthy
func (dc *DeploymentChecker) isDaemonSetHealthy(daemonSet appsv1.DaemonSet) bool {
	return daemonSet.Status.NumberReady == daemonSet.Status.DesiredNumberScheduled &&
		daemonSet.Status.CurrentNumberScheduled == daemonSet.Status.DesiredNumberScheduled &&
		daemonSet.Status.NumberMisscheduled == 0 &&
		daemonSet.Status.NumberUnavailable == 0 &&
		daemonSet.Generation == daemonSet.Status.ObservedGeneration
}

// buildDeploymentHealthMessage creates a descriptive health message for deployments
func (dc *DeploymentChecker) buildDeploymentHealthMessage(healthy bool, analysis *DeploymentAnalysis) string {
	if healthy {
		return fmt.Sprintf("All %d deployments are healthy", analysis.TotalDeployments)
	}

	return fmt.Sprintf("Deployment health check failed: %d/%d healthy",
		analysis.HealthyDeployments, analysis.TotalDeployments)
}

// buildStatefulSetHealthMessage creates a descriptive health message for StatefulSets
func (dc *DeploymentChecker) buildStatefulSetHealthMessage(healthy bool, analysis *StatefulSetAnalysis) string {
	if healthy {
		return fmt.Sprintf("All %d StatefulSets are healthy", analysis.TotalStatefulSets)
	}

	return fmt.Sprintf("StatefulSet health check failed: %d/%d healthy",
		analysis.HealthyStatefulSets, analysis.TotalStatefulSets)
}

// buildDaemonSetHealthMessage creates a descriptive health message for DaemonSets
func (dc *DeploymentChecker) buildDaemonSetHealthMessage(healthy bool, analysis *DaemonSetAnalysis) string {
	if healthy {
		return fmt.Sprintf("All %d DaemonSets are healthy", analysis.TotalDaemonSets)
	}

	return fmt.Sprintf("DaemonSet health check failed: %d/%d healthy",
		analysis.HealthyDaemonSets, analysis.TotalDaemonSets)
}
