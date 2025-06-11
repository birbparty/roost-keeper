package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// GRPCChecker implements advanced gRPC health checking with connection pooling,
// streaming support, authentication, retry policies, and circuit breaker patterns
type GRPCChecker struct {
	connectionPool map[string]*ConnectionPool
	poolMutex      sync.RWMutex
	logger         *zap.Logger
	circuitBreaker map[string]*CircuitBreaker
	cbMutex        sync.RWMutex
}

// ConnectionPool manages gRPC connections for a specific target
type ConnectionPool struct {
	connections []*grpc.ClientConn
	target      string
	config      *roostv1alpha1.GRPCConnectionPoolSpec
	mutex       sync.RWMutex
	lastUsed    int
	healthCheck *time.Ticker
	stopCh      chan struct{}
}

// CircuitBreaker implements circuit breaker pattern for gRPC health checks
type CircuitBreaker struct {
	config          *roostv1alpha1.GRPCCircuitBreakerSpec
	state           CircuitState
	failureCount    int32
	successCount    int32
	lastFailureTime time.Time
	mutex           sync.RWMutex
}

// CircuitState represents the current state of the circuit breaker
type CircuitState int

const (
	// CircuitClosed - requests pass through normally
	CircuitClosed CircuitState = iota
	// CircuitOpen - requests are rejected immediately
	CircuitOpen
	// CircuitHalfOpen - limited requests are allowed to test service recovery
	CircuitHalfOpen
)

// HealthResult represents the result of a health check
type HealthResult struct {
	Healthy   bool                   `json:"healthy"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Latency   time.Duration          `json:"latency,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// StreamingHealthMonitor manages streaming health check connections
type StreamingHealthMonitor struct {
	stream   grpc_health_v1.Health_WatchClient
	target   string
	service  string
	stopCh   chan struct{}
	resultCh chan<- *HealthResult
	logger   *zap.Logger
}

// NewGRPCChecker creates a new advanced gRPC health checker
func NewGRPCChecker(logger *zap.Logger) *GRPCChecker {
	return &GRPCChecker{
		connectionPool: make(map[string]*ConnectionPool),
		logger:         logger,
		circuitBreaker: make(map[string]*CircuitBreaker),
	}
}

// ExecuteGRPCCheck performs a comprehensive gRPC health check
func ExecuteGRPCCheck(ctx context.Context, logger *zap.Logger, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, grpcSpec roostv1alpha1.GRPCHealthCheckSpec) (bool, error) {
	checker := NewGRPCChecker(logger)
	result, err := checker.CheckHealth(ctx, roost, checkSpec, grpcSpec)
	if err != nil {
		return false, err
	}
	return result.Healthy, nil
}

// CheckHealth performs a comprehensive gRPC health check with all advanced features
func (gc *GRPCChecker) CheckHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, grpcSpec roostv1alpha1.GRPCHealthCheckSpec) (*HealthResult, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "grpc.health_check", roost.Name, roost.Namespace)
	defer span.End()

	startTime := time.Now()
	target := fmt.Sprintf("%s:%d", grpcSpec.Host, grpcSpec.Port)

	log := gc.logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("target", target),
		zap.String("service", grpcSpec.Service),
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
	)

	log.Debug("Starting advanced gRPC health check")

	// Check circuit breaker state
	if cb := gc.getOrCreateCircuitBreaker(target, grpcSpec.CircuitBreaker); cb != nil {
		if !cb.allowRequest() {
			result := &HealthResult{
				Healthy:   false,
				Message:   "Circuit breaker is open - service unavailable",
				Details:   map[string]interface{}{"circuit_state": "open"},
				Timestamp: time.Now(),
			}
			telemetry.RecordSpanError(ctx, fmt.Errorf("circuit breaker open"))
			return result, nil
		}
	}

	// Execute health check with retry policy
	result, err := gc.executeWithRetry(ctx, target, grpcSpec, checkSpec, log)

	// Update circuit breaker state based on result
	if cb := gc.getCircuitBreaker(target); cb != nil {
		if result != nil && result.Healthy {
			cb.recordSuccess()
		} else {
			cb.recordFailure()
		}
	}

	if result != nil {
		result.Latency = time.Since(startTime)
	}

	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return result, err
	}

	if result.Healthy {
		telemetry.RecordSpanSuccess(ctx)
	}

	log.Debug("gRPC health check completed",
		zap.Bool("healthy", result.Healthy),
		zap.Duration("latency", result.Latency))

	return result, nil
}

