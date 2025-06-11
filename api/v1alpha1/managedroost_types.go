package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json:"-" or json:"fieldName,omitempty"

// ManagedRoostSpec defines the desired state of ManagedRoost
type ManagedRoostSpec struct {
	// Chart specifies the Helm chart configuration
	// +kubebuilder:validation:Required
	Chart ChartSpec `json:"chart"`

	// HealthChecks defines the health check configuration
	// +kubebuilder:validation:Optional
	HealthChecks []HealthCheckSpec `json:"healthChecks,omitempty"`

	// TeardownPolicy defines when and how to teardown roosts
	// +kubebuilder:validation:Optional
	TeardownPolicy *TeardownPolicySpec `json:"teardownPolicy,omitempty"`

	// Tenancy defines multi-tenant configuration
	// +kubebuilder:validation:Optional
	Tenancy *TenancySpec `json:"tenancy,omitempty"`

	// Observability defines observability configuration
	// +kubebuilder:validation:Optional
	Observability *ObservabilitySpec `json:"observability,omitempty"`

	// Metadata for operational tracking
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`

	// Namespace specifies the target namespace for the Helm deployment
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// ChartSpec defines Helm chart configuration
type ChartSpec struct {
	// Repository configuration
	// +kubebuilder:validation:Required
	Repository ChartRepositorySpec `json:"repository"`

	// Chart name
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	Name string `json:"name"`

	// Chart version or version constraint
	// +kubebuilder:validation:Required
	Version string `json:"version"`

	// Values override for the chart
	// +kubebuilder:validation:Optional
	Values *ChartValuesSpec `json:"values,omitempty"`

	// Helm upgrade configuration
	// +kubebuilder:validation:Optional
	UpgradePolicy *UpgradePolicySpec `json:"upgradePolicy,omitempty"`

	// Helm hooks configuration
	// +kubebuilder:validation:Optional
	Hooks *HookPolicySpec `json:"hooks,omitempty"`
}

// ChartRepositorySpec defines chart repository access
type ChartRepositorySpec struct {
	// Repository URL
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^(https?|oci|git\\+https)://.*$"
	URL string `json:"url"`

	// Repository type
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=http;oci;git
	// +kubebuilder:default=http
	Type string `json:"type,omitempty"`

	// Authentication for private repositories
	// +kubebuilder:validation:Optional
	Auth *RepositoryAuthSpec `json:"auth,omitempty"`

	// TLS configuration
	// +kubebuilder:validation:Optional
	TLS *RepositoryTLSSpec `json:"tls,omitempty"`
}

// RepositoryAuthSpec defines repository authentication
type RepositoryAuthSpec struct {
	// Secret containing authentication credentials
	// +kubebuilder:validation:Optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Username for basic auth
	// +kubebuilder:validation:Optional
	Username string `json:"username,omitempty"`

	// Password for basic auth
	// +kubebuilder:validation:Optional
	Password string `json:"password,omitempty"`

	// Token for token-based authentication (OCI registries)
	// +kubebuilder:validation:Optional
	Token string `json:"token,omitempty"`

	// Authentication type for OCI registries
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=basic;token;docker-config
	// +kubebuilder:default=basic
	Type string `json:"type,omitempty"`
}

// RepositoryTLSSpec defines TLS configuration for repository access
type RepositoryTLSSpec struct {
	// Skip TLS verification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// CA certificate bundle
	// +kubebuilder:validation:Optional
	CABundle []byte `json:"caBundle,omitempty"`

	// Secret containing TLS certificates
	// +kubebuilder:validation:Optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// ChartValuesSpec defines values override configuration
type ChartValuesSpec struct {
	// Inline values (YAML string)
	// +kubebuilder:validation:Optional
	Inline string `json:"inline,omitempty"`

	// Values from ConfigMaps
	// +kubebuilder:validation:Optional
	ConfigMapRefs []ValueSourceRef `json:"configMapRefs,omitempty"`

	// Values from Secrets
	// +kubebuilder:validation:Optional
	SecretRefs []ValueSourceRef `json:"secretRefs,omitempty"`

	// Template processing configuration
	// +kubebuilder:validation:Optional
	Template *ValueTemplateSpec `json:"template,omitempty"`
}

// ValueSourceRef defines a reference to a ConfigMap or Secret for values
type ValueSourceRef struct {
	// Name of the ConfigMap or Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Key within the ConfigMap or Secret
	// +kubebuilder:validation:Optional
	Key string `json:"key,omitempty"`

	// Namespace of the ConfigMap or Secret
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`
}

// ValueTemplateSpec defines template processing for values
type ValueTemplateSpec struct {
	// Enable template processing
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Template context variables (key-value pairs as strings)
	// +kubebuilder:validation:Optional
	Context map[string]string `json:"context,omitempty"`
}

