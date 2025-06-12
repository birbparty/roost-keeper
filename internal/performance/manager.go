package performance

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Manager orchestrates all performance optimizations for enterprise-scale operation
type Manager struct {
	client          client.Client
	cache           cache.Cache
	workerPool      *WorkerPool
	resourceCache   *ResourceCache
	memoryOptimizer *MemoryOptimizer
	connectionPool  *ConnectionPool
	metrics         *telemetry.OperatorMetrics
	logger          *zap.Logger
	optimizations   map[string]Optimization
	startTime       time.Time

	// Performance tracking
	totalJobs      int64
	successfulJobs int64
	failedJobs     int64

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Optimization defines the interface for performance optimizations
type Optimization interface {
	Name() string
	Apply(ctx context.Context) error
	Metrics() map[string]interface{}
	Health() HealthStatus
}

// HealthStatus represents the health of an optimization
type HealthStatus struct {
	Healthy bool    `json:"healthy"`
	Message string  `json:"message"`
	Score   float64 `json:"score"` // 0.0 to 1.0
}

// Config holds configuration for the performance manager
type Config struct {
	WorkerCount        int           `yaml:"workerCount"`
	CacheSize          int           `yaml:"cacheSize"`
	CacheTTL           time.Duration `yaml:"cacheTTL"`
	MemoryTarget       int64         `yaml:"memoryTarget"`
	GCTarget           int           `yaml:"gcTarget"`
	MonitoringInterval time.Duration `yaml:"monitoringInterval"`
	EnableProfiling    bool          `yaml:"enableProfiling"`
}

// DefaultConfig returns the default performance configuration
func DefaultConfig() *Config {
	return &Config{
		WorkerCount:        50,
		CacheSize:          10000,
		CacheTTL:           5 * time.Minute,
		MemoryTarget:       500 * 1024 * 1024, // 500MB
		GCTarget:           100,
		MonitoringInterval: 30 * time.Second,
		EnableProfiling:    true,
	}
}

// NewManager creates a new performance manager with enterprise optimizations
func NewManager(client client.Client, cache cache.Cache, metrics *telemetry.OperatorMetrics, logger *zap.Logger) *Manager {
	return NewManagerWithConfig(client, cache, metrics, logger, DefaultConfig())
}

// NewManagerWithConfig creates a new performance manager with custom configuration
func NewManagerWithConfig(client client.Client, cache cache.Cache, metrics *telemetry.OperatorMetrics, logger *zap.Logger, config *Config) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		client:        client,
		cache:         cache,
		metrics:       metrics,
		logger:        logger,
		optimizations: make(map[string]Optimization),
		startTime:     time.Now(),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Initialize performance components
	m.workerPool = NewWorkerPool(config.WorkerCount, logger)
	m.resourceCache = NewResourceCache(config.CacheSize, config.CacheTTL, logger)
	m.memoryOptimizer = NewMemoryOptimizer(config.MemoryTarget, config.GCTarget, logger)
	m.connectionPool = NewConnectionPool(client, logger)

	// Register core optimizations
	m.RegisterOptimization(NewControllerOptimization(client, logger))
	m.RegisterOptimization(NewCacheOptimization(cache, logger))
	m.RegisterOptimization(NewConcurrencyOptimization(config.WorkerCount, logger))
	m.RegisterOptimization(NewAPIOptimization(client, logger))

	return m
}

// Start initializes and starts all performance optimization components
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting enterprise performance optimization manager",
		zap.Int("worker_count", m.workerPool.workerCount),
		zap.Int("cache_size", m.resourceCache.maxSize),
		zap.Duration("cache_ttl", m.resourceCache.defaultTTL),
	)

	// Start worker pool
	if err := m.workerPool.Start(ctx); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Start resource cache
	if err := m.resourceCache.Start(ctx); err != nil {
		return fmt.Errorf("failed to start resource cache: %w", err)
	}

	// Start memory optimizer
	if err := m.memoryOptimizer.Start(ctx); err != nil {
		return fmt.Errorf("failed to start memory optimizer: %w", err)
	}

	// Start connection pool
	if err := m.connectionPool.Start(ctx); err != nil {
		return fmt.Errorf("failed to start connection pool: %w", err)
	}

	// Apply all registered optimizations
	for name, opt := range m.optimizations {
		if err := opt.Apply(ctx); err != nil {
			m.logger.Error("Failed to apply optimization",
				zap.String("optimization", name),
				zap.Error(err))
			// Continue with other optimizations
		} else {
			m.logger.Info("Applied optimization", zap.String("optimization", name))
		}
	}

	// Start performance monitoring
	m.wg.Add(1)
	go m.monitorPerformance(ctx)

	// Record startup metrics
	if m.metrics != nil {
		m.metrics.SetOperatorStartTime(ctx, m.startTime)
	}

	m.logger.Info("Performance optimization manager started successfully")
	return nil
}

