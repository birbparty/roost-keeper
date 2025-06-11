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

// NodeChecker handles node health assessment
type NodeChecker struct {
	client client.Client
	logger *zap.Logger
}

// NewNodeChecker creates a new node health checker
func NewNodeChecker(k8sClient client.Client, logger *zap.Logger) *NodeChecker {
	return &NodeChecker{
		client: k8sClient,
		logger: logger,
	}
}

// CheckNodes performs comprehensive node health checking
func (nc *NodeChecker) CheckNodes(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := nc.logger.With(
		zap.String("component", "nodes"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting node health check")

	// First, get pods to identify which nodes are hosting the application
	var pods corev1.PodList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := nc.client.List(ctx, &pods, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list pods to identify nodes: %w", err)
	}

	// Extract unique node names hosting the application pods
	nodeNames := make(map[string]bool)
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != "" {
			nodeNames[pod.Spec.NodeName] = true
		}
	}

	log.Debug("Found nodes hosting application pods", zap.Int("node_count", len(nodeNames)))

	if len(nodeNames) == 0 {
		return &ComponentHealth{
			Healthy:   true,
			Message:   "No nodes to check (no pods scheduled)",
			CheckedAt: time.Now(),
			Details: map[string]interface{}{
				"total_nodes": 0,
				"reason":      "no_pods_scheduled",
			},
		}, nil
	}

	// Get node information for identified nodes
	analysis, err := nc.analyzeNodes(ctx, nodeNames)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze nodes: %w", err)
	}

	// Determine overall health (all nodes must be healthy)
	healthy := analysis.HealthyNodes == analysis.TotalNodes

	// Build health message
	message := nc.buildNodeHealthMessage(healthy, analysis)

	return &ComponentHealth{
		Healthy:      healthy,
		Message:      message,
		CheckedAt:    time.Now(),
		ErrorCount:   analysis.ErrorNodes,
		WarningCount: analysis.WarningNodes,
		Details: map[string]interface{}{
			"total_nodes":   analysis.TotalNodes,
			"healthy_nodes": analysis.HealthyNodes,
			"error_nodes":   analysis.ErrorNodes,
			"warning_nodes": analysis.WarningNodes,
			"node_details":  analysis.NodeDetails,
		},
	}, nil
}

// NodeAnalysis contains the results of node health analysis
type NodeAnalysis struct {
	TotalNodes   int           `json:"total_nodes"`
	HealthyNodes int           `json:"healthy_nodes"`
	ErrorNodes   int           `json:"error_nodes"`
	WarningNodes int           `json:"warning_nodes"`
	NodeDetails  []NodeDetails `json:"node_details"`
}

// NodeDetails contains detailed information about a specific node
type NodeDetails struct {
	Name             string                 `json:"name"`
	Healthy          bool                   `json:"healthy"`
	Ready            bool                   `json:"ready"`
	KubeletVersion   string                 `json:"kubelet_version,omitempty"`
	ContainerRuntime string                 `json:"container_runtime,omitempty"`
	OperatingSystem  string                 `json:"operating_system,omitempty"`
	Architecture     string                 `json:"architecture,omitempty"`
	Conditions       []NodeConditionInfo    `json:"conditions,omitempty"`
	Addresses        []NodeAddressInfo      `json:"addresses,omitempty"`
	Capacity         map[string]interface{} `json:"capacity,omitempty"`
	Allocatable      map[string]interface{} `json:"allocatable,omitempty"`
	Issues           []string               `json:"issues,omitempty"`
	Reason           string                 `json:"reason,omitempty"`
	Message          string                 `json:"message,omitempty"`
}

// NodeConditionInfo contains information about a node condition
type NodeConditionInfo struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// NodeAddressInfo contains information about a node address
type NodeAddressInfo struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// analyzeNodes performs detailed analysis of node health
func (nc *NodeChecker) analyzeNodes(ctx context.Context, nodeNames map[string]bool) (*NodeAnalysis, error) {
	analysis := &NodeAnalysis{
		TotalNodes:  len(nodeNames),
		NodeDetails: []NodeDetails{},
	}

	for nodeName := range nodeNames {
		var node corev1.Node
		if err := nc.client.Get(ctx, client.ObjectKey{Name: nodeName}, &node); err != nil {
			// Node not found or inaccessible
			details := NodeDetails{
				Name:    nodeName,
				Healthy: false,
				Ready:   false,
				Reason:  "NodeNotFound",
				Message: fmt.Sprintf("Failed to get node information: %v", err),
				Issues:  []string{"Node not accessible"},
			}
			analysis.NodeDetails = append(analysis.NodeDetails, details)
			analysis.ErrorNodes++
			continue
		}

		details := nc.analyzeNode(node)
		analysis.NodeDetails = append(analysis.NodeDetails, details)

		if details.Healthy {
			analysis.HealthyNodes++
		} else {
			if len(details.Issues) > 0 {
				analysis.ErrorNodes++
			} else {
				analysis.WarningNodes++
			}
		}
	}

	return analysis, nil
}

