package prometheus

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// ServiceDiscovery handles automatic discovery of Prometheus endpoints
type ServiceDiscovery struct {
	k8sClient client.Client
	logger    *zap.Logger
}

// NewServiceDiscovery creates a new service discovery manager
func NewServiceDiscovery(k8sClient client.Client, logger *zap.Logger) *ServiceDiscovery {
	return &ServiceDiscovery{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// DiscoverEndpoint discovers a Prometheus endpoint using Kubernetes service discovery
func (s *ServiceDiscovery) DiscoverEndpoint(ctx context.Context, namespace string, config *roostv1alpha1.PrometheusDiscoverySpec) (string, error) {
	log := s.logger.With(
		zap.String("namespace", namespace),
		zap.String("service_name", config.ServiceName),
	)

	log.Debug("Starting Prometheus endpoint discovery")

	// Try service name discovery first
	if config.ServiceName != "" {
		endpoint, err := s.discoverByServiceName(ctx, namespace, config)
		if err == nil {
			log.Info("Prometheus endpoint discovered by service name", zap.String("endpoint", endpoint))
			return endpoint, nil
		}
		log.Warn("Service name discovery failed", zap.Error(err))
	}

	// Try label selector discovery
	if len(config.LabelSelector) > 0 {
		endpoint, err := s.discoverByLabelSelector(ctx, namespace, config)
		if err == nil {
			log.Info("Prometheus endpoint discovered by label selector", zap.String("endpoint", endpoint))
			return endpoint, nil
		}
		log.Warn("Label selector discovery failed", zap.Error(err))
	}

	return "", fmt.Errorf("failed to discover Prometheus endpoint: no valid service found")
}

// discoverByServiceName discovers endpoint using explicit service name
func (s *ServiceDiscovery) discoverByServiceName(ctx context.Context, namespace string, config *roostv1alpha1.PrometheusDiscoverySpec) (string, error) {
	serviceName := config.ServiceName
	serviceNamespace := config.ServiceNamespace
	if serviceNamespace == "" {
		serviceNamespace = namespace
	}

	// Get the service
	service := &corev1.Service{}
	err := s.k8sClient.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: serviceNamespace,
	}, service)
	if err != nil {
		return "", fmt.Errorf("failed to get service %s/%s: %w", serviceNamespace, serviceName, err)
	}

	return s.buildEndpointFromService(service, config)
}

// discoverByLabelSelector discovers endpoint using label selector
func (s *ServiceDiscovery) discoverByLabelSelector(ctx context.Context, namespace string, config *roostv1alpha1.PrometheusDiscoverySpec) (string, error) {
	serviceList := &corev1.ServiceList{}

	// Build label selector
	selector := client.MatchingLabels(config.LabelSelector)

	// Search in specified namespace or current namespace
	searchNamespace := config.ServiceNamespace
	if searchNamespace == "" {
		searchNamespace = namespace
	}

	err := s.k8sClient.List(ctx, serviceList, client.InNamespace(searchNamespace), selector)
	if err != nil {
		return "", fmt.Errorf("failed to list services with labels %v: %w", config.LabelSelector, err)
	}

	if len(serviceList.Items) == 0 {
		return "", fmt.Errorf("no services found with labels %v in namespace %s", config.LabelSelector, searchNamespace)
	}

	// Use the first matching service
	service := &serviceList.Items[0]

	if len(serviceList.Items) > 1 {
		s.logger.Warn("Multiple services found, using first one",
			zap.String("selected_service", service.Name),
			zap.Int("total_services", len(serviceList.Items)),
		)
	}

	return s.buildEndpointFromService(service, config)
}

// buildEndpointFromService constructs the Prometheus endpoint URL from a Kubernetes service
func (s *ServiceDiscovery) buildEndpointFromService(service *corev1.Service, config *roostv1alpha1.PrometheusDiscoverySpec) (string, error) {
	// Determine the port to use
	port, err := s.determineServicePort(service, config.ServicePort)
	if err != nil {
		return "", fmt.Errorf("failed to determine service port: %w", err)
	}

	// Determine the protocol scheme
	scheme := "http"
	if config.TLS {
		scheme = "https"
	}

	// Build the hostname
	hostname := s.buildServiceHostname(service)

	// Build the path
	path := "/api/v1"
	if config.PathPrefix != "" {
		path = strings.TrimSuffix(config.PathPrefix, "/") + "/api/v1"
	}

	// Construct the full URL
	endpoint := fmt.Sprintf("%s://%s:%d%s", scheme, hostname, port, path)

	return endpoint, nil
}

