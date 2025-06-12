package performance

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConnectionPool manages reusable connections for enterprise-scale performance
type ConnectionPool struct {
	k8sClient client.Client
	logger    *zap.Logger
	pools     map[string]*Pool
	stats     *ConnectionPoolStats
	config    *ConnectionPoolConfig

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	mutex  sync.RWMutex
}

// ConnectionPoolConfig holds connection pool configuration
type ConnectionPoolConfig struct {
	MaxConnections      int           // Maximum connections per pool
	MinConnections      int           // Minimum connections to maintain
	MaxIdleTime         time.Duration // Maximum time a connection can be idle
	ConnectionTimeout   time.Duration // Timeout for creating new connections
	HealthCheckInterval time.Duration // How often to health check connections
	CleanupInterval     time.Duration // How often to clean up idle connections
}

// ConnectionPoolStats contains connection pool metrics
type ConnectionPoolStats struct {
	TotalPools           int   `json:"totalPools"`
	TotalConnections     int   `json:"totalConnections"`
	ActiveConnections    int   `json:"activeConnections"`
	IdleConnections      int   `json:"idleConnections"`
	ConnectionsCreated   int64 `json:"connectionsCreated"`
	ConnectionsDestroyed int64 `json:"connectionsDestroyed"`
	ConnectionErrors     int64 `json:"connectionErrors"`

	mutex sync.RWMutex
}

// Pool represents a connection pool for a specific resource type
type Pool struct {
	name        string
	connections chan *Connection
	active      map[string]*Connection
	stats       *PoolStats
	config      *ConnectionPoolConfig
	logger      *zap.Logger
	mutex       sync.RWMutex
}

// Connection represents a pooled connection
type Connection struct {
	ID         string
	Type       string
	Resource   interface{}
	CreatedAt  time.Time
	LastUsed   time.Time
	InUse      bool
	Healthy    bool
	UsageCount int64
}

// PoolStats contains metrics for a specific pool
type PoolStats struct {
	Size           int   `json:"size"`
	Active         int   `json:"active"`
	Idle           int   `json:"idle"`
	Created        int64 `json:"created"`
	Destroyed      int64 `json:"destroyed"`
	Hits           int64 `json:"hits"`
	Misses         int64 `json:"misses"`
	HealthyCount   int   `json:"healthy"`
	UnhealthyCount int   `json:"unhealthy"`

	mutex sync.RWMutex
}

// DefaultConnectionPoolConfig returns the default connection pool configuration
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxConnections:      50,
		MinConnections:      5,
		MaxIdleTime:         5 * time.Minute,
		ConnectionTimeout:   30 * time.Second,
		HealthCheckInterval: 1 * time.Minute,
		CleanupInterval:     2 * time.Minute,
	}
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(k8sClient client.Client, logger *zap.Logger) *ConnectionPool {
	return NewConnectionPoolWithConfig(k8sClient, DefaultConnectionPoolConfig(), logger)
}

// NewConnectionPoolWithConfig creates a new connection pool with custom configuration
func NewConnectionPoolWithConfig(k8sClient client.Client, config *ConnectionPoolConfig, logger *zap.Logger) *ConnectionPool {
	cp := &ConnectionPool{
		k8sClient: k8sClient,
		logger:    logger,
		pools:     make(map[string]*Pool),
		config:    config,
		stats:     &ConnectionPoolStats{},
	}

	return cp
}

// Start initializes the connection pool and starts background management
func (cp *ConnectionPool) Start(ctx context.Context) error {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	cp.ctx, cp.cancel = context.WithCancel(ctx)

	// Start background management routines
	go cp.healthCheckLoop()
	go cp.cleanupLoop()

	cp.logger.Info("Connection pool started",
		zap.Int("max_connections", cp.config.MaxConnections),
		zap.Int("min_connections", cp.config.MinConnections),
		zap.Duration("max_idle_time", cp.config.MaxIdleTime))

	return nil
}

// Stop gracefully shuts down the connection pool
func (cp *ConnectionPool) Stop() error {
	cp.mutex.Lock()
	defer cp.mutex.Unlock()

	if cp.cancel != nil {
		cp.cancel()
	}

	// Close all pools
	for name, pool := range cp.pools {
		cp.closePool(pool)
		cp.logger.Debug("Closed connection pool", zap.String("pool", name))
	}

	cp.pools = make(map[string]*Pool)
	cp.logger.Info("Connection pool stopped")

	return nil
}

// GetConnection retrieves a connection from the specified pool
func (cp *ConnectionPool) GetConnection(poolName string) (*Connection, error) {
	cp.mutex.RLock()
	pool, exists := cp.pools[poolName]
	cp.mutex.RUnlock()

	if !exists {
		cp.mutex.Lock()
		// Double-check pattern
		if pool, exists = cp.pools[poolName]; !exists {
			pool = cp.createPool(poolName)
			cp.pools[poolName] = pool
		}
		cp.mutex.Unlock()
	}

	return pool.getConnection()
}

