package tcp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Connection pool for TCP health checks
var (
	connectionPools = make(map[string]*tcpConnectionPool)
	poolMutex       sync.RWMutex
)

// tcpConnectionPool manages a pool of TCP connections for a specific endpoint
type tcpConnectionPool struct {
	address     string
	connections chan net.Conn
	mutex       sync.Mutex
	maxSize     int
	activeConns int
}

// newTCPConnectionPool creates a new TCP connection pool
func newTCPConnectionPool(address string, maxSize int) *tcpConnectionPool {
	return &tcpConnectionPool{
		address:     address,
		connections: make(chan net.Conn, maxSize),
		maxSize:     maxSize,
	}
}

// getConnection retrieves a connection from the pool or creates a new one
func (p *tcpConnectionPool) getConnection(ctx context.Context, timeout time.Duration) (net.Conn, error) {
	// Try to get an existing connection from the pool
	select {
	case conn := <-p.connections:
		// Test if connection is still valid
		if isConnectionValid(conn) {
			return conn, nil
		}
		// Connection is invalid, close it and create a new one
		conn.Close()
		p.mutex.Lock()
		p.activeConns--
		p.mutex.Unlock()
	default:
		// No connections available in pool
	}

	// Create new connection
	dialer := &net.Dialer{
		Timeout: timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", p.address)
	if err != nil {
		return nil, err
	}

	p.mutex.Lock()
	p.activeConns++
	p.mutex.Unlock()

	return conn, nil
}

// returnConnection returns a connection to the pool
func (p *tcpConnectionPool) returnConnection(conn net.Conn) {
	if conn == nil {
		return
	}

	// Only return to pool if it's still valid and pool isn't full
	if isConnectionValid(conn) {
		select {
		case p.connections <- conn:
			// Successfully returned to pool
			return
		default:
			// Pool is full, close the connection
		}
	}

	// Close connection and decrement counter
	conn.Close()
	p.mutex.Lock()
	p.activeConns--
	p.mutex.Unlock()
}

// isConnectionValid checks if a TCP connection is still valid
func isConnectionValid(conn net.Conn) bool {
	if conn == nil {
		return false
	}

	// Set a very short read deadline to test connection
	conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))

	// Try to read one byte
	buffer := make([]byte, 1)
	_, err := conn.Read(buffer)

	// Reset deadline
	conn.SetReadDeadline(time.Time{})

	// If we get a timeout, connection is likely valid
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	// Any other error means connection is invalid
	return err == nil
}

// ExecuteTCPCheck performs a TCP health check
func ExecuteTCPCheck(ctx context.Context, logger *zap.Logger, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, tcpSpec roostv1alpha1.TCPHealthCheckSpec) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "health.tcp_check", roost.Name, roost.Namespace)
	defer span.End()

	log := logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("host", tcpSpec.Host),
		zap.Int32("port", tcpSpec.Port),
		zap.Bool("pooling_enabled", tcpSpec.EnablePooling),
	)

	log.Debug("Starting TCP health check")

	// Determine timeouts
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	connectionTimeout := timeout
	if tcpSpec.ConnectionTimeout != nil && tcpSpec.ConnectionTimeout.Duration > 0 {
		connectionTimeout = tcpSpec.ConnectionTimeout.Duration
	}

	// Construct address
	address := fmt.Sprintf("%s:%d", tcpSpec.Host, tcpSpec.Port)

	log.Debug("Attempting TCP connection",
		zap.String("address", address),
		zap.Duration("connection_timeout", connectionTimeout),
		zap.Duration("total_timeout", timeout),
	)

	start := time.Now()

	// Get connection (either from pool or create new)
	var conn net.Conn
	var err error

	if tcpSpec.EnablePooling {
		conn, err = getTCPConnectionFromPool(ctx, address, connectionTimeout)
	} else {
		dialer := &net.Dialer{
			Timeout: connectionTimeout,
		}
		conn, err = dialer.DialContext(ctx, "tcp", address)
	}

	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("TCP connection failed", zap.Error(err))
		return false, fmt.Errorf("TCP connection failed: %w", err)
	}

	connectionTime := time.Since(start)
	log.Debug("TCP connection established", zap.Duration("connection_time", connectionTime))

	// Handle connection cleanup
	defer func() {
		if tcpSpec.EnablePooling {
			returnTCPConnectionToPool(address, conn)
		} else {
			conn.Close()
		}
	}()

	// Perform protocol validation if configured
	if tcpSpec.SendData != "" || tcpSpec.ExpectedResponse != "" {
		success, err := performTCPProtocolValidation(ctx, log, conn, tcpSpec, timeout)
		if err != nil {
			telemetry.RecordSpanError(ctx, err)
			log.Error("TCP protocol validation failed", zap.Error(err))
			return false, fmt.Errorf("TCP protocol validation failed: %w", err)
		}
		if !success {
			log.Warn("TCP protocol validation unsuccessful")
			return false, nil
		}
		log.Debug("TCP protocol validation passed")
	}

	totalTime := time.Since(start)
	log.Debug("TCP health check passed",
		zap.Duration("total_time", totalTime),
		zap.Duration("connection_time", connectionTime),
	)
	telemetry.RecordSpanSuccess(ctx)
	return true, nil
}

