package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/birbparty/roost-keeper/internal/api/auth"
	"github.com/birbparty/roost-keeper/internal/api/middleware"
	wsocket "github.com/birbparty/roost-keeper/internal/api/websocket"
	"github.com/birbparty/roost-keeper/internal/rbac"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Server provides HTTP REST API and WebSocket functionality for Roost-Keeper
type Server struct {
	router       *gin.Engine
	logger       *zap.Logger
	metrics      *telemetry.APIMetrics
	authProvider auth.Provider
	rbacManager  *rbac.Manager
	rateLimiter  *middleware.RateLimiter
	websocketHub *wsocket.Hub
	apiManager   *Manager
	server       *http.Server
	config       *ServerConfig
}

// ServerConfig contains configuration for the HTTP server
type ServerConfig struct {
	Port           int             `json:"port" yaml:"port"`
	Address        string          `json:"address" yaml:"address"`
	ReadTimeout    time.Duration   `json:"readTimeout" yaml:"readTimeout"`
	WriteTimeout   time.Duration   `json:"writeTimeout" yaml:"writeTimeout"`
	IdleTimeout    time.Duration   `json:"idleTimeout" yaml:"idleTimeout"`
	MaxHeaderBytes int             `json:"maxHeaderBytes" yaml:"maxHeaderBytes"`
	EnableTLS      bool            `json:"enableTLS" yaml:"enableTLS"`
	TLSCertFile    string          `json:"tlsCertFile" yaml:"tlsCertFile"`
	TLSKeyFile     string          `json:"tlsKeyFile" yaml:"tlsKeyFile"`
	CORSOrigins    []string        `json:"corsOrigins" yaml:"corsOrigins"`
	RateLimit      RateLimitConfig `json:"rateLimit" yaml:"rateLimit"`
}

// RateLimitConfig contains rate limiting configuration
type RateLimitConfig struct {
	RequestsPerSecond int  `json:"requestsPerSecond" yaml:"requestsPerSecond"`
	Burst             int  `json:"burst" yaml:"burst"`
	EnablePerUser     bool `json:"enablePerUser" yaml:"enablePerUser"`
}

// NewServer creates a new HTTP server instance
func NewServer(
	logger *zap.Logger,
	metrics *telemetry.APIMetrics,
	authProvider auth.Provider,
	rbacManager *rbac.Manager,
	apiManager *Manager,
	config *ServerConfig,
) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}

	server := &Server{
		logger:       logger,
		metrics:      metrics,
		authProvider: authProvider,
		rbacManager:  rbacManager,
		apiManager:   apiManager,
		config:       config,
		rateLimiter:  middleware.NewRateLimiter(config.RateLimit.RequestsPerSecond, config.RateLimit.Burst),
		websocketHub: wsocket.NewHub(logger),
	}

	server.setupRouter()
	server.setupHTTPServer()

	return server
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:           8080,
		Address:        "0.0.0.0",
		ReadTimeout:    15 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
		EnableTLS:      false,
		CORSOrigins:    []string{"*"},
		RateLimit: RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             10,
			EnablePerUser:     true,
		},
	}
}

// setupRouter configures the HTTP router with all routes and middleware
func (s *Server) setupRouter() {
	// Set Gin mode based on log level
	gin.SetMode(gin.ReleaseMode)

	s.router = gin.New()

	// Global middleware stack
	s.router.Use(s.loggingMiddleware())
	s.router.Use(s.recoveryMiddleware())
	s.router.Use(s.metricsMiddleware())
	s.router.Use(s.corsMiddleware())
	s.router.Use(s.securityHeadersMiddleware())

	// Health endpoint (no auth required)
	s.router.GET("/health", s.handleHealth)
	s.router.GET("/ready", s.handleReady)
	s.router.GET("/metrics", s.handleMetrics)

	// API documentation endpoints
	s.router.Static("/docs", "./docs/api")
	s.router.GET("/openapi.yaml", s.handleOpenAPISpec)
	s.router.GET("/openapi.json", s.handleOpenAPIJSON)

	// WebSocket endpoint (requires token auth via query parameter)
	s.router.GET("/ws", s.handleWebSocket)

	// API v1 routes with authentication and rate limiting
	v1 := s.router.Group("/api/v1")
	v1.Use(s.rateLimitMiddleware())
	v1.Use(s.authMiddleware())
	{
		// ManagedRoost endpoints
		s.setupManagedRoostRoutes(v1)

		// Health check endpoints
		s.setupHealthCheckRoutes(v1)

		// Metrics endpoints
		s.setupMetricsRoutes(v1)

		// Admin endpoints
		s.setupAdminRoutes(v1)
	}
}

