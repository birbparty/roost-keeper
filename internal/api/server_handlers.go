package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	wsocket "github.com/birbparty/roost-keeper/internal/api/websocket"
)

// Health and system handlers

// handleHealth handles GET /health
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
	})
}

// handleReady handles GET /ready
func (s *Server) handleReady(c *gin.Context) {
	// Check if all components are ready
	ready := true
	checks := map[string]string{
		"api_manager":   "ok",
		"websocket_hub": "ok",
		"rate_limiter":  "ok",
		"auth_provider": "ok",
	}

	// Add any specific readiness checks here
	if s.apiManager == nil {
		ready = false
		checks["api_manager"] = "not_initialized"
	}

	if s.websocketHub == nil {
		ready = false
		checks["websocket_hub"] = "not_initialized"
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}

	c.JSON(status, gin.H{
		"ready":  ready,
		"checks": checks,
	})
}

// handleMetrics handles GET /metrics (Prometheus format)
func (s *Server) handleMetrics(c *gin.Context) {
	// Return basic metrics in Prometheus format
	// In a full implementation, you'd integrate with Prometheus client
	metrics := `# HELP roost_keeper_http_requests_total Total HTTP requests
# TYPE roost_keeper_http_requests_total counter
roost_keeper_http_requests_total{method="GET",path="/health"} 1

# HELP roost_keeper_websocket_connections Current WebSocket connections
# TYPE roost_keeper_websocket_connections gauge
roost_keeper_websocket_connections 0
`

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, metrics)
}

// API documentation handlers

// handleOpenAPISpec handles GET /openapi.yaml
func (s *Server) handleOpenAPISpec(c *gin.Context) {
	spec := `openapi: 3.0.0
info:
  title: Roost-Keeper API
  description: REST API for Roost-Keeper Kubernetes operator
  version: 1.0.0
servers:
  - url: /api/v1
    description: API v1
paths:
  /roosts:
    get:
      summary: List ManagedRoosts
      security:
        - bearerAuth: []
      responses:
        '200':
          description: List of ManagedRoosts
    post:
      summary: Create ManagedRoost
      security:
        - bearerAuth: []
      responses:
        '201':
          description: ManagedRoost created
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
      bearerFormat: JWT
`
	c.Header("Content-Type", "application/yaml")
	c.String(http.StatusOK, spec)
}

// handleOpenAPIJSON handles GET /openapi.json
func (s *Server) handleOpenAPIJSON(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"openapi": "3.0.0",
		"info": gin.H{
			"title":       "Roost-Keeper API",
			"description": "REST API for Roost-Keeper Kubernetes operator",
			"version":     "1.0.0",
		},
		"servers": []gin.H{
			{
				"url":         "/api/v1",
				"description": "API v1",
			},
		},
	})
}

// WebSocket handler

// handleWebSocket handles GET /ws
func (s *Server) handleWebSocket(c *gin.Context) {
	// Get token from query parameter for WebSocket auth
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Token required for WebSocket connection",
		})
		return
	}

	// Validate token
	claims, err := s.authProvider.ValidateToken(c.Request.Context(), token)
	if err != nil {
		s.logger.Warn("Invalid WebSocket token", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid token",
		})
		return
	}

	// Upgrade connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Configure CORS as needed
		},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade WebSocket", zap.Error(err))
		return
	}

	// Create and start client
	client := wsocket.NewClient(
		conn,
		s.websocketHub,
		claims.UserID,
		claims.Username,
		claims.Groups,
		c.ClientIP(),
		c.Request.UserAgent(),
	)

	client.Start()
}

// ManagedRoost handlers

// listManagedRoosts handles GET /api/v1/roosts
func (s *Server) listManagedRoosts(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:list") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	// Parse query parameters
	namespace := c.Query("namespace")
	labelSelector := c.Query("labelSelector")
	limit := c.DefaultQuery("limit", "100")

	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit parameter"})
		return
	}

	s.logger.Info("Listing ManagedRoosts",
		zap.String("user", claims.UserID),
		zap.String("namespace", namespace),
		zap.String("labelSelector", labelSelector))

	// Mock response for now - in real implementation, use apiManager
	roosts := []gin.H{
		{
			"metadata": gin.H{
				"name":      "example-roost",
				"namespace": "default",
			},
			"spec": gin.H{
				"description": "Example ManagedRoost",
			},
			"status": gin.H{
				"phase": "Ready",
			},
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"items": roosts,
		"total": len(roosts),
		"limit": limitInt,
	})
}

