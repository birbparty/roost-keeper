package api

import (
	"fmt"
	"net/url"
	"regexp"

	"go.uber.org/zap"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// ResourceValidator provides comprehensive validation for ManagedRoost resources
type ResourceValidator struct {
	logger *zap.Logger
	rules  []ValidationRule
}

// NewResourceValidator creates a new resource validator with default rules
func NewResourceValidator(logger *zap.Logger) *ResourceValidator {
	rv := &ResourceValidator{
		logger: logger,
		rules:  make([]ValidationRule, 0),
	}

	// Register default validation rules
	rv.registerDefaultRules()

	return rv
}

// Validate runs all validation rules against a ManagedRoost resource
func (rv *ResourceValidator) Validate(managedRoost *roostkeeper.ManagedRoost) []ValidationError {
	var allErrors []ValidationError

	for _, rule := range rv.rules {
		errors := rule.Validator(managedRoost)
		allErrors = append(allErrors, errors...)
	}

	return allErrors
}

// AddRule adds a custom validation rule
func (rv *ResourceValidator) AddRule(rule ValidationRule) {
	rv.rules = append(rv.rules, rule)
}

// registerDefaultRules registers the default validation rules
func (rv *ResourceValidator) registerDefaultRules() {
	// Chart validation
	rv.rules = append(rv.rules, ValidationRule{
		Name:        "chart-validation",
		Description: "Validate Helm chart configuration",
		Validator:   rv.validateChart,
	})

	// Health check validation
	rv.rules = append(rv.rules, ValidationRule{
		Name:        "health-check-validation",
		Description: "Validate health check configuration",
		Validator:   rv.validateHealthChecks,
	})

	// Teardown policy validation
	rv.rules = append(rv.rules, ValidationRule{
		Name:        "teardown-policy-validation",
		Description: "Validate teardown policy configuration",
		Validator:   rv.validateTeardownPolicy,
	})

	// Tenancy validation
	rv.rules = append(rv.rules, ValidationRule{
		Name:        "tenancy-validation",
		Description: "Validate multi-tenancy configuration",
		Validator:   rv.validateTenancy,
	})

	// Observability validation
	rv.rules = append(rv.rules, ValidationRule{
		Name:        "observability-validation",
		Description: "Validate observability configuration",
		Validator:   rv.validateObservability,
	})
}

// validateChart validates Helm chart configuration
func (rv *ResourceValidator) validateChart(managedRoost *roostkeeper.ManagedRoost) []ValidationError {
	var errors []ValidationError

	// Required fields
	if managedRoost.Spec.Chart.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "spec.chart.name",
			Message: "Chart name is required",
			Code:    "CHART_NAME_MISSING",
			Level:   ValidationLevelError,
		})
	}

	if managedRoost.Spec.Chart.Repository.URL == "" {
		errors = append(errors, ValidationError{
			Field:   "spec.chart.repository.url",
			Message: "Chart repository URL is required",
			Code:    "CHART_REPO_URL_MISSING",
			Level:   ValidationLevelError,
		})
	}

	if managedRoost.Spec.Chart.Version == "" {
		errors = append(errors, ValidationError{
			Field:   "spec.chart.version",
			Message: "Chart version is required",
			Code:    "CHART_VERSION_MISSING",
			Level:   ValidationLevelError,
		})
	}

	// Validate URL format
	if managedRoost.Spec.Chart.Repository.URL != "" {
		if !isValidURL(managedRoost.Spec.Chart.Repository.URL) {
			errors = append(errors, ValidationError{
				Field:   "spec.chart.repository.url",
				Message: "Invalid repository URL format",
				Code:    "CHART_REPO_URL_INVALID",
				Level:   ValidationLevelError,
			})
		}
	}

	// Validate chart name pattern
	if managedRoost.Spec.Chart.Name != "" {
		if !isValidChartName(managedRoost.Spec.Chart.Name) {
			errors = append(errors, ValidationError{
				Field:   "spec.chart.name",
				Message: "Chart name must match pattern ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$",
				Code:    "CHART_NAME_INVALID",
				Level:   ValidationLevelError,
			})
		}
	}

	// Validate repository type
	if managedRoost.Spec.Chart.Repository.Type != "" {
		validTypes := []string{"http", "oci", "git"}
		if !contains(validTypes, managedRoost.Spec.Chart.Repository.Type) {
			errors = append(errors, ValidationError{
				Field:   "spec.chart.repository.type",
				Message: "Repository type must be one of: http, oci, git",
				Code:    "CHART_REPO_TYPE_INVALID",
				Level:   ValidationLevelError,
			})
		}
	}

	// Validate authentication configuration
	if managedRoost.Spec.Chart.Repository.Auth != nil {
		errors = append(errors, rv.validateRepositoryAuth(managedRoost.Spec.Chart.Repository.Auth)...)
	}

	return errors
}

