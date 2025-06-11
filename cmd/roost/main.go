package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v2"

	roostcli "github.com/birbparty/roost-keeper/internal/cli"
)

// Version information - will be set during build
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

func main() {
	// Create CLI application
	app := &cli.App{
		Name:        "roost",
		Usage:       "CLI tool for managing ManagedRoost resources",
		Description: "roost is a command-line tool for managing ManagedRoost resources in Kubernetes. It provides CRUD operations, status monitoring, debugging tools, and template generation.",
		Version:     fmt.Sprintf("%s (%s) built on %s", Version, GitCommit, BuildDate),
		Authors: []*cli.Author{
			{
				Name:  "Roost-Keeper Team",
				Email: "team@roost-keeper.io",
			},
		},
		Copyright: "© 2025 Roost-Keeper Project",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "kubeconfig",
				Usage:    "Path to kubeconfig file",
				EnvVars:  []string{"KUBECONFIG"},
				Value:    "",
				Category: "Kubernetes",
			},
			&cli.StringFlag{
				Name:     "context",
				Usage:    "Kubernetes context to use",
				EnvVars:  []string{"KUBECTL_CONTEXT"},
				Value:    "",
				Category: "Kubernetes",
			},
			&cli.StringFlag{
				Name:     "namespace",
				Aliases:  []string{"n"},
				Usage:    "Kubernetes namespace",
				EnvVars:  []string{"KUBECTL_NAMESPACE"},
				Value:    "",
				Category: "Kubernetes",
			},
			&cli.StringFlag{
				Name:     "output",
				Aliases:  []string{"o"},
				Usage:    "Output format (table, yaml, json, wide)",
				Value:    "table",
				Category: "Output",
			},
			&cli.BoolFlag{
				Name:     "verbose",
				Usage:    "Enable verbose output",
				Category: "Output",
			},
			&cli.BoolFlag{
				Name:     "debug",
				Usage:    "Enable debug logging",
				EnvVars:  []string{"ROOST_DEBUG"},
				Category: "Output",
			},
		},
		Before: func(c *cli.Context) error {
			// Initialize global configuration
			return roostcli.InitializeGlobalConfig(c)
		},
		Action: func(c *cli.Context) error {
			// Show help if no subcommand is provided
			return roostcli.ShowAppHelp(c)
		},
		Commands: []*cli.Command{
			roostcli.GetCommand(),
			roostcli.CreateCommand(),
			roostcli.ApplyCommand(),
			roostcli.DeleteCommand(),
			roostcli.DescribeCommand(),
			roostcli.StatusCommand(),
			roostcli.LogsCommand(),
			roostcli.HealthCommand(),
			roostcli.TemplateCommand(),
			roostcli.CompletionCommand(),
			roostcli.VersionCommand(),
		},
		EnableBashCompletion: true,
		HideHelp:             false,
		HideVersion:          false,
	}

	// Handle kubectl plugin mode
	if len(os.Args) > 0 {
		// Check if running as kubectl plugin (kubectl-roost)
		if roostcli.IsKubectlPlugin() {
			app.Name = "kubectl-roost"
			app.Usage = "kubectl plugin for managing ManagedRoost resources"
		}
	}

	// Run the application
	ctx := context.Background()
	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}
