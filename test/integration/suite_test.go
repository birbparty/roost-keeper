package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

var (
	// Test environment configuration
	testEnv       *envtest.Environment
	cfg           *rest.Config
	k8sClient     client.Client
	k8sClientset  *kubernetes.Clientset
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *zap.Logger
	telemetryPath string
	useK3s        bool
	k3sKubeconfig string

	// Test infrastructure helpers
	testInfra *TestInfrastructure
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Roost-Keeper Integration Test Suite")
}

var _ = BeforeSuite(func() {
	// Set up logger for controller-runtime
	logf.SetLogger(GinkgoLogr)

	ctx, cancel = context.WithCancel(context.Background())

	// Initialize logger
	var err error
	logger, err = zap.NewDevelopment()
	Expect(err).NotTo(HaveOccurred())

	// Setup telemetry directory
	telemetryPath = filepath.Join(os.TempDir(), "roost-keeper-integration-telemetry")
	os.RemoveAll(telemetryPath) // Clean from previous runs
	os.MkdirAll(telemetryPath, 0755)

	// Check if we should use k3s cluster
	k3sKubeconfig = filepath.Join(os.Getenv("HOME"), "git/birb-home/.kube/config")
	if _, err := os.Stat(k3sKubeconfig); err == nil {
		useK3s = true
		logger.Info("Using k3s cluster for testing", zap.String("kubeconfig", k3sKubeconfig))
	} else {
		useK3s = false
		logger.Info("Using envtest for testing (k3s not available)")
	}

	// Setup Kubernetes environment
	if useK3s {
		setupK3sEnvironment()
	} else {
		setupEnvTestEnvironment()
	}

	// Initialize test infrastructure
	testInfra = NewTestInfrastructure(k8sClient, k8sClientset, logger, telemetryPath)
	Expect(testInfra).NotTo(BeNil())

	// Setup telemetry for tests
	err = testInfra.SetupTelemetry(ctx)
	Expect(err).NotTo(HaveOccurred())

	logger.Info("Integration test suite initialized",
		zap.Bool("using_k3s", useK3s),
		zap.String("telemetry_path", telemetryPath))
})

var _ = AfterSuite(func() {
	By("Tearing down the test environment")

	if testInfra != nil {
		testInfra.Cleanup(ctx)
	}

	cancel()

	if useK3s {
		// k3s cleanup is handled by test infrastructure
		logger.Info("k3s cluster left running for other tests")
	} else {
		err := testEnv.Stop()
		Expect(err).NotTo(HaveOccurred())
	}

	// Clean up telemetry directory
	os.RemoveAll(telemetryPath)

	logger.Info("Integration test suite cleanup completed")
})

