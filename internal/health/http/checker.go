package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// executeHTTPCheck performs an HTTP health check
func ExecuteHTTPCheck(ctx context.Context, logger *zap.Logger, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "health.http_check", roost.Name, roost.Namespace)
	defer span.End()

	log := logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("url", httpSpec.URL),
		zap.String("method", httpSpec.Method),
	)

	log.Debug("Starting HTTP health check")

	// Process URL template variables
	url := processURLTemplate(httpSpec.URL, roost)

	// Determine HTTP method
	method := httpSpec.Method
	if method == "" {
		method = "GET"
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Add headers
	if httpSpec.Headers != nil {
		for key, value := range httpSpec.Headers {
			req.Header.Set(key, value)
		}
	}

	// Create HTTP client with timeout
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	client := &http.Client{
		Timeout: timeout,
	}

	// Configure TLS if specified
	if httpSpec.TLS != nil && httpSpec.TLS.InsecureSkipVerify {
		// TODO: Implement custom TLS configuration
		log.Warn("TLS configuration not yet implemented")
	}

	log.Debug("Executing HTTP request")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("HTTP request failed", zap.Error(err))
		return false, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	log.Debug("HTTP request completed",
		zap.Int("status_code", resp.StatusCode),
		zap.String("status", resp.Status),
	)

	// Check status code
	if !isValidStatusCode(resp.StatusCode, httpSpec.ExpectedCodes) {
		log.Warn("Unexpected HTTP status code",
			zap.Int("actual", resp.StatusCode),
			zap.Any("expected", httpSpec.ExpectedCodes),
		)
		return false, nil
	}

	// Check response body if expected body pattern is specified
	if httpSpec.ExpectedBody != "" {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			telemetry.RecordSpanError(ctx, err)
			return false, fmt.Errorf("failed to read response body: %w", err)
		}

		bodyStr := string(body)
		matched, err := matchesPattern(bodyStr, httpSpec.ExpectedBody)
		if err != nil {
			telemetry.RecordSpanError(ctx, err)
			return false, fmt.Errorf("failed to match response body pattern: %w", err)
		}

		if !matched {
			log.Warn("Response body does not match expected pattern",
				zap.String("pattern", httpSpec.ExpectedBody),
				zap.String("body_preview", truncateString(bodyStr, 200)),
			)
			return false, nil
		}

		log.Debug("Response body matches expected pattern")
	}

	log.Debug("HTTP health check passed")
	telemetry.RecordSpanSuccess(ctx)
	return true, nil
}

// processURLTemplate processes template variables in the URL
func processURLTemplate(url string, roost *roostv1alpha1.ManagedRoost) string {
	// Replace common template variables
	replacements := map[string]string{
		"${ROOST_NAME}":       roost.Name,
		"${ROOST_NAMESPACE}":  roost.Namespace,
		"{{ROOST_NAME}}":      roost.Name,
		"{{ROOST_NAMESPACE}}": roost.Namespace,
	}

	processed := url
	for placeholder, value := range replacements {
		processed = strings.ReplaceAll(processed, placeholder, value)
	}

	return processed
}

// isValidStatusCode checks if the status code is valid according to expectations
func isValidStatusCode(actual int, expected []int32) bool {
	// If no expected codes are specified, accept 2xx status codes
	if len(expected) == 0 {
		return actual >= 200 && actual < 300
	}

	// Check against expected codes
	for _, code := range expected {
		if actual == int(code) {
			return true
		}
	}

	return false
}

// matchesPattern checks if the text matches the given pattern
func matchesPattern(text, pattern string) (bool, error) {
	// Try as regex first
	matched, err := regexp.MatchString(pattern, text)
	if err == nil {
		return matched, nil
	}

	// Fall back to simple string contains
	return strings.Contains(text, pattern), nil
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
