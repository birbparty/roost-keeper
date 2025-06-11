package telemetry

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// WebhookMetrics contains metrics specific to admission webhook operations
type WebhookMetrics struct {
	// Webhook operation metrics
	WebhookRequestsTotal metric.Int64Counter
	WebhookDuration      metric.Float64Histogram
	WebhookErrors        metric.Int64Counter
	WebhookRejections    metric.Int64Counter
	WebhookMutations     metric.Int64Counter

	// Policy engine metrics
	PolicyEvaluationsTotal metric.Int64Counter
	PolicyEvaluationTime   metric.Float64Histogram
	PolicyRuleExecutions   metric.Int64Counter
	PolicyViolationsTotal  metric.Int64Counter

	// Certificate management metrics
	CertificateRotations    metric.Int64Counter
	CertificateValidityDays metric.Float64Gauge

	// Performance metrics
	ActiveWebhookRequests metric.Int64UpDownCounter
	WebhookQueueLength    metric.Int64UpDownCounter
}

// NewWebhookMetrics creates and initializes webhook-specific metrics
func NewWebhookMetrics() *WebhookMetrics {
	meter := otel.Meter(meterName)

	// Initialize all metrics with error handling
	webhookRequestsTotal, _ := meter.Int64Counter(
		"roost_keeper_webhook_requests_total",
		metric.WithDescription("Total number of webhook requests processed"),
		metric.WithUnit("{requests}"),
	)

	webhookDuration, _ := meter.Float64Histogram(
		"roost_keeper_webhook_duration_seconds",
		metric.WithDescription("Duration of webhook request processing"),
		metric.WithUnit("s"),
	)

	webhookErrors, _ := meter.Int64Counter(
		"roost_keeper_webhook_errors_total",
		metric.WithDescription("Total number of webhook processing errors"),
		metric.WithUnit("{errors}"),
	)

	webhookRejections, _ := meter.Int64Counter(
		"roost_keeper_webhook_rejections_total",
		metric.WithDescription("Total number of webhook request rejections"),
		metric.WithUnit("{rejections}"),
	)

	webhookMutations, _ := meter.Int64Counter(
		"roost_keeper_webhook_mutations_total",
		metric.WithDescription("Total number of webhook mutations applied"),
		metric.WithUnit("{mutations}"),
	)

	policyEvaluationsTotal, _ := meter.Int64Counter(
		"roost_keeper_policy_evaluations_total",
		metric.WithDescription("Total number of policy evaluations"),
		metric.WithUnit("{evaluations}"),
	)

	policyEvaluationTime, _ := meter.Float64Histogram(
		"roost_keeper_policy_evaluation_duration_seconds",
		metric.WithDescription("Duration of policy evaluation"),
		metric.WithUnit("s"),
	)

	policyRuleExecutions, _ := meter.Int64Counter(
		"roost_keeper_policy_rule_executions_total",
		metric.WithDescription("Total number of policy rule executions"),
		metric.WithUnit("{executions}"),
	)

	policyViolationsTotal, _ := meter.Int64Counter(
		"roost_keeper_policy_violations_total",
		metric.WithDescription("Total number of policy violations detected"),
		metric.WithUnit("{violations}"),
	)

	certificateRotations, _ := meter.Int64Counter(
		"roost_keeper_certificate_rotations_total",
		metric.WithDescription("Total number of certificate rotations"),
		metric.WithUnit("{rotations}"),
	)

	certificateValidityDays, _ := meter.Float64Gauge(
		"roost_keeper_certificate_validity_days",
		metric.WithDescription("Days until certificate expiration"),
		metric.WithUnit("d"),
	)

	activeWebhookRequests, _ := meter.Int64UpDownCounter(
		"roost_keeper_webhook_active_requests",
		metric.WithDescription("Number of currently active webhook requests"),
		metric.WithUnit("{requests}"),
	)

	webhookQueueLength, _ := meter.Int64UpDownCounter(
		"roost_keeper_webhook_queue_length",
		metric.WithDescription("Current webhook processing queue length"),
		metric.WithUnit("{requests}"),
	)

	return &WebhookMetrics{
		WebhookRequestsTotal:    webhookRequestsTotal,
		WebhookDuration:         webhookDuration,
		WebhookErrors:           webhookErrors,
		WebhookRejections:       webhookRejections,
		WebhookMutations:        webhookMutations,
		PolicyEvaluationsTotal:  policyEvaluationsTotal,
		PolicyEvaluationTime:    policyEvaluationTime,
		PolicyRuleExecutions:    policyRuleExecutions,
		PolicyViolationsTotal:   policyViolationsTotal,
		CertificateRotations:    certificateRotations,
		CertificateValidityDays: certificateValidityDays,
		ActiveWebhookRequests:   activeWebhookRequests,
		WebhookQueueLength:      webhookQueueLength,
	}
}

