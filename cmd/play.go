package cmd

import (
	"bufio"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nimdanitro/go-sonos-kids/sonos"
)

var playCmd = &cobra.Command{
	Use:   "play [url]",
	Short: "Play a Tidal or Spotify album, playlist or track on a speaker (prompts for anything not given)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		in := bufio.NewReader(cmd.InOrStdin())

		grp, err := pickGroup(ctx, in)
		if err != nil {
			return err
		}

		sp := grp.Coordinator

		var input string
		if len(args) == 1 {
			input = args[0]
		} else {
			input = prompt(in, "Tidal/Spotify URL (album, playlist or track): ")
		}

		ref, err := sonos.ParseRef(input)
		if err != nil {
			return err
		}

		acc, err := sp.Account(ctx, ref.Service)
		if err != nil {
			return err
		}

		if sn := conf.GetString("sn"); sn != "" {
			acc.SN = sn
		}

		fmt.Printf("Detected %s account: sn=%s\n", acc.Name, acc.SN)

		fmt.Printf("Playing %s on %s (via %s)...\n", ref, grp.Name, sp.IP)

		if err := sp.Play(ctx, acc, ref); err != nil {
			return err
		}

		fmt.Println("Playing.")

		return nil
	},
}

func init() {
	registerSpeakerFlag(playCmd.Flags())
	playCmd.Flags().String("sn", "", "account serial number (auto-detected from favorites if empty)")
	rootCmd.AddCommand(playCmd)
}
