package teardown

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/events"
	"github.com/birbparty/roost-keeper/internal/health"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Manager coordinates teardown policy evaluation and execution
type Manager struct {
	client        client.Client
	healthChecker health.Checker
	eventManager  *events.Manager
	registry      *EvaluatorRegistry
	auditor       *AuditLogger
	safetyChecker *SafetyChecker
	executor      *ExecutionEngine
	scheduler     *TeardownScheduler
	logger        *zap.Logger
	metrics       *TeardownMetricsCollector

	// Configuration
	config *ManagerConfig

	// State management
	mu               sync.RWMutex
	activeExecutions map[string]*TeardownExecution
	scheduledTasks   map[string]*TeardownSchedule
}

// ManagerConfig contains configuration for the teardown manager
type ManagerConfig struct {
	// EnableSafetyChecks enables comprehensive safety checking
	EnableSafetyChecks bool

	// DefaultTimeout is the default timeout for teardown operations
	DefaultTimeout time.Duration

	// MaxConcurrentExecutions limits concurrent teardown executions
	MaxConcurrentExecutions int

	// AuditRetentionDays specifies audit log retention
	AuditRetentionDays int

	// EnableScheduling enables scheduled teardown evaluation
	EnableScheduling bool

	// SchedulingInterval is the interval for scheduled evaluations
	SchedulingInterval time.Duration

	// EnableMetrics enables metrics collection
	EnableMetrics bool

	// DryRunMode enables dry run mode for all evaluations
	DryRunMode bool
}

// DefaultManagerConfig returns a default manager configuration
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		EnableSafetyChecks:      true,
		DefaultTimeout:          30 * time.Minute,
		MaxConcurrentExecutions: 5,
		AuditRetentionDays:      90,
		EnableScheduling:        true,
		SchedulingInterval:      5 * time.Minute,
		EnableMetrics:           true,
		DryRunMode:              false,
	}
}

// NewManager creates a new teardown manager
func NewManager(
	client client.Client,
	healthChecker health.Checker,
	eventManager *events.Manager,
	logger *zap.Logger,
	config *ManagerConfig,
) (*Manager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}

	logger = logger.With(zap.String("component", "teardown-manager"))

	// Initialize components
	registry := NewEvaluatorRegistry()
	auditor := NewAuditLogger(logger)
	safetyChecker := NewSafetyChecker(client, logger)
	executor := NewExecutionEngine(client, eventManager, logger)
	scheduler := NewTeardownScheduler(logger)

	var metrics *TeardownMetricsCollector
	if config.EnableMetrics {
		var err error
		metrics, err = NewTeardownMetricsCollector()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize metrics collector: %w", err)
		}
	}

	manager := &Manager{
		client:           client,
		healthChecker:    healthChecker,
		eventManager:     eventManager,
		registry:         registry,
		auditor:          auditor,
		safetyChecker:    safetyChecker,
		executor:         executor,
		scheduler:        scheduler,
		logger:           logger,
		metrics:          metrics,
		config:           config,
		activeExecutions: make(map[string]*TeardownExecution),
		scheduledTasks:   make(map[string]*TeardownSchedule),
	}

	// Register default evaluators
	if err := manager.registerDefaultEvaluators(); err != nil {
		return nil, fmt.Errorf("failed to register default evaluators: %w", err)
	}

	return manager, nil
}

