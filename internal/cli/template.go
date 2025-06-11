package cli

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// TemplateCommand returns the template command
func TemplateCommand() *cli.Command {
	return &cli.Command{
		Name:  "template",
		Usage: "Generate ManagedRoost templates",
		Subcommands: []*cli.Command{
			{
				Name:      "generate",
				Usage:     "Generate ManagedRoost YAML template",
				ArgsUsage: "NAME",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "chart",
						Usage: "Helm chart name",
					},
					&cli.StringFlag{
						Name:  "repo",
						Usage: "Helm repository URL",
					},
					&cli.StringFlag{
						Name:  "version",
						Usage: "Chart version",
					},
					&cli.StringFlag{
						Name:    "namespace",
						Aliases: []string{"n"},
						Usage:   "Target namespace",
						Value:   "default",
					},
					&cli.StringFlag{
						Name:  "output",
						Usage: "Output file (default: stdout)",
					},
					&cli.StringFlag{
						Name:  "preset",
						Usage: "Use preset template (simple, complex, monitoring, webapp)",
						Value: "simple",
					},
				},
				Action: func(c *cli.Context) error {
					return runTemplateGenerate(c)
				},
			},
			{
				Name:  "validate",
				Usage: "Validate ManagedRoost template",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "filename",
						Aliases:  []string{"f"},
						Usage:    "YAML file to validate",
						Required: true,
					},
				},
				Action: func(c *cli.Context) error {
					return runTemplateValidate(c)
				},
			},
		},
	}
}

// runTemplateGenerate generates a ManagedRoost template
func runTemplateGenerate(c *cli.Context) error {
	if c.NArg() == 0 {
		return fmt.Errorf("resource name is required")
	}

	name := c.Args().Get(0)
	chart := c.String("chart")
	repo := c.String("repo")
	version := c.String("version")
	namespace := c.String("namespace")
	preset := c.String("preset")
	outputFile := c.String("output")

	if chart == "" {
		chart = name // Default chart name to resource name
	}

	var template string
	var err error

	switch preset {
	case "simple":
		template = generateSimpleTemplate(name, namespace, chart, repo, version)
	case "complex":
		template = generateComplexTemplate(name, namespace, chart, repo, version)
	case "monitoring":
		template = generateMonitoringTemplate(name, namespace, chart, repo, version)
	case "webapp":
		template = generateWebAppTemplate(name, namespace, chart, repo, version)
	default:
		return fmt.Errorf("unknown preset: %s", preset)
	}

	if outputFile != "" {
		err = os.WriteFile(outputFile, []byte(template), 0644)
		if err != nil {
			return fmt.Errorf("failed to write template to file: %w", err)
		}
		fmt.Printf("Template written to %s\n", outputFile)
	} else {
		fmt.Print(template)
	}

	return nil
}

// runTemplateValidate validates a ManagedRoost template
func runTemplateValidate(c *cli.Context) error {
	filename := c.String("filename")

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	var roost roostv1alpha1.ManagedRoost
	if err := yaml.Unmarshal(data, &roost); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}

	// Validate required fields
	if roost.Name == "" {
		return fmt.Errorf("name is required")
	}

	if roost.Spec.Chart.Name == "" {
		return fmt.Errorf("chart name is required")
	}

	if roost.Spec.Chart.Repository.URL == "" {
		return fmt.Errorf("chart repository URL is required")
	}

	if roost.Spec.Chart.Version == "" {
		return fmt.Errorf("chart version is required")
	}

	// Validate health checks
	for i, check := range roost.Spec.HealthChecks {
		if check.Name == "" {
			return fmt.Errorf("health check %d: name is required", i)
		}
		if check.Type == "" {
			return fmt.Errorf("health check %s: type is required", check.Name)
		}
		if err := validateHealthCheck(&check); err != nil {
			return fmt.Errorf("health check %s: %w", check.Name, err)
		}
	}

	fmt.Printf("✅ Template %s is valid\n", filename)
	return nil
}

