package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nimdanitro/go-sonos-kids/sonos"
)

func discoverGroups(ctx context.Context) ([]sonos.Group, error) {
	return sonos.DiscoverGroups(ctx, sonos.DiscoverOptions{
		Timeout: conf.GetDuration("discover_timeout"),
		NoCache: conf.GetBool("no_cache"),
		Logf:    func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) },
	})
}

func printGroups(groups []sonos.Group) {
	for i, g := range groups {
		fmt.Printf("  [%d] %-28s %-22s %s\n", i+1, g.Name, g.Coordinator.Model, g.Coordinator.IP)
	}
}

// pickGroup resolves the group named by the "speaker" setting (name, room or
// IP), or asks interactively when none is set.
func pickGroup(ctx context.Context, in *bufio.Reader) (sonos.Group, error) {
	want := conf.GetString("speaker")

	groups, err := discoverGroups(ctx)
	if err != nil {
		return sonos.Group{}, err
	}

	if want != "" {
		for _, g := range groups {
			if g.Matches(want) {
				return g, nil
			}
		}

		return sonos.Group{}, fmt.Errorf("no group or room matching %q", want)
	}

	printGroups(groups)

	idx, err := strconv.Atoi(prompt(in, "Select number: "))
	if err != nil || idx < 1 || idx > len(groups) {
		return sonos.Group{}, fmt.Errorf("invalid selection")
	}

	return groups[idx-1], nil
}

func prompt(in *bufio.Reader, msg string) string {
	fmt.Print(msg)

	line, _ := in.ReadString('\n')

	return strings.TrimSpace(line)
}
