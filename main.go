// Command herdr-pane-sort is an agent switcher for Herdr. It lists every
// recognized coding agent, offers several orderings, and jumps to the pane the
// user picks.
//
// The default ordering is the time of each session's last real message, which
// Herdr's own agents sidebar cannot produce on its own.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cgardner/herdr-pane-sort/internal/agents"
	"github.com/cgardner/herdr-pane-sort/internal/herdr"
	"github.com/cgardner/herdr-pane-sort/internal/state"
	"github.com/cgardner/herdr-pane-sort/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	plain := flag.Bool("list", false, "print the sorted agents as plain text and exit")
	sortFlag := flag.String("sort", "", "sort mode: "+modeNames()+" (default: the last mode used)")
	statusFlag := flag.String("status", "", "show one state only: blocked, working, idle, done (or the picker keys b/w/i/d)")
	flag.Parse()

	// Precedence: the flag, then the environment, then the remembered mode,
	// then recency. The environment carries the mode because a plugin pane has
	// one fixed command, and `herdr plugin pane open --env` is how an action
	// asks that command for a different starting sort.
	mode := agents.ParseMode(state.LoadMode())
	if env := os.Getenv("HERDR_PANE_SORT_MODE"); env != "" {
		mode = agents.ParseMode(env)
	}
	if *sortFlag != "" {
		mode = agents.ParseMode(*sortFlag)
	}

	status := agents.ParseStatus(*statusFlag)

	rows, err := agents.Collect()
	if err != nil {
		fmt.Fprintln(os.Stderr, "herdr-pane-sort:", err)
		os.Exit(1)
	}
	agents.Apply(mode, rows)

	if *plain {
		now := time.Now()
		for _, r := range status.Keep(rows) {
			fmt.Printf("%4s  %-10s %-22s %-8s %s\n",
				r.Age(now), r.Agent.PaneID, trunc(r.Space, 22), r.Agent.Status, trunc(r.Last.Text, 60))
		}
		return
	}

	final, err := tea.NewProgram(ui.New(rows, mode, status), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "herdr-pane-sort:", err)
		os.Exit(1)
	}

	m, ok := final.(ui.Model)
	if !ok {
		return
	}
	state.SaveMode(string(m.Mode))

	// The jump runs after the program exits so focus changes do not race the
	// alt screen teardown.
	if m.Chosen == nil {
		return
	}
	if err := herdr.Focus(m.Chosen.Agent); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-pane-sort:", err)
		os.Exit(1)
	}
}

func modeNames() string {
	names := make([]string, len(agents.Modes))
	for i, m := range agents.Modes {
		names[i] = string(m)
	}
	return strings.Join(names, ", ")
}

func trunc(s string, w int) string {
	if r := []rune(s); len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s
}