// EvaluateTeardownPolicy evaluates teardown policies for a roost
func (m *Manager) EvaluateTeardownPolicy(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*TeardownDecision, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "teardown.evaluate", roost.Name, roost.Namespace)
	defer span.End()

	log := m.logger.With(
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
	)

	log.Info("Evaluating teardown policies")

	// Create teardown context
	teardownCtx := &TeardownContext{
		ManagedRoost:   roost,
		EvaluationTime: time.Now(),
		CorrelationID:  GenerateCorrelationID(),
		Metadata:       make(map[string]interface{}),
		DryRun:         m.config.DryRunMode,
		ForceOverride:  false,
	}

	// Record evaluation start
	if m.metrics != nil {
		m.metrics.RecordEvaluationStart(ctx, roost)
	}

	// Check if teardown policy is defined
	if roost.Spec.TeardownPolicy == nil {
		log.Debug("No teardown policy defined")
		decision := &TeardownDecision{
			ShouldTeardown: false,
			Reason:         "No teardown policy configured",
			Urgency:        TeardownUrgencyLow,
			SafetyChecks:   []SafetyCheck{},
			Metadata:       make(map[string]interface{}),
			TriggerType:    "none",
			EvaluatedAt:    time.Now(),
			Score:          0,
		}

		m.auditor.LogEvaluationResult(ctx, roost, decision)
		telemetry.RecordSpanSuccess(ctx)
		return decision, nil
	}

	policy := roost.Spec.TeardownPolicy

	// Get relevant evaluators
	evaluators := m.registry.GetByTrigger(policy.Triggers)
	if len(evaluators) == 0 {
		log.Warn("No evaluators found for configured triggers")
		decision := &TeardownDecision{
			ShouldTeardown: false,
			Reason:         "No matching evaluators for configured triggers",
			Urgency:        TeardownUrgencyLow,
			SafetyChecks:   []SafetyCheck{},
			Metadata:       make(map[string]interface{}),
			TriggerType:    "none",
			EvaluatedAt:    time.Now(),
			Score:          0,
		}

		m.auditor.LogEvaluationResult(ctx, roost, decision)
		telemetry.RecordSpanSuccess(ctx)
		return decision, nil
	}

	// Sort evaluators by priority (highest first)
	sort.Slice(evaluators, func(i, j int) bool {
		return evaluators[i].GetPriority() > evaluators[j].GetPriority()
	})

	// Evaluate triggers
	var decisions []*TeardownDecision
	for _, evaluator := range evaluators {
		log.Debug("Running evaluator", zap.String("type", evaluator.GetType()))

		decision, err := evaluator.ShouldTeardown(ctx, teardownCtx)
		if err != nil {
			log.Error("Evaluator failed",
				zap.String("type", evaluator.GetType()),
				zap.Error(err))
			continue
		}

		if decision != nil && decision.ShouldTeardown {
			log.Info("Teardown triggered",
				zap.String("type", evaluator.GetType()),
				zap.String("reason", decision.Reason))
			decisions = append(decisions, decision)
		}
	}

	// Aggregate decisions
	finalDecision := m.aggregateDecisions(decisions, policy)

	// Perform safety checks if teardown is recommended
	if finalDecision.ShouldTeardown && m.config.EnableSafetyChecks {
		log.Info("Performing safety checks")
		safetyChecks, err := m.safetyChecker.PerformSafetyChecks(ctx, roost, finalDecision)
		if err != nil {
			log.Error("Safety checks failed", zap.Error(err))
			finalDecision.ShouldTeardown = false
			finalDecision.Reason = fmt.Sprintf("Safety check error: %v", err)
		} else {
			finalDecision.SafetyChecks = safetyChecks

			// Check if any critical safety checks failed
			for _, check := range safetyChecks {
				if !check.Passed && !check.CanOverride &&
					(check.Severity == SafetySeverityError || check.Severity == SafetySeverityCritical) {
					finalDecision.ShouldTeardown = false
					finalDecision.Reason = fmt.Sprintf("Critical safety check failed: %s", check.Name)
					break
				}
			}
		}
	}

	// Record evaluation result
	if m.metrics != nil {
		m.metrics.RecordEvaluationResult(ctx, roost, finalDecision)
	}

	// Audit the decision
	m.auditor.LogEvaluationResult(ctx, roost, finalDecision)

	log.Info("Teardown policy evaluation completed",
		zap.Bool("should_teardown", finalDecision.ShouldTeardown),
		zap.String("reason", finalDecision.Reason),
		zap.String("urgency", string(finalDecision.Urgency)))

	telemetry.RecordSpanSuccess(ctx)
	return finalDecision, nil
}

// ExecuteTeardown executes a teardown based on the decision
func (m *Manager) ExecuteTeardown(ctx context.Context, roost *roostv1alpha1.ManagedRoost, decision *TeardownDecision) error {
	ctx, span := telemetry.StartControllerSpan(ctx, "teardown.execute", roost.Name, roost.Namespace)
	defer span.End()

	log := m.logger.With(
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
		zap.String("reason", decision.Reason),
	)

	// Check concurrent execution limit
	m.mu.Lock()
	if len(m.activeExecutions) >= m.config.MaxConcurrentExecutions {
		m.mu.Unlock()
		err := fmt.Errorf("maximum concurrent executions reached (%d)", m.config.MaxConcurrentExecutions)
		telemetry.RecordSpanError(ctx, err)
		return err
	}

	// Create execution
	execution := &TeardownExecution{
		ExecutionID: GenerateExecutionID(),
		Decision:    decision,
		StartTime:   time.Now(),
		Status:      TeardownExecutionStatusPending,
		Progress:    0,
		Steps:       []TeardownExecutionStep{},
		Metadata:    make(map[string]interface{}),
	}

	// Register active execution
	m.activeExecutions[execution.ExecutionID] = execution
	m.mu.Unlock()

	log.Info("Starting teardown execution", zap.String("execution_id", execution.ExecutionID))

	// Defer cleanup
	defer func() {
		m.mu.Lock()
		delete(m.activeExecutions, execution.ExecutionID)
		m.mu.Unlock()
	}()

	// Execute teardown
	err := m.executor.Execute(ctx, roost, execution)
	if err != nil {
		log.Error("Teardown execution failed", zap.Error(err))
		execution.Status = TeardownExecutionStatusFailed
		execution.Error = err.Error()

		// Record failure metrics
		if m.metrics != nil {
			m.metrics.RecordExecutionFailure(ctx, roost, execution)
		}

		// Audit failure
		m.auditor.LogExecutionFailure(ctx, roost, execution, err)

		telemetry.RecordSpanError(ctx, err)
		return err
	}

	// Mark completion
	now := time.Now()
	execution.EndTime = &now
	execution.Status = TeardownExecutionStatusCompleted
	execution.Progress = 100

	// Record success metrics
	if m.metrics != nil {
		m.metrics.RecordExecutionSuccess(ctx, roost, execution)
	}

	// Audit success
	m.auditor.LogExecutionSuccess(ctx, roost, execution)

	log.Info("Teardown execution completed successfully",
		zap.String("execution_id", execution.ExecutionID),
		zap.Duration("duration", time.Since(execution.StartTime)))

	telemetry.RecordSpanSuccess(ctx)
	return nil
}

