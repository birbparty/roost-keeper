package performance

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ResourceCache provides intelligent caching for Kubernetes resources and other data
type ResourceCache struct {
	cache      map[string]*CacheEntry
	mutex      sync.RWMutex
	logger     *zap.Logger
	stats      *CacheStats
	maxSize    int
	defaultTTL time.Duration

	// Lifecycle management
	ctx           context.Context
	cancel        context.CancelFunc
	cleanupTicker *time.Ticker
}

// CacheEntry represents a single cached item
type CacheEntry struct {
	Key         string
	Value       interface{}
	Timestamp   time.Time
	TTL         time.Duration
	AccessCount int64
	LastAccess  time.Time
	Size        int64 // Estimated size in bytes
}

// CacheStats contains metrics about cache performance
type CacheStats struct {
	Size          int     `json:"size"`
	MaxSize       int     `json:"maxSize"`
	HitRate       float64 `json:"hitRate"`
	MissRate      float64 `json:"missRate"`
	TotalRequests int64   `json:"totalRequests"`
	Hits          int64   `json:"hits"`
	Misses        int64   `json:"misses"`
	Evictions     int64   `json:"evictions"`

	// Internal tracking
	mutex sync.RWMutex
}

// CachePolicy defines cache behavior policies
type CachePolicy struct {
	DefaultTTL      time.Duration
	MaxTTL          time.Duration
	MaxSize         int
	EvictionPolicy  string // "lru", "lfu", "ttl"
	CleanupInterval time.Duration
}

// DefaultCachePolicy returns the default cache configuration
func DefaultCachePolicy() *CachePolicy {
	return &CachePolicy{
		DefaultTTL:      5 * time.Minute,
		MaxTTL:          30 * time.Minute,
		MaxSize:         10000,
		EvictionPolicy:  "lru",
		CleanupInterval: 1 * time.Minute,
	}
}

// NewResourceCache creates a new resource cache with the specified configuration
func NewResourceCache(maxSize int, defaultTTL time.Duration, logger *zap.Logger) *ResourceCache {
	return &ResourceCache{
		cache:      make(map[string]*CacheEntry),
		logger:     logger,
		maxSize:    maxSize,
		defaultTTL: defaultTTL,
		stats: &CacheStats{
			MaxSize: maxSize,
		},
	}
}

// Start initializes the resource cache and starts background cleanup
func (rc *ResourceCache) Start(ctx context.Context) error {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	rc.ctx, rc.cancel = context.WithCancel(ctx)
	rc.cleanupTicker = time.NewTicker(1 * time.Minute)

	// Start cleanup goroutine
	go rc.cleanupLoop()

	rc.logger.Info("Resource cache started",
		zap.Int("max_size", rc.maxSize),
		zap.Duration("default_ttl", rc.defaultTTL))

	return nil
}

// Stop gracefully shuts down the resource cache
func (rc *ResourceCache) Stop() error {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	if rc.cancel != nil {
		rc.cancel()
	}

	if rc.cleanupTicker != nil {
		rc.cleanupTicker.Stop()
	}

	rc.logger.Info("Resource cache stopped")
	return nil
}

// Get retrieves a value from the cache
func (rc *ResourceCache) Get(key string) (interface{}, bool) {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	rc.incrementTotalRequests()

	entry, exists := rc.cache[key]
	if !exists {
		rc.incrementMisses()
		return nil, false
	}

	// Check if entry has expired
	if rc.isExpired(entry) {
		rc.mutex.RUnlock()
		rc.mutex.Lock()
		delete(rc.cache, key)
		rc.incrementEvictions()
		rc.mutex.Unlock()
		rc.mutex.RLock()
		rc.incrementMisses()
		return nil, false
	}

	// Update access statistics
	entry.AccessCount++
	entry.LastAccess = time.Now()

	rc.incrementHits()
	return entry.Value, true
}

// Set stores a value in the cache with default TTL
func (rc *ResourceCache) Set(key string, value interface{}) {
	rc.SetWithTTL(key, value, rc.defaultTTL)
}

