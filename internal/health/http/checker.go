package http

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// HTTPChecker implements advanced HTTP health checking with retry logic,
// authentication, caching, and comprehensive observability
type HTTPChecker struct {
	client        client.Client
	logger        *zap.Logger
	httpClient    *retryablehttp.Client
	responseCache *ResponseCache
	metrics       *HTTPMetrics
	mu            sync.RWMutex
}

// HTTPMetrics contains HTTP-specific metrics (simplified for now)
type HTTPMetrics struct {
	// Metrics will be implemented when telemetry is fully integrated
}

// ResponseCache implements TTL-based caching for HTTP responses
type ResponseCache struct {
	cache sync.Map
	ttl   time.Duration
}

// CachedResponse represents a cached HTTP health check response
type CachedResponse struct {
	healthy   bool
	message   string
	details   map[string]interface{}
	timestamp time.Time
}

// HTTPCheckResult represents the result of an HTTP health check
type HTTPCheckResult struct {
	Healthy      bool                   `json:"healthy"`
	Message      string                 `json:"message"`
	Details      map[string]interface{} `json:"details,omitempty"`
	StatusCode   int                    `json:"status_code,omitempty"`
	ResponseTime time.Duration          `json:"response_time,omitempty"`
	FromCache    bool                   `json:"from_cache,omitempty"`
}

// NewHTTPChecker creates a new HTTP health checker with advanced features
func NewHTTPChecker(k8sClient client.Client, logger *zap.Logger) (*HTTPChecker, error) {
	// Create simplified metrics or skip them if telemetry isn't fully initialized
	metrics := &HTTPMetrics{}

	// Create retryable HTTP client with advanced configuration
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RetryWaitMin = 1 * time.Second
	retryClient.RetryWaitMax = 10 * time.Second
	retryClient.Logger = nil // Disable default logging

	// Configure retry policy to only retry on connection errors, not status codes
	// This allows us to validate status codes properly in health checks
	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		// Only retry on actual connection errors, not HTTP status codes
		if err != nil {
			return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		}
		// Don't retry based on status codes - let the health check logic handle them
		return false, nil
	}

	// Configure jitter to prevent thundering herd
	retryClient.Backoff = func(min, max time.Duration, attemptNum int, resp *http.Response) time.Duration {
		base := retryablehttp.DefaultBackoff(min, max, attemptNum, resp)
		jitterMax := int64(float64(base) * 0.1)
		if jitterMax > 0 {
			jitter := time.Duration(rand.Int63n(jitterMax))
			return base + jitter
		}
		return base
	}

	// Configure underlying HTTP client with connection pooling
	retryClient.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			MaxConnsPerHost:     50,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
			DisableCompression:  false,
			ForceAttemptHTTP2:   true,
		},
	}

	// Initialize response cache with 5-minute TTL
	cache := &ResponseCache{
		ttl: 5 * time.Minute,
	}

	return &HTTPChecker{
		client:        k8sClient,
		logger:        logger,
		httpClient:    retryClient,
		responseCache: cache,
		metrics:       metrics,
	}, nil
}

// ExecuteHTTPCheck performs an advanced HTTP health check
func ExecuteHTTPCheck(ctx context.Context, logger *zap.Logger, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec) (bool, error) {
	// Create a temporary checker for backward compatibility
	checker, err := NewHTTPChecker(nil, logger)
	if err != nil {
		return false, fmt.Errorf("failed to create HTTP checker: %w", err)
	}

	result, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)
	if err != nil {
		return false, err
	}

	return result.Healthy, nil
}

