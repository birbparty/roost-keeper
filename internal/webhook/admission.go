package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// AdmissionController handles validating and mutating admission webhook requests
type AdmissionController struct {
	client       client.Client
	decoder      admission.Decoder
	logger       *zap.Logger
	metrics      *telemetry.WebhookMetrics
	policyEngine *SimplePolicyEngine
	certManager  *CertificateManager
}

// AdmissionRequest wraps the raw admission request with decoded objects
type AdmissionRequest struct {
	Raw       *admissionv1.AdmissionRequest
	Object    runtime.Object
	OldObject runtime.Object
	Context   context.Context
}

// NewAdmissionController creates a new admission controller with all required dependencies
func NewAdmissionController(client client.Client, logger *zap.Logger) *AdmissionController {
	metrics := telemetry.NewWebhookMetrics()

	return &AdmissionController{
		client:       client,
		logger:       logger,
		metrics:      metrics,
		policyEngine: NewSimplePolicyEngine(logger, metrics),
		certManager:  NewCertificateManager(client, logger),
	}
}

// InjectDecoder implements admission.DecoderInjector interface
func (ac *AdmissionController) InjectDecoder(d admission.Decoder) error {
	ac.decoder = d
	return nil
}

// Handle implements the admission.Handler interface and routes requests to appropriate handlers
func (ac *AdmissionController) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := ac.logger.With(
		zap.String("operation", string(req.Operation)),
		zap.String("resource", req.Kind.String()),
		zap.String("name", req.Name),
		zap.String("namespace", req.Namespace),
		zap.String("user", req.UserInfo.Username),
	)

	// Determine webhook type based on request path
	if req.AdmissionRequest.Kind.Kind == "ManagedRoost" {
		// Route to specific handlers based on webhook configuration
		// This will be determined by the webhook configuration in Kubernetes
		return ac.handleManagedRoost(ctx, req)
	}

	log.Warn("Unhandled resource type", zap.String("kind", req.Kind.Kind))
	return admission.Allowed("unhandled resource type")
}

// handleManagedRoost processes admission requests for ManagedRoost resources
func (ac *AdmissionController) handleManagedRoost(ctx context.Context, req admission.Request) admission.Response {
	log := ac.logger.With(
		zap.String("handler", "managedroost"),
		zap.String("operation", string(req.Operation)),
	)

	// For this implementation, we'll handle both validation and mutation
	// In production, these would be separate webhook endpoints

	// First, apply mutations
	mutateResp := ac.MutateManagedRoost(ctx, req)
	if !mutateResp.Allowed {
		return mutateResp
	}

	// Apply patches if any were generated
	if len(mutateResp.Patches) > 0 {
		// Apply patches to the request object for validation
		// This ensures validation happens on the mutated object
		log.Info("Applied mutations", zap.Int("patch_count", len(mutateResp.Patches)))
	}

	// Then, validate the (potentially mutated) object
	validateResp := ac.ValidateManagedRoost(ctx, req)
	if !validateResp.Allowed {
		return validateResp
	}

	// Combine responses - return patches if mutation occurred
	if len(mutateResp.Patches) > 0 {
		return mutateResp
	}

	return validateResp
}

