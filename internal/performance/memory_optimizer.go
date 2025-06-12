package performance

import (
	"context"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"go.uber.org/zap"
)

// MemoryOptimizer manages memory optimization for enterprise-scale performance
type MemoryOptimizer struct {
	logger       *zap.Logger
	memoryTarget int64
	gcTarget     int
	stats        *MemoryOptimizerStats

	// Object pools for frequently allocated objects
	pools *ObjectPools

	// Lifecycle management
	ctx           context.Context
	cancel        context.CancelFunc
	monitorTicker *time.Ticker

	// Configuration
	config *MemoryConfig
}

// MemoryConfig holds memory optimization configuration
type MemoryConfig struct {
	MemoryTarget      int64         // Target memory usage in bytes
	GCTarget          int           // GOGC target percentage
	MonitorInterval   time.Duration // How often to check memory usage
	ForceGCThreshold  float64       // Memory usage ratio to force GC
	ObjectPoolEnabled bool          // Enable object pooling
	DebugMemory       bool          // Enable memory debugging
}

// MemoryOptimizerStats contains memory optimization metrics
type MemoryOptimizerStats struct {
	TotalAllocated   uint64        `json:"totalAllocated"`
	CurrentAllocated uint64        `json:"currentAllocated"`
	SystemMemory     uint64        `json:"systemMemory"`
	GCRuns           uint32        `json:"gcRuns"`
	LastGCTime       time.Time     `json:"lastGCTime"`
	GCDuration       time.Duration `json:"gcDuration"`
	ObjectPoolHits   int64         `json:"objectPoolHits"`
	ObjectPoolMisses int64         `json:"objectPoolMisses"`
	MemoryPressure   float64       `json:"memoryPressure"` // 0.0 to 1.0

	mutex sync.RWMutex
}

// ObjectPools manages reusable object pools
type ObjectPools struct {
	stringBuilders  sync.Pool
	byteBuffers     sync.Pool
	sliceBuffers    sync.Pool
	mapBuffers      sync.Pool
	reconcileEvents sync.Pool
	healthResults   sync.Pool

	hitCount  int64
	missCount int64
	mutex     sync.RWMutex
}

// DefaultMemoryConfig returns the default memory optimization configuration
func DefaultMemoryConfig() *MemoryConfig {
	return &MemoryConfig{
		MemoryTarget:      500 * 1024 * 1024, // 500MB
		GCTarget:          100,
		MonitorInterval:   10 * time.Second,
		ForceGCThreshold:  0.8, // Force GC at 80% of target
		ObjectPoolEnabled: true,
		DebugMemory:       false,
	}
}

// NewMemoryOptimizer creates a new memory optimizer
func NewMemoryOptimizer(memoryTarget int64, gcTarget int, logger *zap.Logger) *MemoryOptimizer {
	config := DefaultMemoryConfig()
	config.MemoryTarget = memoryTarget
	config.GCTarget = gcTarget

	return NewMemoryOptimizerWithConfig(config, logger)
}

// NewMemoryOptimizerWithConfig creates a new memory optimizer with custom configuration
func NewMemoryOptimizerWithConfig(config *MemoryConfig, logger *zap.Logger) *MemoryOptimizer {
	mo := &MemoryOptimizer{
		logger:       logger,
		memoryTarget: config.MemoryTarget,
		gcTarget:     config.GCTarget,
		config:       config,
		stats:        &MemoryOptimizerStats{},
		pools:        newObjectPools(),
	}

	return mo
}

// Start initializes the memory optimizer and begins monitoring
func (mo *MemoryOptimizer) Start(ctx context.Context) error {
	mo.ctx, mo.cancel = context.WithCancel(ctx)
	mo.monitorTicker = time.NewTicker(mo.config.MonitorInterval)

	// Set initial GC target
	mo.setGCTarget(mo.gcTarget)

	// Start memory monitoring
	go mo.monitorLoop()

	mo.logger.Info("Memory optimizer started",
		zap.Int64("memory_target_mb", mo.memoryTarget/1024/1024),
		zap.Int("gc_target", mo.gcTarget),
		zap.Duration("monitor_interval", mo.config.MonitorInterval))

	return nil
}