// validateHealthChecks validates health check configuration
func (rv *ResourceValidator) validateHealthChecks(managedRoost *roostkeeper.ManagedRoost) []ValidationError {
	var errors []ValidationError

	for i, healthCheck := range managedRoost.Spec.HealthChecks {
		// Required fields
		if healthCheck.Name == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.healthChecks[%d].name", i),
				Message: "Health check name is required",
				Code:    "HEALTH_CHECK_NAME_MISSING",
				Level:   ValidationLevelError,
			})
		}

		if healthCheck.Type == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.healthChecks[%d].type", i),
				Message: "Health check type is required",
				Code:    "HEALTH_CHECK_TYPE_MISSING",
				Level:   ValidationLevelError,
			})
		}

		// Validate type-specific configuration
		switch healthCheck.Type {
		case "http":
			if healthCheck.HTTP == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.healthChecks[%d].http", i),
					Message: "HTTP configuration is required for HTTP health checks",
					Code:    "HTTP_CONFIG_MISSING",
					Level:   ValidationLevelError,
				})
			} else {
				errors = append(errors, rv.validateHTTPHealthCheck(healthCheck.HTTP, i)...)
			}

		case "tcp":
			if healthCheck.TCP == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.healthChecks[%d].tcp", i),
					Message: "TCP configuration is required for TCP health checks",
					Code:    "TCP_CONFIG_MISSING",
					Level:   ValidationLevelError,
				})
			} else {
				errors = append(errors, rv.validateTCPHealthCheck(healthCheck.TCP, i)...)
			}

		case "grpc":
			if healthCheck.GRPC == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.healthChecks[%d].grpc", i),
					Message: "gRPC configuration is required for gRPC health checks",
					Code:    "GRPC_CONFIG_MISSING",
					Level:   ValidationLevelError,
				})
			} else {
				errors = append(errors, rv.validateGRPCHealthCheck(healthCheck.GRPC, i)...)
			}

		case "prometheus":
			if healthCheck.Prometheus == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.healthChecks[%d].prometheus", i),
					Message: "Prometheus configuration is required for Prometheus health checks",
					Code:    "PROMETHEUS_CONFIG_MISSING",
					Level:   ValidationLevelError,
				})
			} else {
				errors = append(errors, rv.validatePrometheusHealthCheck(healthCheck.Prometheus, i)...)
			}

		case "kubernetes":
			if healthCheck.Kubernetes == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.healthChecks[%d].kubernetes", i),
					Message: "Kubernetes configuration is required for Kubernetes health checks",
					Code:    "KUBERNETES_CONFIG_MISSING",
					Level:   ValidationLevelError,
				})
			}

		default:
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.healthChecks[%d].type", i),
				Message: "Invalid health check type. Must be one of: http, tcp, udp, grpc, prometheus, kubernetes",
				Code:    "HEALTH_CHECK_TYPE_INVALID",
				Level:   ValidationLevelError,
			})
		}

		// Validate thresholds
		if healthCheck.FailureThreshold < 1 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.healthChecks[%d].failureThreshold", i),
				Message: "Failure threshold must be at least 1",
				Code:    "FAILURE_THRESHOLD_INVALID",
				Level:   ValidationLevelError,
			})
		}

		if healthCheck.Weight < 0 || healthCheck.Weight > 100 {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.healthChecks[%d].weight", i),
				Message: "Weight must be between 0 and 100",
				Code:    "WEIGHT_INVALID",
				Level:   ValidationLevelError,
			})
		}
	}

	return errors
}

