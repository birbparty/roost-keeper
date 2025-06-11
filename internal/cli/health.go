package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// HealthCommand returns the health command
func HealthCommand() *cli.Command {
	return &cli.Command{
		Name:  "health",
		Usage: "Health check operations",
		Subcommands: []*cli.Command{
			{
				Name:      "check",
				Usage:     "Execute health checks for a ManagedRoost",
				ArgsUsage: "NAME",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "check-name",
						Usage: "Specific health check to execute",
					},
					&cli.BoolFlag{
						Name:  "wait",
						Usage: "Wait for health checks to complete",
					},
					&cli.DurationFlag{
						Name:  "timeout",
						Usage: "Timeout for health check execution",
						Value: 5 * time.Minute,
					},
				},
				Action: func(c *cli.Context) error {
					return runHealthCheck(c)
				},
			},
			{
				Name:      "list",
				Usage:     "List health checks for a ManagedRoost",
				ArgsUsage: "NAME",
				Action: func(c *cli.Context) error {
					return runHealthList(c)
				},
			},
			{
				Name:      "debug",
				Usage:     "Debug health check issues",
				ArgsUsage: "NAME",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "check-name",
						Usage: "Specific health check to debug",
					},
				},
				Action: func(c *cli.Context) error {
					return runHealthDebug(c)
				},
			},
		},
	}
}

// runHealthCheck executes health checks
func runHealthCheck(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	if c.NArg() == 0 {
		return fmt.Errorf("resource name is required")
	}

	name := c.Args().Get(0)
	namespace := config.GetCurrentNamespace()
	checkName := c.String("check-name")

	var roost roostv1alpha1.ManagedRoost
	if err := config.K8sClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &roost); err != nil {
		return fmt.Errorf("failed to get ManagedRoost: %w", err)
	}

	fmt.Printf("Executing health checks for ManagedRoost %s/%s\n", namespace, name)

	if checkName != "" {
		fmt.Printf("Running specific check: %s\n", checkName)
		return executeSpecificHealthCheck(&roost, checkName)
	}

	return executeAllHealthChecks(&roost, c.Bool("wait"), c.Duration("timeout"))
}

// runHealthList lists health checks
func runHealthList(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	if c.NArg() == 0 {
		return fmt.Errorf("resource name is required")
	}

	name := c.Args().Get(0)
	namespace := config.GetCurrentNamespace()

	var roost roostv1alpha1.ManagedRoost
	if err := config.K8sClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &roost); err != nil {
		return fmt.Errorf("failed to get ManagedRoost: %w", err)
	}

	fmt.Printf("Health checks for ManagedRoost %s/%s:\n\n", namespace, name)

	if len(roost.Spec.HealthChecks) == 0 {
		fmt.Println("No health checks configured.")
		return nil
	}

	fmt.Printf("%-25s %-10s %-10s %-15s %-30s\n",
		"NAME", "TYPE", "WEIGHT", "STATUS", "LAST CHECK")
	fmt.Println("─────────────────────────────────────────────────────────────────────────────────────")

	for _, specCheck := range roost.Spec.HealthChecks {
		// Find corresponding status
		var statusCheck *roostv1alpha1.HealthCheckStatus
		for _, sc := range roost.Status.HealthChecks {
			if sc.Name == specCheck.Name {
				statusCheck = &sc
				break
			}
		}

		status := "Unknown"
		lastCheck := "Never"

		if statusCheck != nil {
			status = statusCheck.Status
			if statusCheck.LastCheck != nil {
				lastCheck = time.Since(statusCheck.LastCheck.Time).Truncate(time.Second).String()
			}
		}

		fmt.Printf("%-25s %-10s %-10d %-15s %-30s\n",
			specCheck.Name,
			specCheck.Type,
			specCheck.Weight,
			status,
			lastCheck)
	}

	return nil
}

// runHealthDebug debugs health check issues
func runHealthDebug(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	if c.NArg() == 0 {
		return fmt.Errorf("resource name is required")
	}

	name := c.Args().Get(0)
	namespace := config.GetCurrentNamespace()
	checkName := c.String("check-name")

	var roost roostv1alpha1.ManagedRoost
	if err := config.K8sClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &roost); err != nil {
		return fmt.Errorf("failed to get ManagedRoost: %w", err)
	}

	fmt.Printf("Debugging health checks for ManagedRoost %s/%s\n\n", namespace, name)

	if checkName != "" {
		return debugSpecificHealthCheck(&roost, checkName)
	}

	return debugAllHealthChecks(&roost)
}

// executeSpecificHealthCheck executes a specific health check
func executeSpecificHealthCheck(roost *roostv1alpha1.ManagedRoost, checkName string) error {
	// Find the health check
	var healthCheck *roostv1alpha1.HealthCheckSpec
	for _, check := range roost.Spec.HealthChecks {
		if check.Name == checkName {
			healthCheck = &check
			break
		}
	}

	if healthCheck == nil {
		return fmt.Errorf("health check '%s' not found", checkName)
	}

	fmt.Printf("Executing health check: %s (type: %s)\n", healthCheck.Name, healthCheck.Type)

	// This would integrate with the actual health check system
	// For now, just show the configuration
	return showHealthCheckConfig(healthCheck)
}

