package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// GetCommand returns the get command
func GetCommand() *cli.Command {
	return &cli.Command{
		Name:    "get",
		Usage:   "Get ManagedRoost resources",
		Aliases: []string{"g"},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "selector",
				Aliases: []string{"l"},
				Usage:   "Label selector",
			},
			&cli.BoolFlag{
				Name:  "all-namespaces",
				Usage: "List resources from all namespaces",
			},
			&cli.BoolFlag{
				Name:    "watch",
				Aliases: []string{"w"},
				Usage:   "Watch for changes",
			},
		},
		Action: func(c *cli.Context) error {
			return runGet(c)
		},
	}
}

// CreateCommand returns the create command
func CreateCommand() *cli.Command {
	return &cli.Command{
		Name:    "create",
		Usage:   "Create ManagedRoost resources",
		Aliases: []string{"c"},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "filename",
				Aliases: []string{"f"},
				Usage:   "YAML file to create from",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Validate without creating",
			},
		},
		Action: func(c *cli.Context) error {
			return runCreate(c)
		},
	}
}

// ApplyCommand returns the apply command
func ApplyCommand() *cli.Command {
	return &cli.Command{
		Name:    "apply",
		Usage:   "Apply ManagedRoost configuration",
		Aliases: []string{"a"},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "filename",
				Aliases:  []string{"f"},
				Usage:    "YAML file to apply",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Validate without applying",
			},
		},
		Action: func(c *cli.Context) error {
			return runApply(c)
		},
	}
}

// DeleteCommand returns the delete command
func DeleteCommand() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Delete ManagedRoost resources",
		Aliases:   []string{"d"},
		ArgsUsage: "[NAME]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "filename",
				Aliases: []string{"f"},
				Usage:   "YAML file to delete from",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "Force deletion without confirmation",
			},
		},
		Action: func(c *cli.Context) error {
			return runDelete(c)
		},
	}
}

// DescribeCommand returns the describe command
func DescribeCommand() *cli.Command {
	return &cli.Command{
		Name:      "describe",
		Usage:     "Describe ManagedRoost resources",
		ArgsUsage: "NAME",
		Action: func(c *cli.Context) error {
			return runDescribe(c)
		},
	}
}

// runGet executes the get command
func runGet(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	namespace := config.GetCurrentNamespace()
	if c.Bool("all-namespaces") {
		namespace = ""
	}

	var roostList roostv1alpha1.ManagedRoostList
	listOptions := &client.ListOptions{}

	if namespace != "" {
		listOptions.Namespace = namespace
	}

	if selector := c.String("selector"); selector != "" {
		labelSelector, err := labels.Parse(selector)
		if err != nil {
			return fmt.Errorf("invalid label selector: %w", err)
		}
		listOptions.LabelSelector = labelSelector
	}

	if err := config.K8sClient.List(ctx, &roostList, listOptions); err != nil {
		return fmt.Errorf("failed to list ManagedRoosts: %w", err)
	}

	return printRoostList(&roostList, config.OutputFormat)
}

