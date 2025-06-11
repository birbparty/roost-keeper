//go:build integration
// +build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/health/prometheus"
)

func TestPrometheusHealthChecker_BasicQueries(t *testing.T) {
	logger := zap.NewNop()

	// Mock Prometheus server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")

		switch {
		case query == `up{job="test-service"}`:
			// Service is up
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"status": "success",
				"data": {
					"resultType": "vector",
					"result": [
						{
							"metric": {"job": "test-service"},
							"value": [1640995200, "1"]
						}
					]
				}
			}`))
		case query == `avg(container_memory_usage_bytes{pod=~"test-service-.*"}) / 1024 / 1024`:
			// Memory usage: 256 MB
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"status": "success",
				"data": {
					"resultType": "vector",
					"result": [
						{
							"metric": {},
							"value": [1640995200, "268435456"]
						}
					]
				}
			}`))
		case query == `rate(http_requests_total{service="test-service", status=~"5.."}[5m]) / rate(http_requests_total{service="test-service"}[5m])`:
			// Error rate: 0.5% (healthy)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"status": "success",
				"data": {
					"resultType": "vector",
					"result": [
						{
							"metric": {},
							"value": [1640995200, "0.005"]
						}
					]
				}
			}`))
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status": "error", "error": "unknown query"}`))
		}
	}))
	defer mockServer.Close()

	tests := []struct {
		name          string
		query         string
		threshold     string
		operator      string
		expectError   bool
		expectHealthy bool
	}{
		{
			name:          "service up check",
			query:         `up{job="test-service"}`,
			threshold:     "1",
			operator:      "eq",
			expectError:   false,
			expectHealthy: true,
		},
		{
			name:          "memory usage check - healthy",
			query:         `avg(container_memory_usage_bytes{pod=~"test-service-.*"}) / 1024 / 1024`,
			threshold:     "512",
			operator:      "lt",
			expectError:   false,
			expectHealthy: true,
		},
		{
			name:          "memory usage check - unhealthy",
			query:         `avg(container_memory_usage_bytes{pod=~"test-service-.*"}) / 1024 / 1024`,
			threshold:     "200",
			operator:      "lt",
			expectError:   false,
			expectHealthy: false,
		},
		{
			name:          "error rate check - healthy",
			query:         `rate(http_requests_total{service="test-service", status=~"5.."}[5m]) / rate(http_requests_total{service="test-service"}[5m])`,
			threshold:     "0.01",
			operator:      "lt",
			expectError:   false,
			expectHealthy: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test roost
			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
				Spec: roostv1alpha1.ManagedRoostSpec{
					Chart: roostv1alpha1.ChartSpec{
						Repository: roostv1alpha1.ChartRepositorySpec{
							URL: "https://charts.example.com",
						},
						Name:    "test-service",
						Version: "1.0.0",
					},
				},
			}

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name:    tt.name,
				Type:    "prometheus",
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			}

			promSpec := roostv1alpha1.PrometheusHealthCheckSpec{
				Endpoint:  mockServer.URL,
				Query:     tt.query,
				Threshold: tt.threshold,
				Operator:  tt.operator,
			}

			// Execute the check
			healthy, err := prometheus.ExecutePrometheusCheck(
				context.Background(),
				logger,
				nil, // No k8s client needed for basic tests
				roost,
				checkSpec,
				promSpec,
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectHealthy, healthy)
			}
		})
	}
}

