package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-logr/logr"
	"github.com/urfave/cli/v2"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// GlobalConfig holds the global CLI configuration
type GlobalConfig struct {
	KubeConfig   string
	Context      string
	Namespace    string
	OutputFormat string
	Verbose      bool
	Debug        bool

	// Kubernetes clients
	K8sClient   client.Client
	RESTClient  kubernetes.Interface
	RESTConfig  *rest.Config
	ConfigFlags *genericclioptions.ConfigFlags

	// Telemetry
	Logger logr.Logger
}

var globalConfig *GlobalConfig

// InitializeGlobalConfig initializes the global CLI configuration
func InitializeGlobalConfig(c *cli.Context) error {
	config := &GlobalConfig{
		KubeConfig:   c.String("kubeconfig"),
		Context:      c.String("context"),
		Namespace:    c.String("namespace"),
		OutputFormat: c.String("output"),
		Verbose:      c.Bool("verbose"),
		Debug:        c.Bool("debug"),
	}

	// Initialize telemetry
	var err error
	config.Logger, err = telemetry.NewLogger()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Only initialize Kubernetes clients for commands that need them
	if needsKubernetesClient(c) {
		if err := config.initKubernetesClients(); err != nil {
			return fmt.Errorf("failed to initialize Kubernetes clients: %w", err)
		}
	}

	globalConfig = config
	return nil
}

// GetGlobalConfig returns the global configuration
func GetGlobalConfig() *GlobalConfig {
	return globalConfig
}

// initKubernetesClients initializes the Kubernetes clients
func (gc *GlobalConfig) initKubernetesClients() error {
	// Create config flags
	gc.ConfigFlags = genericclioptions.NewConfigFlags(true)

	if gc.KubeConfig != "" {
		gc.ConfigFlags.KubeConfig = &gc.KubeConfig
	}
	if gc.Context != "" {
		gc.ConfigFlags.Context = &gc.Context
	}
	if gc.Namespace != "" {
		gc.ConfigFlags.Namespace = &gc.Namespace
	}

	// Get REST config
	var err error
	gc.RESTConfig, err = gc.ConfigFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	// Create controller-runtime client
	scheme, err := roostv1alpha1.SchemeBuilder.Build()
	if err != nil {
		return fmt.Errorf("failed to build scheme: %w", err)
	}
	gc.K8sClient, err = client.New(gc.RESTConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("failed to create controller-runtime client: %w", err)
	}

	// Create REST client
	gc.RESTClient, err = kubernetes.NewForConfig(gc.RESTConfig)
	if err != nil {
		return fmt.Errorf("failed to create REST client: %w", err)
	}

	return nil
}

// GetCurrentNamespace returns the current namespace from config or context
func (gc *GlobalConfig) GetCurrentNamespace() string {
	if gc.Namespace != "" {
		return gc.Namespace
	}

	// Try to get from kubeconfig
	if kubeconfig := gc.getKubeConfigPath(); kubeconfig != "" {
		if config, err := clientcmd.LoadFromFile(kubeconfig); err == nil {
			if context := config.Contexts[config.CurrentContext]; context != nil {
				if context.Namespace != "" {
					return context.Namespace
				}
			}
		}
	}

	return "default"
}

// getKubeConfigPath returns the kubeconfig path
func (gc *GlobalConfig) getKubeConfigPath() string {
	if gc.KubeConfig != "" {
		return gc.KubeConfig
	}

	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return kubeconfig
	}

	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".kube", "config")
	}

	return ""
}

// getLogLevel determines the log level based on flags
func getLogLevel(debug, verbose bool) string {
	if debug {
		return "debug"
	}
	if verbose {
		return "info"
	}
	return "warn"
}

// IsKubectlPlugin determines if the CLI is running as a kubectl plugin
func IsKubectlPlugin() bool {
	if len(os.Args) == 0 {
		return false
	}

	executable := filepath.Base(os.Args[0])
	return strings.HasPrefix(executable, "kubectl-")
}

// ShowAppHelp shows the application help
func ShowAppHelp(c *cli.Context) error {
	return cli.ShowAppHelp(c)
}

// needsKubernetesClient determines if a command needs Kubernetes client initialization
func needsKubernetesClient(c *cli.Context) bool {
	args := c.Args().Slice()
	if len(args) == 0 {
		return false
	}

	command := args[0]

	// Commands that don't need Kubernetes clients
	nonK8sCommands := map[string]bool{
		"version":    true,
		"completion": true,
		"template":   true, // Template generation doesn't need K8s access
		"help":       true,
	}

	// Special case for template validate subcommand - it doesn't need K8s
	if command == "template" && len(args) > 1 && args[1] == "validate" {
		return false
	}

	return !nonK8sCommands[command]
}

// EnsureKubernetesClients ensures Kubernetes clients are initialized
func EnsureKubernetesClients() error {
	if globalConfig == nil {
		return fmt.Errorf("global config not initialized")
	}

	if globalConfig.K8sClient == nil {
		return globalConfig.initKubernetesClients()
	}

	return nil
}
