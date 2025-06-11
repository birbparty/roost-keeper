package prometheus

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// PrometheusChecker implements advanced Prometheus-based health checking
type PrometheusChecker struct {
	k8sClient client.Client
	logger    *zap.Logger
	clients   map[string]v1.API // Cached Prometheus clients by endpoint
	cache     *QueryCache
	discovery *ServiceDiscovery
	auth      *AuthManager
	mu        sync.RWMutex
}

// QueryResult represents the result of a Prometheus query execution
type QueryResult struct {
	Value     float64           `json:"value"`
	Timestamp time.Time         `json:"timestamp"`
	Labels    map[string]string `json:"labels,omitempty"`
	Vector    []VectorResult    `json:"vector,omitempty"`
	IsVector  bool              `json:"is_vector"`
}

// VectorResult represents a single result in a vector query response
type VectorResult struct {
	Value     float64           `json:"value"`
	Timestamp time.Time         `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
}

// HealthCheckResult represents the complete result of a Prometheus health check
type HealthCheckResult struct {
	Healthy         bool                   `json:"healthy"`
	Message         string                 `json:"message"`
	Details         map[string]interface{} `json:"details,omitempty"`
	QueryResult     *QueryResult           `json:"query_result,omitempty"`
	ThresholdMet    bool                   `json:"threshold_met"`
	TrendAnalysis   *TrendResult           `json:"trend_analysis,omitempty"`
	ExecutionTime   time.Duration          `json:"execution_time"`
	FromCache       bool                   `json:"from_cache,omitempty"`
	PrometheusURL   string                 `json:"prometheus_url,omitempty"`
	DiscoveryMethod string                 `json:"discovery_method,omitempty"`
}

// NewPrometheusChecker creates a new Prometheus health checker with advanced capabilities
func NewPrometheusChecker(k8sClient client.Client, logger *zap.Logger) *PrometheusChecker {
	return &PrometheusChecker{
		k8sClient: k8sClient,
		logger:    logger,
		clients:   make(map[string]v1.API),
		cache:     NewQueryCache(logger),
		discovery: NewServiceDiscovery(k8sClient, logger),
		auth:      NewAuthManager(k8sClient, logger),
	}
}

// CheckHealth performs a comprehensive Prometheus health check
func (p *PrometheusChecker) CheckHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, promSpec roostv1alpha1.PrometheusHealthCheckSpec) (*HealthCheckResult, error) {
	start := time.Now()

	log := p.logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("check_type", "prometheus"),
		zap.String("query", promSpec.Query),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	// Start observability span
	ctx, span := telemetry.StartHealthCheckSpan(ctx, "prometheus", promSpec.Query)
	defer span.End()

	span.SetAttributes(
		attribute.String("health_check.name", checkSpec.Name),
		attribute.String("health_check.type", "prometheus"),
		attribute.String("prometheus.query", promSpec.Query),
		attribute.String("prometheus.operator", promSpec.Operator),
		attribute.String("prometheus.threshold", promSpec.Threshold),
	)

	log.Info("Starting Prometheus health check")

	// Resolve Prometheus endpoint
	endpoint, discoveryMethod, err := p.resolvePrometheusEndpoint(ctx, roost, promSpec)
	if err != nil {
		log.Error("Failed to resolve Prometheus endpoint", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return &HealthCheckResult{
			Healthy:         false,
			Message:         fmt.Sprintf("Endpoint resolution failed: %v", err),
			ExecutionTime:   time.Since(start),
			DiscoveryMethod: discoveryMethod,
		}, nil
	}

	span.SetAttributes(
		attribute.String("prometheus.endpoint", endpoint),
		attribute.String("prometheus.discovery_method", discoveryMethod),
	)

	// Get or create Prometheus client
	promClient, err := p.getPrometheusClient(ctx, endpoint, roost, promSpec)
	if err != nil {
		log.Error("Failed to create Prometheus client", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return &HealthCheckResult{
			Healthy:         false,
			Message:         fmt.Sprintf("Client creation failed: %v", err),
			ExecutionTime:   time.Since(start),
			PrometheusURL:   endpoint,
			DiscoveryMethod: discoveryMethod,
		}, nil
	}

	// Template the query with roost information
	templatedQuery, err := p.templateQuery(promSpec.Query, roost)
	if err != nil {
		log.Error("Failed to template query", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return &HealthCheckResult{
			Healthy:         false,
			Message:         fmt.Sprintf("Query templating failed: %v", err),
			ExecutionTime:   time.Since(start),
			PrometheusURL:   endpoint,
			DiscoveryMethod: discoveryMethod,
		}, nil
	}

	span.SetAttributes(attribute.String("prometheus.templated_query", templatedQuery))

	// Check cache first
	cacheKey := p.buildCacheKey(endpoint, templatedQuery, roost, checkSpec)
	if cachedResult := p.cache.Get(cacheKey); cachedResult != nil && p.shouldUseCache(promSpec) {
		log.Debug("Prometheus health check result served from cache")
		span.SetAttributes(attribute.Bool("cache.hit", true))

		cachedResult.FromCache = true
		cachedResult.ExecutionTime = time.Since(start)
		return cachedResult, nil
	}

	span.SetAttributes(attribute.Bool("cache.hit", false))

	// Execute the query
	queryResult, err := p.executeQuery(ctx, promClient, templatedQuery, promSpec, checkSpec)
	if err != nil {
		log.Error("Query execution failed", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return &HealthCheckResult{
			Healthy:         false,
			Message:         fmt.Sprintf("Prometheus query failed: %v", err),
			ExecutionTime:   time.Since(start),
			PrometheusURL:   endpoint,
			DiscoveryMethod: discoveryMethod,
		}, nil
	}

	// Evaluate threshold
	thresholdMet, err := p.evaluateThreshold(queryResult.Value, promSpec.Threshold, promSpec.Operator)
	if err != nil {
		log.Error("Threshold evaluation failed", zap.Error(err))
		telemetry.RecordSpanError(ctx, err)
		return &HealthCheckResult{
			Healthy:         false,
			Message:         fmt.Sprintf("Threshold evaluation failed: %v", err),
			QueryResult:     queryResult,
			ExecutionTime:   time.Since(start),
			PrometheusURL:   endpoint,
			DiscoveryMethod: discoveryMethod,
		}, nil
	}

	// Perform trend analysis if configured
	var trendResult *TrendResult
	if promSpec.TrendAnalysis != nil && promSpec.TrendAnalysis.Enabled {
		trendResult, err = p.analyzeTrend(ctx, promClient, templatedQuery, promSpec.TrendAnalysis)
		if err != nil {
			log.Warn("Trend analysis failed", zap.Error(err))
		}
	}

	// Determine final health status
	healthy := p.determineHealthStatus(thresholdMet, trendResult, promSpec)

	// Build result
	result := &HealthCheckResult{
		Healthy:         healthy,
		Message:         p.buildHealthMessage(healthy, thresholdMet, queryResult.Value, promSpec, trendResult),
		QueryResult:     queryResult,
		ThresholdMet:    thresholdMet,
		TrendAnalysis:   trendResult,
		ExecutionTime:   time.Since(start),
		PrometheusURL:   endpoint,
		DiscoveryMethod: discoveryMethod,
		Details: map[string]interface{}{
			"query":               templatedQuery,
			"threshold":           promSpec.Threshold,
			"operator":            promSpec.Operator,
			"value":               queryResult.Value,
			"timestamp":           queryResult.Timestamp,
			"prometheus_endpoint": endpoint,
			"execution_time_ms":   time.Since(start).Milliseconds(),
		},
	}

	// Add custom labels to details
	if promSpec.Labels != nil {
		result.Details["custom_labels"] = promSpec.Labels
	}

	// Cache successful results
	if healthy && p.shouldUseCache(promSpec) {
		cacheTTL := p.getCacheTTL(promSpec)
		p.cache.Set(cacheKey, result, cacheTTL)
	}

	// Record metrics in span
	span.SetAttributes(
		attribute.Bool("prometheus.threshold_met", thresholdMet),
		attribute.Bool("prometheus.healthy", healthy),
		attribute.Float64("prometheus.value", queryResult.Value),
		attribute.Float64("prometheus.execution_time_seconds", time.Since(start).Seconds()),
	)

	log.Info("Prometheus health check completed",
		zap.Bool("healthy", healthy),
		zap.Bool("threshold_met", thresholdMet),
		zap.Float64("value", queryResult.Value),
		zap.Duration("execution_time", time.Since(start)),
		zap.String("endpoint", endpoint),
	)

	return result, nil
}

// ExecutePrometheusCheck provides backward compatibility with the main health checker
func ExecutePrometheusCheck(ctx context.Context, logger *zap.Logger, k8sClient client.Client, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, promSpec roostv1alpha1.PrometheusHealthCheckSpec) (bool, error) {
	checker := NewPrometheusChecker(k8sClient, logger)
	result, err := checker.CheckHealth(ctx, roost, checkSpec, promSpec)
	if err != nil {
		return false, err
	}
	return result.Healthy, nil
}

// resolvePrometheusEndpoint determines the Prometheus endpoint using service discovery or explicit configuration
func (p *PrometheusChecker) resolvePrometheusEndpoint(ctx context.Context, roost *roostv1alpha1.ManagedRoost, promSpec roostv1alpha1.PrometheusHealthCheckSpec) (string, string, error) {
	// Try service discovery first if configured
	if promSpec.ServiceDiscovery != nil {
		endpoint, err := p.discovery.DiscoverEndpoint(ctx, roost.Namespace, promSpec.ServiceDiscovery)
		if err == nil {
			return endpoint, "service_discovery", nil
		}

		// Log discovery failure
		p.logger.Warn("Service discovery failed, checking fallback options", zap.Error(err))

		// Check if fallback is enabled and explicit endpoint is provided
		if promSpec.ServiceDiscovery.EnableFallback && promSpec.Endpoint != "" {
			p.logger.Info("Using fallback to explicit endpoint", zap.String("endpoint", promSpec.Endpoint))
			return promSpec.Endpoint, "fallback_explicit", nil
		}

		// If no fallback allowed, return discovery error
		if !promSpec.ServiceDiscovery.EnableFallback {
			return "", "service_discovery_failed", fmt.Errorf("service discovery failed and fallback disabled: %w", err)
		}
	}

	// Use explicit endpoint if provided
	if promSpec.Endpoint != "" {
		return promSpec.Endpoint, "explicit", nil
	}

	// No endpoint configuration available
	return "", "none", fmt.Errorf("no Prometheus endpoint configured: either specify 'endpoint' or configure 'serviceDiscovery'")
}

// getPrometheusClient retrieves or creates a Prometheus API client for the given endpoint
func (p *PrometheusChecker) getPrometheusClient(ctx context.Context, endpoint string, roost *roostv1alpha1.ManagedRoost, promSpec roostv1alpha1.PrometheusHealthCheckSpec) (v1.API, error) {
	p.mu.RLock()
	if client, exists := p.clients[endpoint]; exists {
		p.mu.RUnlock()
		return client, nil
	}
	p.mu.RUnlock()

	// Create new client
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if client, exists := p.clients[endpoint]; exists {
		return client, nil
	}

	// Configure client with authentication
	config := api.Config{
		Address: endpoint,
	}

	// Apply authentication if configured
	if promSpec.Auth != nil {
		if err := p.auth.ConfigureClient(ctx, &config, roost.Namespace, promSpec.Auth); err != nil {
			return nil, fmt.Errorf("failed to configure authentication: %w", err)
		}
	}

	// Create Prometheus client
	client, err := api.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	promClient := v1.NewAPI(client)
	p.clients[endpoint] = promClient

	return promClient, nil
}

// executeQuery executes a PromQL query and returns the result
func (p *PrometheusChecker) executeQuery(ctx context.Context, client v1.API, query string, promSpec roostv1alpha1.PrometheusHealthCheckSpec, checkSpec roostv1alpha1.HealthCheckSpec) (*QueryResult, error) {
	// Configure timeout
	timeout := 30 * time.Second
	if promSpec.QueryTimeout != nil && promSpec.QueryTimeout.Duration > 0 {
		timeout = promSpec.QueryTimeout.Duration
	} else if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute query
	value, warnings, err := client.Query(queryCtx, query, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Log warnings
	if len(warnings) > 0 {
		p.logger.Warn("Query execution warnings", zap.Strings("warnings", warnings))
	}

	// Parse result
	return p.parseQueryResult(value)
}

// parseQueryResult parses a Prometheus query result into our internal format
func (p *PrometheusChecker) parseQueryResult(value model.Value) (*QueryResult, error) {
	switch v := value.(type) {
	case model.Vector:
		if len(v) == 0 {
			return nil, fmt.Errorf("query returned no results")
		}

		if len(v) == 1 {
			// Single value result
			sample := v[0]
			labels := make(map[string]string)
			for k, v := range sample.Metric {
				labels[string(k)] = string(v)
			}

			return &QueryResult{
				Value:     float64(sample.Value),
				Timestamp: sample.Timestamp.Time(),
				Labels:    labels,
				IsVector:  false,
			}, nil
		}

		// Multiple values - return as vector
		vector := make([]VectorResult, len(v))
		for i, sample := range v {
			labels := make(map[string]string)
			for k, v := range sample.Metric {
				labels[string(k)] = string(v)
			}

			vector[i] = VectorResult{
				Value:     float64(sample.Value),
				Timestamp: sample.Timestamp.Time(),
				Labels:    labels,
			}
		}

		// Use first value for threshold comparison
		return &QueryResult{
			Value:     float64(v[0].Value),
			Timestamp: v[0].Timestamp.Time(),
			Vector:    vector,
			IsVector:  true,
		}, nil

	case *model.Scalar:
		return &QueryResult{
			Value:     float64(v.Value),
			Timestamp: v.Timestamp.Time(),
			IsVector:  false,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported query result type: %T", value)
	}
}

// evaluateThreshold evaluates whether the query result meets the specified threshold
func (p *PrometheusChecker) evaluateThreshold(value float64, thresholdStr, operator string) (bool, error) {
	// Parse threshold value
	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return false, fmt.Errorf("invalid threshold value '%s': %w", thresholdStr, err)
	}

	// Normalize operator
	operator = strings.ToLower(strings.TrimSpace(operator))
	if operator == "" {
		operator = "gte"
	}

	// Evaluate based on operator
	switch operator {
	case "gt", ">":
		return value > threshold, nil
	case "gte", ">=":
		return value >= threshold, nil
	case "lt", "<":
		return value < threshold, nil
	case "lte", "<=":
		return value <= threshold, nil
	case "eq", "==":
		return math.Abs(value-threshold) < 0.0001, nil // Float comparison with epsilon
	case "ne", "!=":
		return math.Abs(value-threshold) >= 0.0001, nil
	default:
		return false, fmt.Errorf("unsupported operator: %s", operator)
	}
}

// determineHealthStatus determines the final health status considering threshold and trend analysis
func (p *PrometheusChecker) determineHealthStatus(thresholdMet bool, trendResult *TrendResult, promSpec roostv1alpha1.PrometheusHealthCheckSpec) bool {
	// If threshold is met, we're healthy
	if thresholdMet {
		return true
	}

	// If threshold not met, check trend analysis
	if trendResult != nil && promSpec.TrendAnalysis != nil && promSpec.TrendAnalysis.AllowImprovingUnhealthy {
		return trendResult.IsImproving
	}

	// Default: threshold determines health
	return thresholdMet
}

// buildHealthMessage creates a descriptive health message
func (p *PrometheusChecker) buildHealthMessage(healthy, thresholdMet bool, value float64, promSpec roostv1alpha1.PrometheusHealthCheckSpec, trendResult *TrendResult) string {
	if healthy {
		if thresholdMet {
			return "Prometheus health check passed"
		}
		if trendResult != nil && trendResult.IsImproving {
			return "Prometheus health check passed (metric improving despite threshold)"
		}
	}

	// Build failure message
	msg := fmt.Sprintf("Prometheus threshold failed: %.6f %s %s", value, promSpec.Operator, promSpec.Threshold)

	if trendResult != nil {
		if trendResult.IsImproving {
			msg += " (trend: improving)"
		} else if trendResult.IsDegrading {
			msg += " (trend: degrading)"
		} else {
			msg += " (trend: stable)"
		}
	}

	return msg
}

// templateQuery processes template variables in the PromQL query
func (p *PrometheusChecker) templateQuery(queryTemplate string, roost *roostv1alpha1.ManagedRoost) (string, error) {
	result := queryTemplate

	// Replace common template variables
	replacements := map[string]string{
		"{{.ServiceName}}":    roost.Name,
		"{{.Namespace}}":      roost.Namespace,
		"{{.RoostName}}":      roost.Name,
		"{{.RoostNamespace}}": roost.Namespace,
		"${ROOST_NAME}":       roost.Name,
		"${ROOST_NAMESPACE}":  roost.Namespace,
		"${SERVICE_NAME}":     roost.Name,
		"${NAMESPACE}":        roost.Namespace,
	}

	// Replace Helm release information if available
	if roost.Status.HelmRelease != nil {
		replacements["{{.Release.Name}}"] = roost.Status.HelmRelease.Name
		replacements["${RELEASE_NAME}"] = roost.Status.HelmRelease.Name
	}

	// Apply replacements
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
}

// buildCacheKey creates a unique cache key for the health check
func (p *PrometheusChecker) buildCacheKey(endpoint, query string, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", endpoint, roost.Namespace, roost.Name, checkSpec.Name, query)
}

// shouldUseCache determines if caching should be used for this check
func (p *PrometheusChecker) shouldUseCache(promSpec roostv1alpha1.PrometheusHealthCheckSpec) bool {
	return promSpec.CacheTTL == nil || promSpec.CacheTTL.Duration > 0
}

// getCacheTTL returns the cache TTL for this check
func (p *PrometheusChecker) getCacheTTL(promSpec roostv1alpha1.PrometheusHealthCheckSpec) time.Duration {
	if promSpec.CacheTTL != nil && promSpec.CacheTTL.Duration > 0 {
		return promSpec.CacheTTL.Duration
	}
	return 5 * time.Minute // Default TTL
}