// GetActiveExecutions returns currently active teardown executions
func (m *Manager) GetActiveExecutions() []*TeardownExecution {
	m.mu.RLock()
	defer m.mu.RUnlock()

	executions := make([]*TeardownExecution, 0, len(m.activeExecutions))
	for _, execution := range m.activeExecutions {
		executions = append(executions, execution)
	}

	return executions
}

// RegisterEvaluator registers a custom teardown evaluator
func (m *Manager) RegisterEvaluator(evaluator TeardownEvaluator) {
	m.registry.Register(evaluator)
	m.logger.Info("Registered teardown evaluator", zap.String("type", evaluator.GetType()))
}

// aggregateDecisions combines multiple teardown decisions into a final decision
func (m *Manager) aggregateDecisions(decisions []*TeardownDecision, policy *roostv1alpha1.TeardownPolicySpec) *TeardownDecision {
	if len(decisions) == 0 {
		return &TeardownDecision{
			ShouldTeardown: false,
			Reason:         "No triggers activated",
			Urgency:        TeardownUrgencyLow,
			SafetyChecks:   []SafetyCheck{},
			Metadata:       make(map[string]interface{}),
			TriggerType:    "none",
			EvaluatedAt:    time.Now(),
			Score:          0,
		}
	}

	// Find the highest priority decision
	var finalDecision *TeardownDecision
	highestScore := int32(-1)

	for _, decision := range decisions {
		if decision.Score > highestScore {
			highestScore = decision.Score
			finalDecision = decision
		}
	}

	if finalDecision == nil {
		finalDecision = decisions[0] // Fallback to first decision
	}

	// Aggregate metadata from all decisions
	aggregatedMetadata := make(map[string]interface{})
	triggerReasons := make([]string, 0, len(decisions))

	for _, decision := range decisions {
		triggerReasons = append(triggerReasons, fmt.Sprintf("%s: %s", decision.TriggerType, decision.Reason))
		for k, v := range decision.Metadata {
			aggregatedMetadata[fmt.Sprintf("%s_%s", decision.TriggerType, k)] = v
		}
	}

	// Update final decision with aggregated information
	finalDecision.Metadata = aggregatedMetadata
	finalDecision.Metadata["trigger_reasons"] = triggerReasons
	finalDecision.Metadata["decision_count"] = len(decisions)

	return finalDecision
}

// registerDefaultEvaluators registers the built-in evaluators
func (m *Manager) registerDefaultEvaluators() error {
	// Register timeout evaluator
	timeoutEvaluator := NewTimeoutEvaluator(m.logger)
	m.registry.Register(timeoutEvaluator)

	// Register resource threshold evaluator
	resourceEvaluator := NewResourceThresholdEvaluator(m.client, m.logger)
	m.registry.Register(resourceEvaluator)

	// Register health-based evaluator
	healthEvaluator := NewHealthBasedEvaluator(m.healthChecker, m.logger)
	m.registry.Register(healthEvaluator)

	// Register failure count evaluator
	failureEvaluator := NewFailureCountEvaluator(m.logger)
	m.registry.Register(failureEvaluator)

	// Register schedule evaluator
	scheduleEvaluator := NewScheduleEvaluator(m.logger)
	m.registry.Register(scheduleEvaluator)

	// Register webhook evaluator
	webhookEvaluator := NewWebhookEvaluator(m.logger)
	m.registry.Register(webhookEvaluator)

	return nil
}

// Helper functions for ID generation
func GenerateCorrelationID() string {
	return fmt.Sprintf("teardown-%d", time.Now().UnixNano())
}

func GenerateExecutionID() string {
	return fmt.Sprintf("exec-%d", time.Now().UnixNano())
}