// Stop gracefully shuts down the memory optimizer
func (mo *MemoryOptimizer) Stop() error {
	if mo.cancel != nil {
		mo.cancel()
	}

	if mo.monitorTicker != nil {
		mo.monitorTicker.Stop()
	}

	mo.logger.Info("Memory optimizer stopped")
	return nil
}

// monitorLoop continuously monitors memory usage and applies optimizations
func (mo *MemoryOptimizer) monitorLoop() {
	for {
		select {
		case <-mo.monitorTicker.C:
			mo.checkAndOptimize()
		case <-mo.ctx.Done():
			return
		}
	}
}

// checkAndOptimize checks current memory usage and applies optimizations
func (mo *MemoryOptimizer) checkAndOptimize() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Update stats
	mo.updateStats(&memStats)

	// Calculate memory pressure
	pressure := float64(memStats.Alloc) / float64(mo.memoryTarget)

	mo.stats.mutex.Lock()
	mo.stats.MemoryPressure = pressure
	mo.stats.mutex.Unlock()

	// Log current memory status
	mo.logger.Debug("Memory status",
		zap.Uint64("allocated_mb", memStats.Alloc/1024/1024),
		zap.Uint64("system_mb", memStats.Sys/1024/1024),
		zap.Float64("pressure", pressure),
		zap.Uint32("gc_runs", memStats.NumGC))

	// Apply optimizations based on memory pressure
	if pressure >= mo.config.ForceGCThreshold {
		mo.logger.Info("High memory pressure detected, forcing garbage collection",
			zap.Float64("pressure", pressure),
			zap.Uint64("allocated_mb", memStats.Alloc/1024/1024))

		mo.forceGarbageCollection()
	}

	// Adjust GC target based on pressure
	mo.adjustGCTarget(pressure)

	// Clean object pools if needed
	if pressure > 0.7 {
		mo.cleanObjectPools()
	}
}

// updateStats updates internal memory statistics
func (mo *MemoryOptimizer) updateStats(memStats *runtime.MemStats) {
	mo.stats.mutex.Lock()
	defer mo.stats.mutex.Unlock()

	mo.stats.TotalAllocated = memStats.TotalAlloc
	mo.stats.CurrentAllocated = memStats.Alloc
	mo.stats.SystemMemory = memStats.Sys
	mo.stats.GCRuns = memStats.NumGC

	// Calculate GC duration from pause times
	if len(memStats.PauseNs) > 0 {
		mo.stats.GCDuration = time.Duration(memStats.PauseNs[(memStats.NumGC+255)%256])
	}
}

// forceGarbageCollection triggers immediate garbage collection
func (mo *MemoryOptimizer) forceGarbageCollection() {
	start := time.Now()

	// Run garbage collection
	runtime.GC()

	// Force return memory to OS
	debug.FreeOSMemory()

	duration := time.Since(start)

	mo.stats.mutex.Lock()
	mo.stats.LastGCTime = start
	mo.stats.GCDuration = duration
	mo.stats.mutex.Unlock()

	mo.logger.Info("Forced garbage collection completed",
		zap.Duration("duration", duration))
}

// adjustGCTarget dynamically adjusts the GOGC target based on memory pressure
func (mo *MemoryOptimizer) adjustGCTarget(pressure float64) {
	var newTarget int

	switch {
	case pressure > 0.9:
		newTarget = 50 // Aggressive GC
	case pressure > 0.7:
		newTarget = 75 // Moderate GC
	case pressure > 0.5:
		newTarget = 100 // Default GC
	default:
		newTarget = 200 // Relaxed GC
	}

	if newTarget != mo.gcTarget {
		mo.setGCTarget(newTarget)
		mo.gcTarget = newTarget

		mo.logger.Debug("Adjusted GC target",
			zap.Int("new_target", newTarget),
			zap.Float64("memory_pressure", pressure))
	}
}

// setGCTarget sets the GOGC target percentage
func (mo *MemoryOptimizer) setGCTarget(target int) {
	debug.SetGCPercent(target)
	mo.logger.Debug("Set GC target", zap.Int("target", target))
}

