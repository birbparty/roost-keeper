package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// APIMetrics is an alias to avoid import cycles
type APIMetrics = telemetry.APIMetrics

// Manager provides comprehensive API integration for ManagedRoost resources
type Manager struct {
	client      client.Client
	scheme      *runtime.Scheme
	recorder    record.EventRecorder
	metrics     *APIMetrics
	logger      *zap.Logger
	statusCache *StatusCache
	validator   *ResourceValidator
	lifecycle   *LifecycleTracker
}

// NewManager creates a new API manager with all required dependencies
func NewManager(client client.Client, scheme *runtime.Scheme, recorder record.EventRecorder, metrics *APIMetrics, logger *zap.Logger) *Manager {
	return &Manager{
		client:      client,
		scheme:      scheme,
		recorder:    recorder,
		metrics:     metrics,
		logger:      logger,
		statusCache: NewStatusCache(),
		validator:   NewResourceValidator(logger),
		lifecycle:   NewLifecycleTracker(logger, recorder),
	}
}

// ValidateResource performs comprehensive validation of ManagedRoost resource
func (m *Manager) ValidateResource(ctx context.Context, managedRoost *roostkeeper.ManagedRoost) (*ValidationResult, error) {
	log := m.logger.With(
		zap.String("operation", "validate_resource"),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	// Start validation span for observability
	ctx, span := telemetry.StartAPISpan(ctx, "validate", managedRoost.Name, managedRoost.Namespace)
	defer span.End()

	start := time.Now()
	defer func() {
		if m.metrics != nil {
			m.metrics.RecordAPIOperation(ctx, "validate", time.Since(start).Seconds())
		}
	}()

	log.Info("Starting resource validation")

	// Check cache first for performance
	cacheKey := m.generateValidationCacheKey(managedRoost)
	if cached := m.statusCache.GetValidation(cacheKey); cached != nil {
		log.Debug("Using cached validation result")
		if m.metrics != nil {
			m.metrics.RecordCacheHit(ctx, "validation")
		}
		return cached, nil
	}

	if m.metrics != nil {
		m.metrics.RecordCacheMiss(ctx, "validation")
	}

	// Run validation rules
	errors := m.validator.Validate(managedRoost)

	// Categorize validation results
	result := &ValidationResult{
		Valid:    len(filterByLevel(errors, ValidationLevelError)) == 0,
		Errors:   filterByLevel(errors, ValidationLevelError),
		Warnings: filterByLevel(errors, ValidationLevelWarning),
		Info:     filterByLevel(errors, ValidationLevelInfo),
	}

	// Cache the result
	m.statusCache.SetValidation(cacheKey, result)

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordValidationOperation(ctx, result.Valid, len(result.Errors))
	}

	// Emit validation events
	if !result.Valid {
		m.recorder.Event(managedRoost, "Warning", "ValidationFailed",
			fmt.Sprintf("Resource validation failed: %d errors", len(result.Errors)))
	} else if len(result.Warnings) > 0 {
		m.recorder.Event(managedRoost, "Normal", "ValidationWarnings",
			fmt.Sprintf("Resource validation passed with %d warnings", len(result.Warnings)))
	}

	log.Info("Resource validation completed",
		zap.Bool("valid", result.Valid),
		zap.Int("errors", len(result.Errors)),
		zap.Int("warnings", len(result.Warnings)))

	return result, nil
}

