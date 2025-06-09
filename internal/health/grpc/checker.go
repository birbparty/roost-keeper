package grpc

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// ExecuteGRPCCheck performs a gRPC health check
func ExecuteGRPCCheck(ctx context.Context, logger *zap.Logger, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, grpcSpec roostv1alpha1.GRPCHealthCheckSpec) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "health.grpc_check", roost.Name, roost.Namespace)
	defer span.End()

	log := logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("host", grpcSpec.Host),
		zap.Int32("port", grpcSpec.Port),
		zap.String("service", grpcSpec.Service),
	)

	log.Debug("Starting gRPC health check")

	// Determine timeout
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	// Create connection context with timeout
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Construct address
	address := fmt.Sprintf("%s:%d", grpcSpec.Host, grpcSpec.Port)

	// Configure connection options
	var opts []grpc.DialOption

	// Configure TLS
	if grpcSpec.TLS != nil {
		if grpcSpec.TLS.InsecureSkipVerify {
			log.Debug("Using insecure TLS connection")
			tlsConfig := &tls.Config{
				InsecureSkipVerify: true,
			}
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
		} else {
			log.Debug("Using secure TLS connection")
			opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
		}
	} else {
		log.Debug("Using insecure connection")
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	log.Debug("Establishing gRPC connection", zap.String("address", address))

	// Establish connection
	conn, err := grpc.DialContext(dialCtx, address, opts...)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("gRPC connection failed", zap.Error(err))
		return false, fmt.Errorf("gRPC connection failed: %w", err)
	}
	defer conn.Close()

	// Create health check client
	healthClient := grpc_health_v1.NewHealthClient(conn)

	// Determine service name for health check
	serviceName := grpcSpec.Service
	if serviceName == "" {
		serviceName = "" // Empty string means check overall server health
	}

	log.Debug("Performing gRPC health check", zap.String("service", serviceName))

	// Create request context with timeout
	checkCtx, checkCancel := context.WithTimeout(ctx, timeout)
	defer checkCancel()

	// Perform health check
	req := &grpc_health_v1.HealthCheckRequest{
		Service: serviceName,
	}

	resp, err := healthClient.Check(checkCtx, req)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("gRPC health check failed", zap.Error(err))
		return false, fmt.Errorf("gRPC health check failed: %w", err)
	}

	// Check response status
	if resp.Status != grpc_health_v1.HealthCheckResponse_SERVING {
		log.Warn("gRPC service not serving",
			zap.String("status", resp.Status.String()),
			zap.String("service", serviceName),
		)
		return false, nil
	}

	log.Debug("gRPC health check passed", zap.String("status", resp.Status.String()))
	telemetry.RecordSpanSuccess(ctx)
	return true, nil
}
