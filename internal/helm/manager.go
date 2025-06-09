package helm

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// Manager defines the interface for Helm operations
type Manager interface {
	// Install deploys a new Helm release
	Install(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error)

	// Upgrade updates an existing Helm release
	Upgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error)

	// Uninstall removes a Helm release
	Uninstall(ctx context.Context, roost *roostv1alpha1.ManagedRoost) error

	// ReleaseExists checks if a release already exists
	ReleaseExists(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error)

	// NeedsUpgrade determines if the release needs to be upgraded
	NeedsUpgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error)

	// GetStatus returns the current status of a Helm release
	GetStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*roostv1alpha1.HelmReleaseStatus, error)
}

// HelmManager implements the Manager interface
type HelmManager struct {
	client.Client
	Config   *rest.Config
	Logger   *zap.Logger
	Settings *cli.EnvSettings
	Registry *registry.Client
}

// NewManager creates a new Helm manager
func NewManager(client client.Client, config *rest.Config, logger *zap.Logger) (*HelmManager, error) {
	settings := cli.New()

	// Create registry client for OCI support
	registryClient, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}

	return &HelmManager{
		Client:   client,
		Config:   config,
		Logger:   logger,
		Settings: settings,
		Registry: registryClient,
	}, nil
}

// Install deploys a new Helm release
func (hm *HelmManager) Install(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.install", roost.Name, roost.Namespace)
	defer span.End()

	log := hm.Logger.With(
		zap.String("operation", "install"),
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
		zap.String("chart", roost.Spec.Chart.Name),
		zap.String("version", roost.Spec.Chart.Version),
	)

	log.Info("Starting Helm installation")

	// Create action configuration
	actionConfig, err := hm.createActionConfig(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to create action config: %w", err)
	}

	// Create install client
	installClient := action.NewInstall(actionConfig)
	installClient.ReleaseName = hm.getReleaseName(roost)
	installClient.Namespace = hm.getTargetNamespace(roost)
	installClient.CreateNamespace = true
	installClient.Wait = true
	installClient.Timeout = hm.getTimeout(roost)

	// Configure upgrade policy if specified
	if roost.Spec.Chart.UpgradePolicy != nil {
		installClient.Atomic = roost.Spec.Chart.UpgradePolicy.Atomic
	} else {
		installClient.Atomic = true // Default to atomic
	}

	// Load chart
	chart, err := hm.loadChart(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to load chart: %w", err)
	}

	// Prepare values
	values, err := hm.prepareValues(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to prepare values: %w", err)
	}

	// Execute installation
	rel, err := installClient.Run(chart, values)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("Helm installation failed", zap.Error(err))
		return false, fmt.Errorf("helm install failed: %w", err)
	}

	log.Info("Helm installation completed successfully",
		zap.String("release_name", rel.Name),
		zap.String("status", string(rel.Info.Status)),
		zap.Int("revision", rel.Version),
	)

	telemetry.RecordSpanSuccess(ctx)
	return true, nil
}

// Upgrade updates an existing Helm release
func (hm *HelmManager) Upgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.upgrade", roost.Name, roost.Namespace)
	defer span.End()

	log := hm.Logger.With(
		zap.String("operation", "upgrade"),
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
		zap.String("chart", roost.Spec.Chart.Name),
		zap.String("version", roost.Spec.Chart.Version),
	)

	log.Info("Starting Helm upgrade")

	// Create action configuration
	actionConfig, err := hm.createActionConfig(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to create action config: %w", err)
	}

	// Create upgrade client
	upgradeClient := action.NewUpgrade(actionConfig)
	upgradeClient.Wait = true
	upgradeClient.Timeout = hm.getTimeout(roost)

	// Configure upgrade policy
	if roost.Spec.Chart.UpgradePolicy != nil {
		upgradeClient.Force = roost.Spec.Chart.UpgradePolicy.Force
		upgradeClient.Atomic = roost.Spec.Chart.UpgradePolicy.Atomic
		upgradeClient.CleanupOnFail = roost.Spec.Chart.UpgradePolicy.CleanupOnFail
	} else {
		// Set sensible defaults
		upgradeClient.Atomic = true
		upgradeClient.CleanupOnFail = true
	}

	// Load chart
	chart, err := hm.loadChart(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to load chart: %w", err)
	}

	// Prepare values
	values, err := hm.prepareValues(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to prepare values: %w", err)
	}

	// Execute upgrade
	rel, err := upgradeClient.Run(hm.getReleaseName(roost), chart, values)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("Helm upgrade failed", zap.Error(err))
		return false, fmt.Errorf("helm upgrade failed: %w", err)
	}

	log.Info("Helm upgrade completed successfully",
		zap.String("release_name", rel.Name),
		zap.String("status", string(rel.Info.Status)),
		zap.Int("revision", rel.Version),
	)

	telemetry.RecordSpanSuccess(ctx)
	return true, nil
}

