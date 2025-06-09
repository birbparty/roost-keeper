package tcp

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// ExecuteTCPCheck performs a TCP health check
func ExecuteTCPCheck(ctx context.Context, logger *zap.Logger, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, tcpSpec roostv1alpha1.TCPHealthCheckSpec) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "health.tcp_check", roost.Name, roost.Namespace)
	defer span.End()

	log := logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("host", tcpSpec.Host),
		zap.Int32("port", tcpSpec.Port),
	)

	log.Debug("Starting TCP health check")

	// Determine timeout
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	// Create dialer with timeout
	dialer := &net.Dialer{
		Timeout: timeout,
	}

	// Construct address
	address := fmt.Sprintf("%s:%d", tcpSpec.Host, tcpSpec.Port)

	log.Debug("Attempting TCP connection", zap.String("address", address))

	// Attempt to connect
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("TCP connection failed", zap.Error(err))
		return false, fmt.Errorf("TCP connection failed: %w", err)
	}

	// Immediately close the connection since we're just checking connectivity
	conn.Close()

	log.Debug("TCP health check passed")
	telemetry.RecordSpanSuccess(ctx)
	return true, nil
}