// validateHealthCheck validates a health check configuration
func validateHealthCheck(check *roostv1alpha1.HealthCheckSpec) error {
	switch check.Type {
	case "http":
		if check.HTTP == nil {
			return fmt.Errorf("HTTP configuration is required for HTTP health checks")
		}
		if check.HTTP.URL == "" {
			return fmt.Errorf("URL is required for HTTP health checks")
		}
	case "tcp":
		if check.TCP == nil {
			return fmt.Errorf("TCP configuration is required for TCP health checks")
		}
		if check.TCP.Host == "" {
			return fmt.Errorf("host is required for TCP health checks")
		}
		if check.TCP.Port == 0 {
			return fmt.Errorf("port is required for TCP health checks")
		}
	case "grpc":
		if check.GRPC == nil {
			return fmt.Errorf("gRPC configuration is required for gRPC health checks")
		}
		if check.GRPC.Host == "" {
			return fmt.Errorf("host is required for gRPC health checks")
		}
		if check.GRPC.Port == 0 {
			return fmt.Errorf("port is required for gRPC health checks")
		}
	case "prometheus":
		if check.Prometheus == nil {
			return fmt.Errorf("Prometheus configuration is required for Prometheus health checks")
		}
		if check.Prometheus.Query == "" {
			return fmt.Errorf("query is required for Prometheus health checks")
		}
		if check.Prometheus.Threshold == "" {
			return fmt.Errorf("threshold is required for Prometheus health checks")
		}
	case "kubernetes":
		if check.Kubernetes == nil {
			return fmt.Errorf("Kubernetes configuration is required for Kubernetes health checks")
		}
	default:
		return fmt.Errorf("unknown health check type: %s", check.Type)
	}
	return nil
}

// generateSimpleTemplate generates a simple ManagedRoost template
func generateSimpleTemplate(name, namespace, chart, repo, version string) string {
	if repo == "" {
		repo = "https://charts.helm.sh/stable"
	}
	if version == "" {
		version = "latest"
	}

	return fmt.Sprintf(`apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: %s
  namespace: %s
spec:
  chart:
    name: %s
    repository:
      url: %s
    version: %s
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://{{.ServiceName}}.{{.Namespace}}.svc.cluster.local:8080/health"
      interval: 30s
      timeout: 10s
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 1h
`, name, namespace, chart, repo, version)
}

// generateComplexTemplate generates a complex ManagedRoost template
func generateComplexTemplate(name, namespace, chart, repo, version string) string {
	if repo == "" {
		repo = "https://charts.helm.sh/stable"
	}
	if version == "" {
		version = "latest"
	}

	return fmt.Sprintf(`apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/version: %s
spec:
  chart:
    name: %s
    repository:
      url: %s
    version: %s
    values:
      inline: |
        replicaCount: 2
        image:
          pullPolicy: IfNotPresent
        service:
          type: ClusterIP
          port: 80
        resources:
          limits:
            cpu: 500m
            memory: 512Mi
          requests:
            cpu: 250m
            memory: 256Mi
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://{{.ServiceName}}.{{.Namespace}}.svc.cluster.local:8080/health"
        expectedCodes: [200]
      interval: 30s
      timeout: 10s
      weight: 10
    - name: kubernetes-health
      type: kubernetes
      kubernetes:
        checkPods: true
        checkDeployments: true
        checkServices: true
        requiredReadyRatio: 1.0
      interval: 30s
      timeout: 5s
      weight: 5
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 24h
      - type: failure_count
        failureCount: 10
    requireManualApproval: false
    dataPreservation:
      enabled: false
  observability:
    metrics:
      enabled: true
      interval: 15s
    logging:
      level: info
      structured: true
`, name, namespace, name, version, chart, repo, version)
}