// analyzeNode provides detailed analysis of a single node
func (nc *NodeChecker) analyzeNode(node corev1.Node) NodeDetails {
	details := NodeDetails{
		Name:        node.Name,
		Conditions:  []NodeConditionInfo{},
		Addresses:   []NodeAddressInfo{},
		Issues:      []string{},
		Capacity:    make(map[string]interface{}),
		Allocatable: make(map[string]interface{}),
	}

	// Extract basic node information
	if node.Status.NodeInfo.KubeletVersion != "" {
		details.KubeletVersion = node.Status.NodeInfo.KubeletVersion
	}
	if node.Status.NodeInfo.ContainerRuntimeVersion != "" {
		details.ContainerRuntime = node.Status.NodeInfo.ContainerRuntimeVersion
	}
	if node.Status.NodeInfo.OperatingSystem != "" {
		details.OperatingSystem = node.Status.NodeInfo.OperatingSystem
	}
	if node.Status.NodeInfo.Architecture != "" {
		details.Architecture = node.Status.NodeInfo.Architecture
	}

	// Extract node addresses
	for _, address := range node.Status.Addresses {
		details.Addresses = append(details.Addresses, NodeAddressInfo{
			Type:    string(address.Type),
			Address: address.Address,
		})
	}

	// Extract capacity and allocatable resources
	for resource, quantity := range node.Status.Capacity {
		details.Capacity[string(resource)] = quantity.String()
	}
	for resource, quantity := range node.Status.Allocatable {
		details.Allocatable[string(resource)] = quantity.String()
	}

	// Analyze node conditions
	readyConditionFound := false
	for _, condition := range node.Status.Conditions {
		conditionInfo := NodeConditionInfo{
			Type:    string(condition.Type),
			Status:  string(condition.Status),
			Reason:  condition.Reason,
			Message: condition.Message,
		}
		details.Conditions = append(details.Conditions, conditionInfo)

		// Check specific conditions for health assessment
		switch condition.Type {
		case corev1.NodeReady:
			readyConditionFound = true
			if condition.Status == corev1.ConditionTrue {
				details.Ready = true
			} else {
				details.Issues = append(details.Issues, fmt.Sprintf("Node not ready: %s", condition.Message))
			}

		case corev1.NodeMemoryPressure:
			if condition.Status == corev1.ConditionTrue {
				details.Issues = append(details.Issues, "Node has memory pressure")
			}

		case corev1.NodeDiskPressure:
			if condition.Status == corev1.ConditionTrue {
				details.Issues = append(details.Issues, "Node has disk pressure")
			}

		case corev1.NodePIDPressure:
			if condition.Status == corev1.ConditionTrue {
				details.Issues = append(details.Issues, "Node has PID pressure")
			}

		case corev1.NodeNetworkUnavailable:
			if condition.Status == corev1.ConditionTrue {
				details.Issues = append(details.Issues, "Node network is unavailable")
			}
		}
	}

	// If no Ready condition found, that's an issue
	if !readyConditionFound {
		details.Issues = append(details.Issues, "Node Ready condition not found")
	}

	// Check if node is cordoned (unschedulable)
	if node.Spec.Unschedulable {
		details.Issues = append(details.Issues, "Node is cordoned (unschedulable)")
	}

	// Check for taints that might affect scheduling
	criticalTaints := 0
	for _, taint := range node.Spec.Taints {
		if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
			criticalTaints++
		}
	}
	if criticalTaints > 0 {
		details.Issues = append(details.Issues, fmt.Sprintf("Node has %d critical taints", criticalTaints))
	}

	// Determine overall health
	details.Healthy = details.Ready && len(details.Issues) == 0

	// Build health message
	if details.Healthy {
		details.Message = "Node is healthy and ready"
	} else {
		if len(details.Issues) > 0 {
			details.Message = fmt.Sprintf("Node has issues: %v", details.Issues)
			details.Reason = "NodeIssues"
		} else if !details.Ready {
			details.Message = "Node is not ready"
			details.Reason = "NodeNotReady"
		}
	}

	return details
}

// buildNodeHealthMessage creates a descriptive health message for nodes
func (nc *NodeChecker) buildNodeHealthMessage(healthy bool, analysis *NodeAnalysis) string {
	if healthy {
		if analysis.TotalNodes == 1 {
			return "Node hosting application pods is healthy"
		}
		return fmt.Sprintf("All %d nodes hosting application pods are healthy", analysis.TotalNodes)
	}

	if analysis.TotalNodes == 1 {
		return "Node hosting application pods is unhealthy"
	}

	return fmt.Sprintf("Node health check failed: %d/%d nodes healthy",
		analysis.HealthyNodes, analysis.TotalNodes)
}
