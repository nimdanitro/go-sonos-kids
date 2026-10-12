package main

import (
	"context"
	"os"

	"github.com/nimdanitro/go-sonos-kids/cmd"
)

// Set at link time by the container build (see .ko.yaml); a plain `go build`
// keeps these defaults.
var (
	version = "dev"
	commit  = ""
)

func main() {
	cmd.SetBuildInfo(version, commit)

	if err := cmd.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