// GetStats returns current memory optimizer statistics
func (mo *MemoryOptimizer) GetStats() MemoryOptimizerStats {
	mo.stats.mutex.RLock()
	defer mo.stats.mutex.RUnlock()

	// Get current object pool stats
	mo.pools.mutex.RLock()
	poolHits := mo.pools.hitCount
	poolMisses := mo.pools.missCount
	mo.pools.mutex.RUnlock()

	return MemoryOptimizerStats{
		TotalAllocated:   mo.stats.TotalAllocated,
		CurrentAllocated: mo.stats.CurrentAllocated,
		SystemMemory:     mo.stats.SystemMemory,
		GCRuns:           mo.stats.GCRuns,
		LastGCTime:       mo.stats.LastGCTime,
		GCDuration:       mo.stats.GCDuration,
		ObjectPoolHits:   poolHits,
		ObjectPoolMisses: poolMisses,
		MemoryPressure:   mo.stats.MemoryPressure,
	}
}

// Object Pool Management

// newObjectPools creates new object pools
func newObjectPools() *ObjectPools {
	return &ObjectPools{
		stringBuilders: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 1024)
			},
		},
		byteBuffers: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 4096)
			},
		},
		sliceBuffers: sync.Pool{
			New: func() interface{} {
				return make([]interface{}, 0, 10)
			},
		},
		mapBuffers: sync.Pool{
			New: func() interface{} {
				return make(map[string]interface{})
			},
		},
		reconcileEvents: sync.Pool{
			New: func() interface{} {
				return &ReconcileEvent{}
			},
		},
		healthResults: sync.Pool{
			New: func() interface{} {
				return &HealthResult{}
			},
		},
	}
}

// GetByteBuffer retrieves a byte buffer from the pool
func (mo *MemoryOptimizer) GetByteBuffer() []byte {
	if !mo.config.ObjectPoolEnabled {
		return make([]byte, 0, 4096)
	}

	buffer := mo.pools.byteBuffers.Get().([]byte)

	mo.pools.mutex.Lock()
	mo.pools.hitCount++
	mo.pools.mutex.Unlock()

	// Reset buffer
	buffer = buffer[:0]
	return buffer
}

// PutByteBuffer returns a byte buffer to the pool
func (mo *MemoryOptimizer) PutByteBuffer(buffer []byte) {
	if !mo.config.ObjectPoolEnabled {
		return
	}

	// Only pool buffers of reasonable size
	if cap(buffer) <= 64*1024 {
		mo.pools.byteBuffers.Put(buffer)
	}
}

// GetStringBuilder retrieves a string builder buffer from the pool
func (mo *MemoryOptimizer) GetStringBuilder() []byte {
	if !mo.config.ObjectPoolEnabled {
		return make([]byte, 0, 1024)
	}

	buffer := mo.pools.stringBuilders.Get().([]byte)

	mo.pools.mutex.Lock()
	mo.pools.hitCount++
	mo.pools.mutex.Unlock()

	// Reset buffer
	buffer = buffer[:0]
	return buffer
}

// PutStringBuilder returns a string builder buffer to the pool
func (mo *MemoryOptimizer) PutStringBuilder(buffer []byte) {
	if !mo.config.ObjectPoolEnabled {
		return
	}

	// Only pool buffers of reasonable size
	if cap(buffer) <= 8*1024 {
		mo.pools.stringBuilders.Put(buffer)
	}
}

// GetSliceBuffer retrieves a slice buffer from the pool
func (mo *MemoryOptimizer) GetSliceBuffer() []interface{} {
	if !mo.config.ObjectPoolEnabled {
		return make([]interface{}, 0, 10)
	}

	slice := mo.pools.sliceBuffers.Get().([]interface{})

	mo.pools.mutex.Lock()
	mo.pools.hitCount++
	mo.pools.mutex.Unlock()

	// Reset slice
	slice = slice[:0]
	return slice
}