// CheckHealth performs a comprehensive HTTP health check
func (h *HTTPChecker) CheckHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec) (*HTTPCheckResult, error) {
	start := time.Now()

	log := h.logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("check_type", "http"),
		zap.String("url", httpSpec.URL),
		zap.String("roost_name", roost.Name),
		zap.String("roost_namespace", roost.Namespace),
	)

	// Start observability span
	ctx, span := telemetry.StartHealthCheckSpan(ctx, "http", httpSpec.URL)
	defer span.End()

	span.SetAttributes(
		attribute.String("health_check.name", checkSpec.Name),
		attribute.String("health_check.type", "http"),
		attribute.String("http.method", httpSpec.Method),
		attribute.String("http.url", httpSpec.URL),
	)

	// Check cache first
	cacheKey := h.buildCacheKey(roost, checkSpec, httpSpec)
	if cachedResult := h.getCachedResult(cacheKey); cachedResult != nil {
		log.Debug("HTTP health check result served from cache")

		cachedResult.FromCache = true
		span.SetAttributes(attribute.Bool("cache.hit", true))
		return cachedResult, nil
	}

	span.SetAttributes(attribute.Bool("cache.hit", false))

	log.Info("Starting HTTP health check")

	// Prepare the HTTP request
	req, err := h.prepareRequest(ctx, roost, checkSpec, httpSpec)
	if err != nil {
		h.recordError(ctx, "request_preparation_failed", err)
		telemetry.RecordSpanError(ctx, err)
		return &HTTPCheckResult{
			Healthy: false,
			Message: fmt.Sprintf("Request preparation failed: %v", err),
		}, nil
	}

	// Configure client for this specific check
	h.configureClientForCheck(checkSpec, httpSpec)

	// Execute request with retries
	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.recordError(ctx, "request_failed", err)
		telemetry.RecordSpanError(ctx, err)
		log.Error("HTTP request failed", zap.Error(err))
		return &HTTPCheckResult{
			Healthy: false,
			Message: fmt.Sprintf("HTTP request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// Record metrics in span
	duration := time.Since(start)
	span.SetAttributes(
		attribute.Int("http.status_code", resp.StatusCode),
		attribute.Float64("http.duration_seconds", duration.Seconds()),
	)

	// Validate response
	result, err := h.validateResponse(ctx, resp, checkSpec, httpSpec, duration)
	if err != nil {
		h.recordError(ctx, "response_validation_failed", err)
		telemetry.RecordSpanError(ctx, err)
		return result, nil
	}

	// Cache successful results
	if result.Healthy {
		h.cacheResult(cacheKey, result)
	}

	log.Info("HTTP health check completed",
		zap.Bool("healthy", result.Healthy),
		zap.Int("status_code", resp.StatusCode),
		zap.Duration("duration", duration),
	)

	return result, nil
}

// prepareRequest creates and configures the HTTP request
func (h *HTTPChecker) prepareRequest(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec) (*retryablehttp.Request, error) {
	// Template URL with roost information
	templatedURL, err := h.templateURL(httpSpec.URL, roost)
	if err != nil {
		return nil, fmt.Errorf("failed to template URL: %w", err)
	}

	// Validate URL
	parsedURL, err := url.Parse(templatedURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Determine HTTP method
	method := strings.ToUpper(httpSpec.Method)
	if method == "" {
		method = "GET"
	}

	// Create request
	req, err := retryablehttp.NewRequestWithContext(ctx, method, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add standard headers
	req.Header.Set("User-Agent", "roost-keeper/1.0")
	req.Header.Set("Accept", "*/*")

	// Add custom headers
	if httpSpec.Headers != nil {
		for key, value := range httpSpec.Headers {
			templatedValue, err := h.templateHeaderValue(ctx, value, roost)
			if err != nil {
				return nil, fmt.Errorf("failed to template header %s: %w", key, err)
			}
			req.Header.Set(key, templatedValue)
		}
	}

	// Add authentication headers
	if err := h.addAuthentication(ctx, req, roost, httpSpec); err != nil {
		return nil, fmt.Errorf("failed to add authentication: %w", err)
	}

	return req, nil
}

// addAuthentication adds authentication headers to the request
func (h *HTTPChecker) addAuthentication(ctx context.Context, req *retryablehttp.Request, roost *roostv1alpha1.ManagedRoost, httpSpec roostv1alpha1.HTTPHealthCheckSpec) error {
	// Check for Bearer token in headers
	for key, value := range httpSpec.Headers {
		if strings.ToLower(key) == "authorization" {
			if strings.HasPrefix(value, "Bearer {{.Secret.") {
				token, err := h.resolveSecretValue(ctx, value, roost.Namespace)
				if err != nil {
					return fmt.Errorf("failed to resolve Bearer token: %w", err)
				}
				req.Header.Set("Authorization", token)
				delete(httpSpec.Headers, key) // Remove template to avoid double processing
			}
		}
	}

	return nil
}

// resolveSecretValue resolves secret references in templates
func (h *HTTPChecker) resolveSecretValue(ctx context.Context, template, namespace string) (string, error) {
	if h.client == nil {
		return template, nil // Return as-is if no k8s client available
	}

	// Extract secret name and key from template like "Bearer {{.Secret.secret-name.key-name}}"
	re := regexp.MustCompile(`\{\{\.Secret\.([^.]+)\.([^}]+)\}\}`)
	matches := re.FindStringSubmatch(template)
	if len(matches) != 3 {
		re = regexp.MustCompile(`\{\{\.Secret\.([^}]+)\}\}`)
		matches = re.FindStringSubmatch(template)
		if len(matches) != 2 {
			return template, nil // Not a secret template
		}
		// Default key for simpler template
		matches = append(matches, "token")
	}

	secretName := matches[1]
	secretKey := matches[2]

	// Fetch secret from Kubernetes
	secret := &corev1.Secret{}
	if err := h.client.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	// Get value from secret
	value, exists := secret.Data[secretKey]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s/%s", secretKey, namespace, secretName)
	}

	// Replace template with actual value
	return strings.ReplaceAll(template, matches[0], string(value)), nil
}

// configureClientForCheck configures the HTTP client for the specific check
func (h *HTTPChecker) configureClientForCheck(checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Configure timeout
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}
	h.httpClient.HTTPClient.Timeout = timeout

	// Configure TLS
	if httpSpec.TLS != nil {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: httpSpec.TLS.InsecureSkipVerify,
		}

		// Custom TLS configuration would go here
		// For now, just handle InsecureSkipVerify

		transport := h.httpClient.HTTPClient.Transport.(*http.Transport)
		transport.TLSClientConfig = tlsConfig
	}
}

