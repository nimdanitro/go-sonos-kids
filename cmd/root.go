// Package cmd holds the cobra command tree for sonoskids.
package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:          "sonoskids",
	Short:        "Play Tidal content on Sonos speakers from the command line",
	SilenceUsage: true,
	// Merge flags, environment and the config file into conf before any command runs.
	PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return initConfig(cmd) },
}

func init() {
	registerRootFlags(rootCmd.PersistentFlags())
}

// SetBuildInfo makes `sonoskids --version` report the build. An empty version
// means a plain `go build`.
func SetBuildInfo(version, commit string) {
	if version == "" {
		version = "dev"
	}

	if len(commit) > 7 {
		commit = commit[:7]
	}

	if commit != "" {
		version += " (" + commit + ")"
	}

	rootCmd.Version = version
}

// Execute runs the root command.
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