func TestPrometheusHealthChecker_Authentication(t *testing.T) {
	logger := zap.NewNop()

	// Mock Prometheus server that checks authentication
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authentication headers
		authHeader := r.Header.Get("Authorization")
		apiKey := r.Header.Get("X-API-Key")

		if authHeader != "Bearer test-token" && apiKey != "test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"status": "error", "error": "unauthorized"}`))
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {},
						"value": [1640995200, "1"]
					}
				]
			}
		}`))
	}))
	defer mockServer.Close()

	// Create fake k8s client with secrets
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus-token",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"token": []byte("test-token"),
		},
	}

	apiKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-credentials",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"key": []byte("test-api-key"),
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret, apiKeySecret).
		Build()

	tests := []struct {
		name      string
		authSpec  *roostv1alpha1.PrometheusAuthSpec
		expectErr bool
	}{
		{
			name: "bearer token from secret",
			authSpec: &roostv1alpha1.PrometheusAuthSpec{
				BearerToken: "{{.Secret.prometheus-token.token}}",
			},
			expectErr: false,
		},
		{
			name: "custom header authentication",
			authSpec: &roostv1alpha1.PrometheusAuthSpec{
				Headers: map[string]string{
					"X-API-Key": "{{.Secret.api-credentials.key}}",
				},
			},
			expectErr: false,
		},
		{
			name: "invalid secret reference",
			authSpec: &roostv1alpha1.PrometheusAuthSpec{
				BearerToken: "{{.Secret.nonexistent.token}}",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			}

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name:    "auth-test",
				Type:    "prometheus",
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			}

			promSpec := roostv1alpha1.PrometheusHealthCheckSpec{
				Endpoint:  mockServer.URL,
				Query:     `up{job="test"}`,
				Threshold: "1",
				Operator:  "eq",
				Auth:      tt.authSpec,
			}

			healthy, err := prometheus.ExecutePrometheusCheck(
				context.Background(),
				logger,
				k8sClient,
				roost,
				checkSpec,
				promSpec,
			)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, healthy)
			}
		})
	}
}

func TestPrometheusHealthChecker_ServiceDiscovery(t *testing.T) {
	logger := zap.NewNop()

	// Mock Prometheus server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {},
						"value": [1640995200, "1"]
					}
				]
			}
		}`))
	}))
	defer mockServer.Close()

	// Parse mock server URL for service creation
	require.NotEmpty(t, mockServer.URL)

	// Create fake k8s client with prometheus service
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	prometheusService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prometheus",
			Namespace: "monitoring",
			Labels: map[string]string{
				"app":       "prometheus",
				"component": "server",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "web",
					Port: 9090,
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(prometheusService).
		Build()

	tests := []struct {
		name             string
		discoverySpec    *roostv1alpha1.PrometheusDiscoverySpec
		fallbackEndpoint string
		expectErr        bool
	}{
		{
			name: "service discovery by name",
			discoverySpec: &roostv1alpha1.PrometheusDiscoverySpec{
				ServiceName:      "prometheus",
				ServiceNamespace: "monitoring",
				ServicePort:      "web",
			},
			expectErr: false,
		},
		{
			name: "service discovery by labels",
			discoverySpec: &roostv1alpha1.PrometheusDiscoverySpec{
				LabelSelector: map[string]string{
					"app":       "prometheus",
					"component": "server",
				},
				ServiceNamespace: "monitoring",
				ServicePort:      "9090",
			},
			expectErr: false,
		},
		{
			name: "fallback to explicit endpoint",
			discoverySpec: &roostv1alpha1.PrometheusDiscoverySpec{
				ServiceName:      "nonexistent",
				ServiceNamespace: "monitoring",
				EnableFallback:   true,
			},
			fallbackEndpoint: mockServer.URL,
			expectErr:        false,
		},
		{
			name: "service not found without fallback",
			discoverySpec: &roostv1alpha1.PrometheusDiscoverySpec{
				ServiceName:      "nonexistent",
				ServiceNamespace: "monitoring",
				EnableFallback:   false,
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			}

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name:    "discovery-test",
				Type:    "prometheus",
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			}

			promSpec := roostv1alpha1.PrometheusHealthCheckSpec{
				ServiceDiscovery: tt.discoverySpec,
				Endpoint:         tt.fallbackEndpoint,
				Query:            `up{job="test"}`,
				Threshold:        "1",
				Operator:         "eq",
			}

			healthy, err := prometheus.ExecutePrometheusCheck(
				context.Background(),
				logger,
				k8sClient,
				roost,
				checkSpec,
				promSpec,
			)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.True(t, healthy)
			}
		})
	}
}

func TestPrometheusHealthChecker_Operators(t *testing.T) {
	logger := zap.NewNop()

	// Mock Prometheus server that returns value 50
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {},
						"value": [1640995200, "50"]
					}
				]
			}
		}`))
	}))
	defer mockServer.Close()

	tests := []struct {
		name          string
		threshold     string
		operator      string
		expectHealthy bool
	}{
		{"greater than - healthy", "30", "gt", true},
		{"greater than - unhealthy", "60", "gt", false},
		{"greater than equal - healthy", "50", "gte", true},
		{"greater than equal - unhealthy", "51", "gte", false},
		{"less than - healthy", "60", "lt", true},
		{"less than - unhealthy", "40", "lt", false},
		{"less than equal - healthy", "50", "lte", true},
		{"less than equal - unhealthy", "49", "lte", false},
		{"equal - healthy", "50", "eq", true},
		{"equal - unhealthy", "51", "eq", false},
		{"not equal - healthy", "51", "ne", true},
		{"not equal - unhealthy", "50", "ne", false},
		// Test symbol operators too
		{"symbol > - healthy", "30", ">", true},
		{"symbol >= - healthy", "50", ">=", true},
		{"symbol < - healthy", "60", "<", true},
		{"symbol <= - healthy", "50", "<=", true},
		{"symbol == - healthy", "50", "==", true},
		{"symbol != - healthy", "51", "!=", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			}

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name:    "operator-test",
				Type:    "prometheus",
				Timeout: metav1.Duration{Duration: 10 * time.Second},
			}

			promSpec := roostv1alpha1.PrometheusHealthCheckSpec{
				Endpoint:  mockServer.URL,
				Query:     `test_metric`,
				Threshold: tt.threshold,
				Operator:  tt.operator,
			}

			healthy, err := prometheus.ExecutePrometheusCheck(
				context.Background(),
				logger,
				nil,
				roost,
				checkSpec,
				promSpec,
			)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectHealthy, healthy,
				"Expected healthy=%v for value=50 %s %s", tt.expectHealthy, tt.operator, tt.threshold)
		})
	}
}

func TestPrometheusHealthChecker_Caching(t *testing.T) {
	logger := zap.NewNop()
	callCount := 0

	// Mock Prometheus server that counts calls
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {},
						"value": [1640995200, "%d"]
					}
				]
			}
		}`, callCount)))
	}))
	defer mockServer.Close()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-service",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "cache-test",
		Type:    "prometheus",
		Timeout: metav1.Duration{Duration: 10 * time.Second},
	}

	// Create checker instance to test caching
	checker := prometheus.NewPrometheusChecker(nil, logger)

	// First call should hit the server
	promSpec := roostv1alpha1.PrometheusHealthCheckSpec{
		Endpoint:  mockServer.URL,
		Query:     `test_metric`,
		Threshold: "1",
		Operator:  "gte",
		CacheTTL:  &metav1.Duration{Duration: 5 * time.Second},
	}

	result1, err := checker.CheckHealth(context.Background(), roost, checkSpec, promSpec)
	require.NoError(t, err)
	assert.True(t, result1.Healthy)
	assert.Equal(t, 1, callCount)
	assert.False(t, result1.FromCache)

	// Second call should use cache
	result2, err := checker.CheckHealth(context.Background(), roost, checkSpec, promSpec)
	require.NoError(t, err)
	assert.True(t, result2.Healthy)
	assert.Equal(t, 1, callCount) // Should not increment
	assert.True(t, result2.FromCache)

	// Wait for cache to expire and call again
	time.Sleep(6 * time.Second)
	result3, err := checker.CheckHealth(context.Background(), roost, checkSpec, promSpec)
	require.NoError(t, err)
	assert.True(t, result3.Healthy)
	assert.Equal(t, 2, callCount) // Should increment again
	assert.False(t, result3.FromCache)
}