// RecordWebhookDuration records the duration of a webhook operation
func (wm *WebhookMetrics) RecordWebhookDuration(ctx context.Context, operation string, duration float64) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
	)
	wm.WebhookDuration.Record(ctx, duration, labels)
}

// RecordWebhookError records a webhook processing error
func (wm *WebhookMetrics) RecordWebhookError(ctx context.Context, operation, errorType string) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("error_type", errorType),
	)
	wm.WebhookErrors.Add(ctx, 1, labels)
}

// RecordWebhookRejection records a webhook request rejection
func (wm *WebhookMetrics) RecordWebhookRejection(ctx context.Context, operation, reason string) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.String("reason", reason),
	)
	wm.WebhookRejections.Add(ctx, 1, labels)
}

// RecordWebhookMutation records a webhook mutation operation
func (wm *WebhookMetrics) RecordWebhookMutation(ctx context.Context, operation string, mutationSize int) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
		attribute.Int("mutation_size", mutationSize),
	)
	wm.WebhookMutations.Add(ctx, 1, labels)
}

// RecordPolicyEvaluation records a policy evaluation operation
func (wm *WebhookMetrics) RecordPolicyEvaluation(ctx context.Context, policyName string, duration time.Duration, violated bool) {
	labels := metric.WithAttributes(
		attribute.String("policy_name", policyName),
		attribute.Bool("violated", violated),
	)
	wm.PolicyEvaluationsTotal.Add(ctx, 1, labels)
	wm.PolicyEvaluationTime.Record(ctx, duration.Seconds(), labels)

	if violated {
		wm.PolicyViolationsTotal.Add(ctx, 1, labels)
	}
}

// RecordPolicyRuleExecution records execution of a specific policy rule
func (wm *WebhookMetrics) RecordPolicyRuleExecution(ctx context.Context, ruleName, ruleType string, success bool) {
	labels := metric.WithAttributes(
		attribute.String("rule_name", ruleName),
		attribute.String("rule_type", ruleType),
		attribute.Bool("success", success),
	)
	wm.PolicyRuleExecutions.Add(ctx, 1, labels)
}

// RecordCertificateRotation records a certificate rotation event
func (wm *WebhookMetrics) RecordCertificateRotation(ctx context.Context, certType, reason string) {
	labels := metric.WithAttributes(
		attribute.String("cert_type", certType),
		attribute.String("reason", reason),
	)
	wm.CertificateRotations.Add(ctx, 1, labels)
}

// UpdateCertificateValidity updates the certificate validity metric
func (wm *WebhookMetrics) UpdateCertificateValidity(ctx context.Context, certType string, validityDays float64) {
	labels := metric.WithAttributes(
		attribute.String("cert_type", certType),
	)
	wm.CertificateValidityDays.Record(ctx, validityDays, labels)
}

// TrackActiveRequest tracks an active webhook request
func (wm *WebhookMetrics) TrackActiveRequest(ctx context.Context, operation string) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
	)
	wm.ActiveWebhookRequests.Add(ctx, 1, labels)
}

// UntrackActiveRequest untracks an active webhook request
func (wm *WebhookMetrics) UntrackActiveRequest(ctx context.Context, operation string) {
	labels := metric.WithAttributes(
		attribute.String("operation", operation),
	)
	wm.ActiveWebhookRequests.Add(ctx, -1, labels)
}

// UpdateQueueLength updates the webhook processing queue length
func (wm *WebhookMetrics) UpdateQueueLength(ctx context.Context, length int64) {
	wm.WebhookQueueLength.Add(ctx, length)
}

// StartWebhookSpan starts a new trace span for webhook operations
func StartWebhookSpan(ctx context.Context, operation, resource string) (context.Context, trace.Span) {
	tracer := otel.Tracer(meterName)
	return tracer.Start(ctx, "webhook."+operation,
		trace.WithAttributes(
			attribute.String("webhook.operation", operation),
			attribute.String("webhook.resource", resource),
		),
	)
}