// validateTeardownPolicy validates teardown policy configuration
func (rv *ResourceValidator) validateTeardownPolicy(managedRoost *roostkeeper.ManagedRoost) []ValidationError {
	var errors []ValidationError

	if managedRoost.Spec.TeardownPolicy == nil {
		return errors
	}

	policy := managedRoost.Spec.TeardownPolicy

	if len(policy.Triggers) == 0 {
		errors = append(errors, ValidationError{
			Field:   "spec.teardownPolicy.triggers",
			Message: "At least one teardown trigger is required when teardown policy is specified",
			Code:    "TEARDOWN_TRIGGERS_MISSING",
			Level:   ValidationLevelWarning,
		})
	}

	for i, trigger := range policy.Triggers {
		if trigger.Type == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.teardownPolicy.triggers[%d].type", i),
				Message: "Trigger type is required",
				Code:    "TRIGGER_TYPE_MISSING",
				Level:   ValidationLevelError,
			})
		}

		// Validate trigger-specific configuration
		switch trigger.Type {
		case "timeout":
			if trigger.Timeout == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.teardownPolicy.triggers[%d].timeout", i),
					Message: "Timeout duration is required for timeout triggers",
					Code:    "TIMEOUT_MISSING",
					Level:   ValidationLevelError,
				})
			}
		case "failure_count":
			if trigger.FailureCount == nil {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.teardownPolicy.triggers[%d].failureCount", i),
					Message: "Failure count is required for failure count triggers",
					Code:    "FAILURE_COUNT_MISSING",
					Level:   ValidationLevelError,
				})
			}
		case "schedule":
			if trigger.Schedule == "" {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("spec.teardownPolicy.triggers[%d].schedule", i),
					Message: "Schedule is required for schedule triggers",
					Code:    "SCHEDULE_MISSING",
					Level:   ValidationLevelError,
				})
			}
		}
	}

	return errors
}

// validateTenancy validates multi-tenancy configuration
func (rv *ResourceValidator) validateTenancy(managedRoost *roostkeeper.ManagedRoost) []ValidationError {
	var errors []ValidationError

	if managedRoost.Spec.Tenancy == nil {
		return errors
	}

	tenancy := managedRoost.Spec.Tenancy

	// Validate RBAC configuration
	if tenancy.RBAC != nil && tenancy.RBAC.Enabled {
		if tenancy.RBAC.TenantID == "" {
			errors = append(errors, ValidationError{
				Field:   "spec.tenancy.rbac.tenantId",
				Message: "Tenant ID is required when RBAC is enabled",
				Code:    "TENANT_ID_MISSING",
				Level:   ValidationLevelError,
			})
		}
	}

	return errors
}

// validateObservability validates observability configuration
func (rv *ResourceValidator) validateObservability(managedRoost *roostkeeper.ManagedRoost) []ValidationError {
	var errors []ValidationError

	if managedRoost.Spec.Observability == nil {
		return errors
	}

	obs := managedRoost.Spec.Observability

	// Validate tracing configuration
	if obs.Tracing != nil && obs.Tracing.Enabled {
		if obs.Tracing.Endpoint != nil && obs.Tracing.Endpoint.URL == "" {
			errors = append(errors, ValidationError{
				Field:   "spec.observability.tracing.endpoint.url",
				Message: "Tracing endpoint URL is required when tracing is enabled",
				Code:    "TRACING_ENDPOINT_URL_MISSING",
				Level:   ValidationLevelError,
			})
		}
	}

	return errors
}

