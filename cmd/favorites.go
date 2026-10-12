package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nimdanitro/go-sonos-kids/sonos"
)

var favoritesCmd = &cobra.Command{
	Use:   "favorites",
	Short: "Print raw Sonos favorites (URIs + metadata), useful for calibrating Tidal URIs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()

		grp, err := pickGroup(ctx, bufio.NewReader(cmd.InOrStdin()))
		if err != nil {
			return err
		}

		list, err := grp.Coordinator.Favorites(ctx)
		if err != nil {
			return err
		}

		for _, f := range list {
			fmt.Printf("%s\n  res: %s\n  md:  %s\n\n", f.Title, f.URI, f.Metadata)
		}

		return nil
	},
}

var favoritesRenameCmd = &cobra.Command{
	Use:   "rename <title|id> <new title>",
	Short: "Rename a Sonos favorite (household-wide, visible in the Sonos app)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		grp, err := pickGroup(ctx, bufio.NewReader(cmd.InOrStdin()))
		if err != nil {
			return err
		}

		sp := grp.Coordinator

		list, err := sp.Favorites(ctx)
		if err != nil {
			return err
		}

		var matches []sonos.Favorite

		for _, f := range list {
			if f.ID == args[0] || strings.EqualFold(f.Title, args[0]) {
				matches = append(matches, f)
			}
		}

		switch len(matches) {
		case 0:
			return fmt.Errorf("no favorite with title or ID %q", args[0])
		case 1:
		default:
			for _, f := range matches {
				fmt.Fprintf(os.Stderr, "  %s  %s\n", f.ID, f.Title)
			}

			return fmt.Errorf("%q is ambiguous; pass one of the IDs above", args[0])
		}

		f := matches[0]
		fmt.Printf("Renaming %s: %q -> %q\n", f.ID, f.Title, args[1])

		if conf.GetBool("dry_run") {
			fmt.Println("(dry run, nothing changed)")
			return nil
		}

		if err := sp.RenameFavorite(ctx, f.ID, f.Title, args[1]); err != nil {
			return err
		}

		fmt.Println("Renamed.")

		return nil
	},
}

func init() {
	favoritesRenameCmd.Flags().Bool("dry-run", false, "show what would change without changing it")
	favoritesCmd.AddCommand(favoritesRenameCmd)
	registerSpeakerFlag(favoritesCmd.PersistentFlags())
	rootCmd.AddCommand(favoritesCmd)
}