// SetWithTTL stores a value in the cache with custom TTL
func (rc *ResourceCache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	now := time.Now()
	entry := &CacheEntry{
		Key:         key,
		Value:       value,
		Timestamp:   now,
		TTL:         ttl,
		AccessCount: 1,
		LastAccess:  now,
		Size:        rc.estimateSize(value),
	}

	// Check if we need to evict entries
	if len(rc.cache) >= rc.maxSize {
		rc.evictEntries(1)
	}

	rc.cache[key] = entry
	rc.updateCacheSize()

	rc.logger.Debug("Cache entry added",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
		zap.Int64("estimated_size", entry.Size))
}

// Delete removes a value from the cache
func (rc *ResourceCache) Delete(key string) {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	if _, exists := rc.cache[key]; exists {
		delete(rc.cache, key)
		rc.updateCacheSize()
		rc.logger.Debug("Cache entry deleted", zap.String("key", key))
	}
}

// Clear removes all entries from the cache
func (rc *ResourceCache) Clear() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	entryCount := len(rc.cache)
	rc.cache = make(map[string]*CacheEntry)
	rc.updateCacheSize()

	rc.logger.Info("Cache cleared", zap.Int("entries_removed", entryCount))
}

// GetStats returns current cache statistics
func (rc *ResourceCache) GetStats() CacheStats {
	rc.stats.mutex.RLock()
	defer rc.stats.mutex.RUnlock()

	rc.mutex.RLock()
	size := len(rc.cache)
	rc.mutex.RUnlock()

	// Calculate hit rate
	hitRate := 0.0
	missRate := 0.0
	if rc.stats.TotalRequests > 0 {
		hitRate = float64(rc.stats.Hits) / float64(rc.stats.TotalRequests)
		missRate = float64(rc.stats.Misses) / float64(rc.stats.TotalRequests)
	}

	return CacheStats{
		Size:          size,
		MaxSize:       rc.stats.MaxSize,
		HitRate:       hitRate,
		MissRate:      missRate,
		TotalRequests: rc.stats.TotalRequests,
		Hits:          rc.stats.Hits,
		Misses:        rc.stats.Misses,
		Evictions:     rc.stats.Evictions,
	}
}

// GetKeys returns all cache keys (for debugging/monitoring)
func (rc *ResourceCache) GetKeys() []string {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	keys := make([]string, 0, len(rc.cache))
	for key := range rc.cache {
		keys = append(keys, key)
	}

	return keys
}

// GetSize returns the current number of entries in the cache
func (rc *ResourceCache) GetSize() int {
	rc.mutex.RLock()
	defer rc.mutex.RUnlock()

	return len(rc.cache)
}

// cleanupLoop runs periodic cleanup of expired entries
func (rc *ResourceCache) cleanupLoop() {
	for {
		select {
		case <-rc.cleanupTicker.C:
			rc.cleanupExpiredEntries()
		case <-rc.ctx.Done():
			return
		}
	}
}

// cleanupExpiredEntries removes expired entries from the cache
func (rc *ResourceCache) cleanupExpiredEntries() {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	expiredKeys := []string{}
	now := time.Now()

	for key, entry := range rc.cache {
		if now.Sub(entry.Timestamp) > entry.TTL {
			expiredKeys = append(expiredKeys, key)
		}
	}

	for _, key := range expiredKeys {
		delete(rc.cache, key)
		rc.incrementEvictions()
	}

	if len(expiredKeys) > 0 {
		rc.updateCacheSize()
		rc.logger.Debug("Cleaned up expired cache entries",
			zap.Int("expired_count", len(expiredKeys)))
	}
}

// evictEntries removes entries based on eviction policy
func (rc *ResourceCache) evictEntries(count int) {
	if len(rc.cache) == 0 {
		return
	}

	evictedKeys := rc.selectEntriesForEviction(count)

	for _, key := range evictedKeys {
		delete(rc.cache, key)
		rc.incrementEvictions()
	}

	if len(evictedKeys) > 0 {
		rc.logger.Debug("Evicted cache entries",
			zap.Int("evicted_count", len(evictedKeys)))
	}
}

