package health

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/health/grpc"
	"github.com/birbparty/roost-keeper/internal/health/http"
	"github.com/birbparty/roost-keeper/internal/health/tcp"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Checker defines the interface for health checking operations
type Checker interface {
	// CheckHealth performs health checks for a ManagedRoost
	CheckHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error)

	// GetHealthStatus returns detailed health status for all checks
	GetHealthStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost) ([]roostv1alpha1.HealthCheckStatus, error)
}

// HealthChecker implements the Checker interface
type HealthChecker struct {
	Logger *zap.Logger
}

// NewChecker creates a new health checker
func NewChecker(logger *zap.Logger) *HealthChecker {
	return &HealthChecker{
		Logger: logger,
	}
}

// CheckHealth performs all health checks for a ManagedRoost
func (hc *HealthChecker) CheckHealth(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "health.check", roost.Name, roost.Namespace)
	defer span.End()

	log := hc.Logger.With(
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
	)

	// If no health checks are defined, consider healthy
	if len(roost.Spec.HealthChecks) == 0 {
		log.Debug("No health checks defined, considering healthy")
		telemetry.RecordSpanSuccess(ctx)
		return true, nil
	}

	log.Info("Starting health checks", zap.Int("check_count", len(roost.Spec.HealthChecks)))

	allHealthy := true
	var lastError error

	// Execute all health checks
	for _, checkSpec := range roost.Spec.HealthChecks {
		healthy, err := hc.executeHealthCheck(ctx, roost, checkSpec)
		if err != nil {
			log.Error("Health check failed",
				zap.String("check_name", checkSpec.Name),
				zap.Error(err),
			)
			lastError = err
			allHealthy = false
		} else if !healthy {
			log.Warn("Health check unhealthy", zap.String("check_name", checkSpec.Name))
			allHealthy = false
		} else {
			log.Debug("Health check passed", zap.String("check_name", checkSpec.Name))
		}
	}

	if allHealthy {
		log.Info("All health checks passed")
		telemetry.RecordSpanSuccess(ctx)
	} else {
		log.Warn("Some health checks failed")
		if lastError != nil {
			telemetry.RecordSpanError(ctx, lastError)
		}
	}

	return allHealthy, lastError
}

// GetHealthStatus returns detailed health status for all checks
func (hc *HealthChecker) GetHealthStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost) ([]roostv1alpha1.HealthCheckStatus, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "health.get_status", roost.Name, roost.Namespace)
	defer span.End()

	var statuses []roostv1alpha1.HealthCheckStatus

	// If no health checks are defined, return empty status
	if len(roost.Spec.HealthChecks) == 0 {
		telemetry.RecordSpanSuccess(ctx)
		return statuses, nil
	}

	// Execute all health checks and collect statuses
	for _, checkSpec := range roost.Spec.HealthChecks {
		status := hc.getHealthCheckStatus(ctx, roost, checkSpec)
		statuses = append(statuses, status)
	}

	telemetry.RecordSpanSuccess(ctx)
	return statuses, nil
}

// executeHealthCheck executes a single health check
func (hc *HealthChecker) executeHealthCheck(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec) (bool, error) {
	log := hc.Logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("check_type", checkSpec.Type),
	)

	// Set timeout for the health check
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.Debug("Executing health check")

	switch checkSpec.Type {
	case "http":
		if checkSpec.HTTP == nil {
			return false, fmt.Errorf("HTTP health check spec is required")
		}
		return hc.executeHTTPCheck(checkCtx, roost, checkSpec, *checkSpec.HTTP)
	case "tcp":
		if checkSpec.TCP == nil {
			return false, fmt.Errorf("TCP health check spec is required")
		}
		return hc.executeTCPCheck(checkCtx, roost, checkSpec, *checkSpec.TCP)
	case "grpc":
		if checkSpec.GRPC == nil {
			return false, fmt.Errorf("gRPC health check spec is required")
		}
		return hc.executeGRPCCheck(checkCtx, roost, checkSpec, *checkSpec.GRPC)
	case "prometheus":
		if checkSpec.Prometheus == nil {
			return false, fmt.Errorf("Prometheus health check spec is required")
		}
		return hc.executePrometheusCheck(checkCtx, roost, checkSpec, *checkSpec.Prometheus)
	case "kubernetes":
		if checkSpec.Kubernetes == nil {
			return false, fmt.Errorf("Kubernetes health check spec is required")
		}
		return hc.executeKubernetesCheck(checkCtx, roost, checkSpec, *checkSpec.Kubernetes)
	default:
		return false, fmt.Errorf("unsupported health check type: %s", checkSpec.Type)
	}
}

// getHealthCheckStatus gets the status of a single health check
func (hc *HealthChecker) getHealthCheckStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec) roostv1alpha1.HealthCheckStatus {
	status := roostv1alpha1.HealthCheckStatus{
		Name: checkSpec.Name,
	}

	healthy, err := hc.executeHealthCheck(ctx, roost, checkSpec)
	if err != nil {
		status.Status = "unhealthy"
		status.Message = err.Error()
		status.FailureCount = 1 // TODO: Track failure count properly
	} else if healthy {
		status.Status = "healthy"
		status.Message = "Health check passed"
		status.FailureCount = 0
	} else {
		status.Status = "unhealthy"
		status.Message = "Health check failed"
		status.FailureCount = 1 // TODO: Track failure count properly
	}

	now := time.Now()
	status.LastCheck = &metav1.Time{Time: now}

	return status
}

// Health check execution methods

func (hc *HealthChecker) executeHTTPCheck(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, httpSpec roostv1alpha1.HTTPHealthCheckSpec) (bool, error) {
	// Import the http package function
	return http.ExecuteHTTPCheck(ctx, hc.Logger, roost, checkSpec, httpSpec)
}

func (hc *HealthChecker) executeTCPCheck(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, tcpSpec roostv1alpha1.TCPHealthCheckSpec) (bool, error) {
	// Import the tcp package function
	return tcp.ExecuteTCPCheck(ctx, hc.Logger, roost, checkSpec, tcpSpec)
}

func (hc *HealthChecker) executeGRPCCheck(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, grpcSpec roostv1alpha1.GRPCHealthCheckSpec) (bool, error) {
	// Import the grpc package function
	return grpc.ExecuteGRPCCheck(ctx, hc.Logger, roost, checkSpec, grpcSpec)
}

func (hc *HealthChecker) executePrometheusCheck(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, prometheusSpec roostv1alpha1.PrometheusHealthCheckSpec) (bool, error) {
	// TODO: Implement Prometheus health check
	return false, fmt.Errorf("Prometheus health checks not yet implemented")
}

func (hc *HealthChecker) executeKubernetesCheck(ctx context.Context, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, kubernetesSpec roostv1alpha1.KubernetesHealthCheckSpec) (bool, error) {
	// TODO: Implement Kubernetes health check
	return false, fmt.Errorf("Kubernetes health checks not yet implemented")
}
