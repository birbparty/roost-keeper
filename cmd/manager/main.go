package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	zaplib "go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/controllers"
	"github.com/birbparty/roost-keeper/internal/health"
	"github.com/birbparty/roost-keeper/internal/helm"
	"github.com/birbparty/roost-keeper/internal/telemetry"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(roostv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var webhookPort int
	var webhookCertDir string
	var maxConcurrentReconciles int
	var leaderElectionNamespace string
	var cacheSyncPeriod time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.IntVar(&webhookPort, "webhook-port", 9443,
		"Port for the webhook server.")
	flag.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs",
		"Directory containing webhook server certificates.")
	flag.IntVar(&maxConcurrentReconciles, "max-concurrent-reconciles", 10,
		"Maximum number of concurrent reconcile operations.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "",
		"Namespace for leader election ConfigMap/Lease. Defaults to pod namespace.")
	flag.DurationVar(&cacheSyncPeriod, "cache-sync-period", 10*time.Minute,
		"Period for cache resync operations.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Initialize structured logging with OTEL integration
	logger, err := telemetry.NewLogger()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	ctrl.SetLogger(logger)

	// Initialize OTEL SDK for traces and metrics
	ctx := context.Background()
	_, shutdown, err := telemetry.InitOTEL(ctx)
	if err != nil {
		setupLog.Error(err, "Failed to initialize OTEL SDK")
		os.Exit(1)
	}
	defer func() {
		if err := shutdown(ctx); err != nil {
			setupLog.Error(err, "Failed to shutdown OTEL SDK")
		}
	}()

	// Initialize operator metrics
	metrics, err := telemetry.NewOperatorMetrics()
	if err != nil {
		setupLog.Error(err, "Failed to initialize operator metrics")
		os.Exit(1)
	}

	// Set operator start time
	startTime := time.Now()
	metrics.SetOperatorStartTime(ctx, startTime)

	// Start performance monitoring in the background
	go telemetry.PerformanceMonitor(ctx, metrics, 30*time.Second)

	// Get namespace for leader election (defaults to current pod namespace)
	namespace := getNamespaceFromEnv(leaderElectionNamespace)
	setupLog.Info("Using namespace for leader election", "namespace", namespace)

	// Configure advanced TLS settings for security
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2 for security")
		c.NextProtos = []string{"http/1.1"}
		c.MinVersion = tls.VersionTLS12
		c.CipherSuites = []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Configure enterprise webhook server
	webhookServer := webhook.NewServer(webhook.Options{
		Port:    webhookPort,
		CertDir: webhookCertDir,
		TLSOpts: tlsOpts,
	})

	// Configure advanced client-side caching for performance
	cacheOptions := cache.Options{
		SyncPeriod: &cacheSyncPeriod,
		ByObject: map[client.Object]cache.ByObject{
			// Cache ManagedRoost objects in all namespaces with optimized sync
			&roostv1alpha1.ManagedRoost{}: {
				Namespaces: map[string]cache.Config{
					cache.AllNamespaces: {},
				},
			},
			// Cache related Kubernetes resources for efficiency
			&appsv1.Deployment{}: {
				Namespaces: map[string]cache.Config{
					cache.AllNamespaces: {},
				},
			},
			&corev1.Service{}: {
				Namespaces: map[string]cache.Config{
					cache.AllNamespaces: {},
				},
			},
			&corev1.Secret{}: {
				Namespaces: map[string]cache.Config{
					cache.AllNamespaces: {},
				},
			},
			&corev1.ConfigMap{}: {
				Namespaces: map[string]cache.Config{
					cache.AllNamespaces: {},
				},
			},
		},
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,

		// Enterprise metrics configuration
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},

		// Advanced caching configuration for performance
		Cache: cacheOptions,

		// Enhanced webhook server
		WebhookServer: webhookServer,

		// Health probe configuration
		HealthProbeBindAddress: probeAddr,

		// Enterprise leader election configuration
		LeaderElection:                enableLeaderElection,
		LeaderElectionID:              "roost-keeper.roost.birb.party",
		LeaderElectionNamespace:       namespace,
		LeaderElectionReleaseOnCancel: true, // Enable for faster transitions

		// Graceful shutdown configuration
		GracefulShutdownTimeout: &[]time.Duration{30 * time.Second}[0],
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Initialize ZAP logger for Go packages (used by Helm and health checkers)
	zapLogger, err := zaplib.NewDevelopment()
	if err != nil {
		setupLog.Error(err, "Failed to initialize ZAP logger")
		os.Exit(1)
	}

	// Initialize Helm manager
	helmManager, err := helm.NewManager(mgr.GetClient(), mgr.GetConfig(), zapLogger)
	if err != nil {
		setupLog.Error(err, "Failed to initialize Helm manager")
		os.Exit(1)
	}

	// Initialize health checker
	healthChecker := health.NewChecker(zapLogger)

	// Setup controller with enterprise configuration
	if err = (&controllers.ManagedRoostReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		Logger:        logger.WithName("controllers").WithName("ManagedRoost"),
		Metrics:       metrics,
		Config:        mgr.GetConfig(),
		ZapLogger:     zapLogger,
		HelmManager:   helmManager,
		HealthChecker: healthChecker,
		Recorder:      mgr.GetEventRecorderFor("managedroost-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ManagedRoost")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// getNamespaceFromEnv returns the namespace for leader election
func getNamespaceFromEnv(override string) string {
	if override != "" {
		return override
	}

	// Try to get namespace from pod environment
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		return ns
	}

	// Default to roost-keeper-system namespace
	return "roost-keeper-system"
}