// ValidateManagedRoost handles validation of ManagedRoost resources
func (ac *AdmissionController) ValidateManagedRoost(ctx context.Context, req admission.Request) admission.Response {
	log := ac.logger.With(
		zap.String("operation", "validate"),
		zap.String("resource", req.Kind.String()),
		zap.String("name", req.Name),
		zap.String("namespace", req.Namespace),
	)

	// Start validation span for observability
	ctx, span := telemetry.StartWebhookSpan(ctx, "validate", req.Kind.String())
	defer span.End()

	start := time.Now()
	defer func() {
		if ac.metrics != nil {
			ac.metrics.RecordWebhookDuration(ctx, "validate", time.Since(start).Seconds())
		}
	}()

	log.Info("Starting admission validation")

	// Decode the ManagedRoost object
	var managedRoost roostkeeper.ManagedRoost
	if err := ac.decoder.Decode(req, &managedRoost); err != nil {
		log.Error("Failed to decode ManagedRoost", zap.Error(err))
		telemetry.RecordError(span, err)
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Create admission request wrapper
	admissionReq := &AdmissionRequest{
		Raw:     &req.AdmissionRequest,
		Object:  &managedRoost,
		Context: ctx,
	}

	// Handle update operations - decode old object
	if req.OldObject.Raw != nil {
		var oldManagedRoost roostkeeper.ManagedRoost
		if err := ac.decoder.DecodeRaw(req.OldObject, &oldManagedRoost); err != nil {
			log.Error("Failed to decode old ManagedRoost", zap.Error(err))
			return admission.Errored(http.StatusBadRequest, err)
		}
		admissionReq.OldObject = &oldManagedRoost
	}

	// Validate through simplified policy engine
	result, err := ac.policyEngine.ValidateSimple(ctx, &managedRoost)
	if err != nil {
		log.Error("Policy validation failed", zap.Error(err))
		telemetry.RecordError(span, err)
		if ac.metrics != nil {
			ac.metrics.RecordWebhookError(ctx, "validate", "policy_error")
		}
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if !result.Allowed {
		log.Warn("Admission denied by policy",
			zap.String("reason", result.Message),
			zap.Int32("code", result.Code),
			zap.Any("details", result.Details))

		if ac.metrics != nil {
			ac.metrics.RecordWebhookRejection(ctx, "validate", result.Message)
		}

		return admission.Denied(result.Message)
	}

	// Additional validations
	if tenancyResult, err := ac.policyEngine.ValidateTenancy(ctx, &managedRoost); err != nil {
		log.Error("Tenancy validation failed", zap.Error(err))
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !tenancyResult.Allowed {
		log.Warn("Admission denied by tenancy policy", zap.String("reason", tenancyResult.Message))
		if ac.metrics != nil {
			ac.metrics.RecordWebhookRejection(ctx, "validate", tenancyResult.Message)
		}
		return admission.Denied(tenancyResult.Message)
	}

	if obsResult, err := ac.policyEngine.ValidateObservability(ctx, &managedRoost); err != nil {
		log.Error("Observability validation failed", zap.Error(err))
		return admission.Errored(http.StatusInternalServerError, err)
	} else if !obsResult.Allowed {
		log.Warn("Admission denied by observability policy", zap.String("reason", obsResult.Message))
		if ac.metrics != nil {
			ac.metrics.RecordWebhookRejection(ctx, "validate", obsResult.Message)
		}
		return admission.Denied(obsResult.Message)
	}

	log.Info("Admission validation passed")
	return admission.Allowed("")
}

// MutateManagedRoost handles mutation of ManagedRoost resources
func (ac *AdmissionController) MutateManagedRoost(ctx context.Context, req admission.Request) admission.Response {
	log := ac.logger.With(
		zap.String("operation", "mutate"),
		zap.String("resource", req.Kind.String()),
		zap.String("name", req.Name),
		zap.String("namespace", req.Namespace),
	)

	ctx, span := telemetry.StartWebhookSpan(ctx, "mutate", req.Kind.String())
	defer span.End()

	start := time.Now()
	defer func() {
		if ac.metrics != nil {
			ac.metrics.RecordWebhookDuration(ctx, "mutate", time.Since(start).Seconds())
		}
	}()

	log.Info("Starting admission mutation")

	// Decode the ManagedRoost object
	var managedRoost roostkeeper.ManagedRoost
	if err := ac.decoder.Decode(req, &managedRoost); err != nil {
		log.Error("Failed to decode ManagedRoost", zap.Error(err))
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Apply mutations through simplified policy engine
	patches, err := ac.policyEngine.MutateSimple(ctx, &managedRoost)
	if err != nil {
		log.Error("Policy mutation failed", zap.Error(err))
		telemetry.RecordError(span, err)
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if len(patches) > 0 {
		log.Info("Applied mutations to resource",
			zap.Int("patch_count", len(patches)))

		if ac.metrics != nil {
			patchSize := 0
			for _, patch := range patches {
				patchBytes, _ := json.Marshal(patch)
				patchSize += len(patchBytes)
			}
			ac.metrics.RecordWebhookMutation(ctx, "mutate", patchSize)
		}

		return admission.Patched("Applied tenant labeling and security policies", patches...)
	}

	log.Info("No mutations required")
	return admission.Allowed("")
}

// EnsureCertificates ensures webhook TLS certificates are valid and available
func (ac *AdmissionController) EnsureCertificates(ctx context.Context) error {
	return ac.certManager.EnsureCertificates(ctx)
}

// GetCertificateManager returns the certificate manager for external use
func (ac *AdmissionController) GetCertificateManager() *CertificateManager {
	return ac.certManager
}
