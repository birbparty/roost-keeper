package helm

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// loadChart loads a Helm chart based on the ManagedRoost specification
func (hm *HelmManager) loadChart(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*chart.Chart, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.load_chart", roost.Name, roost.Namespace)
	defer span.End()

	log := hm.Logger.With(
		zap.String("chart", roost.Spec.Chart.Name),
		zap.String("version", roost.Spec.Chart.Version),
		zap.String("repository", roost.Spec.Chart.Repository.URL),
	)

	log.Info("Loading Helm chart")

	// Handle different repository types
	switch roost.Spec.Chart.Repository.Type {
	case "oci", "":
		if strings.HasPrefix(roost.Spec.Chart.Repository.URL, "oci://") {
			return hm.loadOCIChart(ctx, roost)
		}
		fallthrough
	case "http":
		return hm.loadHTTPChart(ctx, roost)
	case "git":
		return hm.loadGitChart(ctx, roost)
	default:
		return nil, fmt.Errorf("unsupported repository type: %s", roost.Spec.Chart.Repository.Type)
	}
}

// loadHTTPChart loads a chart from an HTTP repository
func (hm *HelmManager) loadHTTPChart(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*chart.Chart, error) {
	log := hm.Logger.With(zap.String("method", "http"))

	// Create temporary directory for chart download
	tempDir := filepath.Join(hm.Settings.RepositoryCache, "temp")

	// Setup repository entry
	repoEntry := &repo.Entry{
		Name: fmt.Sprintf("%s-%s", roost.Namespace, roost.Name),
		URL:  roost.Spec.Chart.Repository.URL,
	}

	// Configure authentication if provided
	if auth := roost.Spec.Chart.Repository.Auth; auth != nil {
		if auth.Username != "" && auth.Password != "" {
			repoEntry.Username = auth.Username
			repoEntry.Password = auth.Password
		}
		// TODO: Handle secret references for authentication
	}

	// Configure TLS if provided
	if tls := roost.Spec.Chart.Repository.TLS; tls != nil {
		repoEntry.InsecureSkipTLSverify = tls.InsecureSkipVerify
		if len(tls.CABundle) > 0 {
			// TODO: Handle CA bundle configuration
		}
	}

	// Create chart repository
	chartRepo, err := repo.NewChartRepository(repoEntry, getter.All(hm.Settings))
	if err != nil {
		return nil, fmt.Errorf("failed to create chart repository: %w", err)
	}

	// Download repository index
	indexFile, err := chartRepo.DownloadIndexFile()
	if err != nil {
		return nil, fmt.Errorf("failed to download repository index: %w", err)
	}

	log.Info("Repository index downloaded", zap.String("index_file", indexFile))

	// Load repository index
	index, err := repo.LoadIndexFile(indexFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load repository index: %w", err)
	}

	// Find chart version
	chartVersion, err := index.Get(roost.Spec.Chart.Name, roost.Spec.Chart.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to find chart %s version %s: %w",
			roost.Spec.Chart.Name, roost.Spec.Chart.Version, err)
	}

	if len(chartVersion.URLs) == 0 {
		return nil, fmt.Errorf("no URLs found for chart %s version %s",
			roost.Spec.Chart.Name, roost.Spec.Chart.Version)
	}

	// Download chart
	chartURL := chartVersion.URLs[0]
	if !strings.HasPrefix(chartURL, "http") {
		// Relative URL, prepend repository URL
		chartURL = strings.TrimSuffix(roost.Spec.Chart.Repository.URL, "/") + "/" + chartURL
	}

	log.Info("Downloading chart", zap.String("url", chartURL))

	// Create downloader
	dl := downloader.ChartDownloader{
		Verify:  downloader.VerifyNever, // TODO: Add signature verification
		Getters: getter.All(hm.Settings),
	}

	// Download chart archive
	chartPath, _, err := dl.DownloadTo(chartURL, roost.Spec.Chart.Version, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to download chart: %w", err)
	}

	log.Info("Chart downloaded", zap.String("path", chartPath))

	// Load chart from archive
	chart, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart from %s: %w", chartPath, err)
	}

	log.Info("Chart loaded successfully",
		zap.String("name", chart.Metadata.Name),
		zap.String("version", chart.Metadata.Version),
	)

	return chart, nil
}

// loadOCIChart loads a chart from an OCI registry
func (hm *HelmManager) loadOCIChart(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*chart.Chart, error) {
	log := hm.Logger.With(zap.String("method", "oci"))

	// Construct OCI reference
	ref := fmt.Sprintf("%s/%s:%s",
		strings.TrimPrefix(roost.Spec.Chart.Repository.URL, "oci://"),
		roost.Spec.Chart.Name,
		roost.Spec.Chart.Version,
	)

	log.Info("Loading OCI chart", zap.String("reference", ref))

	// TODO: Implement OCI chart loading using registry client
	// For now, return an error indicating OCI is not yet implemented
	return nil, fmt.Errorf("OCI chart loading not yet implemented")
}

// loadGitChart loads a chart from a Git repository
func (hm *HelmManager) loadGitChart(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (*chart.Chart, error) {
	log := hm.Logger.With(zap.String("method", "git"))

	log.Info("Loading Git chart")

	// TODO: Implement Git chart loading
	// For now, return an error indicating Git is not yet implemented
	return nil, fmt.Errorf("Git chart loading not yet implemented")
}