// UpdateStatus updates the status of a ManagedRoost with enhanced caching and observability
func (m *Manager) UpdateStatus(ctx context.Context, managedRoost *roostkeeper.ManagedRoost, statusUpdate *StatusUpdate) error {
	log := m.logger.With(
		zap.String("operation", "update_status"),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	ctx, span := telemetry.StartAPISpan(ctx, "status_update", managedRoost.Name, managedRoost.Namespace)
	defer span.End()

	start := time.Now()
	defer func() {
		if m.metrics != nil {
			m.metrics.RecordAPIOperation(ctx, "status_update", time.Since(start).Seconds())
		}
	}()

	log.Info("Updating resource status")

	// Apply status update
	oldPhase := managedRoost.Status.Phase
	m.applyStatusUpdate(managedRoost, statusUpdate)

	// Update status subresource
	if err := m.client.Status().Update(ctx, managedRoost); err != nil {
		log.Error("Failed to update status", zap.Error(err))
		telemetry.RecordError(span, err)
		if m.metrics != nil {
			m.metrics.RecordAPIError(ctx, "status_update")
		}
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Cache the status
	m.statusCache.Set(getResourceKey(managedRoost), &managedRoost.Status)

	// Emit status change events
	if oldPhase != managedRoost.Status.Phase {
		m.recorder.Event(managedRoost, "Normal", "PhaseChanged",
			fmt.Sprintf("Phase changed from %s to %s", oldPhase, managedRoost.Status.Phase))
	}

	// Emit condition events
	for _, condition := range statusUpdate.Conditions {
		eventType := "Normal"
		if condition.Status == metav1.ConditionFalse {
			eventType = "Warning"
		}
		m.recorder.Event(managedRoost, eventType, condition.Type, condition.Message)
	}

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordStatusUpdate(ctx, string(managedRoost.Status.Phase))
	}

	log.Info("Status updated successfully",
		zap.String("phase", string(managedRoost.Status.Phase)),
		zap.Int("conditions", len(managedRoost.Status.Conditions)))

	return nil
}

// TrackLifecycle records lifecycle events with comprehensive tracking
func (m *Manager) TrackLifecycle(ctx context.Context, managedRoost *roostkeeper.ManagedRoost, event LifecycleEvent) error {
	log := m.logger.With(
		zap.String("operation", "track_lifecycle"),
		zap.String("event", string(event.Type)),
		zap.String("roost", managedRoost.Name),
	)

	ctx, span := telemetry.StartAPISpan(ctx, "lifecycle_track", managedRoost.Name, managedRoost.Namespace)
	defer span.End()

	log.Info("Tracking lifecycle event")

	// Use lifecycle tracker
	if err := m.lifecycle.TrackEvent(ctx, managedRoost, event); err != nil {
		log.Error("Failed to track lifecycle event", zap.Error(err))
		return err
	}

	// Record metrics
	if m.metrics != nil {
		m.metrics.RecordLifecycleEvent(ctx, string(event.Type))
	}

	log.Info("Lifecycle event tracked successfully")
	return nil
}

// GetCachedStatus retrieves cached status for performance optimization
func (m *Manager) GetCachedStatus(managedRoost *roostkeeper.ManagedRoost) *roostkeeper.ManagedRoostStatus {
	return m.statusCache.Get(getResourceKey(managedRoost))
}

// InvalidateCache invalidates cache entries for a resource
func (m *Manager) InvalidateCache(managedRoost *roostkeeper.ManagedRoost) {
	resourceKey := getResourceKey(managedRoost)
	validationKey := m.generateValidationCacheKey(managedRoost)

	m.statusCache.Delete(resourceKey)
	m.statusCache.DeleteValidation(validationKey)
}

// applyStatusUpdate applies a status update to the ManagedRoost
func (m *Manager) applyStatusUpdate(managedRoost *roostkeeper.ManagedRoost, update *StatusUpdate) {
	now := metav1.Time{Time: time.Now()}

	// Update phase if specified
	if update.Phase != "" {
		managedRoost.Status.Phase = roostkeeper.ManagedRoostPhase(update.Phase)
	}

	// Update last update time
	managedRoost.Status.LastUpdateTime = &now

	// Update or add conditions
	for _, newCondition := range update.Conditions {
		found := false
		for i, existingCondition := range managedRoost.Status.Conditions {
			if existingCondition.Type == newCondition.Type {
				// Update existing condition
				managedRoost.Status.Conditions[i] = metav1.Condition{
					Type:               newCondition.Type,
					Status:             newCondition.Status,
					LastTransitionTime: now,
					Reason:             newCondition.Reason,
					Message:            newCondition.Message,
				}
				found = true
				break
			}
		}

		if !found {
			// Add new condition
			managedRoost.Status.Conditions = append(managedRoost.Status.Conditions, metav1.Condition{
				Type:               newCondition.Type,
				Status:             newCondition.Status,
				LastTransitionTime: now,
				Reason:             newCondition.Reason,
				Message:            newCondition.Message,
			})
		}
	}

	// Update observability status
	if update.ObservabilityStatus != nil {
		managedRoost.Status.Observability = update.ObservabilityStatus
	}

	// Update health status
	if update.HealthStatus != nil {
		managedRoost.Status.Health = *update.HealthStatus
	}
}

// generateValidationCacheKey generates a cache key for validation results
func (m *Manager) generateValidationCacheKey(managedRoost *roostkeeper.ManagedRoost) string {
	return fmt.Sprintf("validation:%s:%s:%d",
		managedRoost.Namespace,
		managedRoost.Name,
		managedRoost.Generation)
}

// Helper functions

func getResourceKey(managedRoost *roostkeeper.ManagedRoost) string {
	return fmt.Sprintf("%s/%s", managedRoost.Namespace, managedRoost.Name)
}

func filterByLevel(errors []ValidationError, level ValidationLevel) []ValidationError {
	var filtered []ValidationError
	for _, err := range errors {
		if err.Level == level {
			filtered = append(filtered, err)
		}
	}
	return filtered
}

// StatusCache provides thread-safe caching for status operations
type StatusCache struct {
	statusCache     map[string]*CachedStatus
	validationCache map[string]*CachedValidationResult
	mutex           sync.RWMutex
}

// CachedStatus represents a cached status with TTL
type CachedStatus struct {
	Status    *roostkeeper.ManagedRoostStatus
	Timestamp time.Time
	TTL       time.Duration
}

// CachedValidationResult represents a cached validation result
type CachedValidationResult struct {
	Result    *ValidationResult
	Timestamp time.Time
	TTL       time.Duration
}

// NewStatusCache creates a new status cache
func NewStatusCache() *StatusCache {
	return &StatusCache{
		statusCache:     make(map[string]*CachedStatus),
		validationCache: make(map[string]*CachedValidationResult),
	}
}

// Get retrieves a cached status
func (sc *StatusCache) Get(key string) *roostkeeper.ManagedRoostStatus {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	cached, exists := sc.statusCache[key]
	if !exists {
		return nil
	}

	// Check TTL
	if time.Since(cached.Timestamp) > cached.TTL {
		delete(sc.statusCache, key)
		return nil
	}

	return cached.Status
}

// Set stores a status in cache
func (sc *StatusCache) Set(key string, status *roostkeeper.ManagedRoostStatus) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	sc.statusCache[key] = &CachedStatus{
		Status:    status,
		Timestamp: time.Now(),
		TTL:       30 * time.Second, // 30 second TTL
	}
}

// Delete removes a status from cache
func (sc *StatusCache) Delete(key string) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	delete(sc.statusCache, key)
}

// GetValidation retrieves a cached validation result
func (sc *StatusCache) GetValidation(key string) *ValidationResult {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	cached, exists := sc.validationCache[key]
	if !exists {
		return nil
	}

	// Check TTL
	if time.Since(cached.Timestamp) > cached.TTL {
		delete(sc.validationCache, key)
		return nil
	}

	return cached.Result
}

// SetValidation stores a validation result in cache
func (sc *StatusCache) SetValidation(key string, result *ValidationResult) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	sc.validationCache[key] = &CachedValidationResult{
		Result:    result,
		Timestamp: time.Now(),
		TTL:       5 * time.Minute, // 5 minute TTL for validation results
	}
}

// DeleteValidation removes a validation result from cache
func (sc *StatusCache) DeleteValidation(key string) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	delete(sc.validationCache, key)
}
