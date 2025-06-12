package performance

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Job represents a unit of work to be processed by the worker pool
type Job struct {
	ID       string
	Type     string
	Payload  interface{}
	Callback func(result interface{}, err error)
	Priority int // Higher values = higher priority
}

// WorkerPool manages a pool of concurrent workers for processing jobs
type WorkerPool struct {
	workerCount   int
	jobQueue      chan Job
	priorityQueue chan Job
	workers       []*Worker
	logger        *zap.Logger
	stats         *WorkerPoolStats

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Configuration
	maxQueueSize int
	started      bool
	mutex        sync.RWMutex
}

// Worker represents a single worker in the pool
type Worker struct {
	ID            int
	jobQueue      chan Job
	priorityQueue chan Job
	quit          chan bool
	stats         *WorkerPoolStats
	logger        *zap.Logger

	// Worker-specific metrics
	jobsProcessed int64
	lastJobTime   time.Time
}

// WorkerPoolStats contains metrics about the worker pool performance
type WorkerPoolStats struct {
	ActiveWorkers    int           `json:"activeWorkers"`
	QueueLength      int           `json:"queueLength"`
	TotalJobs        int64         `json:"totalJobs"`
	CompletedJobs    int64         `json:"completedJobs"`
	FailedJobs       int64         `json:"failedJobs"`
	AverageLatency   time.Duration `json:"averageLatency"`
	ThroughputPerSec float64       `json:"throughputPerSec"`

	// Internal tracking
	mutex              sync.RWMutex
	latencySum         time.Duration
	startTime          time.Time
	lastThroughputCalc time.Time
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(workerCount int, logger *zap.Logger) *WorkerPool {
	maxQueueSize := workerCount * 10 // Buffer 10x worker count

	wp := &WorkerPool{
		workerCount:   workerCount,
		jobQueue:      make(chan Job, maxQueueSize),
		priorityQueue: make(chan Job, maxQueueSize/4), // Smaller priority queue
		workers:       make([]*Worker, 0, workerCount),
		logger:        logger,
		maxQueueSize:  maxQueueSize,
		stats: &WorkerPoolStats{
			startTime:          time.Now(),
			lastThroughputCalc: time.Now(),
		},
	}

	return wp
}

// Start initializes and starts all workers in the pool
func (wp *WorkerPool) Start(ctx context.Context) error {
	wp.mutex.Lock()
	defer wp.mutex.Unlock()

	if wp.started {
		return fmt.Errorf("worker pool already started")
	}

	wp.ctx, wp.cancel = context.WithCancel(ctx)
	wp.stats.startTime = time.Now()
	wp.stats.lastThroughputCalc = time.Now()

	wp.logger.Info("Starting worker pool", zap.Int("worker_count", wp.workerCount))

	// Create and start workers
	for i := 0; i < wp.workerCount; i++ {
		worker := &Worker{
			ID:            i,
			jobQueue:      wp.jobQueue,
			priorityQueue: wp.priorityQueue,
			quit:          make(chan bool),
			stats:         wp.stats,
			logger:        wp.logger.With(zap.Int("worker_id", i)),
		}
		wp.workers = append(wp.workers, worker)

		wp.wg.Add(1)
		go worker.start(wp.ctx, &wp.wg)
	}

	// Start priority job dispatcher
	wp.wg.Add(1)
	go wp.priorityDispatcher(wp.ctx)

	// Start metrics collection
	wp.wg.Add(1)
	go wp.metricsCollector(wp.ctx)

	wp.stats.mutex.Lock()
	wp.stats.ActiveWorkers = wp.workerCount
	wp.stats.mutex.Unlock()

	wp.started = true
	wp.logger.Info("Worker pool started successfully")

	return nil
}

// Stop gracefully shuts down the worker pool
func (wp *WorkerPool) Stop() error {
	wp.mutex.Lock()
	defer wp.mutex.Unlock()

	if !wp.started {
		return nil
	}

	wp.logger.Info("Stopping worker pool")

	// Signal shutdown
	wp.cancel()

	// Close job queues
	close(wp.jobQueue)
	close(wp.priorityQueue)

	// Stop all workers
	for _, worker := range wp.workers {
		close(worker.quit)
	}

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wp.logger.Info("All workers stopped gracefully")
	case <-time.After(30 * time.Second):
		wp.logger.Warn("Worker pool shutdown timeout")
	}

	wp.started = false
	return nil
}