// Uninstall removes a Helm release
func (hm *HelmManager) Uninstall(ctx context.Context, roost *roostv1alpha1.ManagedRoost) error {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.uninstall", roost.Name, roost.Namespace)
	defer span.End()

	log := hm.Logger.With(
		zap.String("operation", "uninstall"),
		zap.String("roost", roost.Name),
		zap.String("namespace", roost.Namespace),
	)

	log.Info("Starting Helm uninstall")

	// Create action configuration
	actionConfig, err := hm.createActionConfig(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return fmt.Errorf("failed to create action config: %w", err)
	}

	// Create uninstall client
	uninstallClient := action.NewUninstall(actionConfig)
	uninstallClient.Wait = true
	uninstallClient.Timeout = hm.getTimeout(roost)

	// Configure cleanup policy
	if roost.Spec.TeardownPolicy != nil && roost.Spec.TeardownPolicy.Cleanup != nil {
		uninstallClient.Timeout = roost.Spec.TeardownPolicy.Cleanup.GracePeriod.Duration
	}

	// Execute uninstall
	_, err = uninstallClient.Run(hm.getReleaseName(roost))
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("Helm uninstall failed", zap.Error(err))
		return fmt.Errorf("helm uninstall failed: %w", err)
	}

	log.Info("Helm uninstall completed successfully")
	telemetry.RecordSpanSuccess(ctx)
	return nil
}

// ReleaseExists checks if a release already exists
func (hm *HelmManager) ReleaseExists(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.release_exists", roost.Name, roost.Namespace)
	defer span.End()

	// Create action configuration
	actionConfig, err := hm.createActionConfig(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to create action config: %w", err)
	}

	// Create get client
	getClient := action.NewGet(actionConfig)

	// Check if release exists
	_, err = getClient.Run(hm.getReleaseName(roost))
	if err != nil {
		if err.Error() == "release: not found" {
			return false, nil
		}
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to check release existence: %w", err)
	}

	telemetry.RecordSpanSuccess(ctx)
	return true, nil
}

// NeedsUpgrade determines if the release needs to be upgraded
func (hm *HelmManager) NeedsUpgrade(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.needs_upgrade", roost.Name, roost.Namespace)
	defer span.End()

	// Create action configuration
	actionConfig, err := hm.createActionConfig(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to create action config: %w", err)
	}

	// Get current release
	getClient := action.NewGet(actionConfig)
	currentRelease, err := getClient.Run(hm.getReleaseName(roost))
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to get current release: %w", err)
	}

	// Load target chart
	targetChart, err := hm.loadChart(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return false, fmt.Errorf("failed to load target chart: %w", err)
	}

	// Compare versions
	needsUpgrade := currentRelease.Chart.Metadata.Version != targetChart.Metadata.Version

	if needsUpgrade {
		hm.Logger.Info("Upgrade needed",
			zap.String("current_version", currentRelease.Chart.Metadata.Version),
			zap.String("target_version", targetChart.Metadata.Version),
		)
	}

	telemetry.RecordSpanSuccess(ctx)
	return needsUpgrade, nil
}

// GetStatus returns the current status of a Helm release
func (hm *HelmManager) GetStatus(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*roostv1alpha1.HelmReleaseStatus, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.get_status", roost.Name, roost.Namespace)
	defer span.End()

	// Create action configuration
	actionConfig, err := hm.createActionConfig(ctx, roost)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		return nil, fmt.Errorf("failed to create action config: %w", err)
	}

	// Get release
	getClient := action.NewGet(actionConfig)
	rel, err := getClient.Run(hm.getReleaseName(roost))
	if err != nil {
		if err.Error() == "release: not found" {
			return nil, nil
		}
		telemetry.RecordSpanError(ctx, err)
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	status := &roostv1alpha1.HelmReleaseStatus{
		Name:        rel.Name,
		Revision:    int32(rel.Version),
		Status:      string(rel.Info.Status),
		Description: rel.Info.Description,
		Notes:       rel.Info.Notes,
	}

	if !rel.Info.LastDeployed.IsZero() {
		lastDeployed := rel.Info.LastDeployed.Time
		status.LastDeployed = &metav1.Time{Time: lastDeployed}
	}

	telemetry.RecordSpanSuccess(ctx)
	return status, nil
}

// Helper methods

func (hm *HelmManager) createActionConfig(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)

	namespace := hm.getTargetNamespace(roost)

	// Initialize with Kubernetes client
	err := actionConfig.Init(hm.Settings.RESTClientGetter(), namespace, "secret", func(format string, v ...interface{}) {
		hm.Logger.Debug("Helm action", zap.String("message", fmt.Sprintf(format, v...)))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize action config: %w", err)
	}

	return actionConfig, nil
}

func (hm *HelmManager) getReleaseName(roost *roostv1alpha1.ManagedRoost) string {
	// Use roost name as release name by default
	return roost.Name
}

func (hm *HelmManager) getTargetNamespace(roost *roostv1alpha1.ManagedRoost) string {
	// Use specified namespace or fall back to roost namespace
	if roost.Spec.Namespace != "" {
		return roost.Spec.Namespace
	}
	return roost.Namespace
}

func (hm *HelmManager) getTimeout(roost *roostv1alpha1.ManagedRoost) time.Duration {
	// Default timeout
	timeout := 5 * time.Minute

	// Use upgrade policy timeout if specified
	if roost.Spec.Chart.UpgradePolicy != nil && roost.Spec.Chart.UpgradePolicy.Timeout.Duration > 0 {
		timeout = roost.Spec.Chart.UpgradePolicy.Timeout.Duration
	}

	return timeout
}
