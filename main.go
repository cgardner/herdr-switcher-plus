// Command herdr-switcher-plus is an agent switcher for Herdr. It lists every
// recognized coding agent, offers several orderings, and jumps to the pane the
// user picks.
//
// The default ordering is the time of each session's last real message, which
// Herdr's own agents sidebar cannot produce on its own.
package main

import (
	"os"

	"github.com/cgardner/herdr-switcher-plus/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