// executeWithRetry executes the health check with retry policy
func (gc *GRPCChecker) executeWithRetry(ctx context.Context, target string, grpcSpec roostv1alpha1.GRPCHealthCheckSpec, checkSpec roostv1alpha1.HealthCheckSpec, log *zap.Logger) (*HealthResult, error) {
	retryPolicy := grpcSpec.RetryPolicy
	maxAttempts := int32(1) // Default no retry

	if retryPolicy != nil && retryPolicy.Enabled {
		maxAttempts = retryPolicy.MaxAttempts
		if maxAttempts == 0 {
			maxAttempts = 3 // Default retry count
		}
	}

	var lastErr error
	var lastResult *HealthResult

	for attempt := int32(0); attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			delay := gc.calculateRetryDelay(attempt, retryPolicy)
			log.Debug("Retrying gRPC health check",
				zap.Int32("attempt", attempt+1),
				zap.Duration("delay", delay))

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return lastResult, ctx.Err()
			}
		}

		result, err := gc.executeHealthCheck(ctx, target, grpcSpec, checkSpec, log)
		if err == nil && result.Healthy {
			return result, nil
		}

		lastResult = result
		lastErr = err

		// Check if error is retryable
		if !gc.isRetryableError(err, retryPolicy) {
			break
		}
	}

	return lastResult, lastErr
}

// executeHealthCheck performs the actual gRPC health check
func (gc *GRPCChecker) executeHealthCheck(ctx context.Context, target string, grpcSpec roostv1alpha1.GRPCHealthCheckSpec, checkSpec roostv1alpha1.HealthCheckSpec, log *zap.Logger) (*HealthResult, error) {
	// Get connection from pool or create new one
	conn, err := gc.getConnection(ctx, target, grpcSpec, log)
	if err != nil {
		return &HealthResult{
			Healthy:   false,
			Message:   fmt.Sprintf("Failed to establish gRPC connection: %v", err),
			Details:   map[string]interface{}{"error": err.Error()},
			Timestamp: time.Now(),
		}, err
	}

	// Create health client
	healthClient := grpc_health_v1.NewHealthClient(conn)

	// Set timeout
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Add custom metadata if specified
	if grpcSpec.Metadata != nil {
		md := metadata.New(grpcSpec.Metadata)
		checkCtx = metadata.NewOutgoingContext(checkCtx, md)
	}

	// Perform health check
	req := &grpc_health_v1.HealthCheckRequest{
		Service: grpcSpec.Service,
	}

	log.Debug("Performing gRPC health check RPC", zap.String("service", grpcSpec.Service))

	resp, err := healthClient.Check(checkCtx, req)
	if err != nil {
		return gc.handleGRPCError(err, grpcSpec.Service), err
	}

	return gc.evaluateHealthResponse(resp, grpcSpec.Service), nil
}

// getConnection retrieves a connection from the pool or creates a new one
func (gc *GRPCChecker) getConnection(ctx context.Context, target string, grpcSpec roostv1alpha1.GRPCHealthCheckSpec, log *zap.Logger) (*grpc.ClientConn, error) {
	// Check if connection pooling is enabled
	if grpcSpec.ConnectionPool != nil && grpcSpec.ConnectionPool.Enabled {
		return gc.getPooledConnection(ctx, target, grpcSpec, log)
	}

	// Create a direct connection
	return gc.createConnection(ctx, target, grpcSpec, log)
}

