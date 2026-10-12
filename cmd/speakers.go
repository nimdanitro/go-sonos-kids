package cmd

import "github.com/spf13/cobra"

var speakersCmd = &cobra.Command{
	Use:   "speakers",
	Short: "List Sonos rooms and groups on the network",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		groups, err := discoverGroups(cmd.Context())
		if err != nil {
			return err
		}

		printGroups(groups)

		return nil
	},
}

func init() { rootCmd.AddCommand(speakersCmd) }