// UpgradePolicySpec defines Helm upgrade behavior
type UpgradePolicySpec struct {
	// Force upgrade even if resources are unchanged
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Force bool `json:"force,omitempty"`

	// Timeout for upgrade operations
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="300s"
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Enable atomic upgrades
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Atomic bool `json:"atomic,omitempty"`

	// Cleanup on failure
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`
}

// HookPolicySpec defines Helm hooks configuration
type HookPolicySpec struct {
	// Enable pre-install hooks
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PreInstall bool `json:"preInstall,omitempty"`

	// Enable post-install hooks
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PostInstall bool `json:"postInstall,omitempty"`

	// Enable pre-upgrade hooks
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PreUpgrade bool `json:"preUpgrade,omitempty"`

	// Enable post-upgrade hooks
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	PostUpgrade bool `json:"postUpgrade,omitempty"`

	// Hook timeout
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="60s"
	Timeout metav1.Duration `json:"timeout,omitempty"`
}

// HealthCheckSpec defines health monitoring configuration
type HealthCheckSpec struct {
	// Health check name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Health check type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=http;tcp;udp;grpc;prometheus;kubernetes
	Type string `json:"type"`

	// HTTP health check configuration
	// +kubebuilder:validation:Optional
	HTTP *HTTPHealthCheckSpec `json:"http,omitempty"`

	// TCP health check configuration
	// +kubebuilder:validation:Optional
	TCP *TCPHealthCheckSpec `json:"tcp,omitempty"`

	// UDP health check configuration
	// +kubebuilder:validation:Optional
	UDP *UDPHealthCheckSpec `json:"udp,omitempty"`

	// gRPC health check configuration
	// +kubebuilder:validation:Optional
	GRPC *GRPCHealthCheckSpec `json:"grpc,omitempty"`

	// Prometheus health check configuration
	// +kubebuilder:validation:Optional
	Prometheus *PrometheusHealthCheckSpec `json:"prometheus,omitempty"`

	// Kubernetes native health check configuration
	// +kubebuilder:validation:Optional
	Kubernetes *KubernetesHealthCheckSpec `json:"kubernetes,omitempty"`

	// Check interval
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	Interval metav1.Duration `json:"interval,omitempty"`

	// Timeout for individual checks
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="10s"
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Number of consecutive failures before marking unhealthy
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=3
	FailureThreshold int32 `json:"failureThreshold,omitempty"`

	// Weight for composite health evaluation
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=1
	Weight int32 `json:"weight,omitempty"`
}

// HTTPHealthCheckSpec defines HTTP-based health checks
type HTTPHealthCheckSpec struct {
	// Target URL or URL template
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// HTTP method
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=GET;POST;PUT;HEAD
	// +kubebuilder:default=GET
	Method string `json:"method,omitempty"`

	// Expected status codes
	// +kubebuilder:validation:Optional
	ExpectedCodes []int32 `json:"expectedCodes,omitempty"`

	// Expected response body pattern
	// +kubebuilder:validation:Optional
	ExpectedBody string `json:"expectedBody,omitempty"`

	// HTTP headers
	// +kubebuilder:validation:Optional
	Headers map[string]string `json:"headers,omitempty"`

	// TLS configuration
	// +kubebuilder:validation:Optional
	TLS *HTTPTLSSpec `json:"tls,omitempty"`
}

// TCPHealthCheckSpec defines TCP-based health checks
type TCPHealthCheckSpec struct {
	// Target host
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Target port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Optional data to send for protocol validation
	// +kubebuilder:validation:Optional
	SendData string `json:"sendData,omitempty"`

	// Expected response for protocol validation
	// +kubebuilder:validation:Optional
	ExpectedResponse string `json:"expectedResponse,omitempty"`

	// Connection timeout (overrides general timeout for connection phase)
	// +kubebuilder:validation:Optional
	ConnectionTimeout *metav1.Duration `json:"connectionTimeout,omitempty"`

	// Enable connection pooling for performance
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	EnablePooling bool `json:"enablePooling,omitempty"`
}

// UDPHealthCheckSpec defines UDP-based health checks
type UDPHealthCheckSpec struct {
	// Target host
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Target port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Data to send for UDP validation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="ping"
	SendData string `json:"sendData,omitempty"`

	// Expected response for UDP validation
	// +kubebuilder:validation:Optional
	ExpectedResponse string `json:"expectedResponse,omitempty"`

	// Read timeout for UDP response
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="5s"
	ReadTimeout *metav1.Duration `json:"readTimeout,omitempty"`

	// Number of retry attempts for UDP packets
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=3
	Retries int32 `json:"retries,omitempty"`
}

// GRPCHealthCheckSpec defines gRPC-based health checks
type GRPCHealthCheckSpec struct {
	// Target host
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Target port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Service name for health check (empty for overall server health)
	// +kubebuilder:validation:Optional
	Service string `json:"service,omitempty"`

	// TLS configuration
	// +kubebuilder:validation:Optional
	TLS *GRPCTLSSpec `json:"tls,omitempty"`

	// Authentication configuration
	// +kubebuilder:validation:Optional
	Auth *GRPCAuthSpec `json:"auth,omitempty"`

	// Connection pool configuration
	// +kubebuilder:validation:Optional
	ConnectionPool *GRPCConnectionPoolSpec `json:"connectionPool,omitempty"`

	// Retry policy configuration
	// +kubebuilder:validation:Optional
	RetryPolicy *GRPCRetryPolicySpec `json:"retryPolicy,omitempty"`

	// Enable streaming health checks (Watch method)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	EnableStreaming bool `json:"enableStreaming,omitempty"`

	// Load balancing configuration for multiple endpoints
	// +kubebuilder:validation:Optional
	LoadBalancing *GRPCLoadBalancingSpec `json:"loadBalancing,omitempty"`

	// Circuit breaker configuration
	// +kubebuilder:validation:Optional
	CircuitBreaker *GRPCCircuitBreakerSpec `json:"circuitBreaker,omitempty"`

	// Custom metadata to send with health check requests
	// +kubebuilder:validation:Optional
	Metadata map[string]string `json:"metadata,omitempty"`
}

// PrometheusHealthCheckSpec defines Prometheus-based health checks
type PrometheusHealthCheckSpec struct {
	// Prometheus query (supports templating with roost variables)
	// +kubebuilder:validation:Required
	Query string `json:"query"`

	// Expected threshold value (as string to support decimals)
	// +kubebuilder:validation:Required
	Threshold string `json:"threshold"`

	// Comparison operator
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=gt;gte;lt;lte;eq;ne;>;>=;<;<=;==;!=
	// +kubebuilder:default=gte
	Operator string `json:"operator,omitempty"`

	// Prometheus endpoint URL (explicit configuration)
	// +kubebuilder:validation:Optional
	Endpoint string `json:"endpoint,omitempty"`

	// Service discovery configuration for automatic Prometheus endpoint detection
	// +kubebuilder:validation:Optional
	ServiceDiscovery *PrometheusDiscoverySpec `json:"serviceDiscovery,omitempty"`

	// Authentication configuration for Prometheus access
	// +kubebuilder:validation:Optional
	Auth *PrometheusAuthSpec `json:"auth,omitempty"`

	// Query timeout (overrides health check timeout for query execution)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	QueryTimeout *metav1.Duration `json:"queryTimeout,omitempty"`

	// Cache TTL for query results
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="5m"
	CacheTTL *metav1.Duration `json:"cacheTTL,omitempty"`

	// Trend analysis configuration for time-series evaluation
	// +kubebuilder:validation:Optional
	TrendAnalysis *TrendAnalysisSpec `json:"trendAnalysis,omitempty"`

	// Custom labels to add to metrics and logs
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`
}

// PrometheusDiscoverySpec defines service discovery configuration for Prometheus endpoints
type PrometheusDiscoverySpec struct {
	// Kubernetes service name containing Prometheus
	// +kubebuilder:validation:Optional
	ServiceName string `json:"serviceName,omitempty"`

	// Kubernetes service namespace
	// +kubebuilder:validation:Optional
	ServiceNamespace string `json:"serviceNamespace,omitempty"`

	// Service port name or number
	// +kubebuilder:validation:Optional
	ServicePort string `json:"servicePort,omitempty"`

	// Label selector for service discovery
	// +kubebuilder:validation:Optional
	LabelSelector map[string]string `json:"labelSelector,omitempty"`

	// Path prefix for Prometheus API (e.g., "/prometheus")
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=""
	PathPrefix string `json:"pathPrefix,omitempty"`

	// Enable TLS for discovered endpoints
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	TLS bool `json:"tls,omitempty"`

	// Enable service discovery fallback to explicit endpoint
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	EnableFallback bool `json:"enableFallback,omitempty"`
}