func setupK3sEnvironment() {
	By("Setting up k3s test environment")

	// Load k3s kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", k3sKubeconfig)
	Expect(err).NotTo(HaveOccurred())
	cfg = config

	// Create Kubernetes client
	k8sClientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	// Create controller-runtime client with scheme
	scheme := runtime.NewScheme()
	Expect(roostv1alpha1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Verify cluster connectivity
	_, err = k8sClientset.Discovery().ServerVersion()
	Expect(err).NotTo(HaveOccurred())

	logger.Info("k3s environment setup completed")
}

func setupEnvTestEnvironment() {
	By("Setting up envtest environment")

	// Set up envtest
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "config", "crd", "bases"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Create Kubernetes client
	k8sClientset, err = kubernetes.NewForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())

	// Create controller-runtime client with scheme
	scheme := runtime.NewScheme()
	Expect(roostv1alpha1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	logger.Info("envtest environment setup completed")
}

// TestInfrastructure provides comprehensive test infrastructure management
type TestInfrastructure struct {
	client            client.Client
	clientset         *kubernetes.Clientset
	logger            *zap.Logger
	telemetryPath     string
	metrics           *telemetry.OperatorMetrics
	telemetryShutdown func(context.Context) error
	createdNamespaces []string
}

func NewTestInfrastructure(client client.Client, clientset *kubernetes.Clientset, logger *zap.Logger, telemetryPath string) *TestInfrastructure {
	return &TestInfrastructure{
		client:            client,
		clientset:         clientset,
		logger:            logger,
		telemetryPath:     telemetryPath,
		createdNamespaces: []string{},
	}
}

func (ti *TestInfrastructure) SetupTelemetry(ctx context.Context) error {
	// Change to telemetry directory for test exports
	originalDir, err := os.Getwd()
	if err != nil {
		return err
	}

	err = os.Chdir(ti.telemetryPath)
	if err != nil {
		return err
	}

	// Initialize telemetry
	_, shutdown, err := telemetry.InitOTEL(ctx)
	if err != nil {
		os.Chdir(originalDir)
		return err
	}
	ti.telemetryShutdown = shutdown

	// Initialize metrics
	ti.metrics, err = telemetry.NewOperatorMetrics()
	if err != nil {
		os.Chdir(originalDir)
		return err
	}

	// Change back to original directory
	err = os.Chdir(originalDir)
	if err != nil {
		return err
	}

	ti.logger.Info("Test telemetry initialized", zap.String("export_path", ti.telemetryPath))
	return nil
}

func (ti *TestInfrastructure) CreateTestNamespace(ctx context.Context, baseName string) (string, error) {
	namespace := fmt.Sprintf("%s-%d", baseName, time.Now().Unix())

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				"test.roost-keeper.io/suite":   "integration",
				"test.roost-keeper.io/created": time.Now().Format(time.RFC3339),
			},
		},
	}

	err := ti.client.Create(ctx, ns)
	if err != nil {
		return "", err
	}

	ti.createdNamespaces = append(ti.createdNamespaces, namespace)
	ti.logger.Info("Created test namespace", zap.String("namespace", namespace))

	return namespace, nil
}

func (ti *TestInfrastructure) CleanupNamespace(ctx context.Context, namespace string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	err := ti.client.Delete(ctx, ns)
	if err != nil {
		ti.logger.Warn("Failed to delete namespace", zap.String("namespace", namespace), zap.Error(err))
		return err
	}

	// Wait for namespace deletion
	Eventually(func() bool {
		err := ti.client.Get(ctx, client.ObjectKey{Name: namespace}, ns)
		return err != nil
	}, 2*time.Minute, 5*time.Second).Should(BeTrue())

	ti.logger.Info("Cleaned up test namespace", zap.String("namespace", namespace))
	return nil
}

func (ti *TestInfrastructure) Cleanup(ctx context.Context) {
	ti.logger.Info("Starting test infrastructure cleanup")

	// Cleanup all created namespaces
	for _, namespace := range ti.createdNamespaces {
		err := ti.CleanupNamespace(ctx, namespace)
		if err != nil {
			ti.logger.Warn("Failed to cleanup namespace", zap.String("namespace", namespace), zap.Error(err))
		}
	}

	// Shutdown telemetry
	if ti.telemetryShutdown != nil {
		err := ti.telemetryShutdown(ctx)
		if err != nil {
			ti.logger.Warn("Failed to shutdown telemetry", zap.Error(err))
		}
	}

	ti.logger.Info("Test infrastructure cleanup completed")
}

func (ti *TestInfrastructure) GetMetrics() *telemetry.OperatorMetrics {
	return ti.metrics
}

func (ti *TestInfrastructure) GetTelemetryPath() string {
	return ti.telemetryPath
}

// Global helper functions for tests
func GetTestInfra() *TestInfrastructure {
	return testInfra
}

func GetTestClient() client.Client {
	return k8sClient
}

func GetTestClientset() *kubernetes.Clientset {
	return k8sClientset
}

func GetTestLogger() *zap.Logger {
	return logger
}

func IsUsingK3s() bool {
	return useK3s
}
