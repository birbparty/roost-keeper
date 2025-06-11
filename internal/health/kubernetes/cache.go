package kubernetes

import (
	"sync"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ResourceCache provides intelligent caching for Kubernetes health check results
type ResourceCache struct {
	client        client.Client
	logger        *zap.Logger
	healthResults map[string]*CachedHealthResult
	resourceCache map[string]*CachedResource
	mu            sync.RWMutex
	defaultTTL    time.Duration
	maxCacheSize  int
}

// CachedHealthResult represents a cached health check result
type CachedHealthResult struct {
	Result    *HealthResult
	ExpiresAt time.Time
	CreatedAt time.Time
}

// CachedResource represents a cached Kubernetes resource
type CachedResource struct {
	Resource  interface{}
	ExpiresAt time.Time
	CreatedAt time.Time
}

// NewResourceCache creates a new resource cache with watch-based invalidation
func NewResourceCache(k8sClient client.Client, logger *zap.Logger) *ResourceCache {
	return &ResourceCache{
		client:        k8sClient,
		logger:        logger,
		healthResults: make(map[string]*CachedHealthResult),
		resourceCache: make(map[string]*CachedResource),
		defaultTTL:    30 * time.Second,
		maxCacheSize:  1000,
	}
}

// GetHealthResult retrieves a cached health check result
func (rc *ResourceCache) GetHealthResult(key string) *HealthResult {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	cached, exists := rc.healthResults[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		// Don't remove here to avoid deadlock, will be cleaned up later
		return nil
	}

	return cached.Result
}

// SetHealthResult stores a health check result in cache
func (rc *ResourceCache) SetHealthResult(key string, result *HealthResult, ttl time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Clean up expired entries if cache is getting large
	if len(rc.healthResults) > rc.maxCacheSize {
		rc.cleanExpiredHealthResults()
	}

	if ttl <= 0 {
		ttl = rc.defaultTTL
	}

	cached := &CachedHealthResult{
		Result:    result,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}

	rc.healthResults[key] = cached

	rc.logger.Debug("Cached health result",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
		zap.Bool("healthy", result.Healthy),
	)
}

// GetResource retrieves a cached Kubernetes resource
func (rc *ResourceCache) GetResource(key string) interface{} {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	cached, exists := rc.resourceCache[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	return cached.Resource
}

// SetResource stores a Kubernetes resource in cache
func (rc *ResourceCache) SetResource(key string, resource interface{}, ttl time.Duration) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Clean up expired entries if cache is getting large
	if len(rc.resourceCache) > rc.maxCacheSize {
		rc.cleanExpiredResources()
	}

	if ttl <= 0 {
		ttl = rc.defaultTTL
	}

	cached := &CachedResource{
		Resource:  resource,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
	}

	rc.resourceCache[key] = cached

	rc.logger.Debug("Cached resource",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
	)
}

// InvalidateHealthResult removes a specific health result from cache
func (rc *ResourceCache) InvalidateHealthResult(key string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	delete(rc.healthResults, key)

	rc.logger.Debug("Invalidated health result cache",
		zap.String("key", key),
	)
}

// InvalidateResource removes a specific resource from cache
func (rc *ResourceCache) InvalidateResource(key string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	delete(rc.resourceCache, key)

	rc.logger.Debug("Invalidated resource cache",
		zap.String("key", key),
	)
}

// InvalidateAll clears all cached data
func (rc *ResourceCache) InvalidateAll() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.healthResults = make(map[string]*CachedHealthResult)
	rc.resourceCache = make(map[string]*CachedResource)

	rc.logger.Info("Invalidated all cache entries")
}

// CleanExpired removes all expired entries from cache
func (rc *ResourceCache) CleanExpired() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	healthResultsCleaned := rc.cleanExpiredHealthResults()
	resourcesCleaned := rc.cleanExpiredResources()

	if healthResultsCleaned > 0 || resourcesCleaned > 0 {
		rc.logger.Debug("Cleaned expired cache entries",
			zap.Int("health_results_cleaned", healthResultsCleaned),
			zap.Int("resources_cleaned", resourcesCleaned),
		)
	}
}

// GetCacheStats returns statistics about the cache
func (rc *ResourceCache) GetCacheStats() map[string]interface{} {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	now := time.Now()
	expiredHealthResults := 0
	expiredResources := 0

	for _, cached := range rc.healthResults {
		if now.After(cached.ExpiresAt) {
			expiredHealthResults++
		}
	}

	for _, cached := range rc.resourceCache {
		if now.After(cached.ExpiresAt) {
			expiredResources++
		}
	}

	return map[string]interface{}{
		"health_results_total":   len(rc.healthResults),
		"health_results_expired": expiredHealthResults,
		"resources_total":        len(rc.resourceCache),
		"resources_expired":      expiredResources,
		"max_cache_size":         rc.maxCacheSize,
		"default_ttl_seconds":    rc.defaultTTL.Seconds(),
	}
}

// cleanExpiredHealthResults removes expired health results (must be called with lock held)
func (rc *ResourceCache) cleanExpiredHealthResults() int {
	cleaned := 0
	now := time.Now()

	for key, cached := range rc.healthResults {
		if now.After(cached.ExpiresAt) {
			delete(rc.healthResults, key)
			cleaned++
		}
	}

	return cleaned
}

// cleanExpiredResources removes expired resources (must be called with lock held)
func (rc *ResourceCache) cleanExpiredResources() int {
	cleaned := 0
	now := time.Now()

	for key, cached := range rc.resourceCache {
		if now.After(cached.ExpiresAt) {
			delete(rc.resourceCache, key)
			cleaned++
		}
	}

	return cleaned
}

// StartCleanupRoutine starts a background goroutine to periodically clean expired entries
func (rc *ResourceCache) StartCleanupRoutine(interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute // Default cleanup interval
	}

	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			rc.CleanExpired()
		}
	}()

	rc.logger.Info("Started cache cleanup routine",
		zap.Duration("interval", interval),
	)
}