// PutSliceBuffer returns a slice buffer to the pool
func (mo *MemoryOptimizer) PutSliceBuffer(slice []interface{}) {
	if !mo.config.ObjectPoolEnabled {
		return
	}

	// Only pool slices of reasonable size
	if cap(slice) <= 100 {
		mo.pools.sliceBuffers.Put(slice)
	}
}

// GetMapBuffer retrieves a map buffer from the pool
func (mo *MemoryOptimizer) GetMapBuffer() map[string]interface{} {
	if !mo.config.ObjectPoolEnabled {
		return make(map[string]interface{})
	}

	mapBuf := mo.pools.mapBuffers.Get().(map[string]interface{})

	mo.pools.mutex.Lock()
	mo.pools.hitCount++
	mo.pools.mutex.Unlock()

	// Clear map
	for key := range mapBuf {
		delete(mapBuf, key)
	}

	return mapBuf
}

// PutMapBuffer returns a map buffer to the pool
func (mo *MemoryOptimizer) PutMapBuffer(mapBuf map[string]interface{}) {
	if !mo.config.ObjectPoolEnabled {
		return
	}

	// Only pool maps of reasonable size
	if len(mapBuf) <= 50 {
		mo.pools.mapBuffers.Put(mapBuf)
	}
}

// GetReconcileEvent retrieves a reconcile event from the pool
func (mo *MemoryOptimizer) GetReconcileEvent() *ReconcileEvent {
	if !mo.config.ObjectPoolEnabled {
		return &ReconcileEvent{}
	}

	event := mo.pools.reconcileEvents.Get().(*ReconcileEvent)

	mo.pools.mutex.Lock()
	mo.pools.hitCount++
	mo.pools.mutex.Unlock()

	// Reset event
	event.Reset()
	return event
}

// PutReconcileEvent returns a reconcile event to the pool
func (mo *MemoryOptimizer) PutReconcileEvent(event *ReconcileEvent) {
	if !mo.config.ObjectPoolEnabled {
		return
	}

	mo.pools.reconcileEvents.Put(event)
}

// GetHealthResult retrieves a health result from the pool
func (mo *MemoryOptimizer) GetHealthResult() *HealthResult {
	if !mo.config.ObjectPoolEnabled {
		return &HealthResult{}
	}

	result := mo.pools.healthResults.Get().(*HealthResult)

	mo.pools.mutex.Lock()
	mo.pools.hitCount++
	mo.pools.mutex.Unlock()

	// Reset result
	result.Reset()
	return result
}

// PutHealthResult returns a health result to the pool
func (mo *MemoryOptimizer) PutHealthResult(result *HealthResult) {
	if !mo.config.ObjectPoolEnabled {
		return
	}

	mo.pools.healthResults.Put(result)
}

// cleanObjectPools cleans up object pools to reduce memory usage
func (mo *MemoryOptimizer) cleanObjectPools() {
	// This is a simplified cleanup - in a real implementation,
	// you might implement more sophisticated pool management
	mo.logger.Debug("Cleaning object pools to reduce memory pressure")
}

// Pooled object types

// ReconcileEvent represents a reconcile event that can be pooled
type ReconcileEvent struct {
	RoostName string
	Namespace string
	Operation string
	Timestamp time.Time
	Data      map[string]interface{}
}

// Reset resets the reconcile event for reuse
func (re *ReconcileEvent) Reset() {
	re.RoostName = ""
	re.Namespace = ""
	re.Operation = ""
	re.Timestamp = time.Time{}

	// Clear map
	for key := range re.Data {
		delete(re.Data, key)
	}
	if re.Data == nil {
		re.Data = make(map[string]interface{})
	}
}

// HealthResult represents a health check result that can be pooled
type HealthResult struct {
	CheckName string
	Healthy   bool
	Message   string
	Duration  time.Duration
	Data      map[string]interface{}
}

// Reset resets the health result for reuse
func (hr *HealthResult) Reset() {
	hr.CheckName = ""
	hr.Healthy = false
	hr.Message = ""
	hr.Duration = 0

	// Clear map
	for key := range hr.Data {
		delete(hr.Data, key)
	}
	if hr.Data == nil {
		hr.Data = make(map[string]interface{})
	}
}