// getPooledConnection retrieves a connection from the connection pool
func (gc *GRPCChecker) getPooledConnection(ctx context.Context, target string, grpcSpec roostv1alpha1.GRPCHealthCheckSpec, log *zap.Logger) (*grpc.ClientConn, error) {
	gc.poolMutex.RLock()
	pool, exists := gc.connectionPool[target]
	gc.poolMutex.RUnlock()

	if !exists {
		gc.poolMutex.Lock()
		// Double-check after acquiring write lock
		if pool, exists = gc.connectionPool[target]; !exists {
			var err error
			pool, err = gc.createConnectionPool(ctx, target, grpcSpec, log)
			if err != nil {
				gc.poolMutex.Unlock()
				return nil, err
			}
			gc.connectionPool[target] = pool
		}
		gc.poolMutex.Unlock()
	}

	return pool.getConnection(ctx, log)
}

// createConnectionPool creates a new connection pool for the target
func (gc *GRPCChecker) createConnectionPool(ctx context.Context, target string, grpcSpec roostv1alpha1.GRPCHealthCheckSpec, log *zap.Logger) (*ConnectionPool, error) {
	config := grpcSpec.ConnectionPool
	if config == nil {
		config = &roostv1alpha1.GRPCConnectionPoolSpec{
			Enabled:        true,
			MaxConnections: 10,
		}
	}

	pool := &ConnectionPool{
		target:      target,
		config:      config,
		connections: make([]*grpc.ClientConn, 0, config.MaxConnections),
		stopCh:      make(chan struct{}),
	}

	// Create initial connections
	for i := int32(0); i < config.MaxConnections; i++ {
		conn, err := gc.createConnection(ctx, target, grpcSpec, log)
		if err != nil {
			// Clean up any connections created so far
			pool.close()
			return nil, fmt.Errorf("failed to create connection %d: %w", i, err)
		}
		pool.connections = append(pool.connections, conn)
	}

	// Start health monitoring for the pool
	pool.startHealthMonitoring(gc.logger)

	log.Debug("Created gRPC connection pool",
		zap.String("target", target),
		zap.Int32("size", config.MaxConnections))

	return pool, nil
}

// createConnection creates a new gRPC connection with all configuration options
func (gc *GRPCChecker) createConnection(ctx context.Context, target string, grpcSpec roostv1alpha1.GRPCHealthCheckSpec, log *zap.Logger) (*grpc.ClientConn, error) {
	var opts []grpc.DialOption

	// Configure TLS
	if grpcSpec.TLS != nil && grpcSpec.TLS.Enabled {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: grpcSpec.TLS.InsecureSkipVerify,
		}

		if grpcSpec.TLS.ServerName != "" {
			tlsConfig.ServerName = grpcSpec.TLS.ServerName
		}

		if len(grpcSpec.TLS.CABundle) > 0 {
			// TODO: Add CA bundle support
			log.Debug("CA bundle configuration not yet implemented")
		}

		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		log.Debug("Configured TLS for gRPC connection")
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		log.Debug("Using insecure gRPC connection")
	}

	// Configure authentication
	if grpcSpec.Auth != nil {
		perRPCCreds, err := gc.createAuthCredentials(grpcSpec.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to create auth credentials: %w", err)
		}
		if perRPCCreds != nil {
			opts = append(opts, grpc.WithPerRPCCredentials(perRPCCreds))
			log.Debug("Configured authentication for gRPC connection")
		}
	}

	// Configure keep-alive
	if grpcSpec.ConnectionPool != nil {
		keepAliveParams := keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}

		if grpcSpec.ConnectionPool.KeepAliveTime.Duration > 0 {
			keepAliveParams.Time = grpcSpec.ConnectionPool.KeepAliveTime.Duration
		}
		if grpcSpec.ConnectionPool.KeepAliveTimeout.Duration > 0 {
			keepAliveParams.Timeout = grpcSpec.ConnectionPool.KeepAliveTimeout.Duration
		}

		opts = append(opts, grpc.WithKeepaliveParams(keepAliveParams))
		log.Debug("Configured keep-alive parameters")
	}

	// Create connection
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	log.Debug("Dialing gRPC connection", zap.String("target", target))

	conn, err := grpc.DialContext(dialCtx, target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial gRPC server: %w", err)
	}

	return conn, nil
}