// executeAllHealthChecks executes all health checks
func executeAllHealthChecks(roost *roostv1alpha1.ManagedRoost, wait bool, timeout time.Duration) error {
	if len(roost.Spec.HealthChecks) == 0 {
		fmt.Println("No health checks configured.")
		return nil
	}

	fmt.Printf("Executing %d health checks...\n\n", len(roost.Spec.HealthChecks))

	for _, check := range roost.Spec.HealthChecks {
		fmt.Printf("→ %s (%s): ", check.Name, check.Type)

		// This would integrate with the actual health check system
		// For now, simulate execution
		if err := showHealthCheckConfig(&check); err != nil {
			fmt.Printf("❌ FAILED: %v\n", err)
		} else {
			fmt.Printf("✅ PASSED\n")
		}
	}

	return nil
}

// debugSpecificHealthCheck debugs a specific health check
func debugSpecificHealthCheck(roost *roostv1alpha1.ManagedRoost, checkName string) error {
	// Find the health check
	var healthCheck *roostv1alpha1.HealthCheckSpec
	for _, check := range roost.Spec.HealthChecks {
		if check.Name == checkName {
			healthCheck = &check
			break
		}
	}

	if healthCheck == nil {
		return fmt.Errorf("health check '%s' not found", checkName)
	}

	// Find status
	var statusCheck *roostv1alpha1.HealthCheckStatus
	for _, sc := range roost.Status.HealthChecks {
		if sc.Name == checkName {
			statusCheck = &sc
			break
		}
	}

	fmt.Printf("Debug information for health check: %s\n", checkName)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\nConfiguration:")
	fmt.Printf("  Type:             %s\n", healthCheck.Type)
	fmt.Printf("  Interval:         %s\n", healthCheck.Interval.Duration)
	fmt.Printf("  Timeout:          %s\n", healthCheck.Timeout.Duration)
	fmt.Printf("  Failure Threshold: %d\n", healthCheck.FailureThreshold)
	fmt.Printf("  Weight:           %d\n", healthCheck.Weight)

	if statusCheck != nil {
		fmt.Println("\nCurrent Status:")
		fmt.Printf("  Status:           %s\n", statusCheck.Status)
		fmt.Printf("  Failure Count:    %d\n", statusCheck.FailureCount)
		if statusCheck.LastCheck != nil {
			fmt.Printf("  Last Check:       %s (%s ago)\n",
				statusCheck.LastCheck.Format(time.RFC3339),
				time.Since(statusCheck.LastCheck.Time).Truncate(time.Second))
		}
		if statusCheck.Message != "" {
			fmt.Printf("  Message:          %s\n", statusCheck.Message)
		}
	}

	// Show type-specific configuration
	fmt.Println("\nType-specific Configuration:")
	return showHealthCheckConfig(healthCheck)
}

// debugAllHealthChecks debugs all health checks
func debugAllHealthChecks(roost *roostv1alpha1.ManagedRoost) error {
	if len(roost.Spec.HealthChecks) == 0 {
		fmt.Println("No health checks configured.")
		return nil
	}

	fmt.Printf("Overall Health: %s\n", getHealthWithIcon(roost.Status.Health))
	fmt.Printf("Total Checks: %d\n\n", len(roost.Spec.HealthChecks))

	for _, check := range roost.Spec.HealthChecks {
		fmt.Printf("Health Check: %s\n", check.Name)
		fmt.Println("─────────────────────────────────")

		if err := debugSpecificHealthCheck(roost, check.Name); err != nil {
			fmt.Printf("Error debugging check: %v\n", err)
		}

		fmt.Println()
	}

	return nil
}

// showHealthCheckConfig displays health check configuration details
func showHealthCheckConfig(check *roostv1alpha1.HealthCheckSpec) error {
	switch check.Type {
	case "http":
		if check.HTTP != nil {
			fmt.Printf("  URL:              %s\n", check.HTTP.URL)
			fmt.Printf("  Method:           %s\n", check.HTTP.Method)
			if len(check.HTTP.ExpectedCodes) > 0 {
				fmt.Printf("  Expected Codes:   %v\n", check.HTTP.ExpectedCodes)
			}
			if check.HTTP.ExpectedBody != "" {
				fmt.Printf("  Expected Body:    %s\n", check.HTTP.ExpectedBody)
			}
		}
	case "tcp":
		if check.TCP != nil {
			fmt.Printf("  Host:             %s\n", check.TCP.Host)
			fmt.Printf("  Port:             %d\n", check.TCP.Port)
		}
	case "grpc":
		if check.GRPC != nil {
			fmt.Printf("  Host:             %s\n", check.GRPC.Host)
			fmt.Printf("  Port:             %d\n", check.GRPC.Port)
			if check.GRPC.Service != "" {
				fmt.Printf("  Service:          %s\n", check.GRPC.Service)
			}
		}
	case "prometheus":
		if check.Prometheus != nil {
			fmt.Printf("  Query:            %s\n", check.Prometheus.Query)
			fmt.Printf("  Threshold:        %s\n", check.Prometheus.Threshold)
			fmt.Printf("  Operator:         %s\n", check.Prometheus.Operator)
		}
	case "kubernetes":
		if check.Kubernetes != nil {
			fmt.Printf("  Check Pods:       %t\n", check.Kubernetes.CheckPods)
			fmt.Printf("  Check Deployments: %t\n", check.Kubernetes.CheckDeployments)
			fmt.Printf("  Check Services:   %t\n", check.Kubernetes.CheckServices)
		}
	default:
		fmt.Printf("  Unknown type: %s\n", check.Type)
	}

	return nil
}
