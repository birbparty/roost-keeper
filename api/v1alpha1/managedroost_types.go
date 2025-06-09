package v1alpha1

import (
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
	// +kubebuilder:validation:Enum=http;tcp;grpc;prometheus;kubernetes
	Type string `json:"type"`

	// HTTP health check configuration
	// +kubebuilder:validation:Optional
	HTTP *HTTPHealthCheckSpec `json:"http,omitempty"`

	// TCP health check configuration
	// +kubebuilder:validation:Optional
	TCP *TCPHealthCheckSpec `json:"tcp,omitempty"`

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

	// Service name for health check
	// +kubebuilder:validation:Optional
	Service string `json:"service,omitempty"`

	// TLS configuration
	// +kubebuilder:validation:Optional
	TLS *GRPCTLSSpec `json:"tls,omitempty"`
}

// PrometheusHealthCheckSpec defines Prometheus-based health checks
type PrometheusHealthCheckSpec struct {
	// Prometheus query
	// +kubebuilder:validation:Required
	Query string `json:"query"`

	// Expected threshold value (as string to support decimals)
	// +kubebuilder:validation:Required
	Threshold string `json:"threshold"`

	// Comparison operator
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=gt;gte;lt;lte;eq;ne
	// +kubebuilder:default=gte
	Operator string `json:"operator,omitempty"`

	// Prometheus endpoint URL
	// +kubebuilder:validation:Optional
	Endpoint string `json:"endpoint,omitempty"`
}

// KubernetesHealthCheckSpec defines Kubernetes resource-based health checks
type KubernetesHealthCheckSpec struct {
	// Resources to check
	// +kubebuilder:validation:Required
	Resources []KubernetesResourceCheck `json:"resources"`
}

// KubernetesResourceCheck defines a single Kubernetes resource health check
type KubernetesResourceCheck struct {
	// API version of the resource
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Label selector for resources
	// +kubebuilder:validation:Optional
	LabelSelector map[string]string `json:"labelSelector,omitempty"`

	// Expected conditions
	// +kubebuilder:validation:Optional
	Conditions []ExpectedCondition `json:"conditions,omitempty"`
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
	// Skip TLS verification
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
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

// RBACSpec defines RBAC configuration
type RBACSpec struct {
	// Enable RBAC
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Service account for roost operations
	// +kubebuilder:validation:Optional
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Additional cluster roles
	// +kubebuilder:validation:Optional
	ClusterRoles []string `json:"clusterRoles,omitempty"`

	// Additional roles
	// +kubebuilder:validation:Optional
	Roles []string `json:"roles,omitempty"`
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
