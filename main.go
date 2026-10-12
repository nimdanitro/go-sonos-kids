package main

import (
	"context"
	"os"

	"github.com/nimdanitro/go-sonos-kids/cmd"
)

func main() {
	if err := cmd.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
