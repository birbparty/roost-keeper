package events

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SimpleCircuitBreaker implements CircuitBreaker interface
type SimpleCircuitBreaker struct {
	config          CircuitBreakerConfig
	state           CircuitState
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	lastSuccessTime time.Time
	halfOpenCalls   int
	logger          *zap.Logger
	mu              sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig, logger *zap.Logger) CircuitBreaker {
	return &SimpleCircuitBreaker{
		config: config,
		state:  CircuitClosed,
		logger: logger.With(zap.String("component", "circuit-breaker")),
	}
}

// Execute executes a function with circuit breaker protection
func (cb *SimpleCircuitBreaker) Execute(fn func() error) error {
	if !cb.allowCall() {
		return fmt.Errorf("circuit breaker is open")
	}

	err := fn()
	cb.recordResult(err)
	return err
}

// State returns the current circuit state
func (cb *SimpleCircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset manually resets the circuit breaker
func (cb *SimpleCircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenCalls = 0
	cb.logger.Info("Circuit breaker manually reset")
}

// allowCall determines if a call should be allowed
func (cb *SimpleCircuitBreaker) allowCall() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) >= cb.config.RecoveryTimeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenCalls = 0
			cb.logger.Info("Circuit breaker transitioning from open to half-open")
			return true
		}
		return false
	case CircuitHalfOpen:
		// Allow limited calls in half-open state
		if cb.halfOpenCalls < cb.config.MaxHalfOpenCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	default:
		return false
	}
}

// recordResult records the result of a function execution
func (cb *SimpleCircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
}

// recordFailure records a failure
func (cb *SimpleCircuitBreaker) recordFailure() {
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.state = CircuitOpen
			cb.logger.Warn("Circuit breaker opening due to failure threshold",
				zap.Int("failure_count", cb.failureCount),
				zap.Int("threshold", cb.config.FailureThreshold))
		}
	case CircuitHalfOpen:
		// Any failure in half-open state should open the circuit
		cb.state = CircuitOpen
		cb.logger.Warn("Circuit breaker opening due to failure in half-open state")
	}
}

// recordSuccess records a success
func (cb *SimpleCircuitBreaker) recordSuccess() {
	cb.successCount++
	cb.lastSuccessTime = time.Now()

	switch cb.state {
	case CircuitHalfOpen:
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.state = CircuitClosed
			cb.failureCount = 0 // Reset failure count
			cb.logger.Info("Circuit breaker closing due to success threshold",
				zap.Int("success_count", cb.successCount),
				zap.Int("threshold", cb.config.SuccessThreshold))
		}
	case CircuitClosed:
		// Reset failure count on success in closed state
		cb.failureCount = 0
	}
}