// validateResponse validates the HTTP response according to the check specification
func (h *HTTPChecker) validateResponse(ctx context.Context, resp *http.Response, checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec, responseTime time.Duration) (*HTTPCheckResult, error) {
	result := &HTTPCheckResult{
		StatusCode:   resp.StatusCode,
		ResponseTime: responseTime,
		Details:      make(map[string]interface{}),
	}

	// Validate status code
	if !h.isValidStatusCode(resp.StatusCode, httpSpec.ExpectedCodes) {
		h.recordError(ctx, "invalid_status_code", nil)
		result.Healthy = false
		result.Message = fmt.Sprintf("Invalid status code: got %d, expected %v", resp.StatusCode, httpSpec.ExpectedCodes)
		result.Details["status_code"] = resp.StatusCode
		result.Details["expected_codes"] = httpSpec.ExpectedCodes
		return result, nil
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &HTTPCheckResult{
			Healthy: false,
			Message: fmt.Sprintf("Failed to read response body: %v", err),
		}, nil
	}

	// Response size will be recorded when metrics are implemented

	// Validate response body if pattern is specified
	if httpSpec.ExpectedBody != "" {
		bodyStr := string(body)
		matched, err := h.matchesPattern(bodyStr, httpSpec.ExpectedBody)
		if err != nil {
			return &HTTPCheckResult{
				Healthy: false,
				Message: fmt.Sprintf("Failed to match response body pattern: %v", err),
			}, nil
		}

		if !matched {
			h.recordError(ctx, "body_pattern_mismatch", nil)
			result.Healthy = false
			result.Message = fmt.Sprintf("Response body does not match expected pattern: %s", httpSpec.ExpectedBody)
			result.Details["body_length"] = len(body)
			result.Details["pattern"] = httpSpec.ExpectedBody
			result.Details["body_preview"] = h.truncateString(bodyStr, 200)
			return result, nil
		}
	}

	result.Healthy = true
	result.Message = "HTTP health check passed"
	result.Details["status_code"] = resp.StatusCode
	result.Details["response_time_ms"] = responseTime.Milliseconds()
	result.Details["response_size_bytes"] = len(body)

	return result, nil
}

