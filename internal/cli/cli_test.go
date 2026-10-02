package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/config"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/layout"
	"github.com/cgardner/herdr-switcher-plus/internal/transcript"
	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	"github.com/cgardner/herdr-switcher-plus/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
)

func row(pane, space, status string, age time.Duration) agents.Row {
	return agents.Row{
		Agent:   herdr.Agent{PaneID: pane, Status: status},
		Space:   space,
		Last:    transcript.Message{At: time.Now().Add(-age), Role: "assistant", Text: "msg for " + space},
		HasLast: true,
	}
}

func fixture() []agents.Row {
	return []agents.Row{
		row("w1:p1", "alpha", "working", time.Minute),
		row("w2:p1", "beta", "idle", time.Hour),
		row("w3:p1", "gamma", "blocked", 30*time.Hour),
	}
}

// harness replaces every effect Run performs and records what happened.
type harness struct {
	notices []string
	saved   string
	focused *herdr.Agent
	seen    ui.Model
	ran     bool
}

func setup(t *testing.T, rows []agents.Row, collectErr error) *harness {
	t.Helper()
	h := &harness{}
	c, lm, sm, f, rp, lc, nt := collect, loadMode, saveMode, focus, runProgram, loadConfig, notify
	loadConfig = func() (config.Config, error) { return config.Config{}, nil }
	// A test must never reach the real Herdr, which would show the notice.
	notify = func(title, body string) { h.notices = append(h.notices, title+": "+body) }

	collect = func() ([]agents.Row, error) { return rows, collectErr }
	loadMode = func() string { return "" }
	saveMode = func(m string) { h.saved = m }
	focus = func(a herdr.Agent) error { h.focused = &a; return nil }
	runProgram = func(m ui.Model) (ui.Model, error) {
		h.ran, h.seen = true, m
		return m, nil
	}

	t.Setenv(modeEnv, "")
	t.Cleanup(func() {
		collect, loadMode, saveMode, focus, runProgram, loadConfig, notify = c, lm, sm, f, rp, lc, nt
	})
	return h
}

func run(args ...string) (code int, out, errOut string) {
	var o, e bytes.Buffer
	code = Run(args, &o, &e)
	return code, o.String(), e.String()
}

func TestListPrintsRowsNewestFirst(t *testing.T) {
	setup(t, fixture(), nil)
	code, out, _ := run("--list")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(lines), out)
	}
	if !strings.Contains(lines[0], "alpha") {
		t.Errorf("first line should be the newest: %q", lines[0])
	}
}

func TestListHonoursTheSortFlag(t *testing.T) {
	setup(t, fixture(), nil)
	_, out, _ := run("--list", "--sort", "oldest")
	if !strings.Contains(strings.Split(out, "\n")[0], "gamma") {
		t.Errorf("oldest should lead: %q", out)
	}
}

func TestListHonoursTheStatusFlag(t *testing.T) {
	setup(t, fixture(), nil)
	_, out, _ := run("--list", "--status", "blocked")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "gamma") {
		t.Errorf("want only the blocked row, got %q", out)
	}
}

func TestListAcceptsAPickerKeyAsStatus(t *testing.T) {
	setup(t, fixture(), nil)
	_, out, _ := run("--list", "--status", "w")
	if strings.Count(strings.TrimSpace(out), "\n") != 0 || !strings.Contains(out, "alpha") {
		t.Errorf("want only the working row, got %q", out)
	}
}

