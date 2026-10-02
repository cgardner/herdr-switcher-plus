package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	tea "github.com/charmbracelet/bubbletea"
)

// treeFixture is platform with two linked worktrees, the second holding a
// shell beside its agent, then a single checkout promoted to the root.
func treeFixture() []*tree.Node {
	wt := func(repo string, linked bool) *herdr.Worktree {
		return &herdr.Worktree{RepoName: repo, RepoKey: "/git/" + repo, IsLinked: linked}
	}
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{
			{WorkspaceID: "w1", Label: "auth-service", Number: 1, Worktree: wt("platform", true)},
			{WorkspaceID: "w2", Label: "billing", Number: 2, Worktree: wt("platform", true)},
			{WorkspaceID: "w3", Label: "api-gateway", Number: 3, Worktree: wt("api-gateway", false)},
		},
		Panes: []herdr.Pane{
			{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", Cwd: "/co/auth"},
			{PaneID: "w2:p1", WorkspaceID: "w2", Agent: "claude", Cwd: "/co/billing"},
			{PaneID: "w2:p2", WorkspaceID: "w2", Title: "psql billing", Cwd: "/co/billing"},
			{PaneID: "w3:p1", WorkspaceID: "w3", Agent: "claude", Cwd: "/co/api"},
		},
	}
	rows := []agents.Row{
		testRow("w1:p1", "auth-service", "blocked", "Which do you want?", time.Minute),
		testRow("w2:p1", "billing", "done", "Backfill finished.", 8*time.Minute),
		testRow("w3:p1", "api-gateway", "working", "Rate limiter is in.", time.Hour),
	}
	return tree.Build(snap, rows, func(string) string { return "" })
}