// PrometheusAuthSpec defines authentication configuration for Prometheus access
type PrometheusAuthSpec struct {
	// Bearer token for authentication
	// +kubebuilder:validation:Optional
	BearerToken string `json:"bearerToken,omitempty"`

	// Basic authentication credentials
	// +kubebuilder:validation:Optional
	BasicAuth *BasicAuthSpec `json:"basicAuth,omitempty"`

	// Secret reference for authentication credentials
	// +kubebuilder:validation:Optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Custom headers for authentication
	// +kubebuilder:validation:Optional
	Headers map[string]string `json:"headers,omitempty"`

	// TLS client certificate for mutual TLS
	// +kubebuilder:validation:Optional
	ClientCertificateRef *SecretReference `json:"clientCertificateRef,omitempty"`
}

// TrendAnalysisSpec defines trend analysis configuration for time-series data
type TrendAnalysisSpec struct {
	// Enable trend analysis
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Time window for trend analysis
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="15m"
	TimeWindow metav1.Duration `json:"timeWindow,omitempty"`

	// Threshold for trend improvement detection (percentage)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=10
	ImprovementThreshold int32 `json:"improvementThreshold,omitempty"`

	// Threshold for trend degradation detection (percentage)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=20
	DegradationThreshold int32 `json:"degradationThreshold,omitempty"`

	// Consider improving trends as healthy even if threshold fails
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	AllowImprovingUnhealthy bool `json:"allowImprovingUnhealthy,omitempty"`
}

// KubernetesHealthCheckSpec defines Kubernetes native health checks for Helm-deployed resources
type KubernetesHealthCheckSpec struct {
	// Enable pod health checking (readiness and liveness probes)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	CheckPods bool `json:"checkPods,omitempty"`

	// Enable deployment health checking (replica status and rollouts)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	CheckDeployments bool `json:"checkDeployments,omitempty"`

	// Enable service endpoint checking (endpoint availability)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	CheckServices bool `json:"checkServices,omitempty"`

	// Enable resource dependency checking (PVCs, ConfigMaps, Secrets)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CheckDependencies bool `json:"checkDependencies,omitempty"`

	// Enable StatefulSet health checking
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CheckStatefulSets bool `json:"checkStatefulSets,omitempty"`

	// Enable DaemonSet health checking
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CheckDaemonSets bool `json:"checkDaemonSets,omitempty"`

	// Custom label selector (combined with Helm release labels)
	// +kubebuilder:validation:Optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`

	// Required ready ratio for pods (0.0-1.0, default: 1.0 = all pods must be ready)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=1
	// +kubebuilder:default=1.0
	RequiredReadyRatio *float64 `json:"requiredReadyRatio,omitempty"`

	// Timeout for individual Kubernetes API calls
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="5s"
	APITimeout *metav1.Duration `json:"apiTimeout,omitempty"`

	// Enable resource caching with watch-based updates
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	EnableCaching *bool `json:"enableCaching,omitempty"`

	// Cache TTL for health check results
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	CacheTTL *metav1.Duration `json:"cacheTTL,omitempty"`

	// Include pod restart count in health assessment
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CheckPodRestarts bool `json:"checkPodRestarts,omitempty"`

	// Maximum allowed pod restart count before marking unhealthy
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=5
	MaxPodRestarts *int32 `json:"maxPodRestarts,omitempty"`

	// Check resource requests and limits compliance
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CheckResourceLimits bool `json:"checkResourceLimits,omitempty"`

	// Include node health in assessment (checks if nodes are ready)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	CheckNodeHealth bool `json:"checkNodeHealth,omitempty"`

	// Custom resource types to check (in addition to standard types)
	// +kubebuilder:validation:Optional
	CustomResources []KubernetesCustomResourceCheck `json:"customResources,omitempty"`
}

// KubernetesCustomResourceCheck defines a custom resource type to monitor
type KubernetesCustomResourceCheck struct {
	// API version of the custom resource
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the custom resource
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Health assessment strategy for this custom resource
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=exists;conditions;status
	// +kubebuilder:default=exists
	HealthStrategy string `json:"healthStrategy,omitempty"`

	// Expected conditions for the custom resource (when using 'conditions' strategy)
	// +kubebuilder:validation:Optional
	ExpectedConditions []ExpectedCondition `json:"expectedConditions,omitempty"`

	// JSONPath expression to extract health status (when using 'status' strategy)
	// +kubebuilder:validation:Optional
	StatusPath string `json:"statusPath,omitempty"`

	// Expected status value (when using 'status' strategy)
	// +kubebuilder:validation:Optional
	ExpectedStatus string `json:"expectedStatus,omitempty"`
}

// ExpectedCondition defines an expected condition for a Kubernetes resource
type ExpectedCondition struct {
	// Condition type
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// Expected status
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status string `json:"status"`

	// Optional reason for the condition
	// +kubebuilder:validation:Optional
	Reason string `json:"reason,omitempty"`
}