func TestListWithNoAgentsPrintsNothing(t *testing.T) {
	setup(t, nil, nil)
	code, out, _ := run("--list")
	if code != 0 || out != "" {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestLongSpaceAndMessageAreTruncated(t *testing.T) {
	long := strings.Repeat("x", 90)
	setup(t, []agents.Row{{
		Agent:   herdr.Agent{PaneID: "w1:p1", Status: "idle"},
		Space:   long,
		Last:    transcript.Message{At: time.Now(), Text: long},
		HasLast: true,
	}}, nil)
	_, out, _ := run("--list")
	// Count runes, not bytes: the ellipsis is three bytes wide.
	for _, field := range strings.Fields(out) {
		if n := len([]rune(field)); n > 60 {
			t.Errorf("field is %d runes, want at most 60: %q", n, field)
		}
	}
	if !strings.Contains(out, "…") {
		t.Error("expected an ellipsis marker")
	}
}

// Precedence: the flag beats the environment, which beats the saved mode.
func TestModePrecedence(t *testing.T) {
	h := setup(t, fixture(), nil)
	loadMode = func() string { return "name" }

	if got := resolveMode(""); got != agents.ModeName {
		t.Errorf("saved mode should apply, got %q", got)
	}
	t.Setenv(modeEnv, "attention")
	if got := resolveMode(""); got != agents.ModeAttention {
		t.Errorf("environment should beat the saved mode, got %q", got)
	}
	if got := resolveMode("oldest"); got != agents.ModeOldest {
		t.Errorf("the flag should beat the environment, got %q", got)
	}
	_ = h
}

func TestUnknownModeFallsBackToRecent(t *testing.T) {
	setup(t, fixture(), nil)
	if got := resolveMode("nonsense"); got != agents.ModeRecent {
		t.Errorf("got %q, want recent", got)
	}
}

func TestCollectFailureExitsOne(t *testing.T) {
	setup(t, nil, errors.New("socket gone"))
	code, _, errOut := run("--list")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut, "socket gone") {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestBadFlagExitsTwo(t *testing.T) {
	setup(t, fixture(), nil)
	if code, _, _ := run("--nonsense"); code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
}

func TestInteractiveRunSavesTheMode(t *testing.T) {
	h := setup(t, fixture(), nil)
	code, _, _ := run("--sort", "attention")
	if code != 0 || !h.ran {
		t.Fatalf("code=%d ran=%v", code, h.ran)
	}
	if h.saved != string(agents.ModeAttention) {
		t.Errorf("saved %q, want attention", h.saved)
	}
	if h.focused != nil {
		t.Error("nothing was chosen, so nothing should be focused")
	}
}

func TestChoosingARowFocusesIt(t *testing.T) {
	h := setup(t, fixture(), nil)
	runProgram = func(m ui.Model) (ui.Model, error) {
		chosen := row("w9:p9", "picked", "idle", time.Minute)
		m.Chosen = &chosen
		return m, nil
	}
	if code, _, _ := run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if h.focused == nil || h.focused.PaneID != "w9:p9" {
		t.Errorf("focused = %+v, want w9:p9", h.focused)
	}
}

func TestFocusFailureExitsOne(t *testing.T) {
	setup(t, fixture(), nil)
	runProgram = func(m ui.Model) (ui.Model, error) {
		chosen := row("w9:p9", "picked", "idle", time.Minute)
		m.Chosen = &chosen
		return m, nil
	}
	focus = func(herdr.Agent) error { return errors.New("pane closed") }
	code, _, errOut := run()
	if code != 1 || !strings.Contains(errOut, "pane closed") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

func TestProgramFailureExitsOneAndSavesNothing(t *testing.T) {
	h := setup(t, fixture(), nil)
	runProgram = func(ui.Model) (ui.Model, error) { return ui.Model{}, errors.New("no tty") }
	code, _, errOut := run()
	if code != 1 || !strings.Contains(errOut, "no tty") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
	if h.saved != "" {
		t.Errorf("saved %q on failure, want nothing", h.saved)
	}
}

func TestModeNamesListsEveryMode(t *testing.T) {
	got := modeNames()
	for _, m := range agents.Modes {
		if !strings.Contains(got, string(m)) {
			t.Errorf("modeNames is missing %q: %q", m, got)
		}
	}
}

// fakeProgram stands in for tea.Program so runTea can be exercised without a
// terminal.
type fakeProgram struct {
	model tea.Model
	err   error
}

func (f fakeProgram) Run() (tea.Model, error) { return f.model, f.err }

func stubProgram(t *testing.T, p program) {
	t.Helper()
	prev := newProgram
	newProgram = func(tea.Model) program { return p }
	t.Cleanup(func() { newProgram = prev })
}

func TestRunTeaReturnsTheFinalModel(t *testing.T) {
	want := ui.New(fixture(), agents.ModeOldest, agents.StatusAll, ui.DefaultSpec)
	stubProgram(t, fakeProgram{model: want})
	got, err := runAs(ui.Model{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != agents.ModeOldest {
		t.Errorf("Mode = %q, want oldest", got.Mode)
	}
}

func TestRunTeaPropagatesAProgramError(t *testing.T) {
	stubProgram(t, fakeProgram{err: errors.New("no tty")})
	if _, err := runAs(ui.Model{}); err == nil || !strings.Contains(err.Error(), "no tty") {
		t.Errorf("got %v", err)
	}
}

// Bubble Tea returns the tea.Model interface, so a foreign implementation has
// to be reported rather than panicking on the type assertion.
func TestRunTeaRejectsAForeignModel(t *testing.T) {
	stubProgram(t, fakeProgram{model: otherModel{}})
	_, err := runAs(ui.Model{})
	if err == nil || !strings.Contains(err.Error(), "unexpected model") {
		t.Errorf("got %v", err)
	}
}

type otherModel struct{}

func (otherModel) Init() tea.Cmd                         { return nil }
func (o otherModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return o, nil }
func (otherModel) View() string                          { return "" }

// The release build stamps the version in, so a released binary can say which
// build it is. The default keeps a source build honest about not being one.
func TestVersionFlag(t *testing.T) {
	setup(t, fixture(), nil)
	prev := version
	version = "1.2.3"
	t.Cleanup(func() { version = prev })

	code, out, _ := run("--version")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "1.2.3") || !strings.Contains(out, "herdr-switcher-plus") {
		t.Errorf("stdout = %q", out)
	}
}

func TestVersionFlagSkipsCollecting(t *testing.T) {
	setup(t, nil, errors.New("socket gone"))
	if code, _, _ := run("--version"); code != 0 {
		t.Errorf("exit %d: --version must not need a Herdr server", code)
	}
}

func TestVersionDefaultsToDev(t *testing.T) {
	if version != "dev" {
		t.Errorf("version = %q, want dev in a source build", version)
	}
}

// A malformed config stops the switcher rather than silently reverting to the
// built-in layout, which would leave the user with no idea why nothing changed.
func TestABadConfigExitsOne(t *testing.T) {
	setup(t, fixture(), nil)
	loadConfig = func() (config.Config, error) {
		return config.Config{}, errors.New("config.toml: unknown token \"nonsense\"")
	}
	code, _, errOut := run()
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut, "nonsense") {
		t.Errorf("stderr should carry the config error: %q", errOut)
	}
}

// The layout from the config reaches the model.
func TestTheConfiguredLayoutReachesTheModel(t *testing.T) {
	h := setup(t, fixture(), nil)
	var cfg config.Config
	cfg.UI.Rows = [][]layout.Token{{{Name: "label"}}}
	loadConfig = func() (config.Config, error) { return cfg, nil }

	if code, _, _ := run(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !h.ran {
		t.Fatal("the program should have run")
	}
}

// The real loader validates against the token registry, so a config naming an
// unknown token is rejected rather than silently drawing nothing.
func TestDefaultConfigValidatesAgainstTheTokenRegistry(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)

	path := filepath.Join(dir, config.FileName)
	if err := os.WriteFile(path, []byte("[ui]\nrows = [[\"age\", \"nonsense\"]]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := defaultConfig(); err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("got %v", err)
	}

	if err := os.WriteFile(path, []byte("[ui]\nrows = [[\"age\", \"label\"]]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := defaultConfig()
	if err != nil {
		t.Fatalf("a valid config should load: %v", err)
	}
	if len(cfg.UI.Rows) != 1 {
		t.Errorf("got %+v", cfg.UI.Rows)
	}
}

func stubOpen(t *testing.T) *[][]string {
	t.Helper()
	var calls [][]string
	prev := openPane
	openPane = func(a []string) error { calls = append(calls, a); return nil }
	t.Cleanup(func() { openPane = prev })
	return &calls
}

// --open passes the configured placement through to Herdr. A manifest pane's
// placement is fixed, so this is the only way it can be a setting.
func TestOpenUsesTheConfiguredPlacement(t *testing.T) {
	setup(t, fixture(), nil)
	calls := stubOpen(t)
	var cfg config.Config
	cfg.Pane = config.Pane{Placement: "overlay", Width: "70%", Height: "50%"}
	loadConfig = func() (config.Config, error) { return cfg, nil }

	if code, _, _ := run("--open"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected one call, got %v", *calls)
	}
	got := strings.Join((*calls)[0], " ")
	for _, want := range []string{"--placement overlay", "--width 70%", "--height 50%", config.PluginID} {
		if !strings.Contains(got, want) {
			t.Errorf("%q missing from %q", want, got)
		}
	}
}

func TestOpenPassesTheSortMode(t *testing.T) {
	setup(t, fixture(), nil)
	calls := stubOpen(t)
	if code, _, _ := run("--open", "--sort", "attention"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got := strings.Join((*calls)[0], " "); !strings.Contains(got, "HERDR_SWITCHER_PLUS_MODE=attention") {
		t.Errorf("mode not passed: %q", got)
	}
}

// Opening the pane must not read any agent: the pane that opens runs this
// binary again, and that copy does the reading.
func TestOpenDoesNotCollect(t *testing.T) {
	setup(t, nil, errors.New("collect must not run"))
	stubOpen(t)
	if code, _, errOut := run("--open"); code != 0 {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestOpenReportsAFailure(t *testing.T) {
	setup(t, fixture(), nil)
	prev := openPane
	openPane = func([]string) error { return errors.New("ui_busy") }
	t.Cleanup(func() { openPane = prev })

	code, _, errOut := run("--open")
	if code != 1 || !strings.Contains(errOut, "ui_busy") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

// A bad config stops the open too, rather than falling back to the manifest's
// placement and leaving the setting looking ignored.
func TestOpenStopsOnABadConfig(t *testing.T) {
	setup(t, fixture(), nil)
	stubOpen(t)
	loadConfig = func() (config.Config, error) {
		return config.Config{}, errors.New("config.toml: [pane] placement \"floating\" is not one of ...")
	}
	if code, _, errOut := run("--open"); code != 1 || !strings.Contains(errOut, "floating") {
		t.Errorf("code=%d stderr=%q", code, errOut)
	}
}

// treeHarness replaces the effects of the tree view and records what happened.
type treeHarness struct {
	focused     *herdr.Agent
	focusedPane *herdr.Pane
	seen        ui.TreeModel
	ran         bool
	chosen      *tree.Node
	runErr      error
	focusErr    error
}

func treeRoots() []*tree.Node {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{
			{WorkspaceID: "w1", Label: "auth-service", Worktree: &herdr.Worktree{RepoName: "platform", RepoKey: "k", IsLinked: true}},
			{WorkspaceID: "w2", Label: "billing", Worktree: &herdr.Worktree{RepoName: "platform", RepoKey: "k", IsLinked: true}},
		},
		Panes: []herdr.Pane{
			{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude"},
			{PaneID: "w2:p1", WorkspaceID: "w2", Agent: "claude"},
			{PaneID: "w2:p2", WorkspaceID: "w2", Title: "psql billing", Label: "db"},
		},
	}
	rows := []agents.Row{row("w1:p1", "auth-service", "blocked", time.Minute), row("w2:p1", "billing", "done", time.Hour)}
	rows[0].Agent.Name = "auth-fix"
	return tree.Build(snap, rows, func(string) string { return "" })
}

func setupTree(t *testing.T, collectErr error) *treeHarness {
	t.Helper()
	setup(t, nil, errors.New("the list must not collect"))
	h := &treeHarness{}
	ct, fp, rt := collectTree, focusPane, runTree
	collectTree = func() ([]*tree.Node, error) { return treeRoots(), collectErr }
	focus = func(a herdr.Agent) error { h.focused = &a; return h.focusErr }
	focusPane = func(p herdr.Pane) error { h.focusedPane = &p; return h.focusErr }
	runTree = func(m ui.TreeModel) (ui.TreeModel, error) {
		h.ran, h.seen = true, m
		m.Chosen = h.chosen
		return m, h.runErr
	}
	t.Setenv(viewEnv, "")
	t.Cleanup(func() { collectTree, focusPane, runTree = ct, fp, rt })
	return h
}

func TestTreeListPrintsEveryLevel(t *testing.T) {
	setupTree(t, nil)
	code, out, _ := run("--tree", "--list")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"platform\n", "  ⑂ auth-service\n", "blocked", "shell", "[db] psql billing", "[auth-fix] "} {
		if !strings.Contains(out, want) {
			t.Errorf("%q missing from\n%s", want, out)
		}
	}
}

func TestTreeListHonoursTheStatusFlag(t *testing.T) {
	setupTree(t, nil)
	_, out, _ := run("--tree", "--list", "--status", "done")
	if strings.Contains(out, "auth-service") || !strings.Contains(out, "billing") {
		t.Errorf("got\n%s", out)
	}
}

func TestTheViewEnvironmentSelectsTheTree(t *testing.T) {
	h := setupTree(t, nil)
	t.Setenv(viewEnv, "tree")
	if code, _, errOut := run(); code != 0 || !h.ran {
		t.Errorf("exit %d, ran %v, stderr %q", code, h.ran, errOut)
	}
}

func TestTreeCollectFailureExitsOne(t *testing.T) {
	setupTree(t, errors.New("no server"))
	if code, _, errOut := run("--tree"); code != 1 || !strings.Contains(errOut, "no server") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestTreeProgramFailureExitsOne(t *testing.T) {
	h := setupTree(t, nil)
	h.runErr = errors.New("no tty")
	if code, _, errOut := run("--tree"); code != 1 || !strings.Contains(errOut, "no tty") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestTreeWithNoChoiceFocusesNothing(t *testing.T) {
	h := setupTree(t, nil)
	if code, _, _ := run("--tree"); code != 0 || h.focused != nil || h.focusedPane != nil {
		t.Errorf("exit %d, focused %v %v", code, h.focused, h.focusedPane)
	}
}

// An agent pane goes through `agent focus`, the path the list proved.
func TestChoosingAnAgentPaneFocusesTheAgent(t *testing.T) {
	h := setupTree(t, nil)
	h.chosen = treeRoots()[0].Children[0].Children[0]
	if code, _, _ := run("--tree"); code != 0 || h.focused == nil || h.focused.PaneID != "w1:p1" {
		t.Errorf("exit %d, focused %+v", code, h.focused)
	}
}

func TestChoosingAShellFocusesThePane(t *testing.T) {
	h := setupTree(t, nil)
	h.chosen = &tree.Node{Kind: tree.KindPane, Pane: herdr.Pane{PaneID: "w2:p2"}}
	if code, _, _ := run("--tree"); code != 0 || h.focusedPane == nil || h.focusedPane.PaneID != "w2:p2" {
		t.Errorf("exit %d, focused %+v", code, h.focusedPane)
	}
}

func TestTreeFocusFailureExitsOne(t *testing.T) {
	h := setupTree(t, nil)
	h.chosen = &tree.Node{Kind: tree.KindPane, Pane: herdr.Pane{PaneID: "w2:p2"}}
	h.focusErr = errors.New("pane closed")
	if code, _, errOut := run("--tree"); code != 1 || !strings.Contains(errOut, "pane closed") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestOpenPassesTheTreeView(t *testing.T) {
	setupTree(t, nil)
	calls := stubOpen(t)
	if code, _, _ := run("--open", "--tree"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if got := strings.Join((*calls)[0], " "); !strings.Contains(got, "HERDR_SWITCHER_PLUS_VIEW=tree") {
		t.Errorf("view not passed: %q", got)
	}
}

func TestRunAsRecoversATreeModel(t *testing.T) {
	stubProgram(t, fakeProgram{model: ui.NewTree(nil, agents.StatusDone)})
	if _, err := runAs(ui.TreeModel{}); err != nil {
		t.Fatal(err)
	}
}

// configView makes the loaded config name a view.
func configView(view string) {
	loadConfig = func() (config.Config, error) {
		var c config.Config
		c.UI.View = view
		return c, nil
	}
}

func TestTheConfigCanChooseTheTree(t *testing.T) {
	h := setupTree(t, nil)
	configView("tree")
	if code, _, errOut := run(); code != 0 || !h.ran {
		t.Errorf("exit %d, ran %v, stderr %q", code, h.ran, errOut)
	}
}

func TestTheViewFlagOverridesTheConfig(t *testing.T) {
	h := setupTree(t, nil)
	configView("tree")
	collect = func() ([]agents.Row, error) { return fixture(), nil }
	if code, out, _ := run("--view", "list", "--list"); code != 0 || h.ran || !strings.Contains(out, "alpha") {
		t.Errorf("exit %d, tree ran %v, out %q", code, h.ran, out)
	}
}

func TestTheViewEnvironmentOverridesTheConfig(t *testing.T) {
	h := setupTree(t, nil)
	configView("tree")
	collect = func() ([]agents.Row, error) { return fixture(), nil }
	t.Setenv(viewEnv, "list")
	if code, _, _ := run("--list"); code != 0 || h.ran {
		t.Errorf("exit %d, tree ran %v", code, h.ran)
	}
}

// Every source that names a bad view opens the list, with a warning that
// names the source.
func TestABadViewOpensTheListWithAWarning(t *testing.T) {
	cases := []struct {
		name, flag, env, config, source string
	}{
		{"flag", "grid", "", "tree", "--view"},
		{"environment", "", "grid", "tree", viewEnv},
		{"config", "", "", "grid", "[ui] view"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := setupTree(t, nil)
			configView(c.config)
			collect = func() ([]agents.Row, error) { return fixture(), nil }
			t.Setenv(viewEnv, c.env)
			args := []string{"--list"}
			if c.flag != "" {
				args = append(args, "--view", c.flag)
			}
			code, out, errOut := run(args...)
			if code != 0 || h.ran || !strings.Contains(out, "alpha") {
				t.Errorf("exit %d, tree ran %v, out %q", code, h.ran, out)
			}
			for _, want := range []string{c.source, "grid", "opening the list"} {
				if !strings.Contains(errOut, want) {
					t.Errorf("warning should mention %q: %q", want, errOut)
				}
			}
		})
	}
}

// --open passes the list on, so the pane it opens does not repeat the warning
// or fall back to the config.
func TestOpenWithABadViewPassesTheList(t *testing.T) {
	setupTree(t, nil)
	calls := stubOpen(t)
	configView("tree")
	run("--open", "--view", "grid")
	if got := strings.Join((*calls)[0], " "); !strings.Contains(got, "HERDR_SWITCHER_PLUS_VIEW=list") {
		t.Errorf("got %q", got)
	}
}

func TestOpenPassesAnExplicitList(t *testing.T) {
	setupTree(t, nil)
	calls := stubOpen(t)
	configView("tree")
	run("--open", "--view", "list")
	if got := strings.Join((*calls)[0], " "); !strings.Contains(got, "HERDR_SWITCHER_PLUS_VIEW=list") {
		t.Errorf("view not passed: %q", got)
	}
}

// Without an explicit view, the opened pane reads the config itself.
func TestOpenWithoutAViewPassesNone(t *testing.T) {
	setupTree(t, nil)
	calls := stubOpen(t)
	configView("tree")
	run("--open")
	if got := strings.Join((*calls)[0], " "); strings.Contains(got, "HERDR_SWITCHER_PLUS_VIEW") {
		t.Errorf("no view should travel: %q", got)
	}
}

// A failed open reaches only the plugin log, so it is raised as a notice too.
// ui_busy gets words of its own, because the raw reply suggests no action.
func TestAFailedOpenRaisesANotice(t *testing.T) {
	h := setup(t, fixture(), nil)
	prev := openPane
	t.Cleanup(func() { openPane = prev })
	openPane = func([]string) error {
		return errors.New(`exit status 1: {"error":{"code":"ui_busy","message":"a popup pane is already open"}}`)
	}
	if code, _, errOut := run("--open"); code != 1 || !strings.Contains(errOut, "ui_busy") {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
	if len(h.notices) != 1 || !strings.Contains(h.notices[0], "Another popup is already open") {
		t.Errorf("notices = %q", h.notices)
	}
}

// Any other failure is raised as it is.
func TestAFailureInThePaneRaisesItsError(t *testing.T) {
	h := setup(t, nil, errors.New("herdr api snapshot: no server"))
	run()
	if len(h.notices) != 1 || !strings.Contains(h.notices[0], "no server") {
		t.Errorf("notices = %q", h.notices)
	}
}

// --list runs at a shell, where stderr is seen, so it raises nothing.
func TestListRaisesNoNotice(t *testing.T) {
	h := setup(t, nil, errors.New("no server"))
	if code, _, _ := run("--list"); code != 1 || len(h.notices) != 0 {
		t.Errorf("exit %d, notices %q", code, h.notices)
	}
}

// A bad view is an unexpected state, so its warning is raised as well.
func TestABadViewRaisesANotice(t *testing.T) {
	h := setup(t, fixture(), nil)
	configView("grid")
	run()
	if len(h.notices) != 1 || !strings.Contains(h.notices[0], "opening the list") {
		t.Errorf("notices = %q", h.notices)
	}
}
