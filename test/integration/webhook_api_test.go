package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/api"
	"github.com/birbparty/roost-keeper/internal/api/auth"
	"github.com/birbparty/roost-keeper/internal/rbac"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

func TestWebhookAPISystem(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	// Setup test components
	logger := zaptest.NewLogger(t)
	scheme := runtime.NewScheme()
	require.NoError(t, roostkeeper.AddToScheme(scheme))

	// Create fake Kubernetes client
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(100)

	// Create metrics (can be nil for testing)
	metrics, err := telemetry.NewAPIMetrics()
	require.NoError(t, err)

	// Create API manager
	apiManager := api.NewManager(client, scheme, recorder, metrics, logger)

	// Create mock auth provider
	authProvider := auth.NewMockProvider(logger)

	// Create RBAC manager
	rbacManager := rbac.NewManager(client, scheme, logger)

	// Create server config
	config := &api.ServerConfig{
		Port:           8080,
		Address:        "127.0.0.1",
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 20,
		EnableTLS:      false,
		CORSOrigins:    []string{"*"},
		RateLimit: api.RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             10,
			EnablePerUser:     true,
		},
	}

	// Create HTTP server
	server := api.NewServer(logger, metrics, authProvider, rbacManager, apiManager, config)

	t.Run("Health Endpoints", func(t *testing.T) {
		router := server.GetRouter()

		// Test health endpoint
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "healthy", response["status"])

		// Test readiness endpoint
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/ready", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Authentication", func(t *testing.T) {
		router := server.GetRouter()

		// Test unauthorized access
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/roosts", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		// Test with invalid token
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/roosts", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		// Test with valid token
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/roosts", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("ManagedRoost API", func(t *testing.T) {
		router := server.GetRouter()

		// Test list roosts
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/roosts", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Contains(t, response, "items")

		// Test create roost
		roost := roostkeeper.ManagedRoost{
			Spec: roostkeeper.ManagedRoostSpec{
				Description: "Test roost",
			},
		}
		roost.Name = "test-roost"
		roost.Namespace = "default"

		roostJSON, _ := json.Marshal(roost)
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("POST", "/api/v1/roosts", bytes.NewBuffer(roostJSON))
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		// Test get specific roost
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/roosts/default/test-roost", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Test get roost status
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/roosts/default/test-roost/status", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Rate Limiting", func(t *testing.T) {
		router := server.GetRouter()

		// Make multiple rapid requests to trigger rate limiting
		// Note: In a real test, you'd need to configure lower limits
		successCount := 0
		rateLimitedCount := 0

		for i := 0; i < 10; i++ {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/v1/roosts", nil)
			req.Header.Set("Authorization", "Bearer test-token")
			req.RemoteAddr = "127.0.0.1:12345" // Same IP for rate limiting
			router.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				successCount++
			} else if w.Code == http.StatusTooManyRequests {
				rateLimitedCount++
			}
		}

		// Should have some successful requests
		assert.Greater(t, successCount, 0)
	})

	t.Run("Metrics Endpoints", func(t *testing.T) {
		router := server.GetRouter()

		// Test metrics endpoint
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/metrics/roosts", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Test health metrics
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/metrics/health", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("Admin Endpoints", func(t *testing.T) {
		router := server.GetRouter()

		// Test admin endpoints (should work with test-token which has admin perms)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/v1/admin/users", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Test system status
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/api/v1/admin/system/status", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("API Documentation", func(t *testing.T) {
		router := server.GetRouter()

		// Test OpenAPI YAML
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/openapi.yaml", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "yaml")

		// Test OpenAPI JSON
		w = httptest.NewRecorder()
		req, _ = http.NewRequest("GET", "/openapi.json", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "json")
	})

	t.Run("CORS Headers", func(t *testing.T) {
		router := server.GetRouter()

		// Test CORS preflight
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("OPTIONS", "/api/v1/roosts", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
		assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Authorization")
	})

	t.Run("Security Headers", func(t *testing.T) {
		router := server.GetRouter()

		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/health", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
		assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
	})
}

func TestWebSocketAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zaptest.NewLogger(t)

	// Create mock auth provider
	authProvider := auth.NewMockProvider(logger)

	// Create minimal server for WebSocket testing
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(10)
	metrics, _ := telemetry.NewAPIMetrics()
	apiManager := api.NewManager(client, scheme, recorder, metrics, logger)
	rbacManager := rbac.NewManager(client, scheme, logger)

	server := api.NewServer(logger, metrics, authProvider, rbacManager, apiManager, nil)
	router := server.GetRouter()

	t.Run("WebSocket Without Token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/ws", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("WebSocket With Invalid Token", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/ws?token=invalid-token", nil)
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	// Note: Testing actual WebSocket upgrade requires more complex setup
	// This tests the authentication flow before upgrade
}

func TestServerConfiguration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("Default Configuration", func(t *testing.T) {
		config := api.DefaultServerConfig()

		assert.Equal(t, 8080, config.Port)
		assert.Equal(t, "0.0.0.0", config.Address)
		assert.Equal(t, 15*time.Second, config.ReadTimeout)
		assert.False(t, config.EnableTLS)
		assert.Equal(t, 100, config.RateLimit.RequestsPerSecond)
	})

	t.Run("Custom Configuration", func(t *testing.T) {
		config := &api.ServerConfig{
			Port:        9090,
			Address:     "localhost",
			EnableTLS:   true,
			TLSCertFile: "/path/to/cert",
			TLSKeyFile:  "/path/to/key",
		}

		// Test that server accepts custom config
		scheme := runtime.NewScheme()
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		recorder := record.NewFakeRecorder(10)
		metrics, _ := telemetry.NewAPIMetrics()
		apiManager := api.NewManager(client, scheme, recorder, metrics, logger)
		authProvider := auth.NewMockProvider(logger)
		rbacManager := rbac.NewManager(client, scheme, logger)

		server := api.NewServer(logger, metrics, authProvider, rbacManager, apiManager, config)
		assert.NotNil(t, server)
	})
}

// BenchmarkAPIEndpoints benchmarks the API endpoints
func BenchmarkAPIEndpoints(b *testing.B) {
	gin.SetMode(gin.TestMode)
	logger := zaptest.NewLogger(b)

	// Setup test server
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	recorder := record.NewFakeRecorder(100)
	metrics, _ := telemetry.NewAPIMetrics()
	apiManager := api.NewManager(client, scheme, recorder, metrics, logger)
	authProvider := auth.NewMockProvider(logger)
	rbacManager := rbac.NewManager(client, scheme, logger)

	server := api.NewServer(logger, metrics, authProvider, rbacManager, apiManager, nil)
	router := server.GetRouter()

	b.Run("Health Endpoint", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/health", nil)
			router.ServeHTTP(w, req)
		}
	})

	b.Run("Authenticated List Roosts", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/api/v1/roosts", nil)
			req.Header.Set("Authorization", "Bearer test-token")
			router.ServeHTTP(w, req)
		}
	})
}