// HTTPTLSSpec defines TLS configuration for HTTP health checks
type HTTPTLSSpec struct {
	// Skip TLS verification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// GRPCTLSSpec defines TLS configuration for gRPC health checks
type GRPCTLSSpec struct {
	// Enable TLS
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Skip TLS verification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// Server name for TLS verification
	// +kubebuilder:validation:Optional
	ServerName string `json:"serverName,omitempty"`

	// CA certificate bundle for verification
	// +kubebuilder:validation:Optional
	CABundle []byte `json:"caBundle,omitempty"`

	// Secret reference for TLS certificates
	// +kubebuilder:validation:Optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Client certificate for mutual TLS
	// +kubebuilder:validation:Optional
	ClientCertificateRef *SecretReference `json:"clientCertificateRef,omitempty"`
}

// GRPCAuthSpec defines authentication configuration for gRPC health checks
type GRPCAuthSpec struct {
	// JWT token for authentication
	// +kubebuilder:validation:Optional
	JWT string `json:"jwt,omitempty"`

	// Bearer token for authentication
	// +kubebuilder:validation:Optional
	BearerToken string `json:"bearerToken,omitempty"`

	// Custom authentication headers
	// +kubebuilder:validation:Optional
	Headers map[string]string `json:"headers,omitempty"`

	// OAuth2 configuration
	// +kubebuilder:validation:Optional
	OAuth2 *GRPCOAuth2Spec `json:"oauth2,omitempty"`

	// Basic authentication
	// +kubebuilder:validation:Optional
	BasicAuth *BasicAuthSpec `json:"basicAuth,omitempty"`
}

// GRPCOAuth2Spec defines OAuth2 authentication configuration
type GRPCOAuth2Spec struct {
	// OAuth2 token endpoint
	// +kubebuilder:validation:Required
	TokenURL string `json:"tokenURL"`

	// OAuth2 client ID
	// +kubebuilder:validation:Required
	ClientID string `json:"clientID"`

	// OAuth2 client secret
	// +kubebuilder:validation:Required
	ClientSecret string `json:"clientSecret"`

	// OAuth2 scopes
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`
}

// GRPCConnectionPoolSpec defines connection pool configuration
type GRPCConnectionPoolSpec struct {
	// Enable connection pooling
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Maximum number of connections per target
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=10
	MaxConnections int32 `json:"maxConnections,omitempty"`

	// Maximum connection idle time
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="60s"
	MaxIdleTime metav1.Duration `json:"maxIdleTime,omitempty"`

	// Connection keep-alive time
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	KeepAliveTime metav1.Duration `json:"keepAliveTime,omitempty"`

	// Connection keep-alive timeout
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="5s"
	KeepAliveTimeout metav1.Duration `json:"keepAliveTimeout,omitempty"`

	// Connection health check interval
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	HealthCheckInterval metav1.Duration `json:"healthCheckInterval,omitempty"`
}

// GRPCRetryPolicySpec defines retry policy configuration
type GRPCRetryPolicySpec struct {
	// Enable retry policy
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Maximum number of retry attempts
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=3
	MaxAttempts int32 `json:"maxAttempts,omitempty"`

	// Initial retry delay
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="1s"
	InitialDelay metav1.Duration `json:"initialDelay,omitempty"`

	// Maximum retry delay
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	MaxDelay metav1.Duration `json:"maxDelay,omitempty"`

	// Retry delay multiplier
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="2.0"
	Multiplier string `json:"multiplier,omitempty"`

	// Retryable gRPC status codes
	// +kubebuilder:validation:Optional
	RetryableStatusCodes []string `json:"retryableStatusCodes,omitempty"`
}

// GRPCLoadBalancingSpec defines load balancing configuration
type GRPCLoadBalancingSpec struct {
	// Enable load balancing
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Load balancing policy
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=round_robin;pick_first;grpclb;xds
	// +kubebuilder:default=round_robin
	Policy string `json:"policy,omitempty"`

	// Alternative endpoints for load balancing
	// +kubebuilder:validation:Optional
	Endpoints []GRPCEndpointSpec `json:"endpoints,omitempty"`
}

// GRPCEndpointSpec defines a gRPC endpoint
type GRPCEndpointSpec struct {
	// Endpoint host
	// +kubebuilder:validation:Required
	Host string `json:"host"`

	// Endpoint port
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Endpoint weight for load balancing
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=1
	Weight int32 `json:"weight,omitempty"`
}

// GRPCCircuitBreakerSpec defines circuit breaker configuration
type GRPCCircuitBreakerSpec struct {
	// Enable circuit breaker
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Failure threshold to open circuit
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=5
	FailureThreshold int32 `json:"failureThreshold,omitempty"`

	// Success threshold to close circuit
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=3
	SuccessThreshold int32 `json:"successThreshold,omitempty"`

	// Timeout for half-open state
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="60s"
	HalfOpenTimeout metav1.Duration `json:"halfOpenTimeout,omitempty"`

	// Recovery timeout before attempting to close circuit
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	RecoveryTimeout metav1.Duration `json:"recoveryTimeout,omitempty"`
}

// TeardownPolicySpec defines when and how to teardown roosts
type TeardownPolicySpec struct {
	// Automatic teardown triggers
	// +kubebuilder:validation:Optional
	Triggers []TeardownTriggerSpec `json:"triggers,omitempty"`

	// Manual teardown protection
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	RequireManualApproval bool `json:"requireManualApproval,omitempty"`

	// Data preservation policy
	// +kubebuilder:validation:Optional
	DataPreservation *DataPreservationSpec `json:"dataPreservation,omitempty"`

	// Cleanup configuration
	// +kubebuilder:validation:Optional
	Cleanup *CleanupSpec `json:"cleanup,omitempty"`
}

// TeardownTriggerSpec defines teardown trigger conditions
type TeardownTriggerSpec struct {
	// Trigger type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=timeout;failure_count;resource_threshold;schedule;webhook
	Type string `json:"type"`

	// Timeout-based trigger
	// +kubebuilder:validation:Optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Failure count trigger
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=1
	FailureCount *int32 `json:"failureCount,omitempty"`

	// Resource threshold trigger
	// +kubebuilder:validation:Optional
	ResourceThreshold *ResourceThresholdSpec `json:"resourceThreshold,omitempty"`

	// Schedule-based trigger (cron expression)
	// +kubebuilder:validation:Optional
	Schedule string `json:"schedule,omitempty"`

	// Webhook-based trigger
	// +kubebuilder:validation:Optional
	Webhook *WebhookTriggerSpec `json:"webhook,omitempty"`
}

// ResourceThresholdSpec defines resource usage thresholds for teardown
type ResourceThresholdSpec struct {
	// Memory threshold
	// +kubebuilder:validation:Optional
	Memory *intstr.IntOrString `json:"memory,omitempty"`

	// CPU threshold
	// +kubebuilder:validation:Optional
	CPU *intstr.IntOrString `json:"cpu,omitempty"`

	// Storage threshold
	// +kubebuilder:validation:Optional
	Storage *intstr.IntOrString `json:"storage,omitempty"`

	// Custom resource thresholds
	// +kubebuilder:validation:Optional
	Custom map[string]intstr.IntOrString `json:"custom,omitempty"`
}

// WebhookTriggerSpec defines webhook-based teardown triggers
type WebhookTriggerSpec struct {
	// Webhook URL
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// HTTP method
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=GET;POST;PUT
	// +kubebuilder:default=POST
	Method string `json:"method,omitempty"`

	// HTTP headers
	// +kubebuilder:validation:Optional
	Headers map[string]string `json:"headers,omitempty"`

	// Authentication configuration
	// +kubebuilder:validation:Optional
	Auth *WebhookAuthSpec `json:"auth,omitempty"`
}

// WebhookAuthSpec defines webhook authentication
type WebhookAuthSpec struct {
	// Bearer token
	// +kubebuilder:validation:Optional
	BearerToken string `json:"bearerToken,omitempty"`

	// Basic auth credentials
	// +kubebuilder:validation:Optional
	BasicAuth *BasicAuthSpec `json:"basicAuth,omitempty"`
}

// BasicAuthSpec defines basic authentication credentials
type BasicAuthSpec struct {
	// Username
	// +kubebuilder:validation:Required
	Username string `json:"username"`

	// Password
	// +kubebuilder:validation:Required
	Password string `json:"password"`
}

// DataPreservationSpec defines data preservation policies
type DataPreservationSpec struct {
	// Enable data preservation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Backup policy configuration
	// +kubebuilder:validation:Optional
	BackupPolicy *BackupPolicySpec `json:"backupPolicy,omitempty"`

	// Resources to preserve
	// +kubebuilder:validation:Optional
	PreserveResources []string `json:"preserveResources,omitempty"`
}

// BackupPolicySpec defines backup policies for data preservation
type BackupPolicySpec struct {
	// Backup schedule (cron expression)
	// +kubebuilder:validation:Optional
	Schedule string `json:"schedule,omitempty"`

	// Backup retention period
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="7d"
	Retention string `json:"retention,omitempty"`

	// Backup storage configuration
	// +kubebuilder:validation:Optional
	Storage *BackupStorageSpec `json:"storage,omitempty"`
}

// BackupStorageSpec defines backup storage configuration
type BackupStorageSpec struct {
	// Storage type
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=s3;gcs;azure;local
	Type string `json:"type,omitempty"`

	// Storage configuration
	// +kubebuilder:validation:Optional
	Config map[string]string `json:"config,omitempty"`
}

// CleanupSpec defines cleanup configuration
type CleanupSpec struct {
	// Grace period for resource cleanup
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	GracePeriod metav1.Duration `json:"gracePeriod,omitempty"`

	// Force cleanup if grace period expires
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Force bool `json:"force,omitempty"`

	// Cleanup order for resources
	// +kubebuilder:validation:Optional
	Order []string `json:"order,omitempty"`
}

// TenancySpec defines multi-tenant configuration
type TenancySpec struct {
	// Tenant identifier
	// +kubebuilder:validation:Optional
	TenantID string `json:"tenantId,omitempty"`

	// Namespace isolation configuration
	// +kubebuilder:validation:Optional
	NamespaceIsolation *NamespaceIsolationSpec `json:"namespaceIsolation,omitempty"`

	// RBAC configuration
	// +kubebuilder:validation:Optional
	RBAC *RBACSpec `json:"rbac,omitempty"`

	// Network policies
	// +kubebuilder:validation:Optional
	NetworkPolicy *NetworkPolicySpec `json:"networkPolicy,omitempty"`

	// Resource quotas
	// +kubebuilder:validation:Optional
	ResourceQuota *ResourceQuotaSpec `json:"resourceQuota,omitempty"`
}

// NamespaceIsolationSpec defines namespace isolation configuration
type NamespaceIsolationSpec struct {
	// Enable namespace isolation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Namespace prefix for tenant resources
	// +kubebuilder:validation:Optional
	Prefix string `json:"prefix,omitempty"`

	// Additional namespace labels
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`

	// Additional namespace annotations
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// RBACSpec defines advanced RBAC configuration with enterprise features
type RBACSpec struct {
	// Enable RBAC
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Tenant identifier for scoped permissions
	// +kubebuilder:validation:Required
	TenantID string `json:"tenantId"`

	// Identity provider configuration
	// +kubebuilder:validation:Optional
	IdentityProvider string `json:"identityProvider,omitempty"`

	// Identity provider configuration details
	// +kubebuilder:validation:Optional
	IdentityProviderConfig *IdentityProviderConfig `json:"identityProviderConfig,omitempty"`

	// Service accounts to create and manage
	// +kubebuilder:validation:Optional
	ServiceAccounts []ServiceAccountSpec `json:"serviceAccounts,omitempty"`

	// Roles to create from templates or explicit rules
	// +kubebuilder:validation:Optional
	Roles []RoleSpec `json:"roles,omitempty"`

	// Role bindings to create
	// +kubebuilder:validation:Optional
	RoleBindings []RoleBindingSpec `json:"roleBindings,omitempty"`

	// Policy templates to use for dynamic generation
	// +kubebuilder:validation:Optional
	PolicyTemplates []string `json:"policyTemplates,omitempty"`

	// Template parameters for policy substitution
	// +kubebuilder:validation:Optional
	TemplateParameters map[string]string `json:"templateParameters,omitempty"`

	// Audit configuration
	// +kubebuilder:validation:Optional
	Audit *AuditConfig `json:"audit,omitempty"`

	// Policy validation settings
	// +kubebuilder:validation:Optional
	PolicyValidation *PolicyValidationConfig `json:"policyValidation,omitempty"`
}

// IdentityProviderConfig defines identity provider integration settings
type IdentityProviderConfig struct {
	// OIDC configuration
	// +kubebuilder:validation:Optional
	OIDC *OIDCConfig `json:"oidc,omitempty"`

	// LDAP configuration
	// +kubebuilder:validation:Optional
	LDAP *LDAPConfig `json:"ldap,omitempty"`

	// Custom provider configuration
	// +kubebuilder:validation:Optional
	Custom map[string]string `json:"custom,omitempty"`
}

// OIDCConfig defines OpenID Connect provider configuration
type OIDCConfig struct {
	// OIDC issuer URL
	// +kubebuilder:validation:Required
	IssuerURL string `json:"issuerURL"`

	// Client ID for OIDC authentication
	// +kubebuilder:validation:Required
	ClientID string `json:"clientID"`

	// Client secret reference
	// +kubebuilder:validation:Optional
	ClientSecretRef *SecretReference `json:"clientSecretRef,omitempty"`

	// Additional scopes to request
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`

	// Claims mapping for user/group extraction
	// +kubebuilder:validation:Optional
	ClaimsMapping *OIDCClaimsMapping `json:"claimsMapping,omitempty"`

	// Token validation settings
	// +kubebuilder:validation:Optional
	TokenValidation *TokenValidationConfig `json:"tokenValidation,omitempty"`
}

// OIDCClaimsMapping defines how to extract user/group information from OIDC tokens
type OIDCClaimsMapping struct {
	// Username claim (default: "sub")
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="sub"
	Username string `json:"username,omitempty"`

	// Groups claim (default: "groups")
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="groups"
	Groups string `json:"groups,omitempty"`

	// Email claim (default: "email")
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="email"
	Email string `json:"email,omitempty"`

	// Display name claim (default: "name")
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="name"
	DisplayName string `json:"displayName,omitempty"`

	// Tenant claim for multi-tenant scenarios
	// +kubebuilder:validation:Optional
	Tenant string `json:"tenant,omitempty"`
}

// TokenValidationConfig defines token validation parameters
type TokenValidationConfig struct {
	// Skip token expiry validation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	SkipExpiryCheck bool `json:"skipExpiryCheck,omitempty"`

	// Skip issuer validation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	SkipIssuerCheck bool `json:"skipIssuerCheck,omitempty"`

	// Additional audience values to accept
	// +kubebuilder:validation:Optional
	Audiences []string `json:"audiences,omitempty"`

	// Clock skew tolerance for token validation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30s"
	ClockSkewTolerance metav1.Duration `json:"clockSkewTolerance,omitempty"`
}

// LDAPConfig defines LDAP provider configuration
type LDAPConfig struct {
	// LDAP server URL
	// +kubebuilder:validation:Required
	ServerURL string `json:"serverURL"`

	// Bind DN for LDAP authentication
	// +kubebuilder:validation:Required
	BindDN string `json:"bindDN"`

	// Bind password reference
	// +kubebuilder:validation:Required
	BindPasswordRef *SecretReference `json:"bindPasswordRef"`

	// Base DN for user searches
	// +kubebuilder:validation:Required
	UserBaseDN string `json:"userBaseDN"`

	// User search filter
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="(uid=%s)"
	UserFilter string `json:"userFilter,omitempty"`

	// Base DN for group searches
	// +kubebuilder:validation:Optional
	GroupBaseDN string `json:"groupBaseDN,omitempty"`

	// Group search filter
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="(member=%s)"
	GroupFilter string `json:"groupFilter,omitempty"`

	// TLS configuration
	// +kubebuilder:validation:Optional
	TLS *LDAPTLSConfig `json:"tls,omitempty"`
}

// LDAPTLSConfig defines LDAP TLS settings
type LDAPTLSConfig struct {
	// Enable TLS
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Skip TLS verification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// CA certificate bundle
	// +kubebuilder:validation:Optional
	CABundle []byte `json:"caBundle,omitempty"`
}

// ServiceAccountSpec defines a service account to create and manage
type ServiceAccountSpec struct {
	// Service account name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Annotations to add to the service account
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Image pull secrets to associate
	// +kubebuilder:validation:Optional
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`

	// Automount service account token
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	AutomountToken bool `json:"automountToken,omitempty"`

	// Lifecycle management settings
	// +kubebuilder:validation:Optional
	Lifecycle *ServiceAccountLifecycle `json:"lifecycle,omitempty"`
}

// ServiceAccountLifecycle defines service account lifecycle management
type ServiceAccountLifecycle struct {
	// Token rotation policy
	// +kubebuilder:validation:Optional
	TokenRotation *TokenRotationPolicy `json:"tokenRotation,omitempty"`

	// Cleanup policy when roost is deleted
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=delete;retain
	// +kubebuilder:default=delete
	CleanupPolicy string `json:"cleanupPolicy,omitempty"`
}

// TokenRotationPolicy defines automatic token rotation
type TokenRotationPolicy struct {
	// Enable automatic token rotation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Rotation interval
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="24h"
	RotationInterval metav1.Duration `json:"rotationInterval,omitempty"`

	// Overlap period for graceful rotation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="2h"
	OverlapPeriod metav1.Duration `json:"overlapPeriod,omitempty"`
}

// RoleSpec defines a role to create
type RoleSpec struct {
	// Role name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Policy template to use for role generation
	// +kubebuilder:validation:Optional
	Template string `json:"template,omitempty"`

	// Explicit policy rules (used if template is not specified)
	// +kubebuilder:validation:Optional
	Rules []rbacv1.PolicyRule `json:"rules,omitempty"`

	// Role type (Role or ClusterRole)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=Role;ClusterRole
	// +kubebuilder:default=Role
	Type string `json:"type,omitempty"`

	// Template parameters for this role
	// +kubebuilder:validation:Optional
	TemplateParameters map[string]string `json:"templateParameters,omitempty"`

	// Role annotations
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// RoleBindingSpec defines a role binding to create
type RoleBindingSpec struct {
	// Role binding name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Subjects for the role binding
	// +kubebuilder:validation:Required
	Subjects []SubjectSpec `json:"subjects"`

	// Role reference
	// +kubebuilder:validation:Required
	RoleRef RoleRefSpec `json:"roleRef"`

	// Role binding type (RoleBinding or ClusterRoleBinding)
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=RoleBinding;ClusterRoleBinding
	// +kubebuilder:default=RoleBinding
	Type string `json:"type,omitempty"`

	// Role binding annotations
	// +kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// SubjectSpec defines a subject for role bindings
type SubjectSpec struct {
	// Subject kind
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=User;Group;ServiceAccount
	Kind string `json:"kind"`

	// Subject name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace for ServiceAccount subjects
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`

	// Identity provider validation
	// +kubebuilder:validation:Optional
	ValidateWithProvider bool `json:"validateWithProvider,omitempty"`
}

// RoleRefSpec defines a role reference
type RoleRefSpec struct {
	// Role kind
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Role;ClusterRole
	Kind string `json:"kind"`

	// Role name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// API group (default: rbac.authorization.k8s.io)
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="rbac.authorization.k8s.io"
	APIGroup string `json:"apiGroup,omitempty"`
}

// AuditConfig defines audit logging configuration
type AuditConfig struct {
	// Enable audit logging
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Audit events to log
	// +kubebuilder:validation:Optional
	Events []string `json:"events,omitempty"`

	// Audit log retention period
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="30d"
	RetentionPeriod string `json:"retentionPeriod,omitempty"`

	// External audit webhook
	// +kubebuilder:validation:Optional
	WebhookURL string `json:"webhookURL,omitempty"`

	// Audit log level
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=minimal;standard;detailed
	// +kubebuilder:default=standard
	Level string `json:"level,omitempty"`
}

// PolicyValidationConfig defines policy validation settings
type PolicyValidationConfig struct {
	// Enable policy validation
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Validation mode
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=strict;warn;disabled
	// +kubebuilder:default=strict
	Mode string `json:"mode,omitempty"`

	// Check for least privilege violations
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	CheckLeastPrivilege bool `json:"checkLeastPrivilege,omitempty"`

	// Check for privilege escalation risks
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	CheckPrivilegeEscalation bool `json:"checkPrivilegeEscalation,omitempty"`

	// Custom validation rules
	// +kubebuilder:validation:Optional
	CustomRules []ValidationRule `json:"customRules,omitempty"`
}

// ValidationRule defines a custom policy validation rule
type ValidationRule struct {
	// Rule name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Rule description
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Rule expression (CEL or regex)
	// +kubebuilder:validation:Required
	Expression string `json:"expression"`

	// Rule severity
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=error;warning;info
	// +kubebuilder:default=warning
	Severity string `json:"severity,omitempty"`
}

// NetworkPolicySpec defines network policy configuration
type NetworkPolicySpec struct {
	// Enable network policies
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Ingress rules
	// +kubebuilder:validation:Optional
	Ingress []NetworkPolicyRule `json:"ingress,omitempty"`

	// Egress rules
	// +kubebuilder:validation:Optional
	Egress []NetworkPolicyRule `json:"egress,omitempty"`
}

// NetworkPolicyRule defines a network policy rule
type NetworkPolicyRule struct {
	// Allowed sources/destinations
	// +kubebuilder:validation:Optional
	From []NetworkPolicyPeer `json:"from,omitempty"`

	// Allowed destinations/sources
	// +kubebuilder:validation:Optional
	To []NetworkPolicyPeer `json:"to,omitempty"`

	// Allowed ports
	// +kubebuilder:validation:Optional
	Ports []NetworkPolicyPort `json:"ports,omitempty"`
}

// NetworkPolicyPeer defines a network policy peer
type NetworkPolicyPeer struct {
	// Pod selector
	// +kubebuilder:validation:Optional
	PodSelector map[string]string `json:"podSelector,omitempty"`

	// Namespace selector
	// +kubebuilder:validation:Optional
	NamespaceSelector map[string]string `json:"namespaceSelector,omitempty"`
}

// NetworkPolicyPort defines a network policy port
type NetworkPolicyPort struct {
	// Protocol
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +kubebuilder:default=TCP
	Protocol string `json:"protocol,omitempty"`

	// Port number or name
	// +kubebuilder:validation:Optional
	Port *intstr.IntOrString `json:"port,omitempty"`
}

// ResourceQuotaSpec defines resource quota configuration
type ResourceQuotaSpec struct {
	// Enable resource quotas
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Hard resource limits
	// +kubebuilder:validation:Optional
	Hard map[string]intstr.IntOrString `json:"hard,omitempty"`

	// Scopes for resource quota
	// +kubebuilder:validation:Optional
	Scopes []string `json:"scopes,omitempty"`
}

// ObservabilitySpec defines observability configuration
type ObservabilitySpec struct {
	// Metrics configuration
	// +kubebuilder:validation:Optional
	Metrics *MetricsSpec `json:"metrics,omitempty"`

	// Tracing configuration
	// +kubebuilder:validation:Optional
	Tracing *TracingSpec `json:"tracing,omitempty"`

	// Logging configuration
	// +kubebuilder:validation:Optional
	Logging *LoggingSpec `json:"logging,omitempty"`
}

// MetricsSpec defines metrics collection configuration
type MetricsSpec struct {
	// Enable metrics collection
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Metrics collection interval
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="15s"
	Interval metav1.Duration `json:"interval,omitempty"`

	// Custom metrics to collect
	// +kubebuilder:validation:Optional
	Custom []CustomMetricSpec `json:"custom,omitempty"`
}

// TracingSpec defines distributed tracing configuration
type TracingSpec struct {
	// Enable distributed tracing
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Sampling rate for traces (as string, e.g., "0.1")
	// +kubebuilder:validation:Optional
	// +kubebuilder:default="0.1"
	SamplingRate string `json:"samplingRate,omitempty"`

	// Tracing endpoint configuration
	// +kubebuilder:validation:Optional
	Endpoint *TracingEndpointSpec `json:"endpoint,omitempty"`
}

// LoggingSpec defines logging configuration
type LoggingSpec struct {
	// Logging level
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=debug;info;warn;error
	// +kubebuilder:default=info
	Level string `json:"level,omitempty"`

	// Enable structured logging
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=true
	Structured bool `json:"structured,omitempty"`

	// Additional log outputs
	// +kubebuilder:validation:Optional
	Outputs []LogOutputSpec `json:"outputs,omitempty"`
}

// CustomMetricSpec defines a custom metric to collect
type CustomMetricSpec struct {
	// Metric name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Metric query or source
	// +kubebuilder:validation:Required
	Query string `json:"query"`

	// Metric type
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=counter;gauge;histogram;summary
	// +kubebuilder:default=gauge
	Type string `json:"type,omitempty"`
}

// TracingEndpointSpec defines tracing endpoint configuration
type TracingEndpointSpec struct {
	// Endpoint URL
	// +kubebuilder:validation:Required
	URL string `json:"url"`

	// Authentication configuration
	// +kubebuilder:validation:Optional
	Auth *TracingAuthSpec `json:"auth,omitempty"`
}

// TracingAuthSpec defines tracing authentication
type TracingAuthSpec struct {
	// Bearer token
	// +kubebuilder:validation:Optional
	BearerToken string `json:"bearerToken,omitempty"`

	// Headers for authentication
	// +kubebuilder:validation:Optional
	Headers map[string]string `json:"headers,omitempty"`
}

// LogOutputSpec defines additional log output configuration
type LogOutputSpec struct {
	// Output type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=file;syslog;http;kafka
	Type string `json:"type"`

	// Output configuration
	// +kubebuilder:validation:Optional
	Config map[string]string `json:"config,omitempty"`
}

// SecretReference defines a reference to a Secret
type SecretReference struct {
	// Name of the Secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the Secret
	// +kubebuilder:validation:Optional
	Namespace string `json:"namespace,omitempty"`

	// Key within the Secret
	// +kubebuilder:validation:Optional
	Key string `json:"key,omitempty"`
}

// ManagedRoostStatus defines the observed state of ManagedRoost
type ManagedRoostStatus struct {
	// Phase represents the current phase of the ManagedRoost
	// +kubebuilder:validation:Optional
	Phase ManagedRoostPhase `json:"phase,omitempty"`

	// Helm release information
	// +kubebuilder:validation:Optional
	HelmRelease *HelmReleaseStatus `json:"helmRelease,omitempty"`

	// Health check results
	// +kubebuilder:validation:Optional
	HealthChecks []HealthCheckStatus `json:"healthChecks,omitempty"`

	// Overall health status
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=healthy;unhealthy;degraded;unknown
	Health string `json:"health,omitempty"`

	// Teardown status
	// +kubebuilder:validation:Optional
	Teardown *TeardownStatus `json:"teardown,omitempty"`

	// Conditions represent the latest available observations
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Observability information
	// +kubebuilder:validation:Optional
	Observability *ObservabilityStatus `json:"observability,omitempty"`

	// Lifecycle tracking information
	// +kubebuilder:validation:Optional
	Lifecycle *LifecycleStatus `json:"lifecycle,omitempty"`

	// Last update timestamp
	// +kubebuilder:validation:Optional
	LastUpdateTime *metav1.Time `json:"lastUpdateTime,omitempty"`

	// Generation observed by the controller
	// +kubebuilder:validation:Optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Last reconcile time
	// +kubebuilder:validation:Optional
	LastReconcileTime metav1.Time `json:"lastReconcileTime,omitempty"`
}

// ManagedRoostPhase defines the phase of a ManagedRoost
type ManagedRoostPhase string

const (
	// ManagedRoostPhasePending indicates the ManagedRoost is pending
	ManagedRoostPhasePending ManagedRoostPhase = "Pending"
	// ManagedRoostPhaseDeploying indicates the ManagedRoost is being deployed
	ManagedRoostPhaseDeploying ManagedRoostPhase = "Deploying"
	// ManagedRoostPhaseReady indicates the ManagedRoost is ready
	ManagedRoostPhaseReady ManagedRoostPhase = "Ready"
	// ManagedRoostPhaseFailed indicates the ManagedRoost has failed
	ManagedRoostPhaseFailed ManagedRoostPhase = "Failed"
	// ManagedRoostPhaseTearingDown indicates the ManagedRoost is being torn down
	ManagedRoostPhaseTearingDown ManagedRoostPhase = "TearingDown"
)

// HelmReleaseStatus defines the status of a Helm release
type HelmReleaseStatus struct {
	// Release name
	// +kubebuilder:validation:Optional
	Name string `json:"name,omitempty"`

	// Release revision
	// +kubebuilder:validation:Optional
	Revision int32 `json:"revision,omitempty"`

	// Release status
	// +kubebuilder:validation:Optional
	Status string `json:"status,omitempty"`

	// Release description
	// +kubebuilder:validation:Optional
	Description string `json:"description,omitempty"`

	// Release notes
	// +kubebuilder:validation:Optional
	Notes string `json:"notes,omitempty"`

	// Last deployed time
	// +kubebuilder:validation:Optional
	LastDeployed *metav1.Time `json:"lastDeployed,omitempty"`
}

// HealthCheckStatus defines the status of a health check
type HealthCheckStatus struct {
	// Health check name
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Health check status
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=healthy;unhealthy;unknown
	Status string `json:"status,omitempty"`

	// Last check time
	// +kubebuilder:validation:Optional
	LastCheck *metav1.Time `json:"lastCheck,omitempty"`

	// Failure count
	// +kubebuilder:validation:Optional
	FailureCount int32 `json:"failureCount,omitempty"`

	// Error message
	// +kubebuilder:validation:Optional
	Message string `json:"message,omitempty"`
}

// TeardownStatus defines the status of teardown operations
type TeardownStatus struct {
	// Teardown triggered
	// +kubebuilder:validation:Optional
	Triggered bool `json:"triggered,omitempty"`

	// Trigger reason
	// +kubebuilder:validation:Optional
	TriggerReason string `json:"triggerReason,omitempty"`

	// Trigger time
	// +kubebuilder:validation:Optional
	TriggerTime *metav1.Time `json:"triggerTime,omitempty"`

	// Completion time
	// +kubebuilder:validation:Optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Progress percentage
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	Progress int32 `json:"progress,omitempty"`
}

// ObservabilityStatus defines the status of observability features
type ObservabilityStatus struct {
	// Metrics collection status
	// +kubebuilder:validation:Optional
	Metrics *MetricsStatus `json:"metrics,omitempty"`

	// Tracing status
	// +kubebuilder:validation:Optional
	Tracing *TracingStatus `json:"tracing,omitempty"`

	// Logging status
	// +kubebuilder:validation:Optional
	Logging *LoggingStatus `json:"logging,omitempty"`
}

// MetricsStatus defines metrics collection status
type MetricsStatus struct {
	// Metrics enabled
	// +kubebuilder:validation:Optional
	Enabled bool `json:"enabled,omitempty"`

	// Last collection time
	// +kubebuilder:validation:Optional
	LastCollection *metav1.Time `json:"lastCollection,omitempty"`

	// Collection errors
	// +kubebuilder:validation:Optional
	Errors []string `json:"errors,omitempty"`
}

// TracingStatus defines tracing status
type TracingStatus struct {
	// Tracing enabled
	// +kubebuilder:validation:Optional
	Enabled bool `json:"enabled,omitempty"`

	// Active spans count
	// +kubebuilder:validation:Optional
	ActiveSpans int32 `json:"activeSpans,omitempty"`

	// Endpoint status
	// +kubebuilder:validation:Optional
	EndpointStatus string `json:"endpointStatus,omitempty"`
}

// LoggingStatus defines logging status
type LoggingStatus struct {
	// Current log level
	// +kubebuilder:validation:Optional
	Level string `json:"level,omitempty"`

	// Structured logging enabled
	// +kubebuilder:validation:Optional
	Structured bool `json:"structured,omitempty"`

	// Output statuses
	// +kubebuilder:validation:Optional
	Outputs []LogOutputStatus `json:"outputs,omitempty"`
}

// LogOutputStatus defines log output status
type LogOutputStatus struct {
	// Output type
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// Output status
	// +kubebuilder:validation:Optional
	Status string `json:"status,omitempty"`

	// Error message
	// +kubebuilder:validation:Optional
	Error string `json:"error,omitempty"`
}

// LifecycleStatus extends the CRD with lifecycle tracking information
type LifecycleStatus struct {
	// Creation timestamp
	// +kubebuilder:validation:Optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// Last activity timestamp
	// +kubebuilder:validation:Optional
	LastActivity *metav1.Time `json:"lastActivity,omitempty"`

	// Lifecycle events
	// +kubebuilder:validation:Optional
	Events []LifecycleEventRecord `json:"events,omitempty"`

	// Event counters by type
	// +kubebuilder:validation:Optional
	EventCounts map[string]int32 `json:"eventCounts,omitempty"`

	// Phase durations
	// +kubebuilder:validation:Optional
	PhaseDurations map[string]metav1.Duration `json:"phaseDurations,omitempty"`
}

// LifecycleEventRecord represents a recorded lifecycle event
type LifecycleEventRecord struct {
	// Event type
	// +kubebuilder:validation:Required
	Type string `json:"type"`

	// Event timestamp
	// +kubebuilder:validation:Required
	Timestamp metav1.Time `json:"timestamp"`

	// Event message
	// +kubebuilder:validation:Required
	Message string `json:"message"`

	// Additional event details
	// +kubebuilder:validation:Optional
	Details map[string]string `json:"details,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Health",type="string",JSONPath=".status.health"
//+kubebuilder:printcolumn:name="Chart",type="string",JSONPath=".spec.chart.name"
//+kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.chart.version"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ManagedRoost is the Schema for the managedroosts API
type ManagedRoost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ManagedRoostSpec   `json:"spec,omitempty"`
	Status ManagedRoostStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ManagedRoostList contains a list of ManagedRoost
type ManagedRoostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ManagedRoost `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ManagedRoost{}, &ManagedRoostList{})
}
