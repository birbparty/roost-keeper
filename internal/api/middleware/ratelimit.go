package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// RateLimiter provides rate limiting functionality with per-client tracking
type RateLimiter struct {
	limiters map[string]*ClientLimiter
	mutex    sync.RWMutex
	rate     rate.Limit
	burst    int
	cleanup  time.Duration
	stopCh   chan struct{}
}

// ClientLimiter tracks rate limiting for a specific client
type ClientLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter instance
func NewRateLimiter(requestsPerSecond, burst int) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*ClientLimiter),
		rate:     rate.Limit(requestsPerSecond),
		burst:    burst,
		cleanup:  5 * time.Minute, // Clean up unused limiters every 5 minutes
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanupRoutine()

	return rl
}

// Allow checks if a request from the given client ID should be allowed
func (rl *RateLimiter) Allow(clientID string) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	limiter, exists := rl.limiters[clientID]
	if !exists {
		limiter = &ClientLimiter{
			limiter:  rate.NewLimiter(rl.rate, rl.burst),
			lastSeen: time.Now(),
		}
		rl.limiters[clientID] = limiter
	} else {
		limiter.lastSeen = time.Now()
	}

	return limiter.limiter.Allow()
}

// AllowN checks if N requests from the given client ID should be allowed
func (rl *RateLimiter) AllowN(clientID string, n int) bool {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	limiter, exists := rl.limiters[clientID]
	if !exists {
		limiter = &ClientLimiter{
			limiter:  rate.NewLimiter(rl.rate, rl.burst),
			lastSeen: time.Now(),
		}
		rl.limiters[clientID] = limiter
	} else {
		limiter.lastSeen = time.Now()
	}

	return limiter.limiter.AllowN(time.Now(), n)
}

// Reserve reserves tokens for a request from the given client ID
func (rl *RateLimiter) Reserve(clientID string) *rate.Reservation {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	limiter, exists := rl.limiters[clientID]
	if !exists {
		limiter = &ClientLimiter{
			limiter:  rate.NewLimiter(rl.rate, rl.burst),
			lastSeen: time.Now(),
		}
		rl.limiters[clientID] = limiter
	} else {
		limiter.lastSeen = time.Now()
	}

	return limiter.limiter.Reserve()
}

// Wait blocks until a request from the given client ID can proceed
func (rl *RateLimiter) Wait(clientID string) error {
	rl.mutex.Lock()
	limiter, exists := rl.limiters[clientID]
	if !exists {
		limiter = &ClientLimiter{
			limiter:  rate.NewLimiter(rl.rate, rl.burst),
			lastSeen: time.Now(),
		}
		rl.limiters[clientID] = limiter
	} else {
		limiter.lastSeen = time.Now()
	}
	rl.mutex.Unlock()

	return limiter.limiter.Wait(context.Background())
}

// GetStats returns statistics about the rate limiter
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mutex.RLock()
	defer rl.mutex.RUnlock()

	return map[string]interface{}{
		"total_clients":       len(rl.limiters),
		"requests_per_second": float64(rl.rate),
		"burst_size":          rl.burst,
	}
}

// Stop stops the rate limiter and cleanup routine
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// cleanupRoutine removes old client limiters to prevent memory leaks
func (rl *RateLimiter) cleanupRoutine() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupOldLimiters()
		case <-rl.stopCh:
			return
		}
	}
}

// cleanupOldLimiters removes limiters that haven't been used recently
func (rl *RateLimiter) cleanupOldLimiters() {
	rl.mutex.Lock()
	defer rl.mutex.Unlock()

	now := time.Now()
	for clientID, limiter := range rl.limiters {
		if now.Sub(limiter.lastSeen) > rl.cleanup {
			delete(rl.limiters, clientID)
		}
	}
}

// GinMiddleware returns a Gin middleware function for rate limiting
func (rl *RateLimiter) GinMiddleware(getClientID func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := getClientID(c)

		if !rl.Allow(clientID) {
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

// PerIPRateLimiter creates a rate limiter that limits by IP address
func PerIPRateLimiter(requestsPerSecond, burst int) gin.HandlerFunc {
	rl := NewRateLimiter(requestsPerSecond, burst)
	return rl.GinMiddleware(func(c *gin.Context) string {
		return c.ClientIP()
	})
}

// PerUserRateLimiter creates a rate limiter that limits by user ID
func PerUserRateLimiter(requestsPerSecond, burst int) gin.HandlerFunc {
	rl := NewRateLimiter(requestsPerSecond, burst)
	return rl.GinMiddleware(func(c *gin.Context) string {
		// Try to get user ID from context
		if userID, exists := c.Get("user_id"); exists {
			if uid, ok := userID.(string); ok {
				return uid
			}
		}
		// Fallback to IP address
		return c.ClientIP()
	})
}

// CustomRateLimiter creates a rate limiter with a custom client ID extractor
func CustomRateLimiter(requestsPerSecond, burst int, getClientID func(*gin.Context) string) gin.HandlerFunc {
	rl := NewRateLimiter(requestsPerSecond, burst)
	return rl.GinMiddleware(getClientID)
}