// createAuthCredentials creates per-RPC credentials based on auth configuration
func (gc *GRPCChecker) createAuthCredentials(auth *roostv1alpha1.GRPCAuthSpec) (credentials.PerRPCCredentials, error) {
	if auth.JWT != "" {
		return &jwtCredentials{token: auth.JWT}, nil
	}

	if auth.BearerToken != "" {
		return &bearerTokenCredentials{token: auth.BearerToken}, nil
	}

	if len(auth.Headers) > 0 {
		return &headerCredentials{headers: auth.Headers}, nil
	}

	if auth.BasicAuth != nil {
		return &basicAuthCredentials{
			username: auth.BasicAuth.Username,
			password: auth.BasicAuth.Password,
		}, nil
	}

	// OAuth2 support would be implemented here
	if auth.OAuth2 != nil {
		gc.logger.Debug("OAuth2 authentication not yet implemented")
		return nil, nil
	}

	return nil, nil
}

// StartStreamingHealthCheck starts a streaming health check using the Watch method
func (gc *GRPCChecker) StartStreamingHealthCheck(ctx context.Context, roost *roostv1alpha1.ManagedRoost, grpcSpec roostv1alpha1.GRPCHealthCheckSpec, resultCh chan<- *HealthResult) error {
	target := fmt.Sprintf("%s:%d", grpcSpec.Host, grpcSpec.Port)

	log := gc.logger.With(
		zap.String("target", target),
		zap.String("service", grpcSpec.Service),
		zap.String("mode", "streaming"),
	)

	log.Info("Starting streaming gRPC health check")

	conn, err := gc.getConnection(ctx, target, grpcSpec, log)
	if err != nil {
		return fmt.Errorf("failed to get connection for streaming: %w", err)
	}

	healthClient := grpc_health_v1.NewHealthClient(conn)

	req := &grpc_health_v1.HealthCheckRequest{
		Service: grpcSpec.Service,
	}

	stream, err := healthClient.Watch(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to start health watch: %w", err)
	}

	monitor := &StreamingHealthMonitor{
		stream:   stream,
		target:   target,
		service:  grpcSpec.Service,
		stopCh:   make(chan struct{}),
		resultCh: resultCh,
		logger:   log,
	}

	go monitor.run(ctx)

	return nil
}

// Credential implementations for different authentication methods

type jwtCredentials struct {
	token string
}

func (j *jwtCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + j.token,
	}, nil
}

func (j *jwtCredentials) RequireTransportSecurity() bool {
	return true
}

type bearerTokenCredentials struct {
	token string
}

func (b *bearerTokenCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": "Bearer " + b.token,
	}, nil
}

func (b *bearerTokenCredentials) RequireTransportSecurity() bool {
	return true
}

type headerCredentials struct {
	headers map[string]string
}

func (h *headerCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return h.headers, nil
}

func (h *headerCredentials) RequireTransportSecurity() bool {
	return false
}

type basicAuthCredentials struct {
	username string
	password string
}

func (b *basicAuthCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	auth := b.username + ":" + b.password
	return map[string]string{
		"authorization": "Basic " + auth,
	}, nil
}

func (b *basicAuthCredentials) RequireTransportSecurity() bool {
	return true
}

// Connection pool methods

func (cp *ConnectionPool) getConnection(ctx context.Context, log *zap.Logger) (*grpc.ClientConn, error) {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	if len(cp.connections) == 0 {
		return nil, fmt.Errorf("no connections available in pool")
	}

	// Round-robin selection
	conn := cp.connections[cp.lastUsed]
	cp.lastUsed = (cp.lastUsed + 1) % len(cp.connections)

	// Return the connection - gRPC will handle reconnection if needed
	return conn, nil
}

