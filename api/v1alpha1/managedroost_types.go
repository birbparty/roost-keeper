package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json:"-" or json:"fieldName,omitempty"

// ManagedRoostSpec defines the desired state of ManagedRoost
type ManagedRoostSpec struct {
	// HelmChart specifies the Helm chart to manage
	HelmChart HelmChartSpec `json:"helmChart"`

	// HealthChecks defines the health check configuration
	HealthChecks HealthCheckSpec `json:"healthChecks"`

	// TeardownPolicy specifies when and how to tear down the deployment
	TeardownPolicy TeardownPolicySpec `json:"teardownPolicy"`

	// Namespace specifies the target namespace for the Helm deployment
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// HelmChartSpec defines the Helm chart configuration
type HelmChartSpec struct {
	// Repository is the Helm repository URL
	Repository string `json:"repository"`

	// Chart is the name of the Helm chart
	Chart string `json:"chart"`

	// Version is the version of the Helm chart
	Version string `json:"version"`

	// Values are the Helm values to use for the deployment
	// +optional
	Values map[string]interface{} `json:"values,omitempty"`

	// ValuesFrom specifies ConfigMap/Secret references for values
	// +optional
	ValuesFrom []ValuesReference `json:"valuesFrom,omitempty"`
}

// ValuesReference defines a reference to a ConfigMap or Secret for Helm values
type ValuesReference struct {
	// ConfigMapRef is a reference to a ConfigMap
	// +optional
	ConfigMapRef *ConfigMapReference `json:"configMapRef,omitempty"`

	// SecretRef is a reference to a Secret
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty"`
}

// ConfigMapReference represents a reference to a ConfigMap
type ConfigMapReference struct {
	// Name is the name of the ConfigMap
	Name string `json:"name"`

	// Key is the key within the ConfigMap
	// +optional
	Key string `json:"key,omitempty"`
}

// SecretReference represents a reference to a Secret
type SecretReference struct {
	// Name is the name of the Secret
	Name string `json:"name"`

	// Key is the key within the Secret
	// +optional
	Key string `json:"key,omitempty"`
}

// HealthCheckSpec defines the health check configuration
type HealthCheckSpec struct {
	// Enabled specifies whether health checks are enabled
	Enabled bool `json:"enabled"`

	// HTTP health check configuration
	// +optional
	HTTP *HTTPHealthCheck `json:"http,omitempty"`

	// TCP health check configuration
	// +optional
	TCP *TCPHealthCheck `json:"tcp,omitempty"`

	// UDP health check configuration
	// +optional
	UDP *UDPHealthCheck `json:"udp,omitempty"`

	// GRPC health check configuration
	// +optional
	GRPC *GRPCHealthCheck `json:"grpc,omitempty"`

	// Prometheus health check configuration
	// +optional
	Prometheus *PrometheusHealthCheck `json:"prometheus,omitempty"`

	// Kubernetes native health check configuration
	// +optional
	Kubernetes *KubernetesHealthCheck `json:"kubernetes,omitempty"`

	// Composite health check configuration
	// +optional
	Composite *CompositeHealthCheck `json:"composite,omitempty"`
}

// HTTPHealthCheck defines HTTP-based health checking
type HTTPHealthCheck struct {
	// Path is the HTTP path to check
	Path string `json:"path"`

	// Port is the port to check
	Port int32 `json:"port"`

	// Scheme is the HTTP scheme (HTTP or HTTPS)
	// +optional
	Scheme string `json:"scheme,omitempty"`

	// Headers are additional HTTP headers to send
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// Timeout is the timeout for the health check
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Interval is the interval between health checks
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
}

// TCPHealthCheck defines TCP-based health checking
type TCPHealthCheck struct {
	// Port is the port to check
	Port int32 `json:"port"`

	// Timeout is the timeout for the health check
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Interval is the interval between health checks
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
}

// UDPHealthCheck defines UDP-based health checking
type UDPHealthCheck struct {
	// Port is the port to check
	Port int32 `json:"port"`

	// Timeout is the timeout for the health check
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Interval is the interval between health checks
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
}

// GRPCHealthCheck defines gRPC-based health checking
type GRPCHealthCheck struct {
	// Port is the port to check
	Port int32 `json:"port"`

	// Service is the gRPC service name to check
	// +optional
	Service string `json:"service,omitempty"`

	// Timeout is the timeout for the health check
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Interval is the interval between health checks
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
}

// PrometheusHealthCheck defines Prometheus-based health checking
type PrometheusHealthCheck struct {
	// Endpoint is the Prometheus metrics endpoint
	Endpoint string `json:"endpoint"`

	// Query is the PromQL query to evaluate
	Query string `json:"query"`

	// Threshold is the threshold for the health check
	Threshold float64 `json:"threshold"`

	// Timeout is the timeout for the health check
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Interval is the interval between health checks
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
}

// KubernetesHealthCheck defines Kubernetes-native health checking
type KubernetesHealthCheck struct {
	// Resources specifies the Kubernetes resources to monitor
	Resources []ResourceHealthCheck `json:"resources"`

	// Timeout is the timeout for the health check
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Interval is the interval between health checks
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
}

// ResourceHealthCheck defines health checking for a specific Kubernetes resource
type ResourceHealthCheck struct {
	// APIVersion is the API version of the resource
	APIVersion string `json:"apiVersion"`

	// Kind is the kind of the resource
	Kind string `json:"kind"`

	// Name is the name of the resource
	// +optional
	Name string `json:"name,omitempty"`

	// LabelSelector is the label selector for the resources
	// +optional
	LabelSelector map[string]string `json:"labelSelector,omitempty"`

	// Conditions are the conditions to check
	// +optional
	Conditions []ConditionCheck `json:"conditions,omitempty"`
}

// ConditionCheck defines a condition to check on a Kubernetes resource
type ConditionCheck struct {
	// Type is the condition type
	Type string `json:"type"`

	// Status is the expected condition status
	Status string `json:"status"`
}

// CompositeHealthCheck defines composite health checking logic
type CompositeHealthCheck struct {
	// Logic defines the composite logic (AND, OR)
	Logic CompositeLogic `json:"logic"`

	// Checks are the individual health checks to combine
	Checks []string `json:"checks"`

	// Timeout is the timeout for the health check
	// +optional
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Interval is the interval between health checks
	// +optional
	Interval metav1.Duration `json:"interval,omitempty"`
}

// CompositeLogic defines the logic for combining health checks
type CompositeLogic string

const (
	// CompositeLogicAND requires all health checks to pass
	CompositeLogicAND CompositeLogic = "AND"
	// CompositeLogicOR requires at least one health check to pass
	CompositeLogicOR CompositeLogic = "OR"
)

// TeardownPolicySpec defines the teardown policy
type TeardownPolicySpec struct {
	// TriggerCondition specifies when to trigger teardown
	TriggerCondition TeardownTrigger `json:"triggerCondition"`

	// GracePeriod is the grace period before forced teardown
	// +optional
	GracePeriod metav1.Duration `json:"gracePeriod,omitempty"`

	// PreserveResources specifies which resources to preserve during teardown
	// +optional
	PreserveResources []string `json:"preserveResources,omitempty"`
}

// TeardownTrigger defines when to trigger teardown
type TeardownTrigger string

const (
	// TeardownTriggerHealthFailure triggers teardown on health check failure
	TeardownTriggerHealthFailure TeardownTrigger = "HealthFailure"
	// TeardownTriggerManual requires manual teardown
	TeardownTriggerManual TeardownTrigger = "Manual"
	// TeardownTriggerTTL triggers teardown after a time-to-live period
	TeardownTriggerTTL TeardownTrigger = "TTL"
)

// ManagedRoostStatus defines the observed state of ManagedRoost
type ManagedRoostStatus struct {
	// Phase represents the current phase of the ManagedRoost
	Phase ManagedRoostPhase `json:"phase,omitempty"`

	// Conditions represent the current service state
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// HelmRelease contains information about the Helm release
	HelmRelease *HelmReleaseStatus `json:"helmRelease,omitempty"`

	// HealthStatus contains the current health status
	HealthStatus *HealthStatus `json:"healthStatus,omitempty"`

	// ObservedGeneration reflects the generation most recently observed by the controller
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastReconcileTime is the last time the resource was reconciled
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

// HelmReleaseStatus contains information about the Helm release
type HelmReleaseStatus struct {
	// Name is the name of the Helm release
	Name string `json:"name"`

	// Namespace is the namespace of the Helm release
	Namespace string `json:"namespace"`

	// Version is the version of the Helm release
	Version int `json:"version"`

	// Status is the status of the Helm release
	Status string `json:"status"`

	// LastDeployed is the time the release was last deployed
	LastDeployed metav1.Time `json:"lastDeployed,omitempty"`

	// Notes contains the release notes
	Notes string `json:"notes,omitempty"`
}

// HealthStatus contains the current health status
type HealthStatus struct {
	// Overall health status
	Overall HealthStatusValue `json:"overall"`

	// Individual health check results
	Checks map[string]HealthCheckResult `json:"checks,omitempty"`

	// LastHealthCheck is the time of the last health check
	LastHealthCheck metav1.Time `json:"lastHealthCheck,omitempty"`
}

// HealthStatusValue defines health status values
type HealthStatusValue string

const (
	// HealthStatusHealthy indicates healthy status
	HealthStatusHealthy HealthStatusValue = "Healthy"
	// HealthStatusUnhealthy indicates unhealthy status
	HealthStatusUnhealthy HealthStatusValue = "Unhealthy"
	// HealthStatusUnknown indicates unknown health status
	HealthStatusUnknown HealthStatusValue = "Unknown"
)

// HealthCheckResult contains the result of a health check
type HealthCheckResult struct {
	// Status is the health check status
	Status HealthStatusValue `json:"status"`

	// Message contains additional information about the health check
	Message string `json:"message,omitempty"`

	// LastCheck is the time of the last check
	LastCheck metav1.Time `json:"lastCheck"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Namespaced
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Health",type="string",JSONPath=".status.healthStatus.overall"
//+kubebuilder:printcolumn:name="Helm Release",type="string",JSONPath=".status.helmRelease.name"
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