// Stop gracefully shuts down the performance manager
func (m *Manager) Stop() error {
	m.logger.Info("Stopping performance optimization manager")

	// Cancel context to signal shutdown
	m.cancel()

	// Wait for all goroutines to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	// Wait for graceful shutdown or timeout
	select {
	case <-done:
		m.logger.Info("Performance manager stopped gracefully")
	case <-time.After(30 * time.Second):
		m.logger.Warn("Performance manager shutdown timeout")
	}

	return nil
}

// RegisterOptimization registers a new performance optimization
func (m *Manager) RegisterOptimization(opt Optimization) {
	m.optimizations[opt.Name()] = opt
	m.logger.Info("Registered performance optimization", zap.String("name", opt.Name()))
}

// SubmitJob submits a job to the worker pool for concurrent processing
func (m *Manager) SubmitJob(job Job) error {
	atomic.AddInt64(&m.totalJobs, 1)

	// Wrap job callback to track success/failure
	originalCallback := job.Callback
	job.Callback = func(result interface{}, err error) {
		if err != nil {
			atomic.AddInt64(&m.failedJobs, 1)
		} else {
			atomic.AddInt64(&m.successfulJobs, 1)
		}

		if originalCallback != nil {
			originalCallback(result, err)
		}
	}

	return m.workerPool.Submit(job)
}

// GetCache returns the resource cache for direct access
func (m *Manager) GetCache() *ResourceCache {
	return m.resourceCache
}

// GetConnectionPool returns the connection pool for direct access
func (m *Manager) GetConnectionPool() *ConnectionPool {
	return m.connectionPool
}

// GetPerformanceMetrics returns current performance metrics
func (m *Manager) GetPerformanceMetrics() PerformanceMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return PerformanceMetrics{
		TotalJobs:       atomic.LoadInt64(&m.totalJobs),
		SuccessfulJobs:  atomic.LoadInt64(&m.successfulJobs),
		FailedJobs:      atomic.LoadInt64(&m.failedJobs),
		WorkerPoolStats: m.workerPool.GetStats(),
		CacheStats:      m.resourceCache.GetStats(),
		MemoryStats: MemoryStats{
			AllocatedMB:    memStats.Alloc / 1024 / 1024,
			SystemMB:       memStats.Sys / 1024 / 1024,
			GCRuns:         memStats.NumGC,
			GoroutineCount: runtime.NumGoroutine(),
		},
		Uptime: time.Since(m.startTime),
	}
}

// PerformanceMetrics aggregates all performance-related metrics
type PerformanceMetrics struct {
	TotalJobs       int64           `json:"totalJobs"`
	SuccessfulJobs  int64           `json:"successfulJobs"`
	FailedJobs      int64           `json:"failedJobs"`
	WorkerPoolStats WorkerPoolStats `json:"workerPoolStats"`
	CacheStats      CacheStats      `json:"cacheStats"`
	MemoryStats     MemoryStats     `json:"memoryStats"`
	Uptime          time.Duration   `json:"uptime"`
}

// MemoryStats represents memory-related performance metrics
type MemoryStats struct {
	AllocatedMB    uint64 `json:"allocatedMB"`
	SystemMB       uint64 `json:"systemMB"`
	GCRuns         uint32 `json:"gcRuns"`
	GoroutineCount int    `json:"goroutineCount"`
}

// monitorPerformance continuously monitors and reports performance metrics
func (m *Manager) monitorPerformance(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	m.logger.Info("Started performance monitoring")

	for {
		select {
		case <-ticker.C:
			m.collectAndReportMetrics(ctx)
		case <-ctx.Done():
			m.logger.Info("Stopping performance monitoring")
			return
		}
	}
}

