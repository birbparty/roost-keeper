package cli

import (
	"fmt"
	"runtime"

	"github.com/urfave/cli/v2"
)

// Version information - will be set during build
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
	Compiler  = runtime.Compiler
	Platform  = fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
)

// VersionCommand returns the version command
func VersionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Show version information",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "short",
				Usage: "Show only version number",
			},
			&cli.StringFlag{
				Name:  "output",
				Usage: "Output format (text, json, yaml)",
				Value: "text",
			},
		},
		Action: func(c *cli.Context) error {
			return runVersion(c)
		},
	}
}

// runVersion executes the version command
func runVersion(c *cli.Context) error {
	short := c.Bool("short")
	output := c.String("output")

	if short {
		fmt.Println(Version)
		return nil
	}

	switch output {
	case "json":
		return printVersionJSON()
	case "yaml":
		return printVersionYAML()
	default:
		return printVersionText()
	}
}

// printVersionText prints version information in text format
func printVersionText() error {
	fmt.Printf("roost version %s\n", Version)
	fmt.Printf("Git commit: %s\n", GitCommit)
	fmt.Printf("Built: %s\n", BuildDate)
	fmt.Printf("Go version: %s\n", GoVersion)
	fmt.Printf("Compiler: %s\n", Compiler)
	fmt.Printf("Platform: %s\n", Platform)
	return nil
}

// printVersionJSON prints version information in JSON format
func printVersionJSON() error {
	fmt.Printf(`{
  "version": "%s",
  "gitCommit": "%s",
  "buildDate": "%s",
  "goVersion": "%s",
  "compiler": "%s",
  "platform": "%s"
}
`, Version, GitCommit, BuildDate, GoVersion, Compiler, Platform)
	return nil
}

// printVersionYAML prints version information in YAML format
func printVersionYAML() error {
	fmt.Printf(`version: %s
gitCommit: %s
buildDate: %s
goVersion: %s
compiler: %s
platform: %s
`, Version, GitCommit, BuildDate, GoVersion, Compiler, Platform)
	return nil
}
