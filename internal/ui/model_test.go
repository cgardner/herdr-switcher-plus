package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-pane-sort/internal/agents"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func sized(m Model, w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

func fixture() []agents.Row {
	return []agents.Row{
		testRow("w1:p1", "alpha", "working", "busy", time.Minute),
		testRow("w2:p1", "beta", "idle", "waiting", time.Hour),
		testRow("w3:p1", "gamma", "blocked", "may I?", 2*time.Hour),
		testRow("w4:p1", "delta", "idle", "older", 48*time.Hour),
	}
}

func newModel(t *testing.T) Model {
	t.Helper()
	return sized(New(fixture(), agents.ModeRecent, agents.StatusAll), 100, 24)
}

func paneIDs(m Model) []string {
	out := make([]string, 0, len(m.list.Items()))
	for _, li := range m.list.Items() {
		out = append(out, li.(item).row.Agent.PaneID)
	}
	return out
}

func TestNewSortsAndTitles(t *testing.T) {
	m := newModel(t)
	if got := paneIDs(m); got[0] != "w1:p1" {
		t.Errorf("newest row should lead, got %v", got)
	}
	if m.list.Title != "agents · last message" {
		t.Errorf("title = %q", m.list.Title)
	}
	if m.Mode != agents.ModeRecent {
		t.Errorf("Mode = %q", m.Mode)
	}
}

// New must not reorder the caller's slice underneath it.
func TestNewDoesNotMutateTheCallerSlice(t *testing.T) {
	rows := fixture()
	first := rows[1].Agent.PaneID
	New(rows, agents.ModeName, agents.StatusAll)
	if rows[1].Agent.PaneID != first {
		t.Error("New reordered the caller's slice")
	}
}

func TestSortKeysCycleBothWays(t *testing.T) {
	m := press(newModel(t), "s")
	if m.Mode != agents.ModeRecent.Next(1) {
		t.Errorf("s should advance, got %q", m.Mode)
	}
	if !strings.Contains(m.list.Title, m.Mode.Label()) {
		t.Errorf("title %q should name the mode", m.list.Title)
	}
	m = press(m, "S")
	if m.Mode != agents.ModeRecent {
		t.Errorf("S should step back, got %q", m.Mode)
	}
}

// A sort change is a request to see a new ordering, so the view returns to the
// top rather than chasing the previously selected row.
func TestSortChangeReturnsToTheTop(t *testing.T) {
	m := press(newModel(t), "down", "down")
	if m.list.Index() == 0 {
		t.Fatal("precondition: the cursor should have moved")
	}
	if m = press(m, "s"); m.list.Index() != 0 {
		t.Errorf("index = %d, want 0", m.list.Index())
	}
}

func TestStatusKeysNarrowAndRestore(t *testing.T) {
	m := newModel(t)
	for _, c := range []struct {
		key   string
		want  int
		title string
	}{
		{"b", 1, "blocked"},
		{"w", 1, "working"},
		{"i", 2, "idle"},
		{"d", 0, "done"},
	} {
		got := press(m, c.key)
		if n := len(got.list.Items()); n != c.want {
			t.Errorf("%q kept %d rows, want %d", c.key, n, c.want)
		}
		if !strings.Contains(got.list.Title, c.title) {
			t.Errorf("%q title = %q, want it to name %q", c.key, got.list.Title, c.title)
		}
	}
	if n := len(press(m, "b", "a").list.Items()); n != 4 {
		t.Errorf("a restored %d rows, want 4", n)
	}
}

// An empty result still has to explain itself through the title.
func TestEmptyStatusFilterStillNamesItself(t *testing.T) {
	m := press(newModel(t), "d")
	if len(m.list.Items()) != 0 {
		t.Fatal("precondition: nothing is done")
	}
	if !strings.Contains(m.list.Title, "done") {
		t.Errorf("title = %q, want it to name the filter", m.list.Title)
	}
}

func TestEnterChoosesTheSelectedRow(t *testing.T) {
	m := press(newModel(t), "down", "enter")
	if m.Chosen == nil {
		t.Fatal("Chosen must be set")
	}
	if m.Chosen.Agent.PaneID != "w2:p1" {
		t.Errorf("Chosen = %q, want w2:p1", m.Chosen.Agent.PaneID)
	}
}

// Enter on an empty list must quit without choosing, never panic.
func TestEnterOnAnEmptyListChoosesNothing(t *testing.T) {
	m := press(newModel(t), "d", "enter")
	if m.Chosen != nil {
		t.Errorf("Chosen = %+v, want nil", m.Chosen)
	}
}

func TestQuitKeysSetNoChoice(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		if m := press(newModel(t), k); m.Chosen != nil {
			t.Errorf("%q should not choose a row", k)
		}
	}
}

// While the filter input is open every key belongs to the list, or typing
// "sad" would cycle the sort and change the status filter.
func TestKeysAreInertWhileFiltering(t *testing.T) {
	m := press(newModel(t), "/")
	if m.list.FilterState() != list.Filtering {
		t.Fatal("precondition: the filter should be open")
	}
	got := press(m, "s", "a", "d")
	if got.Mode != agents.ModeRecent {
		t.Errorf("sort changed while filtering: %q", got.Mode)
	}
	if got.Chosen != nil {
		t.Error("a row was chosen while filtering")
	}
}

