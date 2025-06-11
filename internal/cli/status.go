package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// StatusCommand returns the status command
func StatusCommand() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show detailed status of ManagedRoost resources",
		ArgsUsage: "[NAME]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "watch",
				Aliases: []string{"w"},
				Usage:   "Watch for status changes",
			},
			&cli.DurationFlag{
				Name:  "refresh",
				Usage: "Refresh interval for watch mode",
				Value: 5 * time.Second,
			},
		},
		Action: func(c *cli.Context) error {
			return runStatus(c)
		},
	}
}

// runStatus executes the status command
func runStatus(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	if c.NArg() == 0 {
		// Show status for all roosts
		return showAllRoostStatus(ctx, c)
	}

	// Show status for specific roost
	name := c.Args().Get(0)
	namespace := config.GetCurrentNamespace()

	if c.Bool("watch") {
		return watchRoostStatus(ctx, namespace, name, c.Duration("refresh"))
	}

	return showRoostStatus(ctx, namespace, name)
}

// showAllRoostStatus shows status for all roosts
func showAllRoostStatus(ctx context.Context, c *cli.Context) error {
	config := GetGlobalConfig()
	namespace := config.GetCurrentNamespace()

	var roostList roostv1alpha1.ManagedRoostList
	listOptions := &client.ListOptions{}

	if namespace != "" {
		listOptions.Namespace = namespace
	}

	if err := config.K8sClient.List(ctx, &roostList, listOptions); err != nil {
		return fmt.Errorf("failed to list ManagedRoosts: %w", err)
	}

	if len(roostList.Items) == 0 {
		fmt.Println("No ManagedRoost resources found.")
		return nil
	}

	fmt.Printf("%-20s %-15s %-10s %-15s %-10s %-15s\n",
		"NAME", "NAMESPACE", "PHASE", "HEALTH", "CHECKS", "LAST UPDATE")
	fmt.Println(strings.Repeat("-", 95))

	for _, roost := range roostList.Items {
		healthyChecks := 0
		totalChecks := len(roost.Status.HealthChecks)
		for _, check := range roost.Status.HealthChecks {
			if check.Status == "healthy" {
				healthyChecks++
			}
		}

		lastUpdate := "Never"
		if roost.Status.LastUpdateTime != nil {
			lastUpdate = time.Since(roost.Status.LastUpdateTime.Time).Truncate(time.Second).String()
		}

		health := roost.Status.Health
		if health == "" {
			health = "Unknown"
		}

		checksStatus := fmt.Sprintf("%d/%d", healthyChecks, totalChecks)

		fmt.Printf("%-20s %-15s %-10s %-15s %-10s %-15s\n",
			roost.Name,
			roost.Namespace,
			roost.Status.Phase,
			health,
			checksStatus,
			lastUpdate)
	}

	return nil
}

// showRoostStatus shows status for a specific roost
func showRoostStatus(ctx context.Context, namespace, name string) error {
	config := GetGlobalConfig()

	var roost roostv1alpha1.ManagedRoost
	if err := config.K8sClient.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, &roost); err != nil {
		return fmt.Errorf("failed to get ManagedRoost: %w", err)
	}

	return printDetailedStatus(&roost)
}

// watchRoostStatus watches status changes for a specific roost
func watchRoostStatus(ctx context.Context, namespace, name string, refresh time.Duration) error {
	config := GetGlobalConfig()

	fmt.Printf("Watching status for ManagedRoost %s/%s (refresh: %v)\n", namespace, name, refresh)
	fmt.Println("Press Ctrl+C to stop watching...")
	fmt.Println()

	ticker := time.NewTicker(refresh)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Clear screen and move cursor to top
			fmt.Print("\033[2J\033[H")

			var roost roostv1alpha1.ManagedRoost
			if err := config.K8sClient.Get(ctx, client.ObjectKey{
				Namespace: namespace,
				Name:      name,
			}, &roost); err != nil {
				fmt.Printf("Error: %v\n", err)
				continue
			}

			fmt.Printf("ManagedRoost Status - %s\n", time.Now().Format("15:04:05"))
			fmt.Println(strings.Repeat("=", 50))
			if err := printDetailedStatus(&roost); err != nil {
				fmt.Printf("Error printing status: %v\n", err)
			}
		}
	}
}