func (cp *ConnectionPool) startHealthMonitoring(logger *zap.Logger) {
	if cp.config.HealthCheckInterval.Duration <= 0 {
		return
	}

	cp.healthCheck = time.NewTicker(cp.config.HealthCheckInterval.Duration)

	go func() {
		for {
			select {
			case <-cp.healthCheck.C:
				cp.checkConnectionHealth(logger)
			case <-cp.stopCh:
				cp.healthCheck.Stop()
				return
			}
		}
	}()
}

func (cp *ConnectionPool) checkConnectionHealth(logger *zap.Logger) {
	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	// Basic health monitoring - just log that monitoring is active
	logger.Debug("Checking connection pool health",
		zap.String("target", cp.target),
		zap.Int("connections", len(cp.connections)))
}

func (cp *ConnectionPool) close() {
	close(cp.stopCh)

	if cp.healthCheck != nil {
		cp.healthCheck.Stop()
	}

	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	for _, conn := range cp.connections {
		conn.Close()
	}
	cp.connections = nil
}

// Circuit breaker implementation

func (gc *GRPCChecker) getOrCreateCircuitBreaker(target string, config *roostv1alpha1.GRPCCircuitBreakerSpec) *CircuitBreaker {
	if config == nil || !config.Enabled {
		return nil
	}

	gc.cbMutex.RLock()
	cb, exists := gc.circuitBreaker[target]
	gc.cbMutex.RUnlock()

	if !exists {
		gc.cbMutex.Lock()
		// Double-check after acquiring write lock
		if cb, exists = gc.circuitBreaker[target]; !exists {
			cb = &CircuitBreaker{
				config: config,
				state:  CircuitClosed,
			}
			gc.circuitBreaker[target] = cb
		}
		gc.cbMutex.Unlock()
	}

	return cb
}

func (gc *GRPCChecker) getCircuitBreaker(target string) *CircuitBreaker {
	gc.cbMutex.RLock()
	defer gc.cbMutex.RUnlock()
	return gc.circuitBreaker[target]
}

func (cb *CircuitBreaker) allowRequest() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout.Duration {
			cb.state = CircuitHalfOpen
			cb.successCount = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount = 0

	if cb.state == CircuitHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			cb.state = CircuitClosed
			cb.successCount = 0
		}
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= cb.config.FailureThreshold {
		cb.state = CircuitOpen
	}
}

// Streaming health monitor

func (shm *StreamingHealthMonitor) run(ctx context.Context) {
	defer close(shm.resultCh)

	for {
		select {
		case <-ctx.Done():
			shm.logger.Debug("Streaming health check context cancelled")
			return
		case <-shm.stopCh:
			shm.logger.Debug("Streaming health check stopped")
			return
		default:
			resp, err := shm.stream.Recv()
			if err != nil {
				shm.logger.Error("Streaming health check error", zap.Error(err))
				shm.resultCh <- &HealthResult{
					Healthy:   false,
					Message:   fmt.Sprintf("Stream error: %v", err),
					Timestamp: time.Now(),
				}
				return
			}

			result := evaluateHealthResponseStatic(resp, shm.service)
			shm.resultCh <- result
		}
	}
}

// Utility methods

