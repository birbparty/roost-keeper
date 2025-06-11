package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// loggingMiddleware provides structured logging for HTTP requests
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		s.logger.Info("HTTP request",
			zap.String("method", param.Method),
			zap.String("path", param.Path),
			zap.Int("status", param.StatusCode),
			zap.Duration("latency", param.Latency),
			zap.String("client_ip", param.ClientIP),
			zap.String("user_agent", param.Request.UserAgent()),
			zap.String("request_id", s.requestID(&gin.Context{Request: param.Request})),
		)
		return ""
	})
}

// recoveryMiddleware handles panics and returns proper error responses
func (s *Server) recoveryMiddleware() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		s.logger.Error("HTTP request panic",
			zap.Any("panic", recovered),
			zap.String("path", c.Request.URL.Path),
			zap.String("method", c.Request.Method),
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Internal server error",
			"message": "The server encountered an unexpected error",
		})
	})
}

// metricsMiddleware records HTTP metrics
func (s *Server) metricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start)

		if s.metrics != nil {
			recordHTTPRequest(s.metrics, c.Request.Context(), c.Request.Method, c.FullPath(), c.Writer.Status(), duration.Seconds())
		}
	}
}

// corsMiddleware handles Cross-Origin Resource Sharing
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range s.config.CORSOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// securityHeadersMiddleware adds security headers
func (s *Server) securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Add Request ID header for tracing
		requestID := s.requestID(c)
		c.Header("X-Request-ID", requestID)
		c.Set("request_id", requestID)

		c.Next()
	}
}

// authMiddleware validates JWT tokens and sets user context
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authorization header required",
			})
			c.Abort()
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Bearer token required",
			})
			c.Abort()
			return
		}

		claims, err := s.authProvider.ValidateToken(c.Request.Context(), token)
		if err != nil {
			s.logger.Warn("Invalid token",
				zap.Error(err),
				zap.String("client_ip", c.ClientIP()))

			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid token",
			})
			c.Abort()
			return
		}

		// Store claims in context for use by handlers
		c.Set("claims", claims)
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("tenant", claims.Tenant)

		c.Next()
	}
}

// rateLimitMiddleware applies rate limiting
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := s.getClientID(c)

		if !s.rateLimiter.Allow(clientID) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "Rate limit exceeded",
				"message":     "Too many requests, please try again later",
				"retry_after": "1s",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// adminAuthMiddleware provides additional authorization for admin endpoints
func (s *Server) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := s.getClaims(c)
		if claims == nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		// Check for admin permissions
		if !s.hasPermission(c, "admin:*") && !s.hasPermission(c, "*") {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Admin privileges required",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// recordHTTPRequest records HTTP metrics
func recordHTTPRequest(m *telemetry.APIMetrics, ctx context.Context, method, path string, statusCode int, duration float64) {
	if m.APIOperations != nil {
		m.APIOperations.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", method),
				attribute.String("path", path),
				attribute.Int("status_code", statusCode),
			))
	}

	if m.APIOperationDuration != nil {
		m.APIOperationDuration.Record(ctx, duration,
			metric.WithAttributes(
				attribute.String("method", method),
				attribute.String("path", path),
				attribute.Int("status_code", statusCode),
			))
	}

	// Record errors for non-2xx status codes
	if statusCode >= 400 && m.APIErrors != nil {
		m.APIErrors.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("method", method),
				attribute.String("path", path),
				attribute.Int("status_code", statusCode),
			))
	}
}
