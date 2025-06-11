package rbac

import (
	"context"
	"encoding/json"
	"time"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"go.uber.org/zap"
)

// AuditLogger provides comprehensive audit logging for RBAC operations
type AuditLogger struct {
	logger *zap.Logger
}

// AuditEvent represents a single audit event
type AuditEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	EventType string                 `json:"eventType"`
	Source    string                 `json:"source"`
	User      string                 `json:"user,omitempty"`
	Tenant    string                 `json:"tenant,omitempty"`
	Resource  string                 `json:"resource,omitempty"`
	Action    string                 `json:"action"`
	Result    string                 `json:"result"`
	Details   map[string]interface{} `json:"details,omitempty"`
	RequestID string                 `json:"requestId,omitempty"`
	SessionID string                 `json:"sessionId,omitempty"`
	IPAddress string                 `json:"ipAddress,omitempty"`
	UserAgent string                 `json:"userAgent,omitempty"`
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *zap.Logger) *AuditLogger {
	return &AuditLogger{
		logger: logger.With(zap.String("component", "rbac-audit")),
	}
}

// LogRBACSetup logs the completion of RBAC setup
func (a *AuditLogger) LogRBACSetup(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, rbacSpec *roostv1alpha1.RBACSpec) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "rbac_setup",
		Source:    "roost-keeper",
		Tenant:    rbacSpec.TenantID,
		Resource:  managedRoost.Name,
		Action:    "setup_rbac",
		Result:    "success",
		Details: map[string]interface{}{
			"namespace":         managedRoost.Namespace,
			"service_accounts":  len(rbacSpec.ServiceAccounts),
			"roles":             len(rbacSpec.Roles),
			"role_bindings":     len(rbacSpec.RoleBindings),
			"identity_provider": rbacSpec.IdentityProvider,
		},
	}

	a.logEvent(event)
}

// LogRBACCleanup logs the cleanup of RBAC resources
func (a *AuditLogger) LogRBACCleanup(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "rbac_cleanup",
		Source:    "roost-keeper",
		Tenant:    tenantID,
		Resource:  managedRoost.Name,
		Action:    "cleanup_rbac",
		Result:    "success",
		Details: map[string]interface{}{
			"namespace": managedRoost.Namespace,
		},
	}

	a.logEvent(event)
}

// LogPermissionDenied logs when permission is denied
func (a *AuditLogger) LogPermissionDenied(ctx context.Context, username, action, resource string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "permission_denied",
		Source:    "roost-keeper",
		User:      username,
		Resource:  resource,
		Action:    action,
		Result:    "denied",
		Details: map[string]interface{}{
			"reason": "insufficient_permissions",
		},
	}

	a.logEvent(event)
}

// LogPermissionGranted logs when permission is granted
func (a *AuditLogger) LogPermissionGranted(ctx context.Context, username, action, resource string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "permission_granted",
		Source:    "roost-keeper",
		User:      username,
		Resource:  resource,
		Action:    action,
		Result:    "granted",
	}

	a.logEvent(event)
}

// LogPolicyValidation logs policy validation results
func (a *AuditLogger) LogPolicyValidation(ctx context.Context, tenantID string, validationResult string, issues []string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "policy_validation",
		Source:    "roost-keeper",
		Tenant:    tenantID,
		Action:    "validate_policy",
		Result:    validationResult,
		Details: map[string]interface{}{
			"issues_count": len(issues),
			"issues":       issues,
		},
	}

	a.logEvent(event)
}

// LogIdentityProviderAuth logs identity provider authentication events
func (a *AuditLogger) LogIdentityProviderAuth(ctx context.Context, providerName, username, result string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "identity_provider_auth",
		Source:    "roost-keeper",
		User:      username,
		Action:    "authenticate",
		Result:    result,
		Details: map[string]interface{}{
			"provider": providerName,
		},
	}

	a.logEvent(event)
}

