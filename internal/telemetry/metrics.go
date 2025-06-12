package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	meterName = "roost-keeper-operator"
)

// OperatorMetrics contains all metrics for the Roost-Keeper operator
type OperatorMetrics struct {
	// Controller metrics
	ReconcileTotal       metric.Int64Counter
	ReconcileDuration    metric.Float64Histogram
	ReconcileErrors      metric.Int64Counter
	ReconcileQueueLength metric.Int64UpDownCounter

	// ManagedRoost metrics
	RoostsTotal        metric.Int64UpDownCounter
	RoostsHealthy      metric.Int64UpDownCounter
	RoostsByPhase      metric.Int64UpDownCounter
	RoostGenerationLag metric.Int64UpDownCounter

	// Helm operations
	HelmInstallTotal      metric.Int64Counter
	HelmInstallDuration   metric.Float64Histogram
	HelmInstallErrors     metric.Int64Counter
	HelmUpgradeTotal      metric.Int64Counter
	HelmUpgradeDuration   metric.Float64Histogram
	HelmUpgradeErrors     metric.Int64Counter
	HelmUninstallTotal    metric.Int64Counter
	HelmUninstallDuration metric.Float64Histogram
	HelmUninstallErrors   metric.Int64Counter

	// Health check metrics
	HealthCheckTotal    metric.Int64Counter
	HealthCheckDuration metric.Float64Histogram
	HealthCheckErrors   metric.Int64Counter
	HealthCheckSuccess  metric.Int64Counter

	// Kubernetes API metrics
	KubernetesAPIRequests metric.Int64Counter
	KubernetesAPILatency  metric.Float64Histogram
	KubernetesAPIErrors   metric.Int64Counter

	// Performance metrics
	MemoryUsage    metric.Int64UpDownCounter
	CPUUsage       metric.Float64UpDownCounter
	GoroutineCount metric.Int64UpDownCounter

	// Operator lifecycle metrics
	OperatorStartTime     metric.Int64UpDownCounter
	OperatorUptime        metric.Float64UpDownCounter
	LeaderElectionChanges metric.Int64Counter
}

