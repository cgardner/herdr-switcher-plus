// Package cli holds the command's behavior so it can be tested without a
// terminal. main is a shim over Run.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/state"
	"github.com/cgardner/herdr-switcher-plus/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// The effects Run performs, as package variables so tests can observe them
// without a Herdr server or a terminal.
var (
	collect  = agents.Collect
	loadMode = state.LoadMode
	saveMode = state.SaveMode
	focus    = herdr.Focus

	runProgram = runTea

	// newProgram is the one line that needs a real terminal, kept separate so
	// everything around it stays testable.
	newProgram = func(m ui.Model) program { return tea.NewProgram(m, tea.WithAltScreen()) }
)

// program is the slice of tea.Program that this command uses.
type program interface {
	Run() (tea.Model, error)
}

// runTea drives the Bubble Tea program and recovers the final model.
func runTea(m ui.Model) (ui.Model, error) {
	final, err := newProgram(m).Run()
	if err != nil {
		return ui.Model{}, err
	}
	out, ok := final.(ui.Model)
	if !ok {
		return ui.Model{}, fmt.Errorf("unexpected model %T", final)
	}
	return out, nil
}

// modeEnv carries a starting sort from an action. A plugin pane entrypoint has
// one fixed command, so `herdr plugin pane open --env` is the only way an
// action can ask that command for a different ordering.
const modeEnv = "HERDR_SWITCHER_PLUS_MODE"

// Run executes the command and returns a process exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("herdr-switcher-plus", flag.ContinueOnError)
	fs.SetOutput(stderr)
	plain := fs.Bool("list", false, "print the sorted agents as plain text and exit")
	sortFlag := fs.String("sort", "", "sort mode: "+modeNames()+" (default: the last mode used)")
	statusFlag := fs.String("status", "", "show one state only: blocked, working, idle, done (or the picker keys b/w/i/d)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	mode := resolveMode(*sortFlag)
	status := agents.ParseStatus(*statusFlag)

	rows, err := collect()
	if err != nil {
		fmt.Fprintln(stderr, "herdr-switcher-plus:", err)
		return 1
	}
	agents.Apply(mode, rows)

	if *plain {
		writeList(stdout, status.Keep(rows))
		return 0
	}

	final, err := runProgram(ui.New(rows, mode, status))
	if err != nil {
		fmt.Fprintln(stderr, "herdr-switcher-plus:", err)
		return 1
	}
	saveMode(string(final.Mode))

	// The jump runs after the program exits so focus changes do not race the
	// alt screen teardown.
	if final.Chosen == nil {
		return 0
	}
	if err := focus(final.Chosen.Agent); err != nil {
		fmt.Fprintln(stderr, "herdr-switcher-plus:", err)
		return 1
	}
	return 0
}

// resolveMode applies the precedence: the flag, then the environment, then the
// remembered mode, then recency.
func resolveMode(flagValue string) agents.Mode {
	mode := agents.ParseMode(loadMode())
	if env := os.Getenv(modeEnv); env != "" {
		mode = agents.ParseMode(env)
	}
	if flagValue != "" {
		mode = agents.ParseMode(flagValue)
	}
	return mode
}

func writeList(w io.Writer, rows []agents.Row) {
	now := time.Now()
	for _, r := range rows {
		fmt.Fprintf(w, "%4s  %-10s %-22s %-8s %s\n",
			r.Age(now), r.Agent.PaneID, trunc(r.Space, 22), r.Agent.Status, trunc(r.Last.Text, 60))
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