// LogServiceAccountCreation logs service account creation
func (a *AuditLogger) LogServiceAccountCreation(ctx context.Context, tenantID, serviceAccountName, namespace string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "service_account_created",
		Source:    "roost-keeper",
		Tenant:    tenantID,
		Resource:  serviceAccountName,
		Action:    "create_service_account",
		Result:    "success",
		Details: map[string]interface{}{
			"namespace": namespace,
		},
	}

	a.logEvent(event)
}

// LogRoleCreation logs role creation
func (a *AuditLogger) LogRoleCreation(ctx context.Context, tenantID, roleName, namespace string, rulesCount int) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "role_created",
		Source:    "roost-keeper",
		Tenant:    tenantID,
		Resource:  roleName,
		Action:    "create_role",
		Result:    "success",
		Details: map[string]interface{}{
			"namespace":   namespace,
			"rules_count": rulesCount,
		},
	}

	a.logEvent(event)
}

// LogRoleBindingCreation logs role binding creation
func (a *AuditLogger) LogRoleBindingCreation(ctx context.Context, tenantID, bindingName, namespace string, subjectsCount int) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "role_binding_created",
		Source:    "roost-keeper",
		Tenant:    tenantID,
		Resource:  bindingName,
		Action:    "create_role_binding",
		Result:    "success",
		Details: map[string]interface{}{
			"namespace":      namespace,
			"subjects_count": subjectsCount,
		},
	}

	a.logEvent(event)
}

// LogTokenRotation logs service account token rotation
func (a *AuditLogger) LogTokenRotation(ctx context.Context, tenantID, serviceAccountName string, success bool) {
	result := "success"
	if !success {
		result = "failed"
	}

	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "token_rotation",
		Source:    "roost-keeper",
		Tenant:    tenantID,
		Resource:  serviceAccountName,
		Action:    "rotate_token",
		Result:    result,
	}

	a.logEvent(event)
}

// LogPolicyTemplateUsage logs when a policy template is used
func (a *AuditLogger) LogPolicyTemplateUsage(ctx context.Context, tenantID, templateName, roleName string) {
	event := AuditEvent{
		Timestamp: time.Now(),
		EventType: "policy_template_used",
		Source:    "roost-keeper",
		Tenant:    tenantID,
		Resource:  roleName,
		Action:    "apply_template",
		Result:    "success",
		Details: map[string]interface{}{
			"template": templateName,
		},
	}

	a.logEvent(event)
}

// logEvent writes the audit event to the logger
func (a *AuditLogger) logEvent(event AuditEvent) {
	// Convert event to JSON for structured logging
	eventJSON, err := json.Marshal(event)
	if err != nil {
		a.logger.Error("Failed to marshal audit event", zap.Error(err))
		return
	}

	// Log the event
	a.logger.Info("RBAC audit event",
		zap.String("event_type", event.EventType),
		zap.String("source", event.Source),
		zap.String("user", event.User),
		zap.String("tenant", event.Tenant),
		zap.String("resource", event.Resource),
		zap.String("action", event.Action),
		zap.String("result", event.Result),
		zap.Time("timestamp", event.Timestamp),
		zap.String("event_json", string(eventJSON)),
	)
}

// SetWebhookURL configures an external webhook for audit events
func (a *AuditLogger) SetWebhookURL(webhookURL string) {
	// This is a placeholder for webhook integration
	// In a real implementation, this would configure HTTP client
	// to send audit events to external systems
	a.logger.Info("Audit webhook configured", zap.String("webhook_url", webhookURL))
}

// GetAuditEvents retrieves audit events (placeholder for future implementation)
func (a *AuditLogger) GetAuditEvents(ctx context.Context, tenantID string, since time.Time) ([]AuditEvent, error) {
	// This is a placeholder for audit event retrieval
	// In a real implementation, this would query audit storage
	return []AuditEvent{}, nil
}