func (gc *GRPCChecker) calculateRetryDelay(attempt int32, retryPolicy *roostv1alpha1.GRPCRetryPolicySpec) time.Duration {
	if retryPolicy == nil {
		return time.Second
	}

	initialDelay := time.Second
	if retryPolicy.InitialDelay.Duration > 0 {
		initialDelay = retryPolicy.InitialDelay.Duration
	}

	maxDelay := 30 * time.Second
	if retryPolicy.MaxDelay.Duration > 0 {
		maxDelay = retryPolicy.MaxDelay.Duration
	}

	multiplier := 2.0
	if retryPolicy.Multiplier != "" {
		if m, err := strconv.ParseFloat(retryPolicy.Multiplier, 64); err == nil {
			multiplier = m
		}
	}

	delay := time.Duration(float64(initialDelay) * multiplier * float64(attempt))
	if delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

func (gc *GRPCChecker) isRetryableError(err error, retryPolicy *roostv1alpha1.GRPCRetryPolicySpec) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		return true // Network errors are generally retryable
	}

	// Default retryable status codes
	defaultRetryable := []codes.Code{
		codes.Unavailable,
		codes.DeadlineExceeded,
		codes.ResourceExhausted,
		codes.Aborted,
	}

	// Check configured retryable status codes
	if retryPolicy != nil && len(retryPolicy.RetryableStatusCodes) > 0 {
		for _, codeStr := range retryPolicy.RetryableStatusCodes {
			if strings.ToLower(codeStr) == strings.ToLower(st.Code().String()) {
				return true
			}
		}
		return false
	}

	// Use default retryable codes
	for _, code := range defaultRetryable {
		if st.Code() == code {
			return true
		}
	}

	return false
}

func (gc *GRPCChecker) handleGRPCError(err error, service string) *HealthResult {
	st, ok := status.FromError(err)
	if !ok {
		return &HealthResult{
			Healthy:   false,
			Message:   fmt.Sprintf("gRPC health check failed: %v", err),
			Details:   map[string]interface{}{"error": err.Error()},
			Timestamp: time.Now(),
		}
	}

	details := map[string]interface{}{
		"grpc_code":    st.Code().String(),
		"grpc_message": st.Message(),
		"service":      service,
	}

	var message string
	switch st.Code() {
	case codes.NotFound:
		message = fmt.Sprintf("gRPC service '%s' not found", service)
	case codes.Unimplemented:
		message = "gRPC health checking not implemented by service"
	case codes.DeadlineExceeded:
		message = "gRPC health check timeout"
	case codes.Unavailable:
		message = "gRPC service unavailable"
	case codes.PermissionDenied:
		message = "gRPC permission denied"
	case codes.Unauthenticated:
		message = "gRPC authentication failed"
	default:
		message = fmt.Sprintf("gRPC error: %s", st.Message())
	}

	return &HealthResult{
		Healthy:   false,
		Message:   message,
		Details:   details,
		Timestamp: time.Now(),
	}
}

func (gc *GRPCChecker) evaluateHealthResponse(resp *grpc_health_v1.HealthCheckResponse, service string) *HealthResult {
	return evaluateHealthResponseStatic(resp, service)
}

func evaluateHealthResponseStatic(resp *grpc_health_v1.HealthCheckResponse, service string) *HealthResult {
	details := map[string]interface{}{
		"service": service,
		"status":  resp.Status.String(),
	}

	switch resp.Status {
	case grpc_health_v1.HealthCheckResponse_SERVING:
		return &HealthResult{
			Healthy:   true,
			Message:   "gRPC service is healthy",
			Details:   details,
			Timestamp: time.Now(),
		}
	case grpc_health_v1.HealthCheckResponse_NOT_SERVING:
		return &HealthResult{
			Healthy:   false,
			Message:   "gRPC service is not serving",
			Details:   details,
			Timestamp: time.Now(),
		}
	case grpc_health_v1.HealthCheckResponse_UNKNOWN:
		return &HealthResult{
			Healthy:   false,
			Message:   "gRPC service health status unknown",
			Details:   details,
			Timestamp: time.Now(),
		}
	default:
		return &HealthResult{
			Healthy:   false,
			Message:   fmt.Sprintf("Unknown gRPC health status: %v", resp.Status),
			Details:   details,
			Timestamp: time.Now(),
		}
	}
}

// Close cleans up all resources used by the gRPC checker
func (gc *GRPCChecker) Close() error {
	gc.poolMutex.Lock()
	defer gc.poolMutex.Unlock()

	for target, pool := range gc.connectionPool {
		gc.logger.Debug("Closing connection pool", zap.String("target", target))
		pool.close()
	}

	gc.connectionPool = make(map[string]*ConnectionPool)
	gc.circuitBreaker = make(map[string]*CircuitBreaker)

	return nil
}
