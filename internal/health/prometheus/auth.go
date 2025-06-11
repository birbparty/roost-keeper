package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/api"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// AuthManager handles authentication for Prometheus clients
type AuthManager struct {
	k8sClient client.Client
	logger    *zap.Logger
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(k8sClient client.Client, logger *zap.Logger) *AuthManager {
	return &AuthManager{
		k8sClient: k8sClient,
		logger:    logger,
	}
}

// ConfigureClient configures authentication for a Prometheus API client
func (a *AuthManager) ConfigureClient(ctx context.Context, config *api.Config, namespace string, authSpec *roostv1alpha1.PrometheusAuthSpec) error {
	if authSpec == nil {
		return nil // No authentication configured
	}

	log := a.logger.With(
		zap.String("namespace", namespace),
		zap.String("endpoint", config.Address),
	)

	log.Debug("Configuring Prometheus client authentication")

	// Create a custom round tripper for authentication
	transport := &AuthenticatedTransport{
		Base:      http.DefaultTransport,
		authSpec:  authSpec,
		namespace: namespace,
		authMgr:   a,
		ctx:       ctx,
		logger:    log,
	}

	config.RoundTripper = transport

	return nil
}

// AuthenticatedTransport is a custom HTTP transport that handles authentication
type AuthenticatedTransport struct {
	Base      http.RoundTripper
	authSpec  *roostv1alpha1.PrometheusAuthSpec
	namespace string
	authMgr   *AuthManager
	ctx       context.Context
	logger    *zap.Logger
}

// RoundTrip implements the http.RoundTripper interface with authentication
func (t *AuthenticatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone the request to avoid modifying the original
	authReq := req.Clone(t.ctx)

	// Apply authentication headers
	if err := t.applyAuthentication(authReq); err != nil {
		t.logger.Error("Failed to apply authentication", zap.Error(err))
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	// Perform the request with the base transport
	return t.Base.RoundTrip(authReq)
}

// applyAuthentication applies the configured authentication to the request
func (t *AuthenticatedTransport) applyAuthentication(req *http.Request) error {
	// Apply bearer token authentication
	if t.authSpec.BearerToken != "" {
		token, err := t.resolveTokenValue(t.authSpec.BearerToken)
		if err != nil {
			return fmt.Errorf("failed to resolve bearer token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		t.logger.Debug("Applied bearer token authentication")
		return nil
	}

	// Apply basic authentication
	if t.authSpec.BasicAuth != nil {
		username, err := t.resolveSecretValue(t.authSpec.BasicAuth.Username, "username")
		if err != nil {
			return fmt.Errorf("failed to resolve basic auth username: %w", err)
		}

		password, err := t.resolveSecretValue(t.authSpec.BasicAuth.Password, "password")
		if err != nil {
			return fmt.Errorf("failed to resolve basic auth password: %w", err)
		}

		req.SetBasicAuth(username, password)
		t.logger.Debug("Applied basic authentication")
		return nil
	}

	// Apply secret reference authentication
	if t.authSpec.SecretRef != nil {
		return t.applySecretAuthentication(req)
	}

	// Apply custom headers
	if len(t.authSpec.Headers) > 0 {
		for key, value := range t.authSpec.Headers {
			resolvedValue, err := t.resolveSecretValue(value, key)
			if err != nil {
				return fmt.Errorf("failed to resolve header %s: %w", key, err)
			}
			req.Header.Set(key, resolvedValue)
		}
		t.logger.Debug("Applied custom header authentication")
		return nil
	}

	t.logger.Debug("No authentication configured")
	return nil
}

// applySecretAuthentication applies authentication using a secret reference
func (t *AuthenticatedTransport) applySecretAuthentication(req *http.Request) error {
	secretRef := t.authSpec.SecretRef
	secretNamespace := secretRef.Namespace
	if secretNamespace == "" {
		secretNamespace = t.namespace
	}

	// Get the secret
	secret := &corev1.Secret{}
	err := t.authMgr.k8sClient.Get(t.ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: secretNamespace,
	}, secret)
	if err != nil {
		return fmt.Errorf("failed to get secret %s/%s: %w", secretNamespace, secretRef.Name, err)
	}

	// Apply authentication based on secret contents
	if secretRef.Key != "" {
		// Use specific key
		value, exists := secret.Data[secretRef.Key]
		if !exists {
			return fmt.Errorf("key %s not found in secret %s/%s", secretRef.Key, secretNamespace, secretRef.Name)
		}

		// Determine authentication type based on key name
		keyLower := strings.ToLower(secretRef.Key)
		if strings.Contains(keyLower, "token") || strings.Contains(keyLower, "bearer") {
			req.Header.Set("Authorization", "Bearer "+string(value))
		} else {
			req.Header.Set("Authorization", string(value))
		}
	} else {
		// Auto-detect authentication type from secret keys
		if err := t.autoDetectSecretAuth(req, secret); err != nil {
			return fmt.Errorf("failed to auto-detect authentication from secret: %w", err)
		}
	}

	t.logger.Debug("Applied secret-based authentication")
	return nil
}

// autoDetectSecretAuth automatically detects and applies authentication from secret contents
func (t *AuthenticatedTransport) autoDetectSecretAuth(req *http.Request, secret *corev1.Secret) error {
	// Look for common authentication keys

	// Bearer token patterns
	tokenKeys := []string{"token", "bearer-token", "bearer_token", "auth-token", "auth_token"}
	for _, key := range tokenKeys {
		if value, exists := secret.Data[key]; exists {
			req.Header.Set("Authorization", "Bearer "+string(value))
			return nil
		}
	}

	// Basic auth patterns
	if username, hasUsername := secret.Data["username"]; hasUsername {
		if password, hasPassword := secret.Data["password"]; hasPassword {
			req.SetBasicAuth(string(username), string(password))
			return nil
		}
	}

	// Authorization header pattern
	if authHeader, exists := secret.Data["authorization"]; exists {
		req.Header.Set("Authorization", string(authHeader))
		return nil
	}

	return fmt.Errorf("no recognized authentication keys found in secret")
}

// resolveTokenValue resolves a token value that may contain secret references
func (t *AuthenticatedTransport) resolveTokenValue(tokenTemplate string) (string, error) {
	return t.resolveSecretValue(tokenTemplate, "token")
}

// resolveSecretValue resolves a value that may contain secret references
func (t *AuthenticatedTransport) resolveSecretValue(valueTemplate, fieldName string) (string, error) {
	// Check if it's a secret reference template
	if strings.Contains(valueTemplate, "{{.Secret.") {
		return t.resolveSecretTemplate(valueTemplate)
	}

	// Return as-is if not a template
	return valueTemplate, nil
}

// resolveSecretTemplate resolves secret references in templates like {{.Secret.secret-name.key-name}}
func (t *AuthenticatedTransport) resolveSecretTemplate(template string) (string, error) {
	// Pattern: {{.Secret.secret-name.key-name}}
	re := regexp.MustCompile(`\{\{\.Secret\.([^.]+)\.([^}]+)\}\}`)
	matches := re.FindStringSubmatch(template)
	if len(matches) != 3 {
		// Try simpler pattern: {{.Secret.secret-name}} (default to 'token' key)
		re = regexp.MustCompile(`\{\{\.Secret\.([^}]+)\}\}`)
		matches = re.FindStringSubmatch(template)
		if len(matches) != 2 {
			return template, nil // Not a secret template
		}
		// Use default 'token' key
		matches = append(matches, "token")
	}

	secretName := matches[1]
	secretKey := matches[2]

	// Get the secret
	secret := &corev1.Secret{}
	err := t.authMgr.k8sClient.Get(t.ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: t.namespace,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", t.namespace, secretName, err)
	}

	// Get the value from the secret
	value, exists := secret.Data[secretKey]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s/%s", secretKey, t.namespace, secretName)
	}

	// Replace the template with the actual value
	result := strings.ReplaceAll(template, matches[0], string(value))
	return result, nil
}

// ValidateAuthConfig validates the authentication configuration
func (a *AuthManager) ValidateAuthConfig(authSpec *roostv1alpha1.PrometheusAuthSpec) error {
	if authSpec == nil {
		return nil // No authentication is valid
	}

	authMethods := 0

	// Count configured authentication methods
	if authSpec.BearerToken != "" {
		authMethods++
	}
	if authSpec.BasicAuth != nil {
		authMethods++
		// Validate basic auth fields
		if authSpec.BasicAuth.Username == "" || authSpec.BasicAuth.Password == "" {
			return fmt.Errorf("basic auth requires both username and password")
		}
	}
	if authSpec.SecretRef != nil {
		authMethods++
		// Validate secret reference
		if authSpec.SecretRef.Name == "" {
			return fmt.Errorf("secret reference requires a name")
		}
	}
	if len(authSpec.Headers) > 0 {
		authMethods++
	}

	// Only one authentication method should be specified
	if authMethods > 1 {
		return fmt.Errorf("only one authentication method should be specified")
	}

	if authMethods == 0 {
		return fmt.Errorf("no authentication method specified")
	}

	return nil
}

// TestAuthentication tests the authentication configuration without making actual requests
func (a *AuthManager) TestAuthentication(ctx context.Context, namespace string, authSpec *roostv1alpha1.PrometheusAuthSpec) error {
	if authSpec == nil {
		return nil
	}

	// Create a dummy transport to test authentication resolution
	transport := &AuthenticatedTransport{
		Base:      http.DefaultTransport,
		authSpec:  authSpec,
		namespace: namespace,
		authMgr:   a,
		ctx:       ctx,
		logger:    a.logger,
	}

	// Create a dummy request to test authentication
	req, err := http.NewRequestWithContext(ctx, "GET", "http://test.example.com", nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %w", err)
	}

	// Try to apply authentication (this will resolve secrets but not make actual requests)
	return transport.applyAuthentication(req)
}