// printDetailedStatus prints detailed status information
func printDetailedStatus(roost *roostv1alpha1.ManagedRoost) error {
	fmt.Printf("Name:         %s\n", roost.Name)
	fmt.Printf("Namespace:    %s\n", roost.Namespace)
	fmt.Printf("Phase:        %s\n", roost.Status.Phase)
	fmt.Printf("Health:       %s\n", getHealthWithIcon(roost.Status.Health))
	fmt.Printf("Created:      %s (%s ago)\n",
		roost.CreationTimestamp.Format(time.RFC3339),
		time.Since(roost.CreationTimestamp.Time).Truncate(time.Second))

	if roost.Status.LastUpdateTime != nil {
		fmt.Printf("Last Update:  %s (%s ago)\n",
			roost.Status.LastUpdateTime.Format(time.RFC3339),
			time.Since(roost.Status.LastUpdateTime.Time).Truncate(time.Second))
	}

	// Chart information
	fmt.Println("\nChart Information:")
	fmt.Printf("  Name:       %s\n", roost.Spec.Chart.Name)
	fmt.Printf("  Version:    %s\n", roost.Spec.Chart.Version)
	fmt.Printf("  Repository: %s\n", roost.Spec.Chart.Repository.URL)

	// Helm release status
	if roost.Status.HelmRelease != nil {
		fmt.Println("\nHelm Release:")
		fmt.Printf("  Name:     %s\n", roost.Status.HelmRelease.Name)
		fmt.Printf("  Revision: %d\n", roost.Status.HelmRelease.Revision)
		fmt.Printf("  Status:   %s\n", roost.Status.HelmRelease.Status)
		if roost.Status.HelmRelease.LastDeployed != nil {
			fmt.Printf("  Deployed: %s\n", roost.Status.HelmRelease.LastDeployed.Format(time.RFC3339))
		}
	}

	// Health checks
	if len(roost.Status.HealthChecks) > 0 {
		fmt.Println("\nHealth Checks:")
		healthyCount := 0
		for _, check := range roost.Status.HealthChecks {
			status := "❌ UNHEALTHY"
			if check.Status == "healthy" {
				status = "✅ HEALTHY"
				healthyCount++
			} else if check.Status == "unknown" {
				status = "❓ UNKNOWN"
			}

			fmt.Printf("  %-30s %s\n", check.Name, status)
			if check.Message != "" {
				fmt.Printf("    Message: %s\n", check.Message)
			}
			if check.LastCheck != nil {
				fmt.Printf("    Last Check: %s\n",
					time.Since(check.LastCheck.Time).Truncate(time.Second))
			}
			if check.FailureCount > 0 {
				fmt.Printf("    Failures: %d\n", check.FailureCount)
			}
		}
		fmt.Printf("\nHealth Summary: %d/%d checks healthy\n", healthyCount, len(roost.Status.HealthChecks))
	}

	// Conditions
	if len(roost.Status.Conditions) > 0 {
		fmt.Println("\nConditions:")
		for _, condition := range roost.Status.Conditions {
			status := condition.Status
			if status == "True" {
				status = "✅ " + status
			} else if status == "False" {
				status = "❌ " + status
			} else {
				status = "❓ " + status
			}

			fmt.Printf("  %-20s %s\n", condition.Type, status)
			if condition.Message != "" {
				fmt.Printf("    Message: %s\n", condition.Message)
			}
			if condition.Reason != "" {
				fmt.Printf("    Reason:  %s\n", condition.Reason)
			}
		}
	}

	// Teardown status
	if roost.Status.Teardown != nil && roost.Status.Teardown.Triggered {
		fmt.Println("\nTeardown Status:")
		fmt.Printf("  Triggered: %s\n", roost.Status.Teardown.TriggerTime.Format(time.RFC3339))
		fmt.Printf("  Reason:    %s\n", roost.Status.Teardown.TriggerReason)
		fmt.Printf("  Progress:  %d%%\n", roost.Status.Teardown.Progress)
		if roost.Status.Teardown.CompletionTime != nil {
			fmt.Printf("  Completed: %s\n", roost.Status.Teardown.CompletionTime.Format(time.RFC3339))
		}
	}

	return nil
}

// getHealthWithIcon returns health status with appropriate icon
func getHealthWithIcon(health string) string {
	switch health {
	case "healthy":
		return "✅ Healthy"
	case "unhealthy":
		return "❌ Unhealthy"
	case "degraded":
		return "⚠️  Degraded"
	default:
		return "❓ Unknown"
	}
}