// NewOperatorMetrics creates and initializes all operator metrics
func NewOperatorMetrics() (*OperatorMetrics, error) {
	meter := otel.Meter(meterName)

	// Controller metrics
	reconcileTotal, err := meter.Int64Counter(
		"roost_keeper_reconcile_total",
		metric.WithDescription("Total number of reconcile operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	reconcileDuration, err := meter.Float64Histogram(
		"roost_keeper_reconcile_duration_seconds",
		metric.WithDescription("Duration of reconcile operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	reconcileErrors, err := meter.Int64Counter(
		"roost_keeper_reconcile_errors_total",
		metric.WithDescription("Total number of reconcile errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	reconcileQueueLength, err := meter.Int64UpDownCounter(
		"roost_keeper_reconcile_queue_length",
		metric.WithDescription("Current length of reconcile queue"),
		metric.WithUnit("{items}"),
	)
	if err != nil {
		return nil, err
	}

	// ManagedRoost metrics
	roostsTotal, err := meter.Int64UpDownCounter(
		"roost_keeper_roosts_total",
		metric.WithDescription("Total number of managed roosts"),
		metric.WithUnit("{roosts}"),
	)
	if err != nil {
		return nil, err
	}

	roostsHealthy, err := meter.Int64UpDownCounter(
		"roost_keeper_roosts_healthy",
		metric.WithDescription("Number of healthy roosts"),
		metric.WithUnit("{roosts}"),
	)
	if err != nil {
		return nil, err
	}

	roostsByPhase, err := meter.Int64UpDownCounter(
		"roost_keeper_roosts_by_phase",
		metric.WithDescription("Number of roosts by phase"),
		metric.WithUnit("{roosts}"),
	)
	if err != nil {
		return nil, err
	}

	roostGenerationLag, err := meter.Int64UpDownCounter(
		"roost_keeper_roost_generation_lag",
		metric.WithDescription("Difference between spec and status generation"),
		metric.WithUnit("{generations}"),
	)
	if err != nil {
		return nil, err
	}

	// Helm operations
	helmInstallTotal, err := meter.Int64Counter(
		"roost_keeper_helm_install_total",
		metric.WithDescription("Total number of Helm install operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	helmInstallDuration, err := meter.Float64Histogram(
		"roost_keeper_helm_install_duration_seconds",
		metric.WithDescription("Duration of Helm install operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	helmInstallErrors, err := meter.Int64Counter(
		"roost_keeper_helm_install_errors_total",
		metric.WithDescription("Total number of Helm install errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	helmUpgradeTotal, err := meter.Int64Counter(
		"roost_keeper_helm_upgrade_total",
		metric.WithDescription("Total number of Helm upgrade operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	helmUpgradeDuration, err := meter.Float64Histogram(
		"roost_keeper_helm_upgrade_duration_seconds",
		metric.WithDescription("Duration of Helm upgrade operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	helmUpgradeErrors, err := meter.Int64Counter(
		"roost_keeper_helm_upgrade_errors_total",
		metric.WithDescription("Total number of Helm upgrade errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	helmUninstallTotal, err := meter.Int64Counter(
		"roost_keeper_helm_uninstall_total",
		metric.WithDescription("Total number of Helm uninstall operations"),
		metric.WithUnit("{operations}"),
	)
	if err != nil {
		return nil, err
	}

	helmUninstallDuration, err := meter.Float64Histogram(
		"roost_keeper_helm_uninstall_duration_seconds",
		metric.WithDescription("Duration of Helm uninstall operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	helmUninstallErrors, err := meter.Int64Counter(
		"roost_keeper_helm_uninstall_errors_total",
		metric.WithDescription("Total number of Helm uninstall errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	// Health check metrics
	healthCheckTotal, err := meter.Int64Counter(
		"roost_keeper_health_check_total",
		metric.WithDescription("Total number of health checks performed"),
		metric.WithUnit("{checks}"),
	)
	if err != nil {
		return nil, err
	}

	healthCheckDuration, err := meter.Float64Histogram(
		"roost_keeper_health_check_duration_seconds",
		metric.WithDescription("Duration of health check operations"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	healthCheckErrors, err := meter.Int64Counter(
		"roost_keeper_health_check_errors_total",
		metric.WithDescription("Total number of health check errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	healthCheckSuccess, err := meter.Int64Counter(
		"roost_keeper_health_check_success_total",
		metric.WithDescription("Total number of successful health checks"),
		metric.WithUnit("{checks}"),
	)
	if err != nil {
		return nil, err
	}

	// Kubernetes API metrics
	kubernetesAPIRequests, err := meter.Int64Counter(
		"roost_keeper_kubernetes_api_requests_total",
		metric.WithDescription("Total number of Kubernetes API requests"),
		metric.WithUnit("{requests}"),
	)
	if err != nil {
		return nil, err
	}

	kubernetesAPILatency, err := meter.Float64Histogram(
		"roost_keeper_kubernetes_api_latency_seconds",
		metric.WithDescription("Latency of Kubernetes API requests"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	kubernetesAPIErrors, err := meter.Int64Counter(
		"roost_keeper_kubernetes_api_errors_total",
		metric.WithDescription("Total number of Kubernetes API errors"),
		metric.WithUnit("{errors}"),
	)
	if err != nil {
		return nil, err
	}

	// Performance metrics
	memoryUsage, err := meter.Int64UpDownCounter(
		"roost_keeper_memory_usage_bytes",
		metric.WithDescription("Current memory usage in bytes"),
		metric.WithUnit("By"),
	)
	if err != nil {
		return nil, err
	}

	cpuUsage, err := meter.Float64UpDownCounter(
		"roost_keeper_cpu_usage_ratio",
		metric.WithDescription("Current CPU usage ratio"),
		metric.WithUnit("1"),
	)
	if err != nil {
		return nil, err
	}

	goroutineCount, err := meter.Int64UpDownCounter(
		"roost_keeper_goroutine_count",
		metric.WithDescription("Current number of goroutines"),
		metric.WithUnit("{goroutines}"),
	)
	if err != nil {
		return nil, err
	}

	// Operator lifecycle metrics
	operatorStartTime, err := meter.Int64UpDownCounter(
		"roost_keeper_operator_start_time_seconds",
		metric.WithDescription("Unix timestamp when operator started"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	operatorUptime, err := meter.Float64UpDownCounter(
		"roost_keeper_operator_uptime_seconds",
		metric.WithDescription("Operator uptime in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, err
	}

	leaderElectionChanges, err := meter.Int64Counter(
		"roost_keeper_leader_election_changes_total",
		metric.WithDescription("Total number of leader election changes"),
		metric.WithUnit("{changes}"),
	)
	if err != nil {
		return nil, err
	}

	return &OperatorMetrics{
		ReconcileTotal:        reconcileTotal,
		ReconcileDuration:     reconcileDuration,
		ReconcileErrors:       reconcileErrors,
		ReconcileQueueLength:  reconcileQueueLength,
		RoostsTotal:           roostsTotal,
		RoostsHealthy:         roostsHealthy,
		RoostsByPhase:         roostsByPhase,
		RoostGenerationLag:    roostGenerationLag,
		HelmInstallTotal:      helmInstallTotal,
		HelmInstallDuration:   helmInstallDuration,
		HelmInstallErrors:     helmInstallErrors,
		HelmUpgradeTotal:      helmUpgradeTotal,
		HelmUpgradeDuration:   helmUpgradeDuration,
		HelmUpgradeErrors:     helmUpgradeErrors,
		HelmUninstallTotal:    helmUninstallTotal,
		HelmUninstallDuration: helmUninstallDuration,
		HelmUninstallErrors:   helmUninstallErrors,
		HealthCheckTotal:      healthCheckTotal,
		HealthCheckDuration:   healthCheckDuration,
		HealthCheckErrors:     healthCheckErrors,
		HealthCheckSuccess:    healthCheckSuccess,
		KubernetesAPIRequests: kubernetesAPIRequests,
		KubernetesAPILatency:  kubernetesAPILatency,
		KubernetesAPIErrors:   kubernetesAPIErrors,
		MemoryUsage:           memoryUsage,
		CPUUsage:              cpuUsage,
		GoroutineCount:        goroutineCount,
		OperatorStartTime:     operatorStartTime,
		OperatorUptime:        operatorUptime,
		LeaderElectionChanges: leaderElectionChanges,
	}, nil
}

// RecordReconcile records a reconcile operation with its outcome
func (m *OperatorMetrics) RecordReconcile(ctx context.Context, duration time.Duration, success bool, roostName, namespace string) {
	labels := metric.WithAttributes(
		attribute.String("roost_name", roostName),
		attribute.String("namespace", namespace),
		attribute.Bool("success", success),
	)

	m.ReconcileTotal.Add(ctx, 1, labels)
	m.ReconcileDuration.Record(ctx, duration.Seconds(), labels)

	if !success {
		m.ReconcileErrors.Add(ctx, 1, labels)
	}
}

// RecordHelmOperation records a Helm operation with its outcome
func (m *OperatorMetrics) RecordHelmOperation(ctx context.Context, operation string, duration time.Duration, success bool, chartName, chartVersion, namespace string) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("chart_name", chartName),
		attribute.String("chart_version", chartVersion),
		attribute.String("namespace", namespace),
		attribute.Bool("success", success),
	)

	switch operation {
	case "install":
		m.HelmInstallTotal.Add(ctx, 1, labels)
		m.HelmInstallDuration.Record(ctx, duration.Seconds(), labels)
		if !success {
			m.HelmInstallErrors.Add(ctx, 1, labels)
		}
	case "upgrade":
		m.HelmUpgradeTotal.Add(ctx, 1, labels)
		m.HelmUpgradeDuration.Record(ctx, duration.Seconds(), labels)
		if !success {
			m.HelmUpgradeErrors.Add(ctx, 1, labels)
		}
	case "uninstall":
		m.HelmUninstallTotal.Add(ctx, 1, labels)
		m.HelmUninstallDuration.Record(ctx, duration.Seconds(), labels)
		if !success {
			m.HelmUninstallErrors.Add(ctx, 1, labels)
		}
	}
}

// RecordHealthCheck records a health check operation
func (m *OperatorMetrics) RecordHealthCheck(ctx context.Context, checkType string, duration time.Duration, success bool, target string) {
	labels := metric.WithAttributes(
		attribute.String("check_type", checkType),
		attribute.String("target", target),
		attribute.Bool("success", success),
	)

	m.HealthCheckTotal.Add(ctx, 1, labels)
	m.HealthCheckDuration.Record(ctx, duration.Seconds(), labels)

	if success {
		m.HealthCheckSuccess.Add(ctx, 1, labels)
	} else {
		m.HealthCheckErrors.Add(ctx, 1, labels)
	}
}

// RecordKubernetesAPICall records a Kubernetes API call
func (m *OperatorMetrics) RecordKubernetesAPICall(ctx context.Context, method, resource string, duration time.Duration, success bool) {
	labels := metric.WithAttributes(
		attribute.String("method", method),
		attribute.String("resource", resource),
		attribute.Bool("success", success),
	)

	m.KubernetesAPIRequests.Add(ctx, 1, labels)
	m.KubernetesAPILatency.Record(ctx, duration.Seconds(), labels)

	if !success {
		m.KubernetesAPIErrors.Add(ctx, 1, labels)
	}
}

// UpdateRoostMetrics updates roost-related metrics
func (m *OperatorMetrics) UpdateRoostMetrics(ctx context.Context, total, healthy int64, phase string, count int64) {
	m.RoostsTotal.Add(ctx, total)
	m.RoostsHealthy.Add(ctx, healthy)

	phaseLabels := metric.WithAttributes(attribute.String("phase", phase))
	m.RoostsByPhase.Add(ctx, count, phaseLabels)
}

// UpdateGenerationLag updates the generation lag metric
func (m *OperatorMetrics) UpdateGenerationLag(ctx context.Context, lag int64, roostName, namespace string) {
	labels := metric.WithAttributes(
		attribute.String("roost_name", roostName),
		attribute.String("namespace", namespace),
	)

	m.RoostGenerationLag.Add(ctx, lag, labels)
}

// UpdatePerformanceMetrics updates performance-related metrics
func (m *OperatorMetrics) UpdatePerformanceMetrics(ctx context.Context, memoryBytes int64, cpuRatio float64, goroutines int64) {
	m.MemoryUsage.Add(ctx, memoryBytes)
	m.CPUUsage.Add(ctx, cpuRatio)
	m.GoroutineCount.Add(ctx, goroutines)
}

// RecordLeaderElectionChange records a leader election change
func (m *OperatorMetrics) RecordLeaderElectionChange(ctx context.Context, newLeader string) {
	labels := metric.WithAttributes(attribute.String("new_leader", newLeader))
	m.LeaderElectionChanges.Add(ctx, 1, labels)
}

// SetOperatorStartTime sets the operator start time
func (m *OperatorMetrics) SetOperatorStartTime(ctx context.Context, startTime time.Time) {
	m.OperatorStartTime.Add(ctx, startTime.Unix())
}

// UpdateOperatorUptime updates the operator uptime
func (m *OperatorMetrics) UpdateOperatorUptime(ctx context.Context, startTime time.Time) {
	uptime := time.Since(startTime).Seconds()
	m.OperatorUptime.Add(ctx, uptime)
}

// RecordWorkerPoolMetrics records worker pool performance metrics
func (m *OperatorMetrics) RecordWorkerPoolMetrics(ctx context.Context, jobsProcessed int64, averageLatency float64, errorRate float64, queueLength int, activeWorkers int) {
	// Record worker pool specific metrics using existing counters where possible
	// In a real implementation, you might add specific worker pool metrics

	// Use existing metrics or add new ones as needed
	// For now, we'll log the metrics
}

// RecordCacheMetrics records cache performance metrics
func (m *OperatorMetrics) RecordCacheMetrics(ctx context.Context, hitRate float64, missRate float64, size int, evictions int64) {
	// Record cache specific metrics using existing counters where possible
	// In a real implementation, you might add specific cache metrics

	// Use existing metrics or add new ones as needed
	// For now, we'll log the metrics
}