// setupManagedRoostRoutes configures ManagedRoost-related API routes
func (s *Server) setupManagedRoostRoutes(group *gin.RouterGroup) {
	roosts := group.Group("/roosts")
	{
		roosts.GET("", s.listManagedRoosts)
		roosts.POST("", s.createManagedRoost)
		roosts.GET("/:namespace/:name", s.getManagedRoost)
		roosts.PUT("/:namespace/:name", s.updateManagedRoost)
		roosts.DELETE("/:namespace/:name", s.deleteManagedRoost)
		roosts.GET("/:namespace/:name/status", s.getManagedRoostStatus)
		roosts.GET("/:namespace/:name/logs", s.getManagedRoostLogs)
		roosts.POST("/:namespace/:name/actions/:action", s.executeManagedRoostAction)
		roosts.GET("/:namespace/:name/events", s.getManagedRoostEvents)
	}
}

// setupHealthCheckRoutes configures health check API routes
func (s *Server) setupHealthCheckRoutes(group *gin.RouterGroup) {
	health := group.Group("/health-checks")
	{
		health.GET("/:namespace/:name", s.getHealthChecks)
		health.POST("/:namespace/:name/execute", s.executeHealthCheck)
		health.GET("/:namespace/:name/history", s.getHealthCheckHistory)
	}
}

// setupMetricsRoutes configures metrics API routes
func (s *Server) setupMetricsRoutes(group *gin.RouterGroup) {
	metrics := group.Group("/metrics")
	{
		metrics.GET("/roosts", s.getRoostMetrics)
		metrics.GET("/health", s.getHealthMetrics)
		metrics.GET("/performance", s.getPerformanceMetrics)
		metrics.GET("/usage", s.getUsageMetrics)
	}
}

// setupAdminRoutes configures administrative API routes
func (s *Server) setupAdminRoutes(group *gin.RouterGroup) {
	admin := group.Group("/admin")
	admin.Use(s.adminAuthMiddleware()) // Additional admin authorization
	{
		admin.GET("/users", s.listUsers)
		admin.GET("/audit", s.getAuditLogs)
		admin.POST("/cache/clear", s.clearCache)
		admin.GET("/system/status", s.getSystemStatus)
	}
}

// setupHTTPServer configures the underlying HTTP server
func (s *Server) setupHTTPServer() {
	s.server = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", s.config.Address, s.config.Port),
		Handler:        s.router,
		ReadTimeout:    s.config.ReadTimeout,
		WriteTimeout:   s.config.WriteTimeout,
		IdleTimeout:    s.config.IdleTimeout,
		MaxHeaderBytes: s.config.MaxHeaderBytes,
	}
}

// Start begins serving HTTP requests
func (s *Server) Start(ctx context.Context) error {
	// Start WebSocket hub
	go s.websocketHub.Run(ctx)

	s.logger.Info("Starting HTTP server",
		zap.String("address", s.server.Addr),
		zap.Bool("tls_enabled", s.config.EnableTLS))

	var err error
	if s.config.EnableTLS {
		err = s.server.ListenAndServeTLS(s.config.TLSCertFile, s.config.TLSKeyFile)
	} else {
		err = s.server.ListenAndServe()
	}

	if err != nil && err != http.ErrServerClosed {
		s.logger.Error("HTTP server failed", zap.Error(err))
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the HTTP server
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP server")

	// Stop WebSocket hub
	s.websocketHub.Stop()

	// Shutdown HTTP server
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("HTTP server shutdown failed", zap.Error(err))
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	s.logger.Info("HTTP server shutdown complete")
	return nil
}

// GetRouter returns the configured Gin router (for testing)
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// Helper methods for extracting request context
func (s *Server) getClaims(c *gin.Context) *auth.Claims {
	claims, exists := c.Get("claims")
	if !exists {
		return nil
	}
	return claims.(*auth.Claims)
}

func (s *Server) getClientID(c *gin.Context) string {
	claims := s.getClaims(c)
	if claims != nil && claims.UserID != "" {
		return claims.UserID
	}
	return c.ClientIP()
}

func (s *Server) hasPermission(c *gin.Context, permission string) bool {
	claims := s.getClaims(c)
	if claims == nil {
		return false
	}

	// Check RBAC permissions
	// Note: rbac.Manager doesn't have CheckPermission method yet
	// This is a placeholder for future RBAC integration
	if s.rbacManager != nil {
		s.logger.Debug("RBAC check requested",
			zap.String("user", claims.UserID),
			zap.String("permission", permission))
		// For now, allow all authenticated users
		// In a real implementation, integrate with RBAC system
	}

	// Fallback to basic permission checking
	for _, perm := range claims.Permissions {
		if perm == permission || perm == "*" {
			return true
		}
	}

	return false
}

// requestID generates or extracts a request ID for tracing
func (s *Server) requestID(c *gin.Context) string {
	requestID := c.GetHeader("X-Request-ID")
	if requestID == "" {
		requestID = c.GetHeader("X-Trace-ID")
	}
	if requestID == "" {
		requestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return requestID
}