// generateMonitoringTemplate generates a monitoring-focused template
func generateMonitoringTemplate(name, namespace, chart, repo, version string) string {
	if repo == "" {
		repo = "https://prometheus-community.github.io/helm-charts"
	}
	if chart == "" {
		chart = "prometheus"
	}
	if version == "" {
		version = "latest"
	}

	return fmt.Sprintf(`apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/component: monitoring
spec:
  chart:
    name: %s
    repository:
      url: %s
    version: %s
  healthChecks:
    - name: prometheus-http
      type: http
      http:
        url: "http://{{.ServiceName}}.{{.Namespace}}.svc.cluster.local:9090/-/healthy"
        expectedCodes: [200]
      interval: 30s
      timeout: 10s
      weight: 10
    - name: prometheus-query
      type: prometheus
      prometheus:
        query: "up{job='prometheus'}"
        threshold: "1"
        operator: "gte"
        endpoint: "http://{{.ServiceName}}.{{.Namespace}}.svc.cluster.local:9090"
      interval: 60s
      timeout: 30s
      weight: 15
    - name: kubernetes-health
      type: kubernetes
      kubernetes:
        checkPods: true
        checkDeployments: true
        checkServices: true
        checkStatefulSets: true
        requiredReadyRatio: 1.0
      interval: 30s
      timeout: 5s
      weight: 5
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 168h # 1 week
    requireManualApproval: true
    dataPreservation:
      enabled: true
      preserveResources: ["PersistentVolumeClaim"]
  observability:
    metrics:
      enabled: true
      interval: 15s
    tracing:
      enabled: true
      samplingRate: "0.1"
    logging:
      level: info
      structured: true
`, name, namespace, name, chart, repo, version)
}

// generateWebAppTemplate generates a web application template
func generateWebAppTemplate(name, namespace, chart, repo, version string) string {
	if repo == "" {
		repo = "https://charts.helm.sh/stable"
	}
	if version == "" {
		version = "latest"
	}

	return fmt.Sprintf(`apiVersion: roost-keeper.io/v1alpha1
kind: ManagedRoost
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/component: webapp
spec:
  chart:
    name: %s
    repository:
      url: %s
    version: %s
    values:
      inline: |
        replicaCount: 3
        image:
          pullPolicy: IfNotPresent
        service:
          type: LoadBalancer
          port: 80
        ingress:
          enabled: true
          className: nginx
          hosts:
            - host: %s.example.com
              paths:
                - path: /
                  pathType: Prefix
        resources:
          limits:
            cpu: 1000m
            memory: 1Gi
          requests:
            cpu: 500m
            memory: 512Mi
        autoscaling:
          enabled: true
          minReplicas: 3
          maxReplicas: 10
          targetCPUUtilizationPercentage: 80
  healthChecks:
    - name: http-health
      type: http
      http:
        url: "http://{{.ServiceName}}.{{.Namespace}}.svc.cluster.local:8080/health"
        expectedCodes: [200]
      interval: 30s
      timeout: 10s
      weight: 10
    - name: http-readiness
      type: http
      http:
        url: "http://{{.ServiceName}}.{{.Namespace}}.svc.cluster.local:8080/ready"
        expectedCodes: [200]
      interval: 15s
      timeout: 5s
      weight: 5
    - name: kubernetes-health
      type: kubernetes
      kubernetes:
        checkPods: true
        checkDeployments: true
        checkServices: true
        requiredReadyRatio: 0.8 # Allow some pods to be down
      interval: 30s
      timeout: 5s
      weight: 8
  teardownPolicy:
    triggers:
      - type: timeout
        timeout: 72h # 3 days
      - type: failure_count
        failureCount: 20
    requireManualApproval: false
  observability:
    metrics:
      enabled: true
      interval: 15s
      custom:
        - name: request_rate
          query: "rate(http_requests_total[5m])"
          type: gauge
        - name: error_rate
          query: "rate(http_requests_total{status=~'5..'}[5m])"
          type: gauge
    logging:
      level: info
      structured: true
      outputs:
        - type: http
          config:
            url: "https://logs.example.com/api/v1/logs"
`, name, namespace, name, chart, repo, version, name)
}