// isValidStatusCode checks if the status code is valid according to expectations
func (h *HTTPChecker) isValidStatusCode(statusCode int, expectedCodes []int32) bool {
	// If no expected codes are specified, accept 2xx status codes
	if len(expectedCodes) == 0 {
		return statusCode >= 200 && statusCode < 300
	}

	// Check against expected codes
	for _, code := range expectedCodes {
		if statusCode == int(code) {
			return true
		}
	}

	return false
}

// matchesPattern checks if the text matches the given pattern
func (h *HTTPChecker) matchesPattern(text, pattern string) (bool, error) {
	// Try as regex first
	matched, err := regexp.MatchString(pattern, text)
	if err == nil {
		return matched, nil
	}

	// Fall back to simple string contains
	return strings.Contains(text, pattern), nil
}

// templateURL processes template variables in the URL
func (h *HTTPChecker) templateURL(urlTemplate string, roost *roostv1alpha1.ManagedRoost) (string, error) {
	result := urlTemplate

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

	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
}

// templateHeaderValue processes template variables in header values
func (h *HTTPChecker) templateHeaderValue(ctx context.Context, headerTemplate string, roost *roostv1alpha1.ManagedRoost) (string, error) {
	// Handle secret references
	if strings.Contains(headerTemplate, "{{.Secret.") {
		return h.resolveSecretValue(ctx, headerTemplate, roost.Namespace)
	}

	// Handle other templates similar to URL templating
	return h.templateURL(headerTemplate, roost)
}

// buildCacheKey creates a unique cache key for the health check
func (h *HTTPChecker) buildCacheKey(roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec) string {
	return fmt.Sprintf("%s/%s/%s/%s", roost.Namespace, roost.Name, checkSpec.Name, httpSpec.URL)
}

// getCachedResult retrieves a cached result if it's still valid
func (h *HTTPChecker) getCachedResult(cacheKey string) *HTTPCheckResult {
	if value, ok := h.responseCache.cache.Load(cacheKey); ok {
		cached := value.(*CachedResponse)
		if time.Since(cached.timestamp) < h.responseCache.ttl {
			return &HTTPCheckResult{
				Healthy:   cached.healthy,
				Message:   cached.message,
				Details:   cached.details,
				FromCache: true,
			}
		}
		// Remove expired entry
		h.responseCache.cache.Delete(cacheKey)
	}
	return nil
}

// cacheResult stores a result in the cache
func (h *HTTPChecker) cacheResult(cacheKey string, result *HTTPCheckResult) {
	cached := &CachedResponse{
		healthy:   result.Healthy,
		message:   result.Message,
		details:   result.Details,
		timestamp: time.Now(),
	}
	h.responseCache.cache.Store(cacheKey, cached)
}

// recordError records an error in metrics (simplified for now)
func (h *HTTPChecker) recordError(ctx context.Context, errorType string, err error) {
	// Error recording will be implemented when metrics are fully integrated
	h.logger.Debug("HTTP health check error recorded",
		zap.String("error_type", errorType),
		zap.Error(err),
	)
}

// truncateString truncates a string to the specified length
func (h *HTTPChecker) truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
