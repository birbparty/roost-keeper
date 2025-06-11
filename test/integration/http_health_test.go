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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	httpchecker "github.com/birbparty/roost-keeper/internal/health/http"
)

func TestHTTPHealthChecker_BasicChecks(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		httpSpec       roostv1alpha1.HTTPHealthCheckSpec
		expectHealthy  bool
		expectError    bool
	}{
		{
			name: "successful_200_response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "healthy"}`))
			},
			httpSpec: roostv1alpha1.HTTPHealthCheckSpec{
				Method: "GET",
			},
			expectHealthy: true,
			expectError:   false,
		},
		{
			name: "custom_status_codes",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte("accepted"))
			},
			httpSpec: roostv1alpha1.HTTPHealthCheckSpec{
				Method:        "GET",
				ExpectedCodes: []int32{200, 202},
			},
			expectHealthy: true,
			expectError:   false,
		},
		{
			name: "unexpected_status_code",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("error"))
			},
			httpSpec: roostv1alpha1.HTTPHealthCheckSpec{
				Method:        "GET",
				ExpectedCodes: []int32{200},
			},
			expectHealthy: false,
			expectError:   false,
		},
		{
			name: "response_body_pattern_match",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "healthy", "version": "1.0.0"}`))
			},
			httpSpec: roostv1alpha1.HTTPHealthCheckSpec{
				Method:       "GET",
				ExpectedBody: `"status": "healthy"`,
			},
			expectHealthy: true,
			expectError:   false,
		},
		{
			name: "response_body_pattern_mismatch",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status": "unhealthy"}`))
			},
			httpSpec: roostv1alpha1.HTTPHealthCheckSpec{
				Method:       "GET",
				ExpectedBody: `"status": "healthy"`,
			},
			expectHealthy: false,
			expectError:   false,
		},
		{
			name: "regex_pattern_match",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"uptime": 12345, "status": "running"}`))
			},
			httpSpec: roostv1alpha1.HTTPHealthCheckSpec{
				Method:       "GET",
				ExpectedBody: `"uptime": \d+`,
			},
			expectHealthy: true,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			// Set URL in spec
			tt.httpSpec.URL = server.URL

			// Create checker
			checker, err := httpchecker.NewHTTPChecker(nil, logger)
			require.NoError(t, err)

			// Create test roost
			roost := &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-roost",
					Namespace: "default",
				},
			}

			// Create check spec
			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name:    "test-check",
				Type:    "http",
				Timeout: metav1.Duration{Duration: 5 * time.Second},
			}

			// Execute health check
			ctx := context.Background()
			result, err := checker.CheckHealth(ctx, roost, checkSpec, tt.httpSpec)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, result, "Result should not be nil")
				assert.Equal(t, tt.expectHealthy, result.Healthy)
				assert.NotEmpty(t, result.Message)
				assert.NotNil(t, result.Details)
			}
		})
	}
}

func TestHTTPHealthChecker_Authentication(t *testing.T) {
	logger := zap.NewNop()

	t.Run("bearer_token_header", func(t *testing.T) {
		// Create test server that checks for authorization
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "Bearer test-token-123" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("authorized"))
			} else {
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte("unauthorized"))
			}
		}))
		defer server.Close()

		httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
			URL:    server.URL,
			Method: "GET",
			Headers: map[string]string{
				"Authorization": "Bearer test-token-123",
			},
		}

		checker, err := httpchecker.NewHTTPChecker(nil, logger)
		require.NoError(t, err)

		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
		}

		checkSpec := roostv1alpha1.HealthCheckSpec{
			Name: "auth-test",
			Type: "http",
		}

		ctx := context.Background()
		result, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)

		assert.NoError(t, err)
		assert.True(t, result.Healthy)
	})

	t.Run("custom_headers", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			userAgent := r.Header.Get("User-Agent")

			if apiKey == "secret-key" && userAgent == "roost-keeper/1.0" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			} else {
				w.WriteHeader(http.StatusForbidden)
				w.Write([]byte("forbidden"))
			}
		}))
		defer server.Close()

		httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
			URL:    server.URL,
			Method: "GET",
			Headers: map[string]string{
				"X-API-Key": "secret-key",
			},
		}

		checker, err := httpchecker.NewHTTPChecker(nil, logger)
		require.NoError(t, err)

		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-roost",
				Namespace: "default",
			},
		}

		checkSpec := roostv1alpha1.HealthCheckSpec{
			Name: "header-test",
			Type: "http",
		}

		ctx := context.Background()
		result, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)

		assert.NoError(t, err)
		assert.True(t, result.Healthy)
	})
}

func TestHTTPHealthChecker_URLTemplating(t *testing.T) {
	logger := zap.NewNop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("templated"))
	}))
	defer server.Close()

	tests := []struct {
		name        string
		urlTemplate string
		roost       *roostv1alpha1.ManagedRoost
		expectedURL string
	}{
		{
			name:        "service_name_template",
			urlTemplate: server.URL + "/service/{{.ServiceName}}/health",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-service",
					Namespace: "production",
				},
			},
			expectedURL: server.URL + "/service/my-service/health",
		},
		{
			name:        "namespace_template",
			urlTemplate: server.URL + "/{{.Namespace}}/status",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app",
					Namespace: "staging",
				},
			},
			expectedURL: server.URL + "/staging/status",
		},
		{
			name:        "environment_variables",
			urlTemplate: server.URL + "/${ROOST_NAME}/${ROOST_NAMESPACE}",
			roost: &roostv1alpha1.ManagedRoost{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "web-app",
					Namespace: "test",
				},
			},
			expectedURL: server.URL + "/web-app/test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
				URL:    tt.urlTemplate,
				Method: "GET",
			}

			checker, err := httpchecker.NewHTTPChecker(nil, logger)
			require.NoError(t, err)

			checkSpec := roostv1alpha1.HealthCheckSpec{
				Name: "template-test",
				Type: "http",
			}

			ctx := context.Background()
			result, err := checker.CheckHealth(ctx, tt.roost, checkSpec, httpSpec)

			assert.NoError(t, err)
			assert.True(t, result.Healthy)
		})
	}
}

func TestHTTPHealthChecker_TLSConfiguration(t *testing.T) {
	logger := zap.NewNop()

	// Create HTTPS test server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure"))
	}))
	defer server.Close()

	t.Run("insecure_skip_verify", func(t *testing.T) {
		httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
			URL:    server.URL,
			Method: "GET",
			TLS: &roostv1alpha1.HTTPTLSSpec{
				InsecureSkipVerify: true,
			},
		}

		checker, err := httpchecker.NewHTTPChecker(nil, logger)
		require.NoError(t, err)

		roost := &roostv1alpha1.ManagedRoost{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tls-test",
				Namespace: "default",
			},
		}

		checkSpec := roostv1alpha1.HealthCheckSpec{
			Name: "tls-test",
			Type: "http",
		}

		ctx := context.Background()
		result, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)

		assert.NoError(t, err)
		assert.True(t, result.Healthy)
	})
}

func TestHTTPHealthChecker_Caching(t *testing.T) {
	logger := zap.NewNop()
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("request_%d", requestCount)))
	}))
	defer server.Close()

	httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
		URL:    server.URL,
		Method: "GET",
	}

	checker, err := httpchecker.NewHTTPChecker(nil, logger)
	require.NoError(t, err)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cache-test",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name: "cache-test",
		Type: "http",
	}

	ctx := context.Background()

	// First request should hit the server
	result1, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)
	assert.NoError(t, err)
	assert.True(t, result1.Healthy)
	assert.False(t, result1.FromCache)
	assert.Equal(t, 1, requestCount)

	// Second request should be served from cache
	result2, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)
	assert.NoError(t, err)
	assert.True(t, result2.Healthy)
	assert.True(t, result2.FromCache)
	assert.Equal(t, 1, requestCount) // Should not increment
}

func TestHTTPHealthChecker_RetryLogic(t *testing.T) {
	logger := zap.NewNop()
	attemptCount := 0

	// Server that returns 500 - should NOT retry (health checks evaluate status codes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
		URL:    server.URL,
		Method: "GET",
	}

	checker, err := httpchecker.NewHTTPChecker(nil, logger)
	require.NoError(t, err)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "retry-test",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name: "retry-test",
		Type: "http",
	}

	ctx := context.Background()
	result, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)

	assert.NoError(t, err)
	assert.False(t, result.Healthy)  // Should be unhealthy due to 500 status
	assert.Equal(t, 1, attemptCount) // Should NOT retry on HTTP status codes
	assert.Contains(t, result.Message, "Invalid status code")
}

func TestHTTPHealthChecker_Timeouts(t *testing.T) {
	logger := zap.NewNop()

	// Server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("delayed"))
	}))
	defer server.Close()

	httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
		URL:    server.URL,
		Method: "GET",
	}

	checker, err := httpchecker.NewHTTPChecker(nil, logger)
	require.NoError(t, err)

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout-test",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name:    "timeout-test",
		Type:    "http",
		Timeout: metav1.Duration{Duration: 1 * time.Second},
	}

	ctx := context.Background()
	result, err := checker.CheckHealth(ctx, roost, checkSpec, httpSpec)

	assert.NoError(t, err)
	assert.False(t, result.Healthy)
	assert.Contains(t, result.Message, "HTTP request failed")
}

func TestHTTPHealthChecker_BackwardCompatibility(t *testing.T) {
	logger := zap.NewNop()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compat-test",
			Namespace: "default",
		},
	}

	checkSpec := roostv1alpha1.HealthCheckSpec{
		Name: "compat-test",
		Type: "http",
	}

	httpSpec := roostv1alpha1.HTTPHealthCheckSpec{
		URL:    server.URL,
		Method: "GET",
	}

	ctx := context.Background()

	// Test the backward-compatible function
	healthy, err := httpchecker.ExecuteHTTPCheck(ctx, logger, roost, checkSpec, httpSpec)

	assert.NoError(t, err)
	assert.True(t, healthy)
}
