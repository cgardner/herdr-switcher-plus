package ui

import (
	"strings"
	"testing"

	"github.com/cgardner/herdr-switcher-plus/internal/tree"
)

// panelText is the visible move panel, title included, without the footer.
func panelText(m TreeModel) string {
	lines := strings.Split(visible(m.View()), "\n")
	var out []string
	for _, l := range lines[:len(lines)-3] {
		if s := strings.TrimSpace(strings.TrimLeft(l, " ▌")); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, "\n")
}

func TestMoveNeedsAPane(t *testing.T) {
	stubOps(t)
	m := drive(at(t, atBilling), "m")
	if m.mover != nil || !strings.Contains(statusText(m), "move to a pane") {
		t.Errorf("status = %q", statusText(m))
	}
}

// A pane alone in its workspace is offered every other workspace and every
// other pane, and never its own workspace, which it fills already.
func TestTheMovePanelListsEveryDestination(t *testing.T) {
	stubOps(t)
	m := drive(at(t, atAuthAgent), "m")
	want := `move claude from auth-service to
+ a new workspace
billing  new tab
beside claude   ‹ Backfill finished.
beside shell    psql billing
api-gateway  new tab
beside claude   ‹ Rate limiter is in.`
	if got := panelText(m); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if helpText(m) != moveHelp {
		t.Errorf("help = %q", helpText(m))
	}
}

func TestMoveToAWorkspaceSaysTheOldOneCloses(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atAuthAgent), "m", "down", "enter")
	if len(log.calls) != 1 || log.calls[0] != "move w1:p1 to w2" {
		t.Fatalf("calls = %v", log.calls)
	}
	if statusText(m) != "moved claude to a new tab in billing, and auth-service closed with no pane left" {
		t.Errorf("status = %q", statusText(m))
	}
	if m.mover != nil {
		t.Error("the panel closes once the move starts")
	}
}

func TestMoveBesideSplitsEitherWay(t *testing.T) {
	log := stubOps(t)
	drive(at(t, atAuthAgent), "m", "down", "down", "enter")
	drive(at(t, atAuthAgent), "m", "down", "down", "down", "s")
	want := "move w1:p1 beside w2:p1 in  right; move w1:p1 beside w2:p2 in  down"
	if got := strings.Join(log.calls, "; "); got != want {
		t.Errorf("calls = %q", got)
	}
}

// s splits down beside a pane, and means nothing on a workspace line.
func TestSOnAWorkspaceLineDoesNothing(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atAuthAgent), "m", "down", "s")
	if len(log.calls) != 0 || m.mover == nil {
		t.Errorf("calls %v, panel open %v", log.calls, m.mover != nil)
	}
}

func TestMoveToANewWorkspaceAsksForItsName(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atAuthAgent), "m", "enter")
	if m.ask == nil || !strings.Contains(statusText(m), "move claude to a new workspace, name:") {
		t.Fatalf("status = %q", statusText(m))
	}
	if !strings.HasSuffix(helpText(m), "blank gives Herdr's default") {
		t.Errorf("help = %q", helpText(m))
	}
	m = drive(typeText(m, "spike"), "enter")
	if len(log.calls) != 1 || log.calls[0] != `move w1:p1 out "spike"` {
		t.Errorf("calls = %v", log.calls)
	}
}

// A pane that shares its tab can move to a tab of its own in the same
// workspace, and the workspace does not close behind it.
func TestAPaneThatSharesItsTabCanTakeATabOfItsOwn(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atShell), "m")
	if !strings.Contains(panelText(m), "billing  new tab\nbeside claude   ‹ Backfill finished.") {
		t.Fatalf("got\n%s", panelText(m))
	}
	m = drive(m, "down", "down", "down", "enter")
	if len(log.calls) != 1 || log.calls[0] != "move w2:p2 to w2" || strings.Contains(statusText(m), "closed") {
		t.Errorf("calls %v status %q", log.calls, statusText(m))
	}
}

// A pane alone in its tab, beside others in its workspace, gets no line for
// its own workspace, so the first pane there names the workspace.
func TestTheOwnWorkspaceIsNamedOnItsFirstPane(t *testing.T) {
	stubOps(t)
	roots := treeFixture()
	billing := roots[0].Children[1]
	billing.Children[0].Pane.TabID, billing.Children[1].Pane.TabID = "w2:t1", "w2:t2"
	m := newTree(t, 24)
	m.roots = roots
	m.relayout("pane:w2:p2")
	m = drive(m, "m")
	got := panelText(m)
	if strings.Contains(got, "billing  new tab") || !strings.Contains(got, "billing  beside claude") {
		t.Errorf("got\n%s", got)
	}
}

func TestTheMovePanelMovesClampsAndCloses(t *testing.T) {
	stubOps(t)
	m := drive(at(t, atAuthAgent), "m", "up", "pgdown")
	if m.mover.cursor != len(m.mover.choices)-1 {
		t.Errorf("cursor = %d", m.mover.cursor)
	}
	if m = drive(m, "down", "pgup"); m.mover.cursor != 0 {
		t.Errorf("cursor = %d", m.mover.cursor)
	}
	for _, key := range []string{"esc", "m"} {
		if m := drive(at(t, atAuthAgent), "m", key); m.mover != nil || !strings.Contains(visible(m.View()), "agents · tree") {
			t.Errorf("%s should close the panel", key)
		}
	}
	if _, cmd := drive(at(t, atAuthAgent), "m").Update(treeKey("q")); cmd == nil {
		t.Error("q should quit from the panel")
	}
}

// A panel longer than the pane turns whole pages, as the tree does.
func TestTheMovePanelPages(t *testing.T) {
	stubOps(t)
	m := drive(at(t, atAuthAgent), "m")
	m.height = treeChrome + 3
	if strings.Contains(panelText(m), "api-gateway") {
		t.Fatalf("the second page should be out of sight:\n%s", panelText(m))
	}
	m = drive(m, "down", "down", "down", "down")
	if !strings.Contains(panelText(m), "api-gateway") || strings.Contains(panelText(m), "a new workspace") {
		t.Errorf("got\n%s", panelText(m))
	}
}

func TestChoiceTextNamesWhatIsInThePane(t *testing.T) {
	if got := choiceText(&tree.Node{Pane: treeFixture()[0].Children[1].Children[1].Pane}); got != "psql billing" {
		t.Errorf("got %q", got)
	}
}
