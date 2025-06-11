package kubernetes

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// KubernetesChecker implements comprehensive Kubernetes-native health checking
type KubernetesChecker struct {
	client            client.Client
	logger            *zap.Logger
	cache             *ResourceCache
	podChecker        *PodChecker
	deploymentChecker *DeploymentChecker
	serviceChecker    *ServiceChecker
	resourceChecker   *ResourceChecker
	nodeChecker       *NodeChecker
}

// HealthResult represents the complete result of a Kubernetes health check
type HealthResult struct {
	Healthy         bool                        `json:"healthy"`
	Message         string                      `json:"message"`
	Details         map[string]interface{}      `json:"details,omitempty"`
	ComponentHealth map[string]*ComponentHealth `json:"component_health,omitempty"`
	ExecutionTime   time.Duration               `json:"execution_time"`
	FromCache       bool                        `json:"from_cache,omitempty"`
	ResourceCounts  *ResourceCounts             `json:"resource_counts,omitempty"`
}

// ComponentHealth represents the health status of a specific component type
type ComponentHealth struct {
	Healthy      bool                   `json:"healthy"`
	Message      string                 `json:"message"`
	Details      map[string]interface{} `json:"details,omitempty"`
	CheckedAt    time.Time              `json:"checked_at"`
	ErrorCount   int                    `json:"error_count,omitempty"`
	WarningCount int                    `json:"warning_count,omitempty"`
}

// ResourceCounts provides a summary of discovered resources
type ResourceCounts struct {
	Pods            int `json:"pods"`
	Deployments     int `json:"deployments"`
	Services        int `json:"services"`
	StatefulSets    int `json:"stateful_sets"`
	DaemonSets      int `json:"daemon_sets"`
	Dependencies    int `json:"dependencies"`
	CustomResources int `json:"custom_resources"`
}

// NewKubernetesChecker creates a new Kubernetes health checker with all capabilities
func NewKubernetesChecker(k8sClient client.Client, logger *zap.Logger) *KubernetesChecker {
	cache := NewResourceCache(k8sClient, logger)

	return &KubernetesChecker{
		client:            k8sClient,
		logger:            logger,
		cache:             cache,
		podChecker:        NewPodChecker(k8sClient, logger),
		deploymentChecker: NewDeploymentChecker(k8sClient, logger),
		serviceChecker:    NewServiceChecker(k8sClient, logger),
		resourceChecker:   NewResourceChecker(k8sClient, logger),
		nodeChecker:       NewNodeChecker(k8sClient, logger),
	}
}

