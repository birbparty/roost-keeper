package composite

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	// CircuitBreakerClosed indicates the circuit breaker is closed (normal operation)
	CircuitBreakerClosed CircuitBreakerState = iota
	// CircuitBreakerOpen indicates the circuit breaker is open (failing fast)
	CircuitBreakerOpen
	// CircuitBreakerHalfOpen indicates the circuit breaker is in half-open state (testing)
	CircuitBreakerHalfOpen
)

// String returns the string representation of the circuit breaker state
func (s CircuitBreakerState) String() string {
	switch s {
	case CircuitBreakerClosed:
		return "closed"
	case CircuitBreakerOpen:
		return "open"
	case CircuitBreakerHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig defines the configuration for a circuit breaker
type CircuitBreakerConfig struct {
	Name             string        `json:"name"`
	FailureThreshold int32         `json:"failure_threshold"`
	SuccessThreshold int32         `json:"success_threshold"`
	Timeout          time.Duration `json:"timeout"`
	HalfOpenTimeout  time.Duration `json:"half_open_timeout"`
}

// CircuitBreaker implements the circuit breaker pattern for health checks
type CircuitBreaker struct {
	config       CircuitBreakerConfig
	state        CircuitBreakerState
	failureCount int32
	successCount int32
	lastFailure  *time.Time
	stateChanged time.Time
	logger       *zap.Logger
	mutex        sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config:       config,
		state:        CircuitBreakerClosed,
		stateChanged: time.Now(),
		logger:       zap.L().With(zap.String("circuit_breaker", config.Name)),
	}
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case CircuitBreakerOpen:
		// Check if timeout has elapsed to transition to half-open
		if time.Since(cb.stateChanged) > cb.config.Timeout {
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			// Double-check pattern to avoid race condition
			if cb.state == CircuitBreakerOpen && time.Since(cb.stateChanged) > cb.config.Timeout {
				cb.transitionToHalfOpen()
			}
			cb.mutex.Unlock()
			cb.mutex.RLock()
		}
		return cb.state == CircuitBreakerOpen
	case CircuitBreakerHalfOpen:
		// Check if half-open timeout has elapsed
		if time.Since(cb.stateChanged) > cb.config.HalfOpenTimeout {
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			// Double-check pattern
			if cb.state == CircuitBreakerHalfOpen && time.Since(cb.stateChanged) > cb.config.HalfOpenTimeout {
				cb.transitionToOpen()
			}
			cb.mutex.Unlock()
			cb.mutex.RLock()
		}
		return false
	default:
		return false
	}
}

// RecordResult records the result of a health check and updates the circuit breaker state
func (cb *CircuitBreaker) RecordResult(result *HealthResult, err error) {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	isSuccess := err == nil && result != nil && result.Healthy

	switch cb.state {
	case CircuitBreakerClosed:
		if isSuccess {
			cb.resetFailureCount()
		} else {
			cb.incrementFailureCount()
			if cb.failureCount >= cb.config.FailureThreshold {
				cb.transitionToOpen()
			}
		}

	case CircuitBreakerHalfOpen:
		if isSuccess {
			cb.incrementSuccessCount()
			if cb.successCount >= cb.config.SuccessThreshold {
				cb.transitionToClosed()
			}
		} else {
			cb.transitionToOpen()
		}

	case CircuitBreakerOpen:
		// Record failure but don't change state
		// State transition is handled by timeout in IsOpen()
		if !isSuccess {
			now := time.Now()
			cb.lastFailure = &now
		}
	}
}

// GetStatus returns the current status of the circuit breaker
func (cb *CircuitBreaker) GetStatus() *CircuitBreakerStatus {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	status := &CircuitBreakerStatus{
		State:        cb.state,
		FailureCount: cb.failureCount,
		SuccessCount: cb.successCount,
		LastFailure:  cb.lastFailure,
		StateChanged: cb.stateChanged,
	}

	// Calculate next attempt time for open state
	if cb.state == CircuitBreakerOpen {
		nextAttempt := cb.stateChanged.Add(cb.config.Timeout)
		status.NextAttempt = &nextAttempt
	}

	return status
}

// GetConfig returns the circuit breaker configuration
func (cb *CircuitBreaker) GetConfig() CircuitBreakerConfig {
	return cb.config
}

// transitionToOpen transitions the circuit breaker to open state (must be called with write lock)
func (cb *CircuitBreaker) transitionToOpen() {
	if cb.state != CircuitBreakerOpen {
		previousState := cb.state
		cb.state = CircuitBreakerOpen
		cb.stateChanged = time.Now()
		cb.successCount = 0

		cb.logger.Warn("Circuit breaker opened",
			zap.String("previous_state", previousState.String()),
			zap.Int32("failure_count", cb.failureCount),
			zap.Duration("timeout", cb.config.Timeout))
	}
}

// transitionToHalfOpen transitions the circuit breaker to half-open state (must be called with write lock)
func (cb *CircuitBreaker) transitionToHalfOpen() {
	if cb.state != CircuitBreakerHalfOpen {
		previousState := cb.state
		cb.state = CircuitBreakerHalfOpen
		cb.stateChanged = time.Now()
		cb.successCount = 0

		cb.logger.Info("Circuit breaker transitioned to half-open",
			zap.String("previous_state", previousState.String()),
			zap.Duration("half_open_timeout", cb.config.HalfOpenTimeout))
	}
}

// transitionToClosed transitions the circuit breaker to closed state (must be called with write lock)
func (cb *CircuitBreaker) transitionToClosed() {
	if cb.state != CircuitBreakerClosed {
		previousState := cb.state
		cb.state = CircuitBreakerClosed
		cb.stateChanged = time.Now()
		cb.failureCount = 0
		cb.successCount = 0

		cb.logger.Info("Circuit breaker closed",
			zap.String("previous_state", previousState.String()),
			zap.Int32("success_count", cb.successCount))
	}
}

// incrementFailureCount increments the failure counter (must be called with write lock)
func (cb *CircuitBreaker) incrementFailureCount() {
	cb.failureCount++
	now := time.Now()
	cb.lastFailure = &now
}

// resetFailureCount resets the failure counter (must be called with write lock)
func (cb *CircuitBreaker) resetFailureCount() {
	cb.failureCount = 0
}

// incrementSuccessCount increments the success counter (must be called with write lock)
func (cb *CircuitBreaker) incrementSuccessCount() {
	cb.successCount++
}

// Reset resets the circuit breaker to its initial state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = CircuitBreakerClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.lastFailure = nil
	cb.stateChanged = time.Now()

	cb.logger.Info("Circuit breaker reset")
}

// GetMetrics returns circuit breaker metrics for monitoring
func (cb *CircuitBreaker) GetMetrics() map[string]interface{} {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	metrics := map[string]interface{}{
		"name":          cb.config.Name,
		"state":         cb.state.String(),
		"failure_count": cb.failureCount,
		"success_count": cb.successCount,
		"state_changed": cb.stateChanged,
		"config":        cb.config,
	}

	if cb.lastFailure != nil {
		metrics["last_failure"] = *cb.lastFailure
	}

	if cb.state == CircuitBreakerOpen {
		nextAttempt := cb.stateChanged.Add(cb.config.Timeout)
		metrics["next_attempt"] = nextAttempt
		metrics["time_until_retry"] = time.Until(nextAttempt)
	}

	return metrics
}
