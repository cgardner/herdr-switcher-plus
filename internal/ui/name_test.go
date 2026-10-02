package ui

import (
	"strings"
	"testing"

	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	"github.com/charmbracelet/lipgloss"
)

// namedTree is the fixture with the auth-service agent named through
// `herdr agent rename` and the billing shell named through
// `herdr pane rename`.
func namedTree(t *testing.T) TreeModel {
	t.Helper()
	roots := treeFixture()
	agent := roots[0].Children[0].Children[0]
	agent.Label, agent.AgentNamed = "auth-fix", true
	roots[0].Children[1].Children[1].Label = "db"
	m := newTree(t, 24)
	m.roots = roots
	m.relayout("")
	return m
}

// lineWith is the rendered line that holds text, escapes and all.
func lineWith(m TreeModel, text string) string {
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(visible(l), text) {
			return l
		}
	}
	return ""
}

func TestANamedPaneShowsItsNameAfterItsKind(t *testing.T) {
	m := namedTree(t)
	if got := visible(lineWith(m, "auth-fix")); !strings.Contains(got, "claude  auth-fix  ‹ Which do you want?") {
		t.Errorf("agent line = %q", got)
	}
	if got := visible(lineWith(m, "psql billing")); !strings.Contains(got, "shell    db  psql billing") {
		t.Errorf("shell line = %q", got)
	}
	if got := visible(lineWith(m, "Rate limiter")); strings.Contains(got, "claude    ") {
		t.Errorf("an unnamed pane gains no gap: %q", got)
	}
}

// The name is the one thing on the line drawn bold in the accent color, so
// the eye finds it first.
func TestThePaneNameIsHighlighted(t *testing.T) {
	seg := func(text, fg string, bold, dim bool) string {
		return styled(lipgloss.NewStyle(), fg, bold, dim).Render(text)
	}
	got := nameSeg(&tree.Node{Kind: tree.KindPane, Label: "auth-fix"}, seg)
	if !hasSGR(got, "1") || !hasSGR(got, "36") {
		t.Errorf("the name should be bold and cyan: %q", got)
	}
	if nameSeg(&tree.Node{Kind: tree.KindPane}, seg) != "" {
		t.Error("a pane with no name draws nothing")
	}
}

// R renames through the method that set the name, so the agent's name and
// the pane's label never disagree.
func TestRenameGoesBackTheWayTheNameCame(t *testing.T) {
	log := stubOps(t)
	m := namedTree(t)
	m.relayout("pane:w1:p1")
	m = drive(m, "R")
	if m.ask.input.Value() != "auth-fix" || !strings.Contains(statusText(m), "name pane auth-fix:") {
		t.Fatalf("prompt %q value %q", statusText(m), m.ask.input.Value())
	}
	drive(typeText(m, "-2"), "enter")
	m = namedTree(t)
	m.relayout("pane:w2:p2")
	drive(m, "R", "enter")
	want := `rename-agent w1:p1 "auth-fix-2"; rename-pane w2:p2 "db"`
	if got := strings.Join(log.calls, "; "); got != want {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func TestMessagesCallAPaneByItsName(t *testing.T) {
	stubOps(t)
	m := namedTree(t)
	m.relayout("pane:w2:p2")
	if got := statusText(drive(m, "x")); !strings.Contains(got, "close pane db in billing?") {
		t.Errorf("close asks %q", got)
	}
	m = drive(m, "m")
	if got := visible(m.View()); !strings.Contains(got, "move db from billing to") {
		t.Errorf("move title missing in\n%s", got)
	}
	m = namedTree(t)
	m.relayout("pane:w3:p1")
	m = drive(m, "m")
	if got := visible(lineWith(m, "Which do you want?")); !strings.Contains(got, "beside claude   auth-fix  ‹ Which do you want?") {
		t.Errorf("move panel line = %q", got)
	}
}
