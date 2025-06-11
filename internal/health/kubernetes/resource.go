package kubernetes

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// ResourceChecker handles dependency and custom resource health assessment
type ResourceChecker struct {
	client client.Client
	logger *zap.Logger
}

// NewResourceChecker creates a new resource health checker
func NewResourceChecker(k8sClient client.Client, logger *zap.Logger) *ResourceChecker {
	return &ResourceChecker{
		client: k8sClient,
		logger: logger,
	}
}

// CheckDependencies performs dependency resource health checking
func (rc *ResourceChecker) CheckDependencies(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := rc.logger.With(
		zap.String("component", "dependencies"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting dependency resource health check")

	analysis := &DependencyAnalysis{
		ConfigMapDetails: []ResourceDetails{},
		SecretDetails:    []ResourceDetails{},
		PVCDetails:       []ResourceDetails{},
	}

	var allHealthy = true
	var errors []string

	// Check ConfigMaps
	configMapHealth, err := rc.checkConfigMaps(ctx, roost, selector)
	if err != nil {
		errors = append(errors, fmt.Sprintf("ConfigMap check failed: %v", err))
		allHealthy = false
	} else {
		analysis.TotalConfigMaps = configMapHealth.Total
		analysis.HealthyConfigMaps = configMapHealth.Healthy
		analysis.ConfigMapDetails = configMapHealth.Details
		if configMapHealth.Healthy < configMapHealth.Total {
			allHealthy = false
		}
	}

	// Check Secrets
	secretHealth, err := rc.checkSecrets(ctx, roost, selector)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Secret check failed: %v", err))
		allHealthy = false
	} else {
		analysis.TotalSecrets = secretHealth.Total
		analysis.HealthySecrets = secretHealth.Healthy
		analysis.SecretDetails = secretHealth.Details
		if secretHealth.Healthy < secretHealth.Total {
			allHealthy = false
		}
	}

	// Check PVCs
	pvcHealth, err := rc.checkPVCs(ctx, roost, selector)
	if err != nil {
		errors = append(errors, fmt.Sprintf("PVC check failed: %v", err))
		allHealthy = false
	} else {
		analysis.TotalPVCs = pvcHealth.Total
		analysis.HealthyPVCs = pvcHealth.Healthy
		analysis.PVCDetails = pvcHealth.Details
		if pvcHealth.Healthy < pvcHealth.Total {
			allHealthy = false
		}
	}

	// Build health message
	message := rc.buildDependencyHealthMessage(allHealthy, analysis, errors)

	totalResources := analysis.TotalConfigMaps + analysis.TotalSecrets + analysis.TotalPVCs

	return &ComponentHealth{
		Healthy:   allHealthy,
		Message:   message,
		CheckedAt: time.Now(),
		Details: map[string]interface{}{
			"total_dependencies":  totalResources,
			"total_config_maps":   analysis.TotalConfigMaps,
			"healthy_config_maps": analysis.HealthyConfigMaps,
			"total_secrets":       analysis.TotalSecrets,
			"healthy_secrets":     analysis.HealthySecrets,
			"total_pvcs":          analysis.TotalPVCs,
			"healthy_pvcs":        analysis.HealthyPVCs,
			"config_map_details":  analysis.ConfigMapDetails,
			"secret_details":      analysis.SecretDetails,
			"pvc_details":         analysis.PVCDetails,
			"errors":              errors,
		},
	}, nil
}

// CheckCustomResources performs custom resource health checking
func (rc *ResourceChecker) CheckCustomResources(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := rc.logger.With(
		zap.String("component", "custom_resources"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting custom resource health check")

	if len(kubernetesSpec.CustomResources) == 0 {
		return &ComponentHealth{
			Healthy:   true,
			Message:   "No custom resources configured for health check",
			CheckedAt: time.Now(),
			Details: map[string]interface{}{
				"total_custom_resources": 0,
			},
		}, nil
	}

	analysis := &CustomResourceAnalysis{
		TotalCustomResources:  len(kubernetesSpec.CustomResources),
		CustomResourceDetails: []CustomResourceDetails{},
	}

	allHealthy := true
	var errors []string

	for _, customResource := range kubernetesSpec.CustomResources {
		details, err := rc.checkCustomResource(ctx, roost, selector, customResource)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s/%s check failed: %v", customResource.APIVersion, customResource.Kind, err))
			allHealthy = false
			continue
		}

		analysis.CustomResourceDetails = append(analysis.CustomResourceDetails, *details)
		if details.Healthy {
			analysis.HealthyCustomResources++
		} else {
			allHealthy = false
		}
	}

	// Build health message
	message := rc.buildCustomResourceHealthMessage(allHealthy, analysis, errors)

	return &ComponentHealth{
		Healthy:   allHealthy,
		Message:   message,
		CheckedAt: time.Now(),
		Details: map[string]interface{}{
			"total_custom_resources":   analysis.TotalCustomResources,
			"healthy_custom_resources": analysis.HealthyCustomResources,
			"custom_resource_details":  analysis.CustomResourceDetails,
			"errors":                   errors,
		},
	}, nil
}

// DependencyAnalysis contains the results of dependency resource health analysis
type DependencyAnalysis struct {
	TotalConfigMaps   int               `json:"total_config_maps"`
	HealthyConfigMaps int               `json:"healthy_config_maps"`
	TotalSecrets      int               `json:"total_secrets"`
	HealthySecrets    int               `json:"healthy_secrets"`
	TotalPVCs         int               `json:"total_pvcs"`
	HealthyPVCs       int               `json:"healthy_pvcs"`
	ConfigMapDetails  []ResourceDetails `json:"config_map_details"`
	SecretDetails     []ResourceDetails `json:"secret_details"`
	PVCDetails        []ResourceDetails `json:"pvc_details"`
}

// CustomResourceAnalysis contains the results of custom resource health analysis
type CustomResourceAnalysis struct {
	TotalCustomResources   int                     `json:"total_custom_resources"`
	HealthyCustomResources int                     `json:"healthy_custom_resources"`
	CustomResourceDetails  []CustomResourceDetails `json:"custom_resource_details"`
}

// ResourceDetails contains detailed information about a dependency resource
type ResourceDetails struct {
	Name    string                 `json:"name"`
	Healthy bool                   `json:"healthy"`
	Type    string                 `json:"type"`
	Reason  string                 `json:"reason,omitempty"`
	Message string                 `json:"message,omitempty"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// CustomResourceDetails contains detailed information about a custom resource
type CustomResourceDetails struct {
	Name       string                 `json:"name"`
	Healthy    bool                   `json:"healthy"`
	APIVersion string                 `json:"api_version"`
	Kind       string                 `json:"kind"`
	Strategy   string                 `json:"strategy"`
	Reason     string                 `json:"reason,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

// ResourceHealthResult represents the health check result for a specific resource type
type ResourceHealthResult struct {
	Total   int               `json:"total"`
	Healthy int               `json:"healthy"`
	Details []ResourceDetails `json:"details"`
}

// checkConfigMaps checks the health of ConfigMaps
func (rc *ResourceChecker) checkConfigMaps(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector) (*ResourceHealthResult, error) {
	var configMaps corev1.ConfigMapList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := rc.client.List(ctx, &configMaps, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list ConfigMaps: %w", err)
	}

	result := &ResourceHealthResult{
		Total:   len(configMaps.Items),
		Details: []ResourceDetails{},
	}

	for _, cm := range configMaps.Items {
		details := ResourceDetails{
			Name:    cm.Name,
			Type:    "ConfigMap",
			Healthy: true, // ConfigMaps are healthy if they exist
			Message: "ConfigMap exists and is accessible",
			Details: map[string]interface{}{
				"data_keys":        len(cm.Data),
				"binary_data_keys": len(cm.BinaryData),
			},
		}

		result.Details = append(result.Details, details)
		result.Healthy++
	}

	return result, nil
}

// checkSecrets checks the health of Secrets
func (rc *ResourceChecker) checkSecrets(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector) (*ResourceHealthResult, error) {
	var secrets corev1.SecretList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := rc.client.List(ctx, &secrets, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list Secrets: %w", err)
	}

	result := &ResourceHealthResult{
		Total:   len(secrets.Items),
		Details: []ResourceDetails{},
	}

	for _, secret := range secrets.Items {
		details := ResourceDetails{
			Name:    secret.Name,
			Type:    "Secret",
			Healthy: true, // Secrets are healthy if they exist
			Message: "Secret exists and is accessible",
			Details: map[string]interface{}{
				"type":      string(secret.Type),
				"data_keys": len(secret.Data),
			},
		}

		result.Details = append(result.Details, details)
		result.Healthy++
	}

	return result, nil
}

// checkPVCs checks the health of PersistentVolumeClaims
func (rc *ResourceChecker) checkPVCs(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector) (*ResourceHealthResult, error) {
	var pvcs corev1.PersistentVolumeClaimList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := rc.client.List(ctx, &pvcs, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list PVCs: %w", err)
	}

	result := &ResourceHealthResult{
		Total:   len(pvcs.Items),
		Details: []ResourceDetails{},
	}

	for _, pvc := range pvcs.Items {
		healthy := pvc.Status.Phase == corev1.ClaimBound
		message := fmt.Sprintf("PVC phase: %s", pvc.Status.Phase)

		details := ResourceDetails{
			Name:    pvc.Name,
			Type:    "PersistentVolumeClaim",
			Healthy: healthy,
			Message: message,
			Details: map[string]interface{}{
				"phase":         string(pvc.Status.Phase),
				"volume_name":   pvc.Spec.VolumeName,
				"storage_class": pvc.Spec.StorageClassName,
			},
		}

		if !healthy {
			details.Reason = "NotBound"
		}

		result.Details = append(result.Details, details)
		if healthy {
			result.Healthy++
		}
	}

	return result, nil
}

// checkCustomResource checks the health of a specific custom resource
func (rc *ResourceChecker) checkCustomResource(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, customResourceSpec roostv1alpha1.KubernetesCustomResourceCheck) (*CustomResourceDetails, error) {
	// Parse API version and kind
	gv, err := schema.ParseGroupVersion(customResourceSpec.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid API version %s: %w", customResourceSpec.APIVersion, err)
	}

	gvk := schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    customResourceSpec.Kind,
	}

	// Create unstructured list to query the custom resource
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)

	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := rc.client.List(ctx, list, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list custom resources: %w", err)
	}

	details := &CustomResourceDetails{
		APIVersion: customResourceSpec.APIVersion,
		Kind:       customResourceSpec.Kind,
		Strategy:   customResourceSpec.HealthStrategy,
	}

	if len(list.Items) == 0 {
		details.Name = fmt.Sprintf("%s (none found)", customResourceSpec.Kind)
		details.Healthy = customResourceSpec.HealthStrategy == "exists" // Not healthy if we expect resources to exist
		details.Message = fmt.Sprintf("No %s resources found", customResourceSpec.Kind)
		if !details.Healthy {
			details.Reason = "NotFound"
		}
		return details, nil
	}

	// For simplicity, check the first resource found
	// In a more sophisticated implementation, you might check all resources
	resource := list.Items[0]
	details.Name = resource.GetName()

	switch customResourceSpec.HealthStrategy {
	case "exists":
		details.Healthy = true
		details.Message = fmt.Sprintf("%s exists", customResourceSpec.Kind)

	case "conditions":
		healthy, message := rc.checkResourceConditions(resource, customResourceSpec.ExpectedConditions)
		details.Healthy = healthy
		details.Message = message

	case "status":
		healthy, message := rc.checkResourceStatus(resource, customResourceSpec.StatusPath, customResourceSpec.ExpectedStatus)
		details.Healthy = healthy
		details.Message = message

	default:
		details.Healthy = true
		details.Message = fmt.Sprintf("%s exists (default strategy)", customResourceSpec.Kind)
	}

	return details, nil
}

// checkResourceConditions checks if a resource has the expected conditions
func (rc *ResourceChecker) checkResourceConditions(resource unstructured.Unstructured, expectedConditions []roostv1alpha1.ExpectedCondition) (bool, string) {
	if len(expectedConditions) == 0 {
		return true, "No conditions to check"
	}

	conditions, found, err := unstructured.NestedSlice(resource.Object, "status", "conditions")
	if err != nil || !found {
		return false, "Resource has no status conditions"
	}

	for _, expectedCondition := range expectedConditions {
		conditionMet := false
		for _, condition := range conditions {
			conditionMap, ok := condition.(map[string]interface{})
			if !ok {
				continue
			}

			conditionType, _ := conditionMap["type"].(string)
			conditionStatus, _ := conditionMap["status"].(string)
			conditionReason, _ := conditionMap["reason"].(string)

			if conditionType == expectedCondition.Type && conditionStatus == expectedCondition.Status {
				if expectedCondition.Reason == "" || conditionReason == expectedCondition.Reason {
					conditionMet = true
					break
				}
			}
		}

		if !conditionMet {
			return false, fmt.Sprintf("Expected condition %s=%s not met", expectedCondition.Type, expectedCondition.Status)
		}
	}

	return true, "All expected conditions are met"
}

// checkResourceStatus checks if a resource has the expected status value
func (rc *ResourceChecker) checkResourceStatus(resource unstructured.Unstructured, statusPath, expectedStatus string) (bool, string) {
	if statusPath == "" || expectedStatus == "" {
		return true, "No status path or expected status specified"
	}

	// Simple JSONPath-like extraction (could be enhanced with a proper JSONPath library)
	// For now, support simple dot notation like "status.phase"
	statusValue, found, err := unstructured.NestedString(resource.Object, "status", statusPath)
	if err != nil || !found {
		return false, fmt.Sprintf("Status path %s not found", statusPath)
	}

	if statusValue == expectedStatus {
		return true, fmt.Sprintf("Status %s matches expected value %s", statusPath, expectedStatus)
	}

	return false, fmt.Sprintf("Status %s=%s does not match expected %s", statusPath, statusValue, expectedStatus)
}

// buildDependencyHealthMessage creates a descriptive health message for dependencies
func (rc *ResourceChecker) buildDependencyHealthMessage(healthy bool, analysis *DependencyAnalysis, errors []string) string {
	totalResources := analysis.TotalConfigMaps + analysis.TotalSecrets + analysis.TotalPVCs

	if totalResources == 0 {
		return "No dependency resources found"
	}

	if healthy {
		return fmt.Sprintf("All %d dependency resources are healthy", totalResources)
	}

	totalHealthy := analysis.HealthyConfigMaps + analysis.HealthySecrets + analysis.HealthyPVCs
	message := fmt.Sprintf("Dependency health check failed: %d/%d resources healthy", totalHealthy, totalResources)

	if len(errors) > 0 {
		message += fmt.Sprintf(" (Errors: %v)", errors)
	}

	return message
}

// buildCustomResourceHealthMessage creates a descriptive health message for custom resources
func (rc *ResourceChecker) buildCustomResourceHealthMessage(healthy bool, analysis *CustomResourceAnalysis, errors []string) string {
	if analysis.TotalCustomResources == 0 {
		return "No custom resources configured for health check"
	}

	if healthy {
		return fmt.Sprintf("All %d custom resources are healthy", analysis.TotalCustomResources)
	}

	message := fmt.Sprintf("Custom resource health check failed: %d/%d resources healthy",
		analysis.HealthyCustomResources, analysis.TotalCustomResources)

	if len(errors) > 0 {
		message += fmt.Sprintf(" (Errors: %v)", errors)
	}

	return message
}