// getTCPConnectionFromPool gets a connection from the pool
func getTCPConnectionFromPool(ctx context.Context, address string, timeout time.Duration) (net.Conn, error) {
	poolMutex.Lock()
	pool, exists := connectionPools[address]
	if !exists {
		pool = newTCPConnectionPool(address, 5) // Max 5 connections per pool
		connectionPools[address] = pool
	}
	poolMutex.Unlock()

	return pool.getConnection(ctx, timeout)
}

// returnTCPConnectionToPool returns a connection to the pool
func returnTCPConnectionToPool(address string, conn net.Conn) {
	poolMutex.RLock()
	pool, exists := connectionPools[address]
	poolMutex.RUnlock()

	if exists {
		pool.returnConnection(conn)
	} else {
		// Pool doesn't exist, just close connection
		if conn != nil {
			conn.Close()
		}
	}
}

// performTCPProtocolValidation performs protocol-specific validation
func performTCPProtocolValidation(ctx context.Context, log *zap.Logger, conn net.Conn, tcpSpec roostv1alpha1.TCPHealthCheckSpec, timeout time.Duration) (bool, error) {
	// Set overall deadline for protocol validation
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return false, fmt.Errorf("failed to set connection deadline: %w", err)
	}

	// Send data if specified
	if tcpSpec.SendData != "" {
		log.Debug("Sending TCP data", zap.String("data", tcpSpec.SendData))

		_, err := conn.Write([]byte(tcpSpec.SendData))
		if err != nil {
			return false, fmt.Errorf("failed to send TCP data: %w", err)
		}

		log.Debug("TCP data sent successfully")
	}

	// Read and validate response if expected
	if tcpSpec.ExpectedResponse != "" {
		log.Debug("Reading TCP response", zap.String("expected", tcpSpec.ExpectedResponse))

		buffer := make([]byte, 4096) // Larger buffer for responses
		n, err := conn.Read(buffer)
		if err != nil {
			return false, fmt.Errorf("failed to read TCP response: %w", err)
		}

		response := string(buffer[:n])
		log.Debug("Received TCP response",
			zap.String("response", response),
			zap.Int("bytes_received", n),
		)

		// Validate response
		if !validateTCPResponse(response, tcpSpec.ExpectedResponse) {
			return false, fmt.Errorf("unexpected TCP response: got '%s', expected '%s'", response, tcpSpec.ExpectedResponse)
		}

		log.Debug("TCP response validation passed")
	}

	return true, nil
}

// validateTCPResponse validates if the received response matches expectations
func validateTCPResponse(response, expected string) bool {
	// Exact match
	if response == expected {
		return true
	}

	// Case-insensitive match
	if strings.EqualFold(response, expected) {
		return true
	}

	// Contains match (for flexible protocol responses)
	if strings.Contains(response, expected) {
		return true
	}

	// Trim whitespace and try again
	trimmedResponse := strings.TrimSpace(response)
	trimmedExpected := strings.TrimSpace(expected)

	if trimmedResponse == trimmedExpected {
		return true
	}

	return false
}