func treeKey(s string) tea.KeyMsg {
	switch s {
	case "up", "down", "left", "right", "pgup", "pgdown", "home", "end":
		types := map[string]tea.KeyType{
			"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
			"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
		}
		return tea.KeyMsg{Type: types[s]}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return keyMsg(s)
}

func pressTree(m TreeModel, keys ...string) TreeModel {
	for _, k := range keys {
		next, _ := m.Update(treeKey(k))
		m = next.(TreeModel)
	}
	return m
}

func newTree(t *testing.T, h int) TreeModel {
	t.Helper()
	next, _ := NewTree(treeFixture(), agents.StatusAll).Update(tea.WindowSizeMsg{Width: 90, Height: h})
	return next.(TreeModel)
}

// shown is the visible text of the tree lines, without the title, the status
// line, the pager or the help.
func shown(m TreeModel) []string {
	lines := strings.Split(visible(m.View()), "\n")
	var out []string
	for _, l := range lines[2 : len(lines)-3] {
		if s := strings.TrimRight(l, " "); s != "" {
			out = append(out, strings.TrimLeft(s, " ▌"))
		}
	}
	return out
}

func cursorText(m TreeModel) string {
	for _, l := range strings.Split(visible(m.View()), "\n") {
		if strings.HasPrefix(l, "▌") {
			return strings.TrimSpace(strings.TrimPrefix(l, "▌"))
		}
	}
	return ""
}

func TestTreeRendersEveryLevel(t *testing.T) {
	got := strings.Join(shown(newTree(t, 20)), "\n")
	for _, want := range []string{
		"▾ platform",
		"▾ ⑂ auth-service",
		"1m ● blocked claude  ‹ Which do you want?",
		"· shell    psql billing",
		"▾ api-gateway",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if strings.Contains(got, "agents") {
		t.Errorf("an open group shows no summary:\n%s", got)
	}
}

func TestTreeTitleNamesTheFilter(t *testing.T) {
	m := newTree(t, 20)
	if !strings.Contains(visible(m.View()), "agents · tree") {
		t.Error("title missing")
	}
	m = pressTree(m, "b")
	if !strings.Contains(visible(m.View()), "agents · tree · blocked") {
		t.Error("filter missing from title")
	}
}

func TestTreeCursorMovesAndClamps(t *testing.T) {
	m := newTree(t, 20)
	m = pressTree(m, "up")
	if got := cursorText(m); got != "▾ platform" {
		t.Errorf("up at the top stays: %q", got)
	}
	m = pressTree(m, "j", "down")
	if got := cursorText(m); !strings.HasPrefix(got, "1m") {
		t.Errorf("got %q", got)
	}
	m = pressTree(m, "end")
	if got := cursorText(m); !strings.Contains(got, "Rate limiter") {
		t.Errorf("end lands on the last line: %q", got)
	}
	m = pressTree(m, "down", "k", "home")
	if got := cursorText(m); got != "▾ platform" {
		t.Errorf("home lands on the first line: %q", got)
	}
	m = pressTree(m, "G", "g")
	if got := cursorText(m); got != "▾ platform" {
		t.Errorf("g lands on the first line: %q", got)
	}
}

// pagerLine is the line above the help, where the pager sits.
func pagerLine(m TreeModel) string {
	lines := strings.Split(visible(m.View()), "\n")
	return strings.TrimSpace(lines[len(lines)-2])
}

// The tree turns whole pages, as the list does, so the pager can say which
// page is on screen.
func TestTreeTurnsWholePages(t *testing.T) {
	m := newTree(t, 8) // three tree lines fit, so eight lines make three pages
	if n := len(shown(m)); n != 3 {
		t.Fatalf("got %d lines, want 3", n)
	}
	if got := pagerLine(m); got != "•••" {
		t.Errorf("pager = %q", got)
	}
	m = pressTree(m, "down", "down", "down")
	if got := shown(m); len(got) != 3 || got[0] != "▾ ⑂ billing" {
		t.Errorf("the fourth line opens the second page: %v", got)
	}
	m = pressTree(m, "pgdown", "pgdown")
	if got := cursorText(m); !strings.Contains(got, "Rate limiter") {
		t.Errorf("got %q", got)
	}
	if got := shown(m); len(got) != 2 {
		t.Errorf("the last page holds what is left: %v", got)
	}
	m = pressTree(m, "pgup", "pgup", "pgup")
	if got := cursorText(m); got != "▾ platform" {
		t.Errorf("got %q", got)
	}
}

func TestTreeOnOnePageHasNoPager(t *testing.T) {
	if got := pagerLine(newTree(t, 20)); got != "" {
		t.Errorf("got %q", got)
	}
}

// Dots too wide for the pane become a page count, as in the list.
func TestTreePagerFallsBackToAPageCount(t *testing.T) {
	next, _ := NewTree(treeFixture(), agents.StatusAll).Update(tea.WindowSizeMsg{Width: 4, Height: 6})
	m := pressTree(next.(TreeModel), "down")
	if got := pagerLine(m); got != "2/8" {
		t.Errorf("got %q", got)
	}
}

func TestTreeFoldsAndClimbs(t *testing.T) {
	m := newTree(t, 20)
	m = pressTree(m, "left")
	if got := cursorText(m); got != "▸ platform  ● 1m  2 agents" {
		t.Errorf("a closed group shows its summary: %q", got)
	}
	if len(shown(m)) != 3 {
		t.Errorf("children should hide: %v", shown(m))
	}
	m = pressTree(m, "right", "l", "down", "down", "h")
	if got := cursorText(m); got != "▾ ⑂ auth-service" {
		t.Errorf("left on a pane climbs to its workspace: %q", got)
	}
	m = pressTree(m, "left", "left")
	if got := cursorText(m); got != "▾ platform" {
		t.Errorf("left on a closed group climbs again: %q", got)
	}
	m = pressTree(m, "left", "left")
	if got := cursorText(m); !strings.HasPrefix(got, "▸ platform") {
		t.Errorf("left at the root stays put: %q", got)
	}
}

func TestTreeSpaceAndEnterToggleAGroup(t *testing.T) {
	m := newTree(t, 20)
	m = pressTree(m, " ")
	if !strings.HasPrefix(cursorText(m), "▸") {
		t.Error("space should close")
	}
	m = pressTree(m, "enter")
	if !strings.HasPrefix(cursorText(m), "▾") || m.Chosen != nil {
		t.Error("enter on a group opens it and chooses nothing")
	}
}

func TestTreeSummaryCountsASingleAgent(t *testing.T) {
	m := pressTree(newTree(t, 20), "end", "left", "left")
	if got := cursorText(m); got != "▸ api-gateway  ● 1h  1 agent" {
		t.Errorf("got %q", got)
	}
}

func TestTreeExpandAndCollapseAll(t *testing.T) {
	m := pressTree(newTree(t, 20), "down", "down", "c")
	if got := len(shown(m)); got != 2 {
		t.Errorf("collapse all leaves the roots, got %v", shown(m))
	}
	if got := cursorText(m); !strings.HasPrefix(got, "▸ platform") {
		t.Errorf("the cursor climbs to its root: %q", got)
	}
	m = pressTree(m, "e")
	if got := len(shown(m)); got != 8 {
		t.Errorf("expand all shows everything, got %v", shown(m))
	}
}

func TestTreeStatusFilterOpensEveryGroup(t *testing.T) {
	m := pressTree(newTree(t, 20), "c", "d")
	got := shown(m)
	if len(got) != 3 || !strings.Contains(got[2], "done") {
		t.Errorf("got %v", got)
	}
	m = pressTree(m, "w", "i")
	if got := shown(m); len(got) != 1 || got[0] != "nothing to show" {
		t.Errorf("nothing is idle, and the tree says so: %v", got)
	}
	m = pressTree(m, "enter", "left", "right", " ", "c", "a")
	if m.Chosen != nil || len(shown(m)) != 8 {
		t.Errorf("keys on an empty tree are inert, then a restores: %v", shown(m))
	}
}

func TestTreeEnterOnAPaneChoosesIt(t *testing.T) {
	m := newTree(t, 20)
	next, cmd := pressTree(m, "down", "down").Update(treeKey("enter"))
	m = next.(TreeModel)
	if m.Chosen == nil || m.Chosen.Pane.PaneID != "w1:p1" {
		t.Fatalf("chosen = %+v", m.Chosen)
	}
	if cmd == nil || m.View() != "" {
		t.Error("choosing quits and clears the view")
	}
}

func TestTreeQuitKeys(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		next, cmd := newTree(t, 20).Update(treeKey(k))
		if cmd == nil || next.(TreeModel).Chosen != nil {
			t.Errorf("%s should quit with no choice", k)
		}
	}
}

func TestTreeRefreshKeepsTheCursor(t *testing.T) {
	prev := collectTree
	t.Cleanup(func() { collectTree = prev })
	collectTree = func() ([]*tree.Node, error) { return treeFixture(), nil }

	m := pressTree(newTree(t, 20), "down", "down", "down")
	before := cursorText(m)
	_, cmd := m.Update(treeKey("r"))
	next, _ := m.Update(cmd())
	if got := cursorText(next.(TreeModel)); got != before {
		t.Errorf("got %q, want %q", got, before)
	}
}

func TestTreeFailedRefreshShowsTheError(t *testing.T) {
	m := newTree(t, 20)
	next, _ := m.Update(treeRefreshedMsg{err: errors.New("no server")})
	if !strings.Contains(next.(TreeModel).View(), "no server") {
		t.Error("the error should show")
	}
}

func TestTreeInitIssuesNoCommand(t *testing.T) {
	if NewTree(nil, agents.StatusAll).Init() != nil {
		t.Error("Init should start nothing")
	}
}

// Before the first size message the tree draws every line, rather than a
// window of none.
func TestTreeWithoutASizeDrawsEverything(t *testing.T) {
	if got := len(shown(NewTree(treeFixture(), agents.StatusAll))); got != 8 {
		t.Errorf("got %d lines", got)
	}
}

func TestTreeSelectedLineCarriesTheBackground(t *testing.T) {
	line := strings.Split(newTree(t, 20).View(), "\n")[2]
	if !strings.Contains(line, bgSequence) {
		t.Errorf("no selection background: %q", line)
	}
}

func TestAChildlessGroupHasNoArrow(t *testing.T) {
	roots := []*tree.Node{{Kind: tree.KindWorkspace, ID: "ws:w9", Name: "scratchpad"}}
	if got := shown(NewTree(roots, agents.StatusAll)); len(got) != 1 || got[0] != "scratchpad" {
		t.Errorf("got %v", got)
	}
}

func TestShellTextFallsBackToTheDirectory(t *testing.T) {
	prev := home
	t.Cleanup(func() { home = prev })
	home = "/home/demo"
	n := &tree.Node{Pane: herdr.Pane{Cwd: "/home/demo/dotfiles"}}
	if got := shellText(n); got != "~/dotfiles" {
		t.Errorf("got %q", got)
	}
	n.Pane.ForegroundCwd = "/srv/web-client"
	if got := shellText(n); got != "/srv/web-client" {
		t.Errorf("the foreground directory wins: %q", got)
	}
}

func TestWFoldsEveryWorkspaceAndOpensThemAgain(t *testing.T) {
	m := pressTree(newTree(t, 20), "down", "down", "W")
	want := []string{"▾ platform", "▸ ⑂ auth-service", "▸ ⑂ billing", "▸ api-gateway"}
	for i, got := range shown(m) {
		if i >= len(want) || !strings.HasPrefix(got, want[i]) {
			t.Fatalf("got %v, want %v", shown(m), want)
		}
	}
	if got := cursorText(m); !strings.HasPrefix(got, "▸ ⑂ auth-service") {
		t.Errorf("the cursor climbs to its workspace: %q", got)
	}
	m = pressTree(m, "W")
	if got := len(shown(m)); got != 8 {
		t.Errorf("a second W opens them all, got %v", shown(m))
	}
	if got := cursorText(m); !strings.HasPrefix(got, "▾ ⑂ auth-service") {
		t.Errorf("the cursor stays on its workspace: %q", got)
	}
}

// W folds whatever is still open, so one open workspace among folded ones
// folds rather than opening the rest.
func TestWFoldsWhenAnyWorkspaceIsOpen(t *testing.T) {
	m := pressTree(newTree(t, 20), "W", "down", "right", "W")
	if got := len(shown(m)); got != 4 {
		t.Errorf("got %v", shown(m))
	}
}

func TestTFoldsEveryRepositoryAndKeepsTheWorkspaces(t *testing.T) {
	m := pressTree(newTree(t, 20), "W", "down", "down", "T")
	want := []string{"▸ platform", "▸ api-gateway"}
	if got := shown(m); len(got) != 2 || !strings.HasPrefix(got[0], want[0]) || !strings.HasPrefix(got[1], want[1]) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := cursorText(m); !strings.HasPrefix(got, "▸ platform") {
		t.Errorf("the cursor climbs to its repository: %q", got)
	}
	// Opening the repositories again shows their workspaces still folded.
	if got := shown(pressTree(m, "T")); len(got) != 4 || !strings.HasPrefix(got[1], "▸ ⑂ auth-service") {
		t.Errorf("got %v", got)
	}
}

func TestTLeavesAWorkspaceAtTheRootOpen(t *testing.T) {
	m := pressTree(newTree(t, 20), "end", "T")
	if got := len(shown(m)); got != 3 {
		t.Errorf("got %v", shown(m))
	}
	if got := cursorText(m); !strings.Contains(got, "Rate limiter") {
		t.Errorf("a visible cursor stays put: %q", got)
	}
}

func TestFoldingAnEmptyTreeDoesNothing(t *testing.T) {
	m := pressTree(NewTree(nil, agents.StatusAll), "W", "T")
	if len(m.lines) != 0 {
		t.Errorf("lines = %v", m.lines)
	}
}
