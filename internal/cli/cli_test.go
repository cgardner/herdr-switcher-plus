package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/transcript"
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
	saved   string
	focused *herdr.Agent
	seen    ui.Model
	ran     bool
}

func setup(t *testing.T, rows []agents.Row, collectErr error) *harness {
	t.Helper()
	h := &harness{}
	c, lm, sm, f, rp := collect, loadMode, saveMode, focus, runProgram

	collect = func() ([]agents.Row, error) { return rows, collectErr }
	loadMode = func() string { return "" }
	saveMode = func(m string) { h.saved = m }
	focus = func(a herdr.Agent) error { h.focused = &a; return nil }
	runProgram = func(m ui.Model) (ui.Model, error) {
		h.ran, h.seen = true, m
		return m, nil
	}

	t.Setenv(modeEnv, "")
	t.Cleanup(func() { collect, loadMode, saveMode, focus, runProgram = c, lm, sm, f, rp })
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
	newProgram = func(ui.Model) program { return p }
	t.Cleanup(func() { newProgram = prev })
}

func TestRunTeaReturnsTheFinalModel(t *testing.T) {
	want := ui.New(fixture(), agents.ModeOldest, agents.StatusAll)
	stubProgram(t, fakeProgram{model: want})
	got, err := runTea(ui.Model{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != agents.ModeOldest {
		t.Errorf("Mode = %q, want oldest", got.Mode)
	}
}

func TestRunTeaPropagatesAProgramError(t *testing.T) {
	stubProgram(t, fakeProgram{err: errors.New("no tty")})
	if _, err := runTea(ui.Model{}); err == nil || !strings.Contains(err.Error(), "no tty") {
		t.Errorf("got %v", err)
	}
}

// Bubble Tea returns the tea.Model interface, so a foreign implementation has
// to be reported rather than panicking on the type assertion.
func TestRunTeaRejectsAForeignModel(t *testing.T) {
	stubProgram(t, fakeProgram{model: otherModel{}})
	_, err := runTea(ui.Model{})
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
