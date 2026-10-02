// Package cli holds the command's behavior so it can be tested without a
// terminal. main is a shim over Run.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/config"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/state"
	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	"github.com/cgardner/herdr-switcher-plus/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

// The effects Run performs, as package variables so tests can observe them
// without a Herdr server or a terminal.
var (
	collect     = agents.Collect
	collectTree = tree.Collect
	loadConfig  = defaultConfig
	loadMode    = state.LoadMode
	saveMode    = state.SaveMode
	focus       = herdr.Focus
	focusPane   = herdr.FocusPane
	notify      = herdr.Notify

	// openPane shells out to Herdr to open the switcher's own pane. It is a
	// variable so a test can read the arguments without a running server.
	openPane = func(args []string) error {
		out, err := exec.Command(herdr.Bin(), args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("herdr %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	runProgram = runAs[ui.Model]
	runTree    = runAs[ui.TreeModel]

	// newProgram is the one line that needs a real terminal, kept separate so
	// everything around it stays testable.
	newProgram = func(m tea.Model) program { return tea.NewProgram(m, tea.WithAltScreen()) }
)

// program is the slice of tea.Program that this command uses.
type program interface {
	Run() (tea.Model, error)
}

// runAs drives the Bubble Tea program and recovers the final model as the
// type it started as.
func runAs[M tea.Model](m M) (M, error) {
	var zero M
	final, err := newProgram(m).Run()
	if err != nil {
		return zero, err
	}
	out, ok := final.(M)
	if !ok {
		return zero, fmt.Errorf("unexpected model %T", final)
	}
	return out, nil
}

// version is stamped at build time from the version in herdr-plugin.toml, so
// a released binary can state which build it is. See the Makefile.
var version = "dev"

// defaultConfig reads the plugin config, validating token names against the
// registry so an unknown name is reported with its row number.
func defaultConfig() (config.Config, error) { return config.Load(ui.KnownToken) }

// modeEnv carries a starting sort from an action. A plugin pane entrypoint has
// one fixed command, so `herdr plugin pane open --env` is the only way an
// action can ask that command for a different ordering.
const modeEnv = "HERDR_SWITCHER_PLUS_MODE"

// viewEnv carries a view from an action, for the same reason.
const viewEnv = "HERDR_SWITCHER_PLUS_VIEW"

// reporter prints a problem to stderr, and raises it as a Herdr notification
// when nobody reads stderr. That covers every run except --list: an action's
// output reaches only the plugin log, and the switcher's own pane closes as
// soon as it exits.
type reporter struct {
	w      io.Writer
	notify bool
}

func (r reporter) fail(err error) {
	fmt.Fprintln(r.w, "herdr-switcher-plus:", err)
	if r.notify {
		notify("switcher+ could not open", describe(err))
	}
}

func (r reporter) warn(msg string) {
	fmt.Fprintln(r.w, "herdr-switcher-plus:", msg)
	if r.notify {
		notify("switcher+", msg)
	}
}

// describe turns an error into a notice. ui_busy gets words of its own,
// because the raw reply names no popup and suggests no action, and the popup
// in the way is often open in another client where it cannot be seen.
func describe(err error) string {
	if strings.Contains(err.Error(), "ui_busy") {
		return "Another popup is already open, possibly in another Herdr client. Close it, then try again."
	}
	return err.Error()
}

// Run executes the command and returns a process exit status.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("herdr-switcher-plus", flag.ContinueOnError)
	fs.SetOutput(stderr)
	plain := fs.Bool("list", false, "print the sorted agents as plain text and exit")
	showVersion := fs.Bool("version", false, "print the version and exit")
	open := fs.Bool("open", false, "open the switcher in a Herdr pane, using the placement from the config")
	sortFlag := fs.String("sort", "", "sort mode: "+modeNames()+" (default: the last mode used)")
	statusFlag := fs.String("status", "", "show one state only: blocked, working, idle, done (or the picker keys b/w/i/d)")
	viewFlag := fs.String("view", "", "view: "+strings.Join(config.ViewNames(), ", ")+" (default: the view in the config, else list)")
	treeFlag := fs.Bool("tree", false, "the same as --view tree")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// The view named on the command line, which overrides every other source.
	explicit := *viewFlag
	if *treeFlag {
		explicit = "tree"
	}
	report := reporter{w: stderr, notify: !*plain}
	explicit = checkView(explicit, "--view", report)

	if *showVersion {
		fmt.Fprintln(stdout, "herdr-switcher-plus", version)
		return 0
	}

	mode := resolveMode(*sortFlag)
	status := agents.ParseStatus(*statusFlag)

	// A malformed config stops the switcher rather than silently reverting to
	// the built-in layout, which would leave the user staring at an unchanged
	// pane with no idea why their file did nothing.
	cfg, err := loadConfig()
	if err != nil {
		report.fail(err)
		return 1
	}

	// Opening the pane happens before any agent is read: the pane that opens
	// runs this same binary again, and that copy does the reading. Only an
	// explicit view travels, so a pane opened without one reads the config.
	if *open {
		if err := openPane(cfg.Pane.OpenArgs(*sortFlag, explicit)); err != nil {
			report.fail(err)
			return 1
		}
		return 0
	}

	if resolveView(explicit, cfg.UI.View, report) == "tree" {
		return runTreeView(*plain, status, stdout, report)
	}

	rows, err := collect()
	if err != nil {
		report.fail(err)
		return 1
	}
	agents.Apply(mode, rows)

	if *plain {
		writeList(stdout, status.Keep(rows))
		return 0
	}

	final, err := runProgram(ui.New(rows, mode, status, cfg.UI.Spec))
	if err != nil {
		report.fail(err)
		return 1
	}
	saveMode(string(final.Mode))

	// The jump runs after the program exits so focus changes do not race the
	// alt screen teardown.
	if final.Chosen == nil {
		return 0
	}
	if err := focus(final.Chosen.Agent); err != nil {
		report.fail(err)
		return 1
	}
	return 0
}

// runTreeView is Run for the tree: print it, or show it and jump to the pane
// the user picks. It keeps no sort mode, because the tree has one ordering.
func runTreeView(plain bool, status agents.StatusFilter, stdout io.Writer, report reporter) int {
	roots, err := collectTree()
	if err != nil {
		report.fail(err)
		return 1
	}
	if plain {
		writeTree(stdout, tree.Filter(roots, status))
		return 0
	}

	final, err := runTree(ui.NewTree(roots, status))
	if err != nil {
		report.fail(err)
		return 1
	}
	if final.Chosen == nil {
		return 0
	}
	// An agent pane goes through `agent focus`, the path the list uses. A
	// shell has no agent to name, so it goes through the socket.
	if r := final.Chosen.Agent; r != nil {
		err = focus(r.Agent)
	} else {
		err = focusPane(final.Chosen.Pane)
	}
	if err != nil {
		report.fail(err)
		return 1
	}
	return 0
}

// writeTree prints the tree fully expanded, two columns of indent a level.
func writeTree(w io.Writer, roots []*tree.Node) {
	now := time.Now()
	for _, l := range tree.Flatten(roots, nil) {
		n, indent := l.Node, strings.Repeat("  ", l.Depth)
		switch {
		case n.IsGroup():
			name := n.Name
			if n.Linked {
				name = "⑂ " + name
			}
			fmt.Fprintf(w, "%s%s\n", indent, name)
		case n.Agent != nil:
			r := n.Agent
			fmt.Fprintf(w, "%s%4s  %-8s %-8s %s%s\n", indent, r.Age(now), n.Name, r.Agent.Status, named(n), trunc(r.Last.Text, 50))
		default:
			fmt.Fprintf(w, "%s%4s  %-8s %-8s %s%s\n", indent, "", n.Name, "", named(n), trunc(n.Pane.Title, 50))
		}
	}
}

// named is the name the user gave a pane, in brackets so that it reads apart
// from the message after it, or nothing for a pane with no name.
func named(n *tree.Node) string {
	if n.Label == "" {
		return ""
	}
	return "[" + n.Label + "] "
}

// resolveView applies the precedence: the command line, then the environment
// an action set, then the config, then the list. The first source that names a
// view decides, and a name that is no view opens the list.
func resolveView(explicit, configured string, report reporter) string {
	if explicit != "" {
		return explicit
	}
	if env := os.Getenv(viewEnv); env != "" {
		return checkView(env, viewEnv, report)
	}
	if configured != "" {
		return checkView(configured, "[ui] view", report)
	}
	return config.DefaultView
}

// checkView passes a valid view through, and turns a bad one into the list
// with a warning. Falling back matches the sort mode and the status filter,
// and the warning says why the view that was asked for did not open.
func checkView(name, source string, report reporter) string {
	if err := config.ValidateView(name); err != nil {
		report.warn(fmt.Sprintf("%s: %v, opening the %s", source, err, config.DefaultView))
		return config.DefaultView
	}
	return name
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
		fmt.Fprintf(w, "%4s  %-10s %-34s %-8s %s\n",
			r.Age(now), r.Agent.PaneID, trunc(r.Label(), 34), r.Agent.Status, trunc(r.Last.Text, 50))
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