func TestRefreshReplacesRowsAndKeepsTheCursor(t *testing.T) {
	m := press(newModel(t), "down")
	selected := m.list.SelectedItem().(item).row.Agent.PaneID

	next, _ := m.Update(refreshedMsg{rows: fixture()})
	m = next.(Model)
	if m.err != nil {
		t.Fatalf("unexpected error %v", m.err)
	}
	if got := m.list.SelectedItem().(item).row.Agent.PaneID; got != selected {
		t.Errorf("cursor moved to %q, want %q", got, selected)
	}
}

// A failed refresh must keep the rows already on screen.
func TestFailedRefreshKeepsTheExistingRows(t *testing.T) {
	m := newModel(t)
	next, _ := m.Update(refreshedMsg{err: errors.New("socket gone")})
	m = next.(Model)
	if m.err == nil {
		t.Fatal("error should be recorded")
	}
	if len(m.list.Items()) != 4 {
		t.Errorf("rows dropped on failure: %d", len(m.list.Items()))
	}
	if !strings.Contains(m.View(), "socket gone") {
		t.Errorf("View should surface the error: %q", m.View())
	}
}

func TestRefreshKeyIssuesTheCollectCommand(t *testing.T) {
	prev := collectRows
	collectRows = func() ([]agents.Row, error) { return fixture()[:1], nil }
	t.Cleanup(func() { collectRows = prev })

	_, cmd := newModel(t).Update(keyMsg("r"))
	if cmd == nil {
		t.Fatal("r must issue a command")
	}
	msg, ok := cmd().(refreshedMsg)
	if !ok {
		t.Fatalf("got %T, want refreshedMsg", cmd())
	}
	if msg.err != nil || len(msg.rows) != 1 {
		t.Errorf("msg = %+v", msg)
	}
}

func TestViewIsEmptyOnceQuitting(t *testing.T) {
	if got := press(newModel(t), "q").View(); got != "" {
		t.Errorf("View = %q, want empty", got)
	}
}

func TestViewRendersRows(t *testing.T) {
	got := visible(newModel(t).View())
	for _, want := range []string{"agents · last message", "alpha", "beta"} {
		if !strings.Contains(got, want) {
			t.Errorf("View is missing %q", want)
		}
	}
}

func TestInitIssuesNoCommand(t *testing.T) {
	if New(nil, agents.ModeRecent, agents.StatusAll).Init() != nil {
		t.Error("Init must issue no command")
	}
}

func TestTitleNamesModeAndFilter(t *testing.T) {
	if got := title(agents.ModeOldest, agents.StatusAll); got != "agents · oldest first" {
		t.Errorf("got %q", got)
	}
	if got := title(agents.ModeRecent, agents.StatusBlocked); got != "agents · last message · blocked" {
		t.Errorf("got %q", got)
	}
}

func TestFilterValueCoversEveryColumn(t *testing.T) {
	r := testRow("w9:p2", "spacename", "blocked", "message text", time.Minute)
	r.Agent.Kind = "codex"
	r.Agent.Cwd = "/src/thing"
	got := item{row: r, now: time.Now()}.FilterValue()
	for _, want := range []string{"spacename", "blocked", "codex", "/src/thing", "w9:p2", "message text"} {
		if !strings.Contains(got, want) {
			t.Errorf("FilterValue is missing %q: %q", want, got)
		}
	}
}

func TestPadAndPadLeft(t *testing.T) {
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
	if got := pad("abcdefg", 4); got != "abc…" {
		t.Errorf("pad truncation = %q", got)
	}
	if got := padLeft("7m", 4); got != "  7m" {
		t.Errorf("padLeft = %q", got)
	}
	if got := padLeft("12345", 4); got != "12345" {
		t.Errorf("padLeft must not truncate, got %q", got)
	}
}

func TestGlyphAndColorHelpers(t *testing.T) {
	if glyph("idle") != "○" || glyph("unknown") != "○" {
		t.Error("resting states use the hollow glyph")
	}
	if glyph("working") != "●" || glyph("blocked") != "●" {
		t.Error("active states use the filled glyph")
	}
	now := time.Now()
	fresh := testRow("p", "s", "idle", "t", time.Minute)
	stale := testRow("p", "s", "idle", "t", 48*time.Hour)
	none := agents.Row{}
	if ageColorFor(fresh, now, false) != accentColor {
		t.Error("a recent age should use the accent color")
	}
	if ageColorFor(stale, now, false) != staleColor {
		t.Error("a stale age should be dim")
	}
	if ageColorFor(none, now, true) != selectedDimColor {
		t.Error("on the bar a dim value should step up")
	}
	if textColorFor(false) != "" || textColorFor(true) != selectedTextColor {
		t.Error("text color should brighten only when selected")
	}
	if previewColorFor(false) != previewColor || previewColorFor(true) != selectedDimColor {
		t.Error("preview color should brighten only when selected")
	}
}
