package security

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// SecurityWebhookHandler integrates security policies with admission control
type SecurityWebhookHandler struct {
	securityManager *SecurityManager
	logger          *zap.Logger
	metrics         *telemetry.OperatorMetrics
	decoder         admission.Decoder
}

func NewSecurityWebhookHandler(securityManager *SecurityManager, logger *zap.Logger, metrics *telemetry.OperatorMetrics) *SecurityWebhookHandler {
	return &SecurityWebhookHandler{
		securityManager: securityManager,
		logger:          logger.With(zap.String("component", "security-webhook")),
		metrics:         metrics,
	}
}

// InjectDecoder implements admission.DecoderInjector
func (swh *SecurityWebhookHandler) InjectDecoder(d admission.Decoder) error {
	swh.decoder = d
	return nil
}

// Handle processes admission requests with security policy validation
func (swh *SecurityWebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := swh.logger.With(
		zap.String("operation", string(req.Operation)),
		zap.String("resource", req.Kind.String()),
		zap.String("name", req.Name),
		zap.String("namespace", req.Namespace),
		zap.String("user", req.UserInfo.Username),
	)

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		log.Debug("Security webhook processing completed", zap.Duration("duration", duration))
	}()

	// Handle different resource types
	switch req.Kind.Kind {
	case "ManagedRoost":
		return swh.handleManagedRoost(ctx, req)
	case "Pod":
		return swh.handlePod(ctx, req)
	case "Deployment":
		return swh.handleDeployment(ctx, req)
	case "Secret":
		return swh.handleSecret(ctx, req)
	default:
		// For unknown resources, apply general security policies
		return swh.handleGenericResource(ctx, req)
	}
}

// handleManagedRoost applies security policies to ManagedRoost resources
func (swh *SecurityWebhookHandler) handleManagedRoost(ctx context.Context, req admission.Request) admission.Response {
	var managedRoost roostv1alpha1.ManagedRoost
	if err := swh.decoder.Decode(req, &managedRoost); err != nil {
		swh.logger.Error("Failed to decode ManagedRoost", zap.Error(err))
		return admission.Errored(500, err)
	}

	// Apply security policies
	result := swh.validateManagedRoostSecurity(ctx, &managedRoost, req.UserInfo.Username)

	// Log the validation result
	swh.securityManager.auditLogger.LogResourceAccess(ctx,
		req.UserInfo.Username,
		string(req.Operation),
		fmt.Sprintf("ManagedRoost/%s", managedRoost.Name),
		map[string]interface{}{
			"namespace": managedRoost.Namespace,
			"allowed":   result.Allowed,
			"reason":    result.Message,
		})

	if !result.Allowed {
		swh.securityManager.auditLogger.LogPolicyViolation(ctx,
			"security-policy",
			fmt.Sprintf("ManagedRoost/%s", managedRoost.Name),
			result.Message,
			"warning")
	}

	// In trust-based mode, we log violations but don't block
	return admission.Allowed(result.Message)
}

// handlePod applies security policies to Pod resources
func (swh *SecurityWebhookHandler) handlePod(ctx context.Context, req admission.Request) admission.Response {
	var pod corev1.Pod
	if err := swh.decoder.Decode(req, &pod); err != nil {
		swh.logger.Error("Failed to decode Pod", zap.Error(err))
		return admission.Errored(500, err)
	}

	// Apply pod security policies
	result := swh.validatePodSecurity(ctx, &pod, req.UserInfo.Username)

	// Log the validation result
	swh.securityManager.auditLogger.LogResourceAccess(ctx,
		req.UserInfo.Username,
		string(req.Operation),
		fmt.Sprintf("Pod/%s", pod.Name),
		map[string]interface{}{
			"namespace": pod.Namespace,
			"allowed":   result.Allowed,
			"reason":    result.Message,
		})

	if !result.Allowed {
		swh.securityManager.auditLogger.LogPolicyViolation(ctx,
			"pod-security-policy",
			fmt.Sprintf("Pod/%s", pod.Name),
			result.Message,
			"warning")
	}

	// In trust-based mode, we log violations but don't block
	return admission.Allowed(result.Message)
}

// handleDeployment applies security policies to Deployment resources
func (swh *SecurityWebhookHandler) handleDeployment(ctx context.Context, req admission.Request) admission.Response {
	// Similar pattern for Deployments
	return swh.handleGenericResource(ctx, req)
}

// handleSecret applies security policies to Secret resources
func (swh *SecurityWebhookHandler) handleSecret(ctx context.Context, req admission.Request) admission.Response {
	var secret corev1.Secret
	if err := swh.decoder.Decode(req, &secret); err != nil {
		swh.logger.Error("Failed to decode Secret", zap.Error(err))
		return admission.Errored(500, err)
	}

	// Log secret access for audit trail
	swh.securityManager.auditLogger.LogSecretAccess(ctx,
		req.UserInfo.Username,
		secret.Name,
		string(req.Operation))

	// Apply secret security policies (e.g., check for Doppler-managed secrets)
	result := swh.validateSecretSecurity(ctx, &secret, req.UserInfo.Username)

	if !result.Allowed {
		swh.securityManager.auditLogger.LogPolicyViolation(ctx,
			"secret-security-policy",
			fmt.Sprintf("Secret/%s", secret.Name),
			result.Message,
			"warning")
	}

	return admission.Allowed(result.Message)
}

