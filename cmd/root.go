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

// Execute runs the root command.
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}