func TestPrometheusHealthChecker_QueryTemplating(t *testing.T) {
	logger := zap.NewNop()

	// Mock Prometheus server that echoes the query
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")

		// Verify template replacement worked
		expectedQuery := `up{job="my-test-service", namespace="test-namespace"}`
		if query == expectedQuery {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"status": "success",
				"data": {
					"resultType": "vector",
					"result": [
						{
							"metric": {},
							"value": [1640995200, "1"]
						}
					]
				}
			}`))
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(fmt.Sprintf(`{"status": "error", "error": "unexpected query: %s"}`, query)))
		}
	}))
	defer mockServer.Close()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-test-service",
			Namespace: "test-namespace",
		},
		Status: roostv1alpha1.ManagedRoostStatus{
			HelmRelease: &roostv1alpha1.HelmReleaseStatus{
				Name: "my-test-release",
			},
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "template-test",
		Type:    "prometheus",
		Timeout: metav1.Duration{Duration: 10 * time.Second},
	}

	promSpec := roostv1alpha1.PrometheusHealthCheckSpec{
		Endpoint:  mockServer.URL,
		Query:     `up{job="{{.ServiceName}}", namespace="{{.Namespace}}"}`,
		Threshold: "1",
		Operator:  "eq",
	}

	healthy, err := prometheus.ExecutePrometheusCheck(
		context.Background(),
		logger,
		nil,
		roost,
		checkSpec,
		promSpec,
	)

	assert.NoError(t, err)
	assert.True(t, healthy)
}

func TestPrometheusHealthChecker_ErrorHandling(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name          string
		serverHandler http.HandlerFunc
		expectError   bool
		expectHealthy bool
	}{
		{
			name: "prometheus server error",
			serverHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"status": "error", "error": "internal server error"}`))
			}),
			expectError:   true,
			expectHealthy: false,
		},
		{
			name: "query returns no results",
			serverHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"status": "success",
					"data": {
						"resultType": "vector",
						"result": []
					}
				}`))
			}),
			expectError:   true,
			expectHealthy: false,
		},
		{
			name: "invalid threshold value",
			serverHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"status": "success",
					"data": {
						"resultType": "vector",
						"result": [
							{
								"metric": {},
								"value": [1640995200, "50"]
							}
						]
					}
				}`))
			}),
			expectError:   true,
			expectHealthy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockServer := httptest.NewServer(tt.serverHandler)
			defer mockServer.Close()

			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-service",
					Namespace: "default",
				},
			}

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name:    "error-test",
				Type:    "prometheus",
				Timeout: metav1.Duration{Duration: 5 * time.Second},
			}

			threshold := "50"
			if tt.name == "invalid threshold value" {
				threshold = "invalid-number"
			}

			promSpec := roostv1alpha1.PrometheusHealthCheckSpec{
				Endpoint:  mockServer.URL,
				Query:     `test_metric`,
				Threshold: threshold,
				Operator:  "eq",
			}

			healthy, err := prometheus.ExecutePrometheusCheck(
				context.Background(),
				logger,
				nil,
				roost,
				checkSpec,
				promSpec,
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectHealthy, healthy)
		})
	}
}