// handleGenericResource applies general security policies to any resource
func (swh *SecurityWebhookHandler) handleGenericResource(ctx context.Context, req admission.Request) admission.Response {
	// Apply general security policies
	swh.securityManager.auditLogger.LogResourceAccess(ctx,
		req.UserInfo.Username,
		string(req.Operation),
		fmt.Sprintf("%s/%s", req.Kind.Kind, req.Name),
		map[string]interface{}{
			"namespace": req.Namespace,
			"allowed":   true,
			"reason":    "Trust-based access granted",
		})

	return admission.Allowed("Trust-based access granted")
}

// Security validation methods

func (swh *SecurityWebhookHandler) validateManagedRoostSecurity(ctx context.Context, roost *roostv1alpha1.ManagedRoost, user string) *SecurityValidationResult {
	log := swh.logger.With(
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
		zap.String("user", user),
	)

	// Apply trust-based policies for ManagedRoost
	for _, policy := range swh.securityManager.policyEngine.policies {
		result := policy.Validate(ctx, roost)
		if !result.Valid {
			log.Warn("Security policy violation detected",
				zap.String("policy", policy.Name()),
				zap.String("severity", result.Severity),
				zap.String("message", result.Message))

			// In trust mode, we log but don't reject
			return &SecurityValidationResult{
				Allowed: true,
				Message: fmt.Sprintf("Policy warning (%s): %s", policy.Name(), result.Message),
				Details: result.Details,
			}
		}
	}

	log.Info("ManagedRoost security validation passed")
	return &SecurityValidationResult{
		Allowed: true,
		Message: "Security validation passed",
	}
}

func (swh *SecurityWebhookHandler) validatePodSecurity(ctx context.Context, pod *corev1.Pod, user string) *SecurityValidationResult {
	log := swh.logger.With(
		zap.String("pod", pod.Name),
		zap.String("namespace", pod.Namespace),
		zap.String("user", user),
	)

	violations := []string{}

	// Check for privileged containers
	for _, container := range pod.Spec.Containers {
		if container.SecurityContext != nil && container.SecurityContext.Privileged != nil && *container.SecurityContext.Privileged {
			violation := fmt.Sprintf("Container %s is running in privileged mode", container.Name)
			violations = append(violations, violation)
			log.Warn("Privileged container detected", zap.String("container", container.Name))
		}

		// Check for missing resource limits
		if container.Resources.Limits == nil || len(container.Resources.Limits) == 0 {
			violation := fmt.Sprintf("Container %s is missing resource limits", container.Name)
			violations = append(violations, violation)
			log.Warn("Missing resource limits", zap.String("container", container.Name))
		}
	}

	// Check for host network/PID
	if pod.Spec.HostNetwork {
		violations = append(violations, "Pod is using host network")
		log.Warn("Host network usage detected")
	}

	if pod.Spec.HostPID {
		violations = append(violations, "Pod is using host PID namespace")
		log.Warn("Host PID usage detected")
	}

	message := "Pod security validation passed"
	if len(violations) > 0 {
		message = fmt.Sprintf("Security concerns detected: %v", violations)
	}

	return &SecurityValidationResult{
		Allowed: true, // Trust-based: allow but log
		Message: message,
		Details: map[string]interface{}{
			"violations": violations,
		},
	}
}

func (swh *SecurityWebhookHandler) validateSecretSecurity(ctx context.Context, secret *corev1.Secret, user string) *SecurityValidationResult {
	log := swh.logger.With(
		zap.String("secret", secret.Name),
		zap.String("namespace", secret.Namespace),
		zap.String("user", user),
	)

	// Check if this is a Doppler-managed secret
	if secret.Labels != nil {
		if managedBy, exists := secret.Labels["secrets.doppler.com/managed"]; exists && managedBy == "true" {
			log.Info("Doppler-managed secret detected")
			return &SecurityValidationResult{
				Allowed: true,
				Message: "Doppler-managed secret validation passed",
				Details: map[string]interface{}{
					"managed_by": "doppler",
				},
			}
		}
	}

	// Check for potential sensitive data in secret names/labels
	warnings := []string{}
	if containsSensitiveTerms(secret.Name) {
		warnings = append(warnings, "Secret name contains potentially sensitive terms")
	}

	message := "Secret security validation passed"
	if len(warnings) > 0 {
		message = fmt.Sprintf("Security warnings: %v", warnings)
	}

	return &SecurityValidationResult{
		Allowed: true,
		Message: message,
		Details: map[string]interface{}{
			"warnings": warnings,
		},
	}
}

// Helper types and functions

type SecurityValidationResult struct {
	Allowed bool
	Message string
	Details map[string]interface{}
}

func containsSensitiveTerms(name string) bool {
	sensitiveTerms := []string{"password", "key", "token", "secret", "api-key", "credential"}
	nameLower := name
	for _, term := range sensitiveTerms {
		if contains(nameLower, term) {
			return true
		}
	}
	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}

// Note: In trust-based security, we focus on audit logging and monitoring
// rather than blocking or mutating resources. This aligns with the principle
// of trusting internal users while maintaining comprehensive observability.