// Submit adds a job to the worker pool queue
func (wp *WorkerPool) Submit(job Job) error {
	wp.mutex.RLock()
	defer wp.mutex.RUnlock()

	if !wp.started {
		return fmt.Errorf("worker pool not started")
	}

	// Choose queue based on priority
	if job.Priority > 5 {
		select {
		case wp.priorityQueue <- job:
			atomic.AddInt64(&wp.stats.TotalJobs, 1)
			wp.updateQueueLength()
			return nil
		default:
			return fmt.Errorf("priority queue is full")
		}
	} else {
		select {
		case wp.jobQueue <- job:
			atomic.AddInt64(&wp.stats.TotalJobs, 1)
			wp.updateQueueLength()
			return nil
		default:
			return fmt.Errorf("job queue is full")
		}
	}
}

// SubmitWithTimeout submits a job with a timeout
func (wp *WorkerPool) SubmitWithTimeout(job Job, timeout time.Duration) error {
	wp.mutex.RLock()
	defer wp.mutex.RUnlock()

	if !wp.started {
		return fmt.Errorf("worker pool not started")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Choose queue based on priority
	targetQueue := wp.jobQueue
	if job.Priority > 5 {
		targetQueue = wp.priorityQueue
	}

	select {
	case targetQueue <- job:
		atomic.AddInt64(&wp.stats.TotalJobs, 1)
		wp.updateQueueLength()
		return nil
	case <-ctx.Done():
		return fmt.Errorf("job submission timeout")
	}
}

// GetStats returns current worker pool statistics
func (wp *WorkerPool) GetStats() WorkerPoolStats {
	wp.stats.mutex.RLock()
	defer wp.stats.mutex.RUnlock()

	// Create a copy to avoid data races
	return WorkerPoolStats{
		ActiveWorkers:    wp.stats.ActiveWorkers,
		QueueLength:      wp.stats.QueueLength,
		TotalJobs:        atomic.LoadInt64(&wp.stats.TotalJobs),
		CompletedJobs:    atomic.LoadInt64(&wp.stats.CompletedJobs),
		FailedJobs:       atomic.LoadInt64(&wp.stats.FailedJobs),
		AverageLatency:   wp.stats.AverageLatency,
		ThroughputPerSec: wp.stats.ThroughputPerSec,
	}
}

// updateQueueLength updates the current queue length
func (wp *WorkerPool) updateQueueLength() {
	queueLen := len(wp.jobQueue) + len(wp.priorityQueue)
	wp.stats.mutex.Lock()
	wp.stats.QueueLength = queueLen
	wp.stats.mutex.Unlock()
}

// priorityDispatcher handles priority job distribution
func (wp *WorkerPool) priorityDispatcher(ctx context.Context) {
	defer wp.wg.Done()

	for {
		select {
		case job, ok := <-wp.priorityQueue:
			if !ok {
				return
			}

			// Try to dispatch priority job immediately
			select {
			case wp.jobQueue <- job:
				// Successfully dispatched
			case <-ctx.Done():
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

// metricsCollector periodically calculates throughput and other metrics
func (wp *WorkerPool) metricsCollector(ctx context.Context) {
	defer wp.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			wp.calculateMetrics()
		case <-ctx.Done():
			return
		}
	}
}

// calculateMetrics calculates throughput and other derived metrics
func (wp *WorkerPool) calculateMetrics() {
	wp.stats.mutex.Lock()
	defer wp.stats.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(wp.stats.lastThroughputCalc).Seconds()

	if elapsed > 0 {
		completedJobs := atomic.LoadInt64(&wp.stats.CompletedJobs)
		wp.stats.ThroughputPerSec = float64(completedJobs) / elapsed
		wp.stats.lastThroughputCalc = now
	}
}

// Worker implementation

// start begins the worker's job processing loop
func (w *Worker) start(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	w.logger.Debug("Worker started")

	for {
		select {
		case job, ok := <-w.jobQueue:
			if !ok {
				w.logger.Debug("Worker stopping - job queue closed")
				return
			}
			w.processJob(job)

		case job, ok := <-w.priorityQueue:
			if !ok {
				w.logger.Debug("Worker stopping - priority queue closed")
				return
			}
			w.processJob(job)

		case <-w.quit:
			w.logger.Debug("Worker stopping - quit signal received")
			return

		case <-ctx.Done():
			w.logger.Debug("Worker stopping - context cancelled")
			return
		}
	}
}

// processJob processes a single job and tracks metrics
func (w *Worker) processJob(job Job) {
	start := time.Now()
	w.lastJobTime = start

	defer func() {
		duration := time.Since(start)
		atomic.AddInt64(&w.jobsProcessed, 1)

		// Update stats
		w.stats.mutex.Lock()
		w.stats.latencySum += duration
		completedJobs := atomic.LoadInt64(&w.stats.CompletedJobs)
		if completedJobs > 0 {
			w.stats.AverageLatency = w.stats.latencySum / time.Duration(completedJobs)
		}
		w.stats.mutex.Unlock()

		w.logger.Debug("Job processed",
			zap.String("job_id", job.ID),
			zap.String("job_type", job.Type),
			zap.Duration("duration", duration))
	}()

	w.logger.Debug("Processing job",
		zap.String("job_id", job.ID),
		zap.String("job_type", job.Type),
		zap.Int("priority", job.Priority))

	// Process job based on type
	var result interface{}
	var err error

	switch job.Type {
	case "reconcile":
		result, err = w.processReconcileJob(job)
	case "health_check":
		result, err = w.processHealthCheckJob(job)
	case "cleanup":
		result, err = w.processCleanupJob(job)
	case "cache_operation":
		result, err = w.processCacheJob(job)
	default:
		err = fmt.Errorf("unknown job type: %s", job.Type)
	}

	// Update completion stats
	if err != nil {
		atomic.AddInt64(&w.stats.FailedJobs, 1)
		w.logger.Error("Job failed",
			zap.String("job_id", job.ID),
			zap.String("job_type", job.Type),
			zap.Error(err))
	} else {
		atomic.AddInt64(&w.stats.CompletedJobs, 1)
	}

	// Execute callback if provided
	if job.Callback != nil {
		job.Callback(result, err)
	}
}

// Job processing methods for different job types

func (w *Worker) processReconcileJob(job Job) (interface{}, error) {
	// Reconcile job processing logic
	if payload, ok := job.Payload.(ReconcileJobPayload); ok {
		w.logger.Debug("Processing reconcile job",
			zap.String("roost_name", payload.RoostName),
			zap.String("namespace", payload.Namespace))

		// Simulate reconcile work
		time.Sleep(100 * time.Millisecond)

		return ReconcileJobResult{
			Success:  true,
			Message:  "Reconciliation completed",
			Duration: 100 * time.Millisecond,
		}, nil
	}

	return nil, fmt.Errorf("invalid reconcile job payload")
}

func (w *Worker) processHealthCheckJob(job Job) (interface{}, error) {
	// Health check job processing logic
	if payload, ok := job.Payload.(HealthCheckJobPayload); ok {
		w.logger.Debug("Processing health check job",
			zap.String("check_name", payload.CheckName),
			zap.String("check_type", payload.CheckType))

		// Simulate health check work
		time.Sleep(50 * time.Millisecond)

		return HealthCheckJobResult{
			Healthy:  true,
			Message:  "Health check passed",
			Duration: 50 * time.Millisecond,
		}, nil
	}

	return nil, fmt.Errorf("invalid health check job payload")
}

func (w *Worker) processCleanupJob(job Job) (interface{}, error) {
	// Cleanup job processing logic
	if payload, ok := job.Payload.(CleanupJobPayload); ok {
		w.logger.Debug("Processing cleanup job",
			zap.String("resource_type", payload.ResourceType),
			zap.String("resource_name", payload.ResourceName))

		// Simulate cleanup work
		time.Sleep(200 * time.Millisecond)

		return CleanupJobResult{
			Cleaned:  true,
			Message:  "Cleanup completed",
			Duration: 200 * time.Millisecond,
		}, nil
	}

	return nil, fmt.Errorf("invalid cleanup job payload")
}

func (w *Worker) processCacheJob(job Job) (interface{}, error) {
	// Cache operation job processing logic
	if payload, ok := job.Payload.(CacheJobPayload); ok {
		w.logger.Debug("Processing cache job",
			zap.String("operation", payload.Operation),
			zap.String("key", payload.Key))

		// Simulate cache work
		time.Sleep(10 * time.Millisecond)

		return CacheJobResult{
			Success:  true,
			Message:  "Cache operation completed",
			Duration: 10 * time.Millisecond,
		}, nil
	}

	return nil, fmt.Errorf("invalid cache job payload")
}

// Job payload and result types

type ReconcileJobPayload struct {
	RoostName string
	Namespace string
	Operation string
}

type ReconcileJobResult struct {
	Success  bool
	Message  string
	Duration time.Duration
}

type HealthCheckJobPayload struct {
	CheckName string
	CheckType string
	Target    string
}

type HealthCheckJobResult struct {
	Healthy  bool
	Message  string
	Duration time.Duration
}

type CleanupJobPayload struct {
	ResourceType string
	ResourceName string
	Namespace    string
}

type CleanupJobResult struct {
	Cleaned  bool
	Message  string
	Duration time.Duration
}

type CacheJobPayload struct {
	Operation string
	Key       string
	Value     interface{}
}

type CacheJobResult struct {
	Success  bool
	Value    interface{}
	Message  string
	Duration time.Duration
}
