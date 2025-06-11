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

// ServiceChecker handles service and endpoint health assessment
type ServiceChecker struct {
	client client.Client
	logger *zap.Logger
}

// NewServiceChecker creates a new service health checker
func NewServiceChecker(k8sClient client.Client, logger *zap.Logger) *ServiceChecker {
	return &ServiceChecker{
		client: k8sClient,
		logger: logger,
	}
}

// CheckServices performs comprehensive service health checking
func (sc *ServiceChecker) CheckServices(ctx context.Context, roost *roostv1alpha1.ManagedRoost, selector labels.Selector, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (*ComponentHealth, error) {
	log := sc.logger.With(
		zap.String("component", "services"),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	log.Debug("Starting service health check")

	// List services matching the selector
	var services corev1.ServiceList
	listOpts := []client.ListOption{
		client.InNamespace(roost.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := sc.client.List(ctx, &services, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	log.Debug("Found services", zap.Int("count", len(services.Items)))

	if len(services.Items) == 0 {
		return &ComponentHealth{
			Healthy:   true,
			Message:   "No services found (not required for health)",
			CheckedAt: time.Now(),
			Details: map[string]interface{}{
				"total_services": 0,
				"selector":       selector.String(),
				"namespace":      roost.Namespace,
			},
		}, nil
	}

	// Analyze service health
	analysis, err := sc.analyzeServices(ctx, services.Items)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze services: %w", err)
	}

	// Determine overall health (all services must have endpoints)
	healthy := analysis.HealthyServices == analysis.TotalServices

	// Build health message
	message := sc.buildServiceHealthMessage(healthy, analysis)

	return &ComponentHealth{
		Healthy:      healthy,
		Message:      message,
		CheckedAt:    time.Now(),
		ErrorCount:   analysis.ErrorServices,
		WarningCount: analysis.WarningServices,
		Details: map[string]interface{}{
			"total_services":   analysis.TotalServices,
			"healthy_services": analysis.HealthyServices,
			"error_services":   analysis.ErrorServices,
			"warning_services": analysis.WarningServices,
			"service_details":  analysis.ServiceDetails,
		},
	}, nil
}

// ServiceAnalysis contains the results of service health analysis
type ServiceAnalysis struct {
	TotalServices   int              `json:"total_services"`
	HealthyServices int              `json:"healthy_services"`
	ErrorServices   int              `json:"error_services"`
	WarningServices int              `json:"warning_services"`
	ServiceDetails  []ServiceDetails `json:"service_details"`
}

// ServiceDetails contains detailed information about a specific service
type ServiceDetails struct {
	Name            string            `json:"name"`
	Healthy         bool              `json:"healthy"`
	Type            string            `json:"type"`
	ClusterIP       string            `json:"cluster_ip,omitempty"`
	ExternalIPs     []string          `json:"external_ips,omitempty"`
	LoadBalancerIP  string            `json:"load_balancer_ip,omitempty"`
	Ports           []ServicePortInfo `json:"ports,omitempty"`
	EndpointCount   int               `json:"endpoint_count"`
	EndpointDetails []EndpointDetails `json:"endpoint_details,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	Message         string            `json:"message,omitempty"`
}

// ServicePortInfo contains information about a service port
type ServicePortInfo struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port,omitempty"`
	Protocol   string `json:"protocol"`
	NodePort   int32  `json:"node_port,omitempty"`
}

// EndpointDetails contains information about service endpoints
type EndpointDetails struct {
	IP       string             `json:"ip"`
	Ready    bool               `json:"ready"`
	NodeName string             `json:"node_name,omitempty"`
	Ports    []EndpointPortInfo `json:"ports,omitempty"`
}

// EndpointPortInfo contains information about an endpoint port
type EndpointPortInfo struct {
	Name     string `json:"name,omitempty"`
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
}

// analyzeServices performs detailed analysis of service health
func (sc *ServiceChecker) analyzeServices(ctx context.Context, services []corev1.Service) (*ServiceAnalysis, error) {
	analysis := &ServiceAnalysis{
		TotalServices:  len(services),
		ServiceDetails: []ServiceDetails{},
	}

	for _, service := range services {
		details, err := sc.analyzeService(ctx, service)
		if err != nil {
			return nil, fmt.Errorf("failed to analyze service %s: %w", service.Name, err)
		}

		analysis.ServiceDetails = append(analysis.ServiceDetails, *details)

		if details.Healthy {
			analysis.HealthyServices++
		} else {
			analysis.ErrorServices++
		}
	}

	return analysis, nil
}

// analyzeService provides detailed analysis of a single service
func (sc *ServiceChecker) analyzeService(ctx context.Context, service corev1.Service) (*ServiceDetails, error) {
	details := &ServiceDetails{
		Name:      service.Name,
		Type:      string(service.Spec.Type),
		ClusterIP: service.Spec.ClusterIP,
	}

	// Extract external IPs
	if len(service.Spec.ExternalIPs) > 0 {
		details.ExternalIPs = service.Spec.ExternalIPs
	}

	// Extract load balancer IP
	if service.Status.LoadBalancer.Ingress != nil && len(service.Status.LoadBalancer.Ingress) > 0 {
		if service.Status.LoadBalancer.Ingress[0].IP != "" {
			details.LoadBalancerIP = service.Status.LoadBalancer.Ingress[0].IP
		}
	}

	// Extract port information
	for _, port := range service.Spec.Ports {
		portInfo := ServicePortInfo{
			Name:     port.Name,
			Port:     port.Port,
			Protocol: string(port.Protocol),
		}

		if port.TargetPort.IntVal != 0 {
			portInfo.TargetPort = fmt.Sprintf("%d", port.TargetPort.IntVal)
		} else if port.TargetPort.StrVal != "" {
			portInfo.TargetPort = port.TargetPort.StrVal
		}

		if port.NodePort != 0 {
			portInfo.NodePort = port.NodePort
		}

		details.Ports = append(details.Ports, portInfo)
	}

	// Get endpoint information
	endpointCount, endpointDetails, err := sc.getServiceEndpoints(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints for service %s: %w", service.Name, err)
	}

	details.EndpointCount = endpointCount
	details.EndpointDetails = endpointDetails

	// Determine health based on endpoint availability
	// Services are healthy if they have at least one ready endpoint
	// Exception: ExternalName services don't have endpoints
	if service.Spec.Type == corev1.ServiceTypeExternalName {
		details.Healthy = true
		details.Message = "ExternalName service (no endpoints required)"
	} else if endpointCount > 0 {
		details.Healthy = true
		details.Message = fmt.Sprintf("Service has %d ready endpoint(s)", endpointCount)
	} else {
		details.Healthy = false
		details.Message = "Service has no ready endpoints"
		details.Reason = "NoEndpoints"
	}

	return details, nil
}

// getServiceEndpoints retrieves and analyzes service endpoints
func (sc *ServiceChecker) getServiceEndpoints(ctx context.Context, service corev1.Service) (int, []EndpointDetails, error) {
	var endpoints corev1.Endpoints
	if err := sc.client.Get(ctx, client.ObjectKey{
		Name:      service.Name,
		Namespace: service.Namespace,
	}, &endpoints); err != nil {
		// Endpoints not found is not necessarily an error for some service types
		return 0, []EndpointDetails{}, nil
	}

	var endpointDetails []EndpointDetails
	readyCount := 0

	for _, subset := range endpoints.Subsets {
		// Process ready addresses
		for _, address := range subset.Addresses {
			detail := EndpointDetails{
				IP:    address.IP,
				Ready: true,
			}

			if address.NodeName != nil {
				detail.NodeName = *address.NodeName
			}

			// Extract port information
			for _, port := range subset.Ports {
				portInfo := EndpointPortInfo{
					Name:     port.Name,
					Port:     port.Port,
					Protocol: string(port.Protocol),
				}
				detail.Ports = append(detail.Ports, portInfo)
			}

			endpointDetails = append(endpointDetails, detail)
			readyCount++
		}

		// Process not ready addresses
		for _, address := range subset.NotReadyAddresses {
			detail := EndpointDetails{
				IP:    address.IP,
				Ready: false,
			}

			if address.NodeName != nil {
				detail.NodeName = *address.NodeName
			}

			// Extract port information
			for _, port := range subset.Ports {
				portInfo := EndpointPortInfo{
					Name:     port.Name,
					Port:     port.Port,
					Protocol: string(port.Protocol),
				}
				detail.Ports = append(detail.Ports, portInfo)
			}

			endpointDetails = append(endpointDetails, detail)
		}
	}

	return readyCount, endpointDetails, nil
}

// buildServiceHealthMessage creates a descriptive health message for services
func (sc *ServiceChecker) buildServiceHealthMessage(healthy bool, analysis *ServiceAnalysis) string {
	if healthy {
		if analysis.TotalServices == 1 {
			return "Service has ready endpoints"
		}
		return fmt.Sprintf("All %d services have ready endpoints", analysis.TotalServices)
	}

	if analysis.TotalServices == 1 {
		return "Service has no ready endpoints"
	}

	return fmt.Sprintf("Service health check failed: %d/%d services have ready endpoints",
		analysis.HealthyServices, analysis.TotalServices)
}