// ReturnConnection returns a connection to its pool
func (cp *ConnectionPool) ReturnConnection(poolName string, conn *Connection) error {
	cp.mutex.RLock()
	pool, exists := cp.pools[poolName]
	cp.mutex.RUnlock()

	if !exists {
		return nil // Pool doesn't exist, connection is orphaned
	}

	return pool.returnConnection(conn)
}

// GetStats returns current connection pool statistics
func (cp *ConnectionPool) GetStats() ConnectionPoolStats {
	cp.stats.mutex.RLock()
	defer cp.stats.mutex.RUnlock()

	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	totalConnections := 0
	activeConnections := 0
	idleConnections := 0

	for _, pool := range cp.pools {
		pool.mutex.RLock()
		totalConnections += len(pool.active)
		for _, conn := range pool.active {
			if conn.InUse {
				activeConnections++
			} else {
				idleConnections++
			}
		}
		pool.mutex.RUnlock()
	}

	return ConnectionPoolStats{
		TotalPools:           len(cp.pools),
		TotalConnections:     totalConnections,
		ActiveConnections:    activeConnections,
		IdleConnections:      idleConnections,
		ConnectionsCreated:   cp.stats.ConnectionsCreated,
		ConnectionsDestroyed: cp.stats.ConnectionsDestroyed,
		ConnectionErrors:     cp.stats.ConnectionErrors,
	}
}

// createPool creates a new connection pool
func (cp *ConnectionPool) createPool(name string) *Pool {
	pool := &Pool{
		name:        name,
		connections: make(chan *Connection, cp.config.MaxConnections),
		active:      make(map[string]*Connection),
		config:      cp.config,
		logger:      cp.logger.With(zap.String("pool", name)),
		stats:       &PoolStats{},
	}

	cp.logger.Info("Created connection pool", zap.String("pool", name))
	return pool
}

// closePool closes a connection pool and all its connections
func (cp *ConnectionPool) closePool(pool *Pool) {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	// Close all active connections
	for _, conn := range pool.active {
		cp.closeConnection(conn)
	}

	// Close pooled connections
	close(pool.connections)
	for conn := range pool.connections {
		cp.closeConnection(conn)
	}

	pool.active = make(map[string]*Connection)
}

// closeConnection closes a single connection
func (cp *ConnectionPool) closeConnection(conn *Connection) {
	// Connection-specific cleanup would go here
	cp.stats.mutex.Lock()
	cp.stats.ConnectionsDestroyed++
	cp.stats.mutex.Unlock()

	cp.logger.Debug("Closed connection", zap.String("connection_id", conn.ID))
}

// healthCheckLoop periodically health checks all connections
func (cp *ConnectionPool) healthCheckLoop() {
	ticker := time.NewTicker(cp.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cp.performHealthChecks()
		case <-cp.ctx.Done():
			return
		}
	}
}

// cleanupLoop periodically cleans up idle connections
func (cp *ConnectionPool) cleanupLoop() {
	ticker := time.NewTicker(cp.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cp.cleanupIdleConnections()
		case <-cp.ctx.Done():
			return
		}
	}
}

// performHealthChecks checks the health of all connections
func (cp *ConnectionPool) performHealthChecks() {
	cp.mutex.RLock()
	pools := make([]*Pool, 0, len(cp.pools))
	for _, pool := range cp.pools {
		pools = append(pools, pool)
	}
	cp.mutex.RUnlock()

	for _, pool := range pools {
		pool.healthCheck()
	}
}

// cleanupIdleConnections removes idle connections that exceed the max idle time
func (cp *ConnectionPool) cleanupIdleConnections() {
	cp.mutex.RLock()
	pools := make([]*Pool, 0, len(cp.pools))
	for _, pool := range cp.pools {
		pools = append(pools, pool)
	}
	cp.mutex.RUnlock()

	for _, pool := range pools {
		pool.cleanup()
	}
}

// Pool implementation

// getConnection retrieves a connection from the pool
func (p *Pool) getConnection() (*Connection, error) {
	// Try to get an existing connection from the pool
	select {
	case conn := <-p.connections:
		if conn.Healthy && time.Since(conn.LastUsed) < p.config.MaxIdleTime {
			conn.InUse = true
			conn.LastUsed = time.Now()
			conn.UsageCount++

			p.stats.mutex.Lock()
			p.stats.Hits++
			p.stats.mutex.Unlock()

			return conn, nil
		} else {
			// Connection is unhealthy or too old, destroy it
			p.destroyConnection(conn)
		}
	default:
		// No available connections in pool
	}

	// Create a new connection
	conn, err := p.createConnection()
	if err != nil {
		p.stats.mutex.Lock()
		p.stats.Misses++
		p.stats.mutex.Unlock()
		return nil, err
	}

	p.stats.mutex.Lock()
	p.stats.Misses++
	p.stats.mutex.Unlock()

	return conn, nil
}