// Helper validation functions

func (rv *ResourceValidator) validateRepositoryAuth(auth *roostkeeper.RepositoryAuthSpec) []ValidationError {
	var errors []ValidationError

	if auth.Type != "" {
		validTypes := []string{"basic", "token", "docker-config"}
		if !contains(validTypes, auth.Type) {
			errors = append(errors, ValidationError{
				Field:   "spec.chart.repository.auth.type",
				Message: "Authentication type must be one of: basic, token, docker-config",
				Code:    "AUTH_TYPE_INVALID",
				Level:   ValidationLevelError,
			})
		}
	}

	return errors
}

func (rv *ResourceValidator) validateHTTPHealthCheck(http *roostkeeper.HTTPHealthCheckSpec, index int) []ValidationError {
	var errors []ValidationError

	if http.URL == "" {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("spec.healthChecks[%d].http.url", index),
			Message: "HTTP URL is required",
			Code:    "HTTP_URL_MISSING",
			Level:   ValidationLevelError,
		})
	}

	if http.Method != "" {
		validMethods := []string{"GET", "POST", "PUT", "HEAD"}
		if !contains(validMethods, http.Method) {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("spec.healthChecks[%d].http.method", index),
				Message: "HTTP method must be one of: GET, POST, PUT, HEAD",
				Code:    "HTTP_METHOD_INVALID",
				Level:   ValidationLevelError,
			})
		}
	}

	return errors
}

func (rv *ResourceValidator) validateTCPHealthCheck(tcp *roostkeeper.TCPHealthCheckSpec, index int) []ValidationError {
	var errors []ValidationError

	if tcp.Host == "" {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("spec.healthChecks[%d].tcp.host", index),
			Message: "TCP host is required",
			Code:    "TCP_HOST_MISSING",
			Level:   ValidationLevelError,
		})
	}

	if tcp.Port < 1 || tcp.Port > 65535 {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("spec.healthChecks[%d].tcp.port", index),
			Message: "TCP port must be between 1 and 65535",
			Code:    "TCP_PORT_INVALID",
			Level:   ValidationLevelError,
		})
	}

	return errors
}

func (rv *ResourceValidator) validateGRPCHealthCheck(grpc *roostkeeper.GRPCHealthCheckSpec, index int) []ValidationError {
	var errors []ValidationError

	if grpc.Host == "" {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("spec.healthChecks[%d].grpc.host", index),
			Message: "gRPC host is required",
			Code:    "GRPC_HOST_MISSING",
			Level:   ValidationLevelError,
		})
	}

	if grpc.Port < 1 || grpc.Port > 65535 {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("spec.healthChecks[%d].grpc.port", index),
			Message: "gRPC port must be between 1 and 65535",
			Code:    "GRPC_PORT_INVALID",
			Level:   ValidationLevelError,
		})
	}

	return errors
}

func (rv *ResourceValidator) validatePrometheusHealthCheck(prom *roostkeeper.PrometheusHealthCheckSpec, index int) []ValidationError {
	var errors []ValidationError

	if prom.Query == "" {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("spec.healthChecks[%d].prometheus.query", index),
			Message: "Prometheus query is required",
			Code:    "PROMETHEUS_QUERY_MISSING",
			Level:   ValidationLevelError,
		})
	}

	if prom.Threshold == "" {
		errors = append(errors, ValidationError{
			Field:   fmt.Sprintf("spec.healthChecks[%d].prometheus.threshold", index),
			Message: "Prometheus threshold is required",
			Code:    "PROMETHEUS_THRESHOLD_MISSING",
			Level:   ValidationLevelError,
		})
	}

	return errors
}

// Utility functions

func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func isValidChartName(name string) bool {
	pattern := `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	matched, _ := regexp.MatchString(pattern, name)
	return matched
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