// CheckHealth performs a comprehensive Kubernetes health check
func (k *KubernetesChecker) CheckHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*HealthResult, error) {
	start := time.Now()

	log := k.logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("check_type", "kubernetes"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	// Start observability span
	ctx, span := telemetry.StartHealthCheckSpan(ctx, "kubernetes", roost.Name)
	defer span.End()

	log.Info("Starting Kubernetes health check")

	// Build resource discovery selector
	selector, err := k.buildResourceSelector(roost, kubernetesSpec)
	if err != nil {
		log.Error("Failed to build resource selector", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return &HealthResult{
			Healthy:       false,
			Message:       fmt.Sprintf("Resource selector build failed: %v", err),
			ExecutionTime: time.Since(start),
		}, nil
	}

	// Check cache if enabled
	if k.shouldUseCache(kubernetesSpec) {
		cacheKey := k.buildCacheKey(roost, checkSpec, kubernetesSpec)
		if cachedResult := k.cache.GetHealthResult(cacheKey); cachedResult != nil {
			log.Debug("Kubernetes health check result served from cache")
			cachedResult.FromCache = true
			cachedResult.ExecutionTime = time.Since(start)
			return cachedResult, nil
		}
	}

	// Perform component health checks
	componentHealth := make(map[string]*ComponentHealth)
	allHealthy := true
	var lastError error

	// Check pods
	if kubernetesSpec.CheckPods {
		podHealth, err := k.podChecker.CheckPods(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("Pod health check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !podHealth.Healthy {
			allHealthy = false
		}
		componentHealth["pods"] = podHealth
	}

	// Check deployments
	if kubernetesSpec.CheckDeployments {
		deploymentHealth, err := k.deploymentChecker.CheckDeployments(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("Deployment health check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !deploymentHealth.Healthy {
			allHealthy = false
		}
		componentHealth["deployments"] = deploymentHealth
	}

	// Check services
	if kubernetesSpec.CheckServices {
		serviceHealth, err := k.serviceChecker.CheckServices(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("Service health check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !serviceHealth.Healthy {
			allHealthy = false
		}
		componentHealth["services"] = serviceHealth
	}

	// Check StatefulSets
	if kubernetesSpec.CheckStatefulSets {
		statefulSetHealth, err := k.deploymentChecker.CheckStatefulSets(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("StatefulSet health check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !statefulSetHealth.Healthy {
			allHealthy = false
		}
		componentHealth["stateful_sets"] = statefulSetHealth
	}

	// Check DaemonSets
	if kubernetesSpec.CheckDaemonSets {
		daemonSetHealth, err := k.deploymentChecker.CheckDaemonSets(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("DaemonSet health check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !daemonSetHealth.Healthy {
			allHealthy = false
		}
		componentHealth["daemon_sets"] = daemonSetHealth
	}

	// Check resource dependencies
	if kubernetesSpec.CheckDependencies {
		resourceHealth, err := k.resourceChecker.CheckDependencies(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("Resource dependency check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !resourceHealth.Healthy {
			allHealthy = false
		}
		componentHealth["dependencies"] = resourceHealth
	}

	// Check node health
	if kubernetesSpec.CheckNodeHealth {
		nodeHealth, err := k.nodeChecker.CheckNodes(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("Node health check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !nodeHealth.Healthy {
			allHealthy = false
		}
		componentHealth["nodes"] = nodeHealth
	}

	// Check custom resources
	if len(kubernetesSpec.CustomResources) > 0 {
		customResourceHealth, err := k.resourceChecker.CheckCustomResources(ctx, roost, selector, kubernetesSpec)
		if err != nil {
			log.Error("Custom resource health check failed", zap.Error(err))
			lastError = err
			allHealthy = false
		} else if !customResourceHealth.Healthy {
			allHealthy = false
		}
		componentHealth["custom_resources"] = customResourceHealth
	}

	// Build overall result
	result := &HealthResult{
		Healthy:         allHealthy,
		Message:         k.buildHealthMessage(allHealthy, componentHealth),
		ComponentHealth: componentHealth,
		ExecutionTime:   time.Since(start),
		ResourceCounts:  k.buildResourceCounts(componentHealth),
		Details: map[string]interface{}{
			"selector":          selector.String(),
			"roost_name":        roost.Name,
			"roost_namespace":   roost.Namespace,
			"execution_time_ms": time.Since(start).Milliseconds(),
			"cache_enabled":     k.shouldUseCache(kubernetesSpec),
		},
	}

	// Cache successful results
	if allHealthy && k.shouldUseCache(kubernetesSpec) {
		cacheKey := k.buildCacheKey(roost, checkSpec, kubernetesSpec)
		cacheTTL := k.getCacheTTL(kubernetesSpec)
		k.cache.SetHealthResult(cacheKey, result, cacheTTL)
	}

	// Record telemetry
	if allHealthy {
		telemetry.RecordSpanSuccess(ctx)
	} else if lastError != nil {
		telemetry.RecordSpanError(ctx, lastError)
	}

	log.Info("Kubernetes health check completed",
		zap.Bool("healthy", allHealthy),
		zap.Duration("execution_time", time.Since(start)),
		zap.Int("component_count", len(componentHealth)),
	)

	return result, lastError
}

// ExecuteKubernetesCheck provides backward compatibility with the main health checker
func ExecuteKubernetesCheck(ctx context.Context, logger *zap.Logger, k8sClient client.Client, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (bool, error) {
	checker := NewKubernetesChecker(k8sClient, logger)
	result, err := checker.CheckHealth(ctx, roost, checkSpec, kubernetesSpec)
	if err != nil {
		return false, err
	}
	return result.Healthy, nil
}

// buildResourceSelector creates a label selector for discovering Helm-deployed resources
func (k *KubernetesChecker) buildResourceSelector(roost *roostv1alpha1.ManagedRoost, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (labels.Selector, error) {
	// Start with Helm release labels
	selectorMap := map[string]string{
		"app.kubernetes.io/instance": roost.Name,
	}

	// Add additional standard Helm labels if available
	if roost.Spec.Chart.Name != "" {
		selectorMap["app.kubernetes.io/name"] = roost.Spec.Chart.Name
	}

	// Merge with custom label selector if provided
	if kubernetesSpec.LabelSelector != nil {
		if kubernetesSpec.LabelSelector.MatchLabels != nil {
			for key, value := range kubernetesSpec.LabelSelector.MatchLabels {
				selectorMap[key] = value
			}
		}
	}

	// Create labels.Selector
	selector := labels.SelectorFromSet(selectorMap)

	// Handle MatchExpressions if provided
	if kubernetesSpec.LabelSelector != nil && len(kubernetesSpec.LabelSelector.MatchExpressions) > 0 {
		return metav1.LabelSelectorAsSelector(kubernetesSpec.LabelSelector)
	}

	return selector, nil
}

// buildHealthMessage creates a descriptive health message
func (k *KubernetesChecker) buildHealthMessage(healthy bool, componentHealth map[string]*ComponentHealth) string {
	if healthy {
		return "All Kubernetes components are healthy"
	}

	unhealthyComponents := []string{}
	for component, health := range componentHealth {
		if !health.Healthy {
			unhealthyComponents = append(unhealthyComponents, component)
		}
	}

	if len(unhealthyComponents) == 1 {
		return fmt.Sprintf("Kubernetes component %s is unhealthy", unhealthyComponents[0])
	}

	return fmt.Sprintf("Kubernetes components %v are unhealthy", unhealthyComponents)
}

// buildResourceCounts creates a summary of resource counts
func (k *KubernetesChecker) buildResourceCounts(componentHealth map[string]*ComponentHealth) *ResourceCounts {
	counts := &ResourceCounts{}

	if podHealth, exists := componentHealth["pods"]; exists && podHealth.Details != nil {
		if totalPods, ok := podHealth.Details["total_pods"].(int); ok {
			counts.Pods = totalPods
		}
	}

	if deploymentHealth, exists := componentHealth["deployments"]; exists && deploymentHealth.Details != nil {
		if totalDeployments, ok := deploymentHealth.Details["total_deployments"].(int); ok {
			counts.Deployments = totalDeployments
		}
	}

	if serviceHealth, exists := componentHealth["services"]; exists && serviceHealth.Details != nil {
		if totalServices, ok := serviceHealth.Details["total_services"].(int); ok {
			counts.Services = totalServices
		}
	}

	if statefulSetHealth, exists := componentHealth["stateful_sets"]; exists && statefulSetHealth.Details != nil {
		if totalStatefulSets, ok := statefulSetHealth.Details["total_stateful_sets"].(int); ok {
			counts.StatefulSets = totalStatefulSets
		}
	}

	if daemonSetHealth, exists := componentHealth["daemon_sets"]; exists && daemonSetHealth.Details != nil {
		if totalDaemonSets, ok := daemonSetHealth.Details["total_daemon_sets"].(int); ok {
			counts.DaemonSets = totalDaemonSets
		}
	}

	return counts
}

// buildCacheKey creates a unique cache key for the health check
func (k *KubernetesChecker) buildCacheKey(roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) string {
	return fmt.Sprintf("k8s:%s:%s:%s", roost.Namespace, roost.Name, checkSpec.Name)
}

// shouldUseCache determines if caching should be used for this check
func (k *KubernetesChecker) shouldUseCache(kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) bool {
	if kubernetesSpec.EnableCaching != nil {
		return *kubernetesSpec.EnableCaching
	}
	return true // Default to enabled
}

// getCacheTTL returns the cache TTL for this check
func (k *KubernetesChecker) getCacheTTL(kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) time.Duration {
	if kubernetesSpec.CacheTTL != nil && kubernetesSpec.CacheTTL.Duration > 0 {
		return kubernetesSpec.CacheTTL.Duration
	}
	return 30 * time.Second // Default TTL
}