// runCreate executes the create command
func runCreate(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	filename := c.String("filename")
	if filename == "" {
		return fmt.Errorf("filename is required")
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	var roost roostv1alpha1.ManagedRoost
	if err := yaml.Unmarshal(data, &roost); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	if c.Bool("dry-run") {
		fmt.Println("ManagedRoost configuration is valid")
		return nil
	}

	if err := config.K8sClient.Create(ctx, &roost); err != nil {
		return fmt.Errorf("failed to create ManagedRoost: %w", err)
	}

	fmt.Printf("ManagedRoost %s/%s created successfully\n", roost.Namespace, roost.Name)
	return nil
}

// runApply executes the apply command
func runApply(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	filename := c.String("filename")
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	var roost roostv1alpha1.ManagedRoost
	if err := yaml.Unmarshal(data, &roost); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	if c.Bool("dry-run") {
		fmt.Println("ManagedRoost configuration is valid")
		return nil
	}

	// Try to get existing resource
	existing := &roostv1alpha1.ManagedRoost{}
	err = config.K8sClient.Get(ctx, client.ObjectKey{
		Namespace: roost.Namespace,
		Name:      roost.Name,
	}, existing)

	if err != nil {
		// Resource doesn't exist, create it
		if err := config.K8sClient.Create(ctx, &roost); err != nil {
			return fmt.Errorf("failed to create ManagedRoost: %w", err)
		}
		fmt.Printf("ManagedRoost %s/%s created successfully\n", roost.Namespace, roost.Name)
	} else {
		// Resource exists, update it
		roost.ResourceVersion = existing.ResourceVersion
		if err := config.K8sClient.Update(ctx, &roost); err != nil {
			return fmt.Errorf("failed to update ManagedRoost: %w", err)
		}
		fmt.Printf("ManagedRoost %s/%s updated successfully\n", roost.Namespace, roost.Name)
	}

	return nil
}

// runDelete executes the delete command
func runDelete(c *cli.Context) error {
	config := GetGlobalConfig()
	ctx := context.Background()

	if filename := c.String("filename"); filename != "" {
		// Delete from file
		data, err := os.ReadFile(filename)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filename, err)
		}

		var roost roostv1alpha1.ManagedRoost
		if err := yaml.Unmarshal(data, &roost); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}

		if err := config.K8sClient.Delete(ctx, &roost); err != nil {
			return fmt.Errorf("failed to delete ManagedRoost: %w", err)
		}

		fmt.Printf("ManagedRoost %s/%s deleted successfully\n", roost.Namespace, roost.Name)
		return nil
	}

	// Delete by name
	if c.NArg() == 0 {
		return fmt.Errorf("resource name is required")
	}

	name := c.Args().Get(0)
	namespace := config.GetCurrentNamespace()

	roost := &roostv1alpha1.ManagedRoost{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if !c.Bool("force") {
		fmt.Printf("Are you sure you want to delete ManagedRoost %s/%s? [y/N]: ", namespace, name)
		var response string
		fmt.Scanln(&response)
		if !strings.EqualFold(response, "y") && !strings.EqualFold(response, "yes") {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	if err := config.K8sClient.Delete(ctx, roost); err != nil {
		return fmt.Errorf("failed to delete ManagedRoost: %w", err)
	}

	fmt.Printf("ManagedRoost %s/%s deleted successfully\n", namespace, name)
	return nil
}

// runDescribe executes the describe command
func runDescribe(c *cli.Context) error {
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

	return printRoostDetails(&roost)
}

// printRoostList prints a list of ManagedRoosts
func printRoostList(roostList *roostv1alpha1.ManagedRoostList, format string) error {
	switch format {
	case "yaml":
		data, err := yaml.Marshal(roostList)
		if err != nil {
			return err
		}
		fmt.Print(string(data))
	case "json":
		data, err := json.MarshalIndent(roostList, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	case "table", "wide":
		printRoostTable(roostList.Items, format == "wide")
	default:
		return fmt.Errorf("unsupported output format: %s", format)
	}
	return nil
}

// printRoostTable prints ManagedRoosts in table format
func printRoostTable(roosts []roostv1alpha1.ManagedRoost, wide bool) {
	if len(roosts) == 0 {
		fmt.Println("No resources found.")
		return
	}

	if wide {
		fmt.Printf("%-20s %-15s %-10s %-15s %-10s %-20s %-10s\n",
			"NAME", "NAMESPACE", "PHASE", "HEALTH", "CHART", "VERSION", "AGE")
	} else {
		fmt.Printf("%-20s %-15s %-10s %-15s %-10s\n",
			"NAME", "NAMESPACE", "PHASE", "HEALTH", "AGE")
	}

	for _, roost := range roosts {
		age := time.Since(roost.CreationTimestamp.Time).Truncate(time.Second)
		health := roost.Status.Health
		if health == "" {
			health = "Unknown"
		}

		if wide {
			fmt.Printf("%-20s %-15s %-10s %-15s %-10s %-20s %-10s\n",
				roost.Name,
				roost.Namespace,
				roost.Status.Phase,
				health,
				roost.Spec.Chart.Name,
				roost.Spec.Chart.Version,
				age)
		} else {
			fmt.Printf("%-20s %-15s %-10s %-15s %-10s\n",
				roost.Name,
				roost.Namespace,
				roost.Status.Phase,
				health,
				age)
		}
	}
}

// printRoostDetails prints detailed information about a ManagedRoost
func printRoostDetails(roost *roostv1alpha1.ManagedRoost) error {
	fmt.Printf("Name:         %s\n", roost.Name)
	fmt.Printf("Namespace:    %s\n", roost.Namespace)
	fmt.Printf("Phase:        %s\n", roost.Status.Phase)
	fmt.Printf("Health:       %s\n", roost.Status.Health)
	fmt.Printf("Created:      %s\n", roost.CreationTimestamp.Format(time.RFC3339))

	if roost.Status.LastUpdateTime != nil {
		fmt.Printf("Last Update:  %s\n", roost.Status.LastUpdateTime.Format(time.RFC3339))
	}

	fmt.Println("\nChart Configuration:")
	fmt.Printf("  Name:       %s\n", roost.Spec.Chart.Name)
	fmt.Printf("  Version:    %s\n", roost.Spec.Chart.Version)
	fmt.Printf("  Repository: %s\n", roost.Spec.Chart.Repository.URL)

	if len(roost.Status.Conditions) > 0 {
		fmt.Println("\nConditions:")
		for _, condition := range roost.Status.Conditions {
			fmt.Printf("  %s: %s\n", condition.Type, condition.Status)
			if condition.Message != "" {
				fmt.Printf("    Message: %s\n", condition.Message)
			}
		}
	}

	if len(roost.Status.HealthChecks) > 0 {
		fmt.Println("\nHealth Checks:")
		for _, check := range roost.Status.HealthChecks {
			status := "❌"
			if check.Status == "healthy" {
				status = "✅"
			}
			fmt.Printf("  %s %s\n", status, check.Name)
			if check.Message != "" {
				fmt.Printf("    Message: %s\n", check.Message)
			}
		}
	}

	return nil
}