// determineServicePort determines which port to use for the Prometheus connection
func (s *ServiceDiscovery) determineServicePort(service *corev1.Service, portSpec string) (int32, error) {
	if len(service.Spec.Ports) == 0 {
		return 0, fmt.Errorf("service %s/%s has no ports defined", service.Namespace, service.Name)
	}

	// If no port specified, use the first port
	if portSpec == "" {
		return service.Spec.Ports[0].Port, nil
	}

	// Try to parse as port number
	if portNum, err := strconv.ParseInt(portSpec, 10, 32); err == nil {
		// Verify the port exists
		for _, p := range service.Spec.Ports {
			if p.Port == int32(portNum) {
				return int32(portNum), nil
			}
		}
		return 0, fmt.Errorf("port %d not found in service %s/%s", portNum, service.Namespace, service.Name)
	}

	// Try to find by port name
	for _, p := range service.Spec.Ports {
		if p.Name == portSpec {
			return p.Port, nil
		}
	}

	// Try common Prometheus port names
	commonNames := []string{"prometheus", "metrics", "http", "web"}
	for _, commonName := range commonNames {
		if strings.Contains(strings.ToLower(portSpec), commonName) {
			for _, p := range service.Spec.Ports {
				if strings.Contains(strings.ToLower(p.Name), commonName) {
					return p.Port, nil
				}
			}
		}
	}

	return 0, fmt.Errorf("port %s not found in service %s/%s", portSpec, service.Namespace, service.Name)
}

// buildServiceHostname constructs the service hostname for cluster-internal access
func (s *ServiceDiscovery) buildServiceHostname(service *corev1.Service) string {
	// Use cluster-internal DNS name: service-name.namespace.svc.cluster.local
	return fmt.Sprintf("%s.%s.svc.cluster.local", service.Name, service.Namespace)
}

// ValidateDiscoveryConfig validates the service discovery configuration
func (s *ServiceDiscovery) ValidateDiscoveryConfig(config *roostv1alpha1.PrometheusDiscoverySpec) error {
	if config == nil {
		return fmt.Errorf("service discovery configuration is required")
	}

	// Must have either service name or label selector
	if config.ServiceName == "" && len(config.LabelSelector) == 0 {
		return fmt.Errorf("either serviceName or labelSelector must be specified")
	}

	// Validate service port if specified
	if config.ServicePort != "" {
		// Try to parse as number
		if portNum, err := strconv.ParseInt(config.ServicePort, 10, 32); err == nil {
			if portNum < 1 || portNum > 65535 {
				return fmt.Errorf("servicePort must be between 1 and 65535")
			}
		}
		// If not a number, assume it's a port name (which is valid)
	}

	return nil
}

// GetServiceInfo returns detailed information about discovered services for debugging
func (s *ServiceDiscovery) GetServiceInfo(ctx context.Context, namespace string, config *roostv1alpha1.PrometheusDiscoverySpec) ([]ServiceInfo, error) {
	var services []ServiceInfo

	// Get services by name
	if config.ServiceName != "" {
		serviceNamespace := config.ServiceNamespace
		if serviceNamespace == "" {
			serviceNamespace = namespace
		}

		service := &corev1.Service{}
		err := s.k8sClient.Get(ctx, types.NamespacedName{
			Name:      config.ServiceName,
			Namespace: serviceNamespace,
		}, service)
		if err == nil {
			services = append(services, s.serviceToInfo(service))
		}
	}

	// Get services by label selector
	if len(config.LabelSelector) > 0 {
		serviceList := &corev1.ServiceList{}
		selector := client.MatchingLabels(config.LabelSelector)

		searchNamespace := config.ServiceNamespace
		if searchNamespace == "" {
			searchNamespace = namespace
		}

		err := s.k8sClient.List(ctx, serviceList, client.InNamespace(searchNamespace), selector)
		if err == nil {
			for _, svc := range serviceList.Items {
				services = append(services, s.serviceToInfo(&svc))
			}
		}
	}

	return services, nil
}

// ServiceInfo represents information about a discovered service
type ServiceInfo struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels"`
	Ports       []PortInfo        `json:"ports"`
	ClusterIP   string            `json:"cluster_ip"`
	ServiceType string            `json:"service_type"`
}

// PortInfo represents information about a service port
type PortInfo struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"target_port"`
	Protocol   string `json:"protocol"`
}

// serviceToInfo converts a Kubernetes service to ServiceInfo
func (s *ServiceDiscovery) serviceToInfo(service *corev1.Service) ServiceInfo {
	var ports []PortInfo
	for _, p := range service.Spec.Ports {
		ports = append(ports, PortInfo{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: p.TargetPort.String(),
			Protocol:   string(p.Protocol),
		})
	}

	return ServiceInfo{
		Name:        service.Name,
		Namespace:   service.Namespace,
		Labels:      service.Labels,
		Ports:       ports,
		ClusterIP:   service.Spec.ClusterIP,
		ServiceType: string(service.Spec.Type),
	}
}