// returnConnection returns a connection to the pool
func (p *Pool) returnConnection(conn *Connection) error {
	if conn == nil {
		return nil
	}

	conn.InUse = false
	conn.LastUsed = time.Now()

	// Try to return to pool if there's space
	select {
	case p.connections <- conn:
		return nil
	default:
		// Pool is full, destroy the connection
		p.destroyConnection(conn)
		return nil
	}
}

// createConnection creates a new connection
func (p *Pool) createConnection() (*Connection, error) {
	connID := generateConnectionID()

	conn := &Connection{
		ID:         connID,
		Type:       p.name,
		CreatedAt:  time.Now(),
		LastUsed:   time.Now(),
		InUse:      true,
		Healthy:    true,
		UsageCount: 1,
	}

	// Connection-specific initialization would go here
	// For example, creating HTTP clients, database connections, etc.

	p.mutex.Lock()
	p.active[connID] = conn
	p.mutex.Unlock()

	p.stats.mutex.Lock()
	p.stats.Created++
	p.stats.Size++
	p.stats.mutex.Unlock()

	p.logger.Debug("Created new connection", zap.String("connection_id", connID))

	return conn, nil
}

// destroyConnection destroys a connection
func (p *Pool) destroyConnection(conn *Connection) {
	if conn == nil {
		return
	}

	p.mutex.Lock()
	delete(p.active, conn.ID)
	p.mutex.Unlock()

	p.stats.mutex.Lock()
	p.stats.Destroyed++
	p.stats.Size--
	p.stats.mutex.Unlock()

	p.logger.Debug("Destroyed connection", zap.String("connection_id", conn.ID))
}

// healthCheck performs health checks on all connections in the pool
func (p *Pool) healthCheck() {
	p.mutex.RLock()
	connections := make([]*Connection, 0, len(p.active))
	for _, conn := range p.active {
		connections = append(connections, conn)
	}
	p.mutex.RUnlock()

	healthyCount := 0
	unhealthyCount := 0

	for _, conn := range connections {
		if p.isConnectionHealthy(conn) {
			conn.Healthy = true
			healthyCount++
		} else {
			conn.Healthy = false
			unhealthyCount++

			// If connection is unhealthy and not in use, destroy it
			if !conn.InUse {
				p.destroyConnection(conn)
			}
		}
	}

	p.stats.mutex.Lock()
	p.stats.HealthyCount = healthyCount
	p.stats.UnhealthyCount = unhealthyCount
	p.stats.mutex.Unlock()

	if unhealthyCount > 0 {
		p.logger.Debug("Health check completed",
			zap.Int("healthy", healthyCount),
			zap.Int("unhealthy", unhealthyCount))
	}
}

// cleanup removes idle connections that exceed the max idle time
func (p *Pool) cleanup() {
	now := time.Now()

	p.mutex.RLock()
	idleConnections := make([]*Connection, 0)
	for _, conn := range p.active {
		if !conn.InUse && now.Sub(conn.LastUsed) > p.config.MaxIdleTime {
			idleConnections = append(idleConnections, conn)
		}
	}
	p.mutex.RUnlock()

	for _, conn := range idleConnections {
		p.destroyConnection(conn)
	}

	if len(idleConnections) > 0 {
		p.logger.Debug("Cleaned up idle connections",
			zap.Int("removed", len(idleConnections)))
	}
}

// isConnectionHealthy checks if a connection is healthy
func (p *Pool) isConnectionHealthy(conn *Connection) bool {
	// This is a simplified health check
	// In a real implementation, you would perform actual health checks
	// based on the connection type (HTTP client, database connection, etc.)

	// Check if connection is too old
	if time.Since(conn.CreatedAt) > 30*time.Minute {
		return false
	}

	// Check if connection has been idle too long
	if time.Since(conn.LastUsed) > p.config.MaxIdleTime {
		return false
	}

	return true
}

// generateConnectionID generates a unique connection ID
func generateConnectionID() string {
	return time.Now().Format("20060102-150405.000000")
}

// GetPoolStats returns statistics for a specific pool
func (cp *ConnectionPool) GetPoolStats(poolName string) (*PoolStats, bool) {
	cp.mutex.RLock()
	pool, exists := cp.pools[poolName]
	cp.mutex.RUnlock()

	if !exists {
		return nil, false
	}

	pool.stats.mutex.RLock()
	defer pool.stats.mutex.RUnlock()

	return &PoolStats{
		Size:           pool.stats.Size,
		Active:         pool.stats.Active,
		Idle:           pool.stats.Idle,
		Created:        pool.stats.Created,
		Destroyed:      pool.stats.Destroyed,
		Hits:           pool.stats.Hits,
		Misses:         pool.stats.Misses,
		HealthyCount:   pool.stats.HealthyCount,
		UnhealthyCount: pool.stats.UnhealthyCount,
	}, true
}

// GetPoolNames returns all pool names
func (cp *ConnectionPool) GetPoolNames() []string {
	cp.mutex.RLock()
	defer cp.mutex.RUnlock()

	names := make([]string, 0, len(cp.pools))
	for name := range cp.pools {
		names = append(names, name)
	}

	return names
}
