package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"go.uber.org/zap"
)

// Provider defines the interface for authentication providers
type Provider interface {
	ValidateToken(ctx context.Context, token string) (*Claims, error)
	GetUserPermissions(ctx context.Context, userID string) ([]string, error)
}

// Claims represents JWT claims with additional Roost-Keeper specific information
type Claims struct {
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	Groups      []string `json:"groups"`
	Tenant      string   `json:"tenant"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

// JWTProvider implements JWT-based authentication
type JWTProvider struct {
	secret       []byte
	issuer       string
	audience     string
	logger       *zap.Logger
	keyFunc      jwt.Keyfunc
	oidcProvider *OIDCProvider
}

// NewJWTProvider creates a new JWT authentication provider
func NewJWTProvider(secret []byte, issuer, audience string, logger *zap.Logger) *JWTProvider {
	provider := &JWTProvider{
		secret:   secret,
		issuer:   issuer,
		audience: audience,
		logger:   logger,
	}

	provider.keyFunc = func(token *jwt.Token) (interface{}, error) {
		// Validate the signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return provider.secret, nil
	}

	return provider
}

// ValidateToken validates a JWT token and returns claims
func (p *JWTProvider) ValidateToken(ctx context.Context, tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, p.keyFunc)
	if err != nil {
		p.logger.Debug("Token parsing failed", zap.Error(err))
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		p.logger.Debug("Invalid token claims")
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer and audience
	if p.issuer != "" && claims.Issuer != p.issuer {
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", p.issuer, claims.Issuer)
	}

	if p.audience != "" && !claims.VerifyAudience(p.audience, true) {
		return nil, fmt.Errorf("invalid audience: expected %s", p.audience)
	}

	// Check expiration
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, fmt.Errorf("token expired")
	}

	return claims, nil
}

// GetUserPermissions retrieves permissions for a user (placeholder implementation)
func (p *JWTProvider) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	// In a real implementation, this would query a database or external service
	// For now, return basic permissions based on user groups or roles
	return []string{"roosts:read", "roosts:write", "health:read"}, nil
}

// OIDCProvider implements OIDC-based authentication
type OIDCProvider struct {
	issuerURL    string
	clientID     string
	clientSecret string
	logger       *zap.Logger
	discoveryURL string
	jwksURL      string
	userInfoURL  string
}

// NewOIDCProvider creates a new OIDC authentication provider
func NewOIDCProvider(issuerURL, clientID, clientSecret string, logger *zap.Logger) *OIDCProvider {
	return &OIDCProvider{
		issuerURL:    issuerURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		logger:       logger,
		discoveryURL: fmt.Sprintf("%s/.well-known/openid_configuration", issuerURL),
		jwksURL:      fmt.Sprintf("%s/.well-known/jwks.json", issuerURL),
		userInfoURL:  fmt.Sprintf("%s/userinfo", issuerURL),
	}
}

// ValidateToken validates an OIDC token
func (o *OIDCProvider) ValidateToken(ctx context.Context, tokenString string) (*Claims, error) {
	// Parse token without verification first to get header information
	token, _, err := new(jwt.Parser).ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	// Get key ID from token header
	keyID, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("token missing key ID")
	}

	// Fetch public key from JWKS endpoint
	publicKey, err := o.fetchPublicKey(ctx, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch public key: %w", err)
	}

	// Parse and validate token with public key
	token, err = jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return publicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("token validation failed: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Validate issuer
	if claims.Issuer != o.issuerURL {
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", o.issuerURL, claims.Issuer)
	}

	// Validate audience
	if !claims.VerifyAudience(o.clientID, true) {
		return nil, fmt.Errorf("invalid audience")
	}

	return claims, nil
}

// GetUserPermissions retrieves permissions for an OIDC user
func (o *OIDCProvider) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	// In a real implementation, this would query the OIDC provider's user info endpoint
	// or use group/role information from the token to determine permissions
	return []string{"roosts:read", "roosts:write", "health:read"}, nil
}

// fetchPublicKey fetches the public key for token validation from JWKS endpoint
func (o *OIDCProvider) fetchPublicKey(ctx context.Context, keyID string) (interface{}, error) {
	// Implementation would fetch from JWKS endpoint and cache keys
	// For now, return an error indicating this needs to be implemented
	return nil, fmt.Errorf("OIDC public key fetching not implemented")
}

// MockProvider implements a mock authentication provider for testing
type MockProvider struct {
	users  map[string]*Claims
	logger *zap.Logger
}

// NewMockProvider creates a new mock authentication provider
func NewMockProvider(logger *zap.Logger) *MockProvider {
	return &MockProvider{
		users: map[string]*Claims{
			"test-token": {
				UserID:      "test-user",
				Username:    "testuser",
				Email:       "test@example.com",
				Groups:      []string{"developers", "admins"},
				Tenant:      "default",
				Permissions: []string{"*"}, // All permissions for testing
			},
			"limited-token": {
				UserID:      "limited-user",
				Username:    "limiteduser",
				Email:       "limited@example.com",
				Groups:      []string{"users"},
				Tenant:      "default",
				Permissions: []string{"roosts:read", "health:read"},
			},
		},
		logger: logger,
	}
}

// ValidateToken validates a mock token
func (m *MockProvider) ValidateToken(ctx context.Context, token string) (*Claims, error) {
	claims, exists := m.users[token]
	if !exists {
		return nil, fmt.Errorf("invalid token")
	}

	// Create a copy to avoid modifying the original
	result := *claims
	result.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	result.IssuedAt = jwt.NewNumericDate(time.Now())

	return &result, nil
}

// GetUserPermissions retrieves permissions for a mock user
func (m *MockProvider) GetUserPermissions(ctx context.Context, userID string) ([]string, error) {
	for _, claims := range m.users {
		if claims.UserID == userID {
			return claims.Permissions, nil
		}
	}
	return []string{}, fmt.Errorf("user not found")
}

// AddUser adds a user to the mock provider (for testing)
func (m *MockProvider) AddUser(token string, claims *Claims) {
	m.users[token] = claims
}