// collectAndReportMetrics collects metrics from all components and reports them
func (m *Manager) collectAndReportMetrics(ctx context.Context) {
	// Collect performance metrics
	perfMetrics := m.GetPerformanceMetrics()

	// Update operator uptime
	if m.metrics != nil {
		m.metrics.UpdateOperatorUptime(ctx, m.startTime)
		m.metrics.UpdatePerformanceMetrics(ctx,
			int64(perfMetrics.MemoryStats.AllocatedMB*1024*1024),
			0.0, // CPU ratio calculation would need additional monitoring
			int64(perfMetrics.MemoryStats.GoroutineCount))
	}

	// Log performance summary
	m.logger.Info("Performance metrics summary",
		zap.Int64("total_jobs", perfMetrics.TotalJobs),
		zap.Int64("successful_jobs", perfMetrics.SuccessfulJobs),
		zap.Int64("failed_jobs", perfMetrics.FailedJobs),
		zap.Float64("success_rate", m.calculateSuccessRate()),
		zap.Int("active_workers", perfMetrics.WorkerPoolStats.ActiveWorkers),
		zap.Int("queue_length", perfMetrics.WorkerPoolStats.QueueLength),
		zap.Float64("cache_hit_rate", perfMetrics.CacheStats.HitRate),
		zap.Int("cache_size", perfMetrics.CacheStats.Size),
		zap.Uint64("memory_mb", perfMetrics.MemoryStats.AllocatedMB),
		zap.Int("goroutines", perfMetrics.MemoryStats.GoroutineCount),
		zap.Duration("uptime", perfMetrics.Uptime),
	)

	// Check for performance issues and alert
	m.checkPerformanceThresholds(perfMetrics)

	// Collect optimization-specific metrics
	for name, opt := range m.optimizations {
		health := opt.Health()
		metrics := opt.Metrics()

		m.logger.Debug("Optimization metrics",
			zap.String("optimization", name),
			zap.Bool("healthy", health.Healthy),
			zap.Float64("score", health.Score),
			zap.Any("metrics", metrics))
	}
}

// calculateSuccessRate calculates the current job success rate
func (m *Manager) calculateSuccessRate() float64 {
	total := atomic.LoadInt64(&m.totalJobs)
	if total == 0 {
		return 1.0
	}
	successful := atomic.LoadInt64(&m.successfulJobs)
	return float64(successful) / float64(total)
}

// checkPerformanceThresholds checks performance metrics against thresholds and alerts
func (m *Manager) checkPerformanceThresholds(metrics PerformanceMetrics) {
	// Memory threshold check
	if metrics.MemoryStats.AllocatedMB > 500 {
		m.logger.Warn("High memory usage detected",
			zap.Uint64("allocated_mb", metrics.MemoryStats.AllocatedMB),
			zap.Uint64("threshold_mb", 500))

		// Trigger garbage collection
		runtime.GC()
	}

	// Goroutine threshold check
	if metrics.MemoryStats.GoroutineCount > 1000 {
		m.logger.Warn("High goroutine count detected",
			zap.Int("goroutine_count", metrics.MemoryStats.GoroutineCount),
			zap.Int("threshold", 1000))
	}

	// Queue length threshold check
	if metrics.WorkerPoolStats.QueueLength > 100 {
		m.logger.Warn("High worker queue length detected",
			zap.Int("queue_length", metrics.WorkerPoolStats.QueueLength),
			zap.Int("threshold", 100))
	}

	// Success rate threshold check
	successRate := m.calculateSuccessRate()
	if successRate < 0.9 && metrics.TotalJobs > 10 {
		m.logger.Warn("Low job success rate detected",
			zap.Float64("success_rate", successRate),
			zap.Float64("threshold", 0.9))
	}

	// Cache hit rate threshold check
	if metrics.CacheStats.HitRate < 0.7 && metrics.CacheStats.TotalRequests > 10 {
		m.logger.Warn("Low cache hit rate detected",
			zap.Float64("hit_rate", metrics.CacheStats.HitRate),
			zap.Float64("threshold", 0.7))
	}
}

// HealthCheck returns the overall health of the performance manager
func (m *Manager) HealthCheck() HealthStatus {
	metrics := m.GetPerformanceMetrics()

	// Calculate overall health score
	score := 1.0
	issues := []string{}

	// Check memory usage
	if metrics.MemoryStats.AllocatedMB > 500 {
		score -= 0.2
		issues = append(issues, "high memory usage")
	}

	// Check success rate
	successRate := m.calculateSuccessRate()
	if successRate < 0.9 && metrics.TotalJobs > 10 {
		score -= 0.3
		issues = append(issues, "low success rate")
	}

	// Check queue length
	if metrics.WorkerPoolStats.QueueLength > 100 {
		score -= 0.2
		issues = append(issues, "high queue length")
	}

	// Check cache performance
	if metrics.CacheStats.HitRate < 0.7 && metrics.CacheStats.TotalRequests > 10 {
		score -= 0.1
		issues = append(issues, "low cache hit rate")
	}

	// Check goroutine count
	if metrics.MemoryStats.GoroutineCount > 1000 {
		score -= 0.2
		issues = append(issues, "high goroutine count")
	}

	healthy := score >= 0.7
	message := "Performance manager is healthy"
	if !healthy {
		message = fmt.Sprintf("Performance issues detected: %v", issues)
	}

	return HealthStatus{
		Healthy: healthy,
		Message: message,
		Score:   score,
	}
}
