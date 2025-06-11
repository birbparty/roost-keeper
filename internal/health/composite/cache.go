package composite

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// CacheEntry represents a cached health check result
type CacheEntry struct {
	Result    *HealthResult `json:"result"`
	Timestamp time.Time     `json:"timestamp"`
	TTL       time.Duration `json:"ttl"`
	HitCount  int64         `json:"hit_count"`
}

// Cache provides TTL-based caching for health check results
type Cache struct {
	entries map[string]*CacheEntry
	mutex   sync.RWMutex
	logger  *zap.Logger
}

// NewCache creates a new cache instance
func NewCache(logger *zap.Logger) *Cache {
	cache := &Cache{
		entries: make(map[string]*CacheEntry),
		logger:  logger.With(zap.String("component", "health_cache")),
	}

	// Start cleanup goroutine
	go cache.cleanupExpiredEntries()

	return cache
}

// Get retrieves a cached health check result if it exists and is not expired
func (c *Cache) Get(checkName string) *HealthResult {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.entries[checkName]
	if !exists {
		return nil
	}

	// Check if entry has expired
	if time.Since(entry.Timestamp) > entry.TTL {
		// Entry expired, remove it (we'll do this with write lock later)
		go c.removeExpired(checkName)
		return nil
	}

	// Increment hit count
	entry.HitCount++

	c.logger.Debug("Cache hit for health check",
		zap.String("check_name", checkName),
		zap.Int64("hit_count", entry.HitCount),
		zap.Duration("age", time.Since(entry.Timestamp)))

	return entry.Result
}

// Set stores a health check result in the cache with the specified TTL
func (c *Cache) Set(checkName string, result *HealthResult, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.entries[checkName] = &CacheEntry{
		Result:    result,
		Timestamp: time.Now(),
		TTL:       ttl,
		HitCount:  0,
	}

	c.logger.Debug("Cached health check result",
		zap.String("check_name", checkName),
		zap.Duration("ttl", ttl),
		zap.Bool("healthy", result.Healthy))
}

// Remove removes a specific cache entry
func (c *Cache) Remove(checkName string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.entries, checkName)

	c.logger.Debug("Removed cache entry",
		zap.String("check_name", checkName))
}

// Clear removes all cache entries
func (c *Cache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entryCount := len(c.entries)
	c.entries = make(map[string]*CacheEntry)

	c.logger.Info("Cleared cache",
		zap.Int("entries_removed", entryCount))
}

// GetInfo returns cache information for a specific health check
func (c *Cache) GetInfo(checkName string) *CacheInfo {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.entries[checkName]
	if !exists {
		return &CacheInfo{
			Cached: false,
		}
	}

	return &CacheInfo{
		Cached:    true,
		CacheTime: entry.Timestamp,
		TTL:       entry.TTL,
		HitCount:  entry.HitCount,
	}
}

// GetStats returns overall cache statistics
func (c *Cache) GetStats() map[string]interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	totalEntries := len(c.entries)
	totalHits := int64(0)
	expiredEntries := 0
	now := time.Now()

	for _, entry := range c.entries {
		totalHits += entry.HitCount
		if now.Sub(entry.Timestamp) > entry.TTL {
			expiredEntries++
		}
	}

	return map[string]interface{}{
		"total_entries":   totalEntries,
		"expired_entries": expiredEntries,
		"total_hits":      totalHits,
		"hit_ratio":       c.calculateHitRatio(),
	}
}

// calculateHitRatio calculates the cache hit ratio
func (c *Cache) calculateHitRatio() float64 {
	if len(c.entries) == 0 {
		return 0.0
	}

	totalHits := int64(0)
	totalRequests := int64(0)

	for _, entry := range c.entries {
		totalHits += entry.HitCount
		totalRequests += entry.HitCount + 1 // +1 for the initial miss
	}

	if totalRequests == 0 {
		return 0.0
	}

	return float64(totalHits) / float64(totalRequests)
}

// removeExpired removes an expired entry (called from goroutine)
func (c *Cache) removeExpired(checkName string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, exists := c.entries[checkName]
	if !exists {
		return
	}

	// Double-check expiration with write lock
	if time.Since(entry.Timestamp) > entry.TTL {
		delete(c.entries, checkName)
		c.logger.Debug("Removed expired cache entry",
			zap.String("check_name", checkName),
			zap.Duration("age", time.Since(entry.Timestamp)))
	}
}

// cleanupExpiredEntries periodically removes expired entries
func (c *Cache) cleanupExpiredEntries() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.performCleanup()
	}
}

// performCleanup removes all expired entries
func (c *Cache) performCleanup() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	now := time.Now()
	expiredKeys := make([]string, 0)

	// Find expired entries
	for checkName, entry := range c.entries {
		if now.Sub(entry.Timestamp) > entry.TTL {
			expiredKeys = append(expiredKeys, checkName)
		}
	}

	// Remove expired entries
	for _, key := range expiredKeys {
		delete(c.entries, key)
	}

	if len(expiredKeys) > 0 {
		c.logger.Debug("Cleaned up expired cache entries",
			zap.Int("expired_count", len(expiredKeys)))
	}
}

// IsExpired checks if a cache entry is expired
func (c *Cache) IsExpired(checkName string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.entries[checkName]
	if !exists {
		return true
	}

	return time.Since(entry.Timestamp) > entry.TTL
}

// GetExpiration returns the expiration time for a cache entry
func (c *Cache) GetExpiration(checkName string) *time.Time {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.entries[checkName]
	if !exists {
		return nil
	}

	expiration := entry.Timestamp.Add(entry.TTL)
	return &expiration
}

// GetSize returns the number of entries in the cache
func (c *Cache) GetSize() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	return len(c.entries)
}

// UpdateTTL updates the TTL for an existing cache entry
func (c *Cache) UpdateTTL(checkName string, newTTL time.Duration) bool {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	entry, exists := c.entries[checkName]
	if !exists {
		return false
	}

	entry.TTL = newTTL
	c.logger.Debug("Updated cache entry TTL",
		zap.String("check_name", checkName),
		zap.Duration("new_ttl", newTTL))

	return true
}
