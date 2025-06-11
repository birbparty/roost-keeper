package webhook

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// CertificateManager handles TLS certificate lifecycle for webhooks
type CertificateManager struct {
	client      client.Client
	logger      *zap.Logger
	metrics     *telemetry.WebhookMetrics
	certSecret  string
	serviceName string
	namespace   string
}

// CertificateInfo contains certificate information
type CertificateInfo struct {
	CertPEM  []byte
	KeyPEM   []byte
	CABundle []byte
}

// NewCertificateManager creates a new certificate manager
func NewCertificateManager(client client.Client, logger *zap.Logger) *CertificateManager {
	return &CertificateManager{
		client:      client,
		logger:      logger,
		metrics:     telemetry.NewWebhookMetrics(),
		certSecret:  "roost-keeper-webhook-certs",
		serviceName: "roost-keeper-webhook-service",
		namespace:   "roost-keeper-system",
	}
}

// EnsureCertificates ensures webhook TLS certificates are valid and available
func (cm *CertificateManager) EnsureCertificates(ctx context.Context) error {
	cm.logger.Info("Ensuring webhook TLS certificates",
		zap.String("secret", cm.certSecret),
		zap.String("namespace", cm.namespace))

	// Check if certificates exist and are valid
	valid, err := cm.validateExistingCertificates(ctx)
	if err != nil {
		cm.logger.Error("Failed to validate existing certificates", zap.Error(err))
		return fmt.Errorf("failed to validate certificates: %w", err)
	}

	if !valid {
		cm.logger.Info("Generating new webhook certificates")
		if err := cm.generateCertificates(ctx); err != nil {
			cm.logger.Error("Failed to generate certificates", zap.Error(err))
			return fmt.Errorf("failed to generate certificates: %w", err)
		}

		if cm.metrics != nil {
			cm.metrics.RecordCertificateRotation(ctx, "webhook", "expired_or_missing")
		}
	}

	// Update certificate validity metrics
	if cm.metrics != nil {
		if err := cm.updateCertificateMetrics(ctx); err != nil {
			cm.logger.Warn("Failed to update certificate metrics", zap.Error(err))
		}
	}

	return nil
}

// validateExistingCertificates checks if current certificates are valid
func (cm *CertificateManager) validateExistingCertificates(ctx context.Context) (bool, error) {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      cm.certSecret,
		Namespace: cm.namespace,
	}

	err := cm.client.Get(ctx, secretKey, secret)
	if err != nil {
		cm.logger.Info("Certificate secret not found, will generate new certificates")
		return false, nil
	}

	// Check if required certificate data exists
	certPEM, exists := secret.Data["tls.crt"]
	if !exists {
		cm.logger.Info("Certificate data not found in secret")
		return false, nil
	}

	keyPEM, exists := secret.Data["tls.key"]
	if !exists {
		cm.logger.Info("Private key data not found in secret")
		return false, nil
	}

	// Parse and validate certificate
	block, _ := pem.Decode(certPEM)
	if block == nil {
		cm.logger.Info("Failed to decode certificate PEM")
		return false, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		cm.logger.Info("Failed to parse certificate", zap.Error(err))
		return false, nil
	}

	// Check if certificate is still valid (not expired and not expiring soon)
	now := time.Now()
	if cert.NotAfter.Before(now) {
		cm.logger.Info("Certificate has expired",
			zap.Time("not_after", cert.NotAfter),
			zap.Time("now", now))
		return false, nil
	}

	// Check if certificate expires within 30 days
	thirtyDaysFromNow := now.Add(30 * 24 * time.Hour)
	if cert.NotAfter.Before(thirtyDaysFromNow) {
		cm.logger.Info("Certificate expires soon",
			zap.Time("not_after", cert.NotAfter),
			zap.Time("threshold", thirtyDaysFromNow))
		return false, nil
	}

	// Verify private key matches certificate
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		cm.logger.Info("Failed to decode private key PEM")
		return false, nil
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		cm.logger.Info("Failed to parse private key", zap.Error(err))
		return false, nil
	}

	// Basic validation that the key matches the certificate
	if cert.PublicKey.(*rsa.PublicKey).N.Cmp(privateKey.PublicKey.N) != 0 {
		cm.logger.Info("Private key does not match certificate")
		return false, nil
	}

	cm.logger.Info("Existing certificates are valid",
		zap.Time("expires", cert.NotAfter))
	return true, nil
}