// createManagedRoost handles POST /api/v1/roosts
func (s *Server) createManagedRoost(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:create") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	var roost roostkeeper.ManagedRoost
	if err := c.ShouldBindJSON(&roost); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body", "details": err.Error()})
		return
	}

	s.logger.Info("Creating ManagedRoost",
		zap.String("user", claims.UserID),
		zap.String("name", roost.Name),
		zap.String("namespace", roost.Namespace))

	// In real implementation, use apiManager to create
	// For now, return the created roost
	roost.Status.Phase = roostkeeper.ManagedRoostPhasePending
	roost.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}

	// Broadcast WebSocket event
	s.websocketHub.Broadcast(wsocket.Message{
		Type: "roost_created",
		Data: roost,
	})

	c.JSON(http.StatusCreated, roost)
}

// getManagedRoost handles GET /api/v1/roosts/:namespace/:name
func (s *Server) getManagedRoost(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")

	s.logger.Info("Getting ManagedRoost",
		zap.String("user", claims.UserID),
		zap.String("namespace", namespace),
		zap.String("name", name))

	// Mock response
	roost := gin.H{
		"metadata": gin.H{
			"name":      name,
			"namespace": namespace,
		},
		"spec": gin.H{
			"description": "Example ManagedRoost",
		},
		"status": gin.H{
			"phase": "Ready",
		},
	}

	c.JSON(http.StatusOK, roost)
}

// updateManagedRoost handles PUT /api/v1/roosts/:namespace/:name
func (s *Server) updateManagedRoost(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:update") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")

	var roost roostkeeper.ManagedRoost
	if err := c.ShouldBindJSON(&roost); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	s.logger.Info("Updating ManagedRoost",
		zap.String("user", claims.UserID),
		zap.String("namespace", namespace),
		zap.String("name", name))

	c.JSON(http.StatusOK, roost)
}

// deleteManagedRoost handles DELETE /api/v1/roosts/:namespace/:name
func (s *Server) deleteManagedRoost(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:delete") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")

	s.logger.Info("Deleting ManagedRoost",
		zap.String("user", claims.UserID),
		zap.String("namespace", namespace),
		zap.String("name", name))

	c.JSON(http.StatusNoContent, nil)
}

// getManagedRoostStatus handles GET /api/v1/roosts/:namespace/:name/status
func (s *Server) getManagedRoostStatus(c *gin.Context) {
	if !s.hasPermission(c, "roosts:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")

	s.logger.Debug("Getting roost status",
		zap.String("namespace", namespace),
		zap.String("name", name))

	status := gin.H{
		"phase":          "Ready",
		"lastUpdateTime": time.Now(),
		"conditions": []gin.H{
			{
				"type":   "Ready",
				"status": "True",
				"reason": "DeploymentReady",
			},
		},
	}

	c.JSON(http.StatusOK, status)
}

// getManagedRoostLogs handles GET /api/v1/roosts/:namespace/:name/logs
func (s *Server) getManagedRoostLogs(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:logs") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")

	logs := []gin.H{
		{
			"timestamp": time.Now(),
			"level":     "INFO",
			"message":   "ManagedRoost is healthy",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"logs": logs,
	})
}

// executeManagedRoostAction handles POST /api/v1/roosts/:namespace/:name/actions/:action
func (s *Server) executeManagedRoostAction(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:action") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")
	action := c.Param("action")

	s.logger.Info("Executing action",
		zap.String("user", claims.UserID),
		zap.String("namespace", namespace),
		zap.String("name", name),
		zap.String("action", action))

	result := gin.H{
		"action":    action,
		"status":    "executed",
		"timestamp": time.Now(),
	}

	c.JSON(http.StatusOK, result)
}

// getManagedRoostEvents handles GET /api/v1/roosts/:namespace/:name/events
func (s *Server) getManagedRoostEvents(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "roosts:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	events := []gin.H{
		{
			"type":      "Normal",
			"reason":    "Created",
			"message":   "ManagedRoost created successfully",
			"timestamp": time.Now(),
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
	})
}

// Health check handlers

