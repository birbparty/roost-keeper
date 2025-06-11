package prometheus

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// QueryCache implements TTL-based caching for Prometheus query results
type QueryCache struct {
	cache  sync.Map
	logger *zap.Logger
}

// CachedResult represents a cached health check result with TTL
type CachedResult struct {
	result    *HealthCheckResult
	timestamp time.Time
	ttl       time.Duration
}

// NewQueryCache creates a new query cache
func NewQueryCache(logger *zap.Logger) *QueryCache {
	cache := &QueryCache{
		logger: logger,
	}

	// Start cleanup goroutine
	go cache.cleanupExpired()

	return cache
}

// Get retrieves a cached result if it exists and is not expired
func (c *QueryCache) Get(key string) *HealthCheckResult {
	if value, ok := c.cache.Load(key); ok {
		cached := value.(*CachedResult)
		if time.Since(cached.timestamp) < cached.ttl {
			// Return a copy to avoid mutation
			return c.copyResult(cached.result)
		}
		// Remove expired entry
		c.cache.Delete(key)
	}
	return nil
}

// Set stores a result in the cache with the specified TTL
func (c *QueryCache) Set(key string, result *HealthCheckResult, ttl time.Duration) {
	cached := &CachedResult{
		result:    c.copyResult(result), // Store a copy to avoid mutation
		timestamp: time.Now(),
		ttl:       ttl,
	}
	c.cache.Store(key, cached)
}

// Delete removes an entry from the cache
func (c *QueryCache) Delete(key string) {
	c.cache.Delete(key)
}

// Clear removes all entries from the cache
func (c *QueryCache) Clear() {
	c.cache.Range(func(key, value interface{}) bool {
		c.cache.Delete(key)
		return true
	})
}

// Size returns the approximate number of entries in the cache
func (c *QueryCache) Size() int {
	count := 0
	c.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// copyResult creates a deep copy of a HealthCheckResult
func (c *QueryCache) copyResult(original *HealthCheckResult) *HealthCheckResult {
	if original == nil {
		return nil
	}

	copy := &HealthCheckResult{
		Healthy:         original.Healthy,
		Message:         original.Message,
		ThresholdMet:    original.ThresholdMet,
		ExecutionTime:   original.ExecutionTime,
		FromCache:       original.FromCache,
		PrometheusURL:   original.PrometheusURL,
		DiscoveryMethod: original.DiscoveryMethod,
	}

	// Copy details map
	if original.Details != nil {
		copy.Details = make(map[string]interface{})
		for k, v := range original.Details {
			copy.Details[k] = v
		}
	}

	// Copy query result
	if original.QueryResult != nil {
		copy.QueryResult = &QueryResult{
			Value:     original.QueryResult.Value,
			Timestamp: original.QueryResult.Timestamp,
			IsVector:  original.QueryResult.IsVector,
		}

		// Copy labels
		if original.QueryResult.Labels != nil {
			copy.QueryResult.Labels = make(map[string]string)
			for k, v := range original.QueryResult.Labels {
				copy.QueryResult.Labels[k] = v
			}
		}

		// Copy vector results
		if original.QueryResult.Vector != nil {
			copy.QueryResult.Vector = make([]VectorResult, len(original.QueryResult.Vector))
			for i, vr := range original.QueryResult.Vector {
				copy.QueryResult.Vector[i] = VectorResult{
					Value:     vr.Value,
					Timestamp: vr.Timestamp,
				}
				if vr.Labels != nil {
					copy.QueryResult.Vector[i].Labels = make(map[string]string)
					for k, v := range vr.Labels {
						copy.QueryResult.Vector[i].Labels[k] = v
					}
				}
			}
		}
	}

	// Copy trend analysis (will be implemented later)
	if original.TrendAnalysis != nil {
		copy.TrendAnalysis = &TrendResult{
			IsImproving: original.TrendAnalysis.IsImproving,
			IsDegrading: original.TrendAnalysis.IsDegrading,
			IsStable:    original.TrendAnalysis.IsStable,
			Slope:       original.TrendAnalysis.Slope,
			Correlation: original.TrendAnalysis.Correlation,
		}
	}

	return copy
}

// cleanupExpired periodically removes expired entries from the cache
func (c *QueryCache) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		expiredKeys := []interface{}{}

		c.cache.Range(func(key, value interface{}) bool {
			cached := value.(*CachedResult)
			if time.Since(cached.timestamp) >= cached.ttl {
				expiredKeys = append(expiredKeys, key)
			}
			return true
		})

		for _, key := range expiredKeys {
			c.cache.Delete(key)
		}

		if len(expiredKeys) > 0 {
			c.logger.Debug("Cleaned up expired cache entries",
				zap.Int("expired_count", len(expiredKeys)),
				zap.Int("remaining_count", c.Size()),
			)
		}
	}
}