// generateCertificates generates new TLS certificates for the webhook
func (cm *CertificateManager) generateCertificates(ctx context.Context) error {
	cm.logger.Info("Generating new TLS certificates for webhook")

	// Generate CA private key
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate CA private key: %w", err)
	}

	// Create CA certificate template
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Roost-Keeper"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// Create CA certificate
	caCertBytes, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Generate server private key
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("failed to generate server private key: %w", err)
	}

	// Create server certificate template
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			Organization:  []string{"Roost-Keeper"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour), // 1 year
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		DNSNames: []string{
			cm.serviceName,
			fmt.Sprintf("%s.%s", cm.serviceName, cm.namespace),
			fmt.Sprintf("%s.%s.svc", cm.serviceName, cm.namespace),
			fmt.Sprintf("%s.%s.svc.cluster.local", cm.serviceName, cm.namespace),
		},
	}

	// Create server certificate
	serverCertBytes, err := x509.CreateCertificate(rand.Reader, &serverTemplate, &caTemplate, &serverKey.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("failed to create server certificate: %w", err)
	}

	// Encode certificates and keys to PEM
	caCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caCertBytes,
	})

	serverCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	})

	serverKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverKey),
	})

	// Create or update the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cm.certSecret,
			Namespace: cm.namespace,
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			"tls.crt": serverCertPEM,
			"tls.key": serverKeyPEM,
			"ca.crt":  caCertPEM,
		},
	}

	// Try to create the secret, if it exists, update it
	err = cm.client.Create(ctx, secret)
	if err != nil {
		// If secret already exists, update it
		existingSecret := &corev1.Secret{}
		secretKey := types.NamespacedName{
			Name:      cm.certSecret,
			Namespace: cm.namespace,
		}

		if getErr := cm.client.Get(ctx, secretKey, existingSecret); getErr == nil {
			existingSecret.Data = secret.Data
			err = cm.client.Update(ctx, existingSecret)
			if err != nil {
				return fmt.Errorf("failed to update certificate secret: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create certificate secret: %w", err)
		}
	}

	cm.logger.Info("Successfully generated and stored new TLS certificates",
		zap.String("secret", cm.certSecret),
		zap.String("namespace", cm.namespace))

	return nil
}

// updateCertificateMetrics updates certificate validity metrics
func (cm *CertificateManager) updateCertificateMetrics(ctx context.Context) error {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      cm.certSecret,
		Namespace: cm.namespace,
	}

	err := cm.client.Get(ctx, secretKey, secret)
	if err != nil {
		return err
	}

	certPEM, exists := secret.Data["tls.crt"]
	if !exists {
		return fmt.Errorf("certificate data not found in secret")
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("failed to decode certificate PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Calculate days until expiration
	now := time.Now()
	daysUntilExpiry := cert.NotAfter.Sub(now).Hours() / 24

	cm.metrics.UpdateCertificateValidity(ctx, "webhook", daysUntilExpiry)

	return nil
}

// GetCertificateInfo returns the current certificate information
func (cm *CertificateManager) GetCertificateInfo(ctx context.Context) (*CertificateInfo, error) {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      cm.certSecret,
		Namespace: cm.namespace,
	}

	err := cm.client.Get(ctx, secretKey, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate secret: %w", err)
	}

	certPEM, exists := secret.Data["tls.crt"]
	if !exists {
		return nil, fmt.Errorf("certificate data not found in secret")
	}

	keyPEM, exists := secret.Data["tls.key"]
	if !exists {
		return nil, fmt.Errorf("private key data not found in secret")
	}

	caBundle, exists := secret.Data["ca.crt"]
	if !exists {
		return nil, fmt.Errorf("CA bundle not found in secret")
	}

	return &CertificateInfo{
		CertPEM:  certPEM,
		KeyPEM:   keyPEM,
		CABundle: caBundle,
	}, nil
}

// SetCertificateSecret sets the name of the secret used to store certificates
func (cm *CertificateManager) SetCertificateSecret(secretName string) {
	cm.certSecret = secretName
}

// SetServiceName sets the name of the webhook service
func (cm *CertificateManager) SetServiceName(serviceName string) {
	cm.serviceName = serviceName
}

// SetNamespace sets the namespace for the webhook
func (cm *CertificateManager) SetNamespace(namespace string) {
	cm.namespace = namespace
}