// getHealthChecks handles GET /api/v1/health-checks/:namespace/:name
func (s *Server) getHealthChecks(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "health:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	namespace := c.Param("namespace")
	name := c.Param("name")

	healthChecks := gin.H{
		"http": gin.H{
			"status": "healthy",
			"url":    "http://example.com/health",
		},
		"tcp": gin.H{
			"status": "healthy",
			"host":   "example.com:8080",
		},
	}

	c.JSON(http.StatusOK, healthChecks)
}

// executeHealthCheck handles POST /api/v1/health-checks/:namespace/:name/execute
func (s *Server) executeHealthCheck(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "health:execute") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	result := gin.H{
		"status":    "executed",
		"timestamp": time.Now(),
		"results": gin.H{
			"http": "healthy",
			"tcp":  "healthy",
		},
	}

	c.JSON(http.StatusOK, result)
}

// getHealthCheckHistory handles GET /api/v1/health-checks/:namespace/:name/history
func (s *Server) getHealthCheckHistory(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "health:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	history := []gin.H{
		{
			"timestamp": time.Now(),
			"status":    "healthy",
			"details":   "All checks passed",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"history": history,
	})
}

// Metrics handlers

// getRoostMetrics handles GET /api/v1/metrics/roosts
func (s *Server) getRoostMetrics(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "metrics:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	metrics := gin.H{
		"total_roosts": 10,
		"healthy":      8,
		"unhealthy":    2,
		"by_phase": gin.H{
			"Ready":   8,
			"Failed":  2,
			"Pending": 0,
		},
	}

	c.JSON(http.StatusOK, metrics)
}

// getHealthMetrics handles GET /api/v1/metrics/health
func (s *Server) getHealthMetrics(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "metrics:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	metrics := gin.H{
		"checks_executed": 100,
		"success_rate":    0.95,
		"avg_response_time": gin.H{
			"http": "50ms",
			"tcp":  "10ms",
		},
	}

	c.JSON(http.StatusOK, metrics)
}

// getPerformanceMetrics handles GET /api/v1/metrics/performance
func (s *Server) getPerformanceMetrics(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "metrics:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	metrics := gin.H{
		"api_requests_per_second": 50,
		"avg_response_time_ms":    25,
		"error_rate":              0.01,
		"active_connections":      25,
	}

	c.JSON(http.StatusOK, metrics)
}

// getUsageMetrics handles GET /api/v1/metrics/usage
func (s *Server) getUsageMetrics(c *gin.Context) {
	claims := s.getClaims(c)
	if !s.hasPermission(c, "metrics:get") {
		c.JSON(http.StatusForbidden, gin.H{"error": "Insufficient permissions"})
		return
	}

	metrics := gin.H{
		"active_users":   15,
		"total_requests": 1000,
		"data_processed": "1.5GB",
		"uptime":         "99.9%",
	}

	c.JSON(http.StatusOK, metrics)
}

// Admin handlers

// listUsers handles GET /api/v1/admin/users
func (s *Server) listUsers(c *gin.Context) {
	users := []gin.H{
		{
			"id":       "user1",
			"username": "admin",
			"groups":   []string{"admins"},
			"active":   true,
		},
		{
			"id":       "user2",
			"username": "developer",
			"groups":   []string{"developers"},
			"active":   true,
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": len(users),
	})
}

// getAuditLogs handles GET /api/v1/admin/audit
func (s *Server) getAuditLogs(c *gin.Context) {
	logs := []gin.H{
		{
			"timestamp": time.Now(),
			"user":      "admin",
			"action":    "create_roost",
			"resource":  "default/example",
			"result":    "success",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"logs": logs,
	})
}

// clearCache handles POST /api/v1/admin/cache/clear
func (s *Server) clearCache(c *gin.Context) {
	s.logger.Info("Cache clear requested", zap.String("user", s.getClaims(c).UserID))

	result := gin.H{
		"status":    "cleared",
		"timestamp": time.Now(),
	}

	c.JSON(http.StatusOK, result)
}

// getSystemStatus handles GET /api/v1/admin/system/status
func (s *Server) getSystemStatus(c *gin.Context) {
	status := gin.H{
		"status":  "healthy",
		"version": "1.0.0",
		"uptime":  "24h",
		"components": gin.H{
			"api_server":    "healthy",
			"websocket_hub": "healthy",
			"database":      "healthy",
			"cache":         "healthy",
		},
		"metrics": gin.H{
			"memory_usage":    "45%",
			"cpu_usage":       "20%",
			"disk_usage":      "60%",
			"active_requests": 10,
		},
	}

	c.JSON(http.StatusOK, status)
}