// selectEntriesForEviction selects entries for eviction based on LRU policy
func (rc *ResourceCache) selectEntriesForEviction(count int) []string {
	if count <= 0 || len(rc.cache) == 0 {
		return nil
	}

	// Implement LRU eviction - find least recently accessed entries
	type entryInfo struct {
		key        string
		lastAccess time.Time
	}

	entries := make([]entryInfo, 0, len(rc.cache))
	for key, entry := range rc.cache {
		entries = append(entries, entryInfo{
			key:        key,
			lastAccess: entry.LastAccess,
		})
	}

	// Sort by last access time (oldest first)
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].lastAccess.After(entries[j].lastAccess) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Select entries to evict
	evictCount := count
	if evictCount > len(entries) {
		evictCount = len(entries)
	}

	evictedKeys := make([]string, evictCount)
	for i := 0; i < evictCount; i++ {
		evictedKeys[i] = entries[i].key
	}

	return evictedKeys
}

// isExpired checks if a cache entry has expired
func (rc *ResourceCache) isExpired(entry *CacheEntry) bool {
	return time.Since(entry.Timestamp) > entry.TTL
}

// estimateSize estimates the memory size of a value (simplified implementation)
func (rc *ResourceCache) estimateSize(value interface{}) int64 {
	// This is a simplified size estimation
	// In a real implementation, you might use reflection or serialization
	// to get more accurate size estimates

	switch v := value.(type) {
	case string:
		return int64(len(v))
	case []byte:
		return int64(len(v))
	case map[string]interface{}:
		return int64(len(v) * 50) // Rough estimate
	default:
		return 100 // Default estimate for unknown types
	}
}

// updateCacheSize updates the cache size in stats
func (rc *ResourceCache) updateCacheSize() {
	rc.stats.mutex.Lock()
	defer rc.stats.mutex.Unlock()

	rc.stats.Size = len(rc.cache)
}

// Statistics update methods

func (rc *ResourceCache) incrementTotalRequests() {
	rc.stats.mutex.Lock()
	defer rc.stats.mutex.Unlock()
	rc.stats.TotalRequests++
}

func (rc *ResourceCache) incrementHits() {
	rc.stats.mutex.Lock()
	defer rc.stats.mutex.Unlock()
	rc.stats.Hits++
}

func (rc *ResourceCache) incrementMisses() {
	rc.stats.mutex.Lock()
	defer rc.stats.mutex.Unlock()
	rc.stats.Misses++
}

func (rc *ResourceCache) incrementEvictions() {
	rc.stats.mutex.Lock()
	defer rc.stats.mutex.Unlock()
	rc.stats.Evictions++
}

// Convenience methods for common cache patterns

// GetOrSet retrieves a value from cache, or computes and caches it if not present
func (rc *ResourceCache) GetOrSet(key string, computeFunc func() (interface{}, error)) (interface{}, error) {
	// Try to get from cache first
	if value, found := rc.Get(key); found {
		return value, nil
	}

	// Compute the value
	value, err := computeFunc()
	if err != nil {
		return nil, err
	}

	// Cache the computed value
	rc.Set(key, value)
	return value, nil
}

// GetOrSetWithTTL is like GetOrSet but with custom TTL
func (rc *ResourceCache) GetOrSetWithTTL(key string, ttl time.Duration, computeFunc func() (interface{}, error)) (interface{}, error) {
	// Try to get from cache first
	if value, found := rc.Get(key); found {
		return value, nil
	}

	// Compute the value
	value, err := computeFunc()
	if err != nil {
		return nil, err
	}

	// Cache the computed value with custom TTL
	rc.SetWithTTL(key, value, ttl)
	return value, nil
}

// Invalidate removes entries matching a prefix pattern
func (rc *ResourceCache) Invalidate(prefix string) int {
	rc.mutex.Lock()
	defer rc.mutex.Unlock()

	keysToDelete := []string{}

	for key := range rc.cache {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(rc.cache, key)
	}

	if len(keysToDelete) > 0 {
		rc.updateCacheSize()
		rc.logger.Debug("Invalidated cache entries by prefix",
			zap.String("prefix", prefix),
			zap.Int("invalidated_count", len(keysToDelete)))
	}

	return len(keysToDelete)
}
