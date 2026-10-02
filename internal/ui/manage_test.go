package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	tea "github.com/charmbracelet/bubbletea"
)

// opsLog replaces every Herdr change with a recorder. Each call is kept as
// one string, and err is what every call returns.
type opsLog struct {
	calls    []string
	err      error
	sessions []herdr.NamedSession
}

func stubOps(t *testing.T) *opsLog {
	t.Helper()
	l := &opsLog{}
	rec := func(format string, args ...any) error {
		l.calls = append(l.calls, fmt.Sprintf(format, args...))
		return l.err
	}
	prev, prevTree := ops, collectTree
	ops = herdrOps{
		createWorkspace: func(dir, label string) error { return rec("create %s %q", dir, label) },
		renameWorkspace: func(id, label string) error { return rec("rename-ws %s %q", id, label) },
		closeWorkspace:  func(id string) error { return rec("close-ws %s", id) },
		createTab:       func(ws, dir string) error { return rec("tab %s %s", ws, dir) },
		renamePane:      func(id, label string) error { return rec("rename-pane %s %q", id, label) },
		renameAgent:     func(id, name string) error { return rec("rename-agent %s %q", id, name) },
		closePane:       func(id string) error { return rec("close-pane %s", id) },
		splitPane:       func(id, d, dir string) error { return rec("split %s %s %s", id, d, dir) },
		moveToWorkspace: func(id, ws string) error { return rec("move %s to %s", id, ws) },
		moveBeside: func(id, tab, target, split string) error {
			return rec("move %s beside %s in %s %s", id, target, tab, split)
		},
		moveToNew:     func(id, label string) error { return rec("move %s out %q", id, label) },
		sessions:      func() ([]herdr.NamedSession, error) { return l.sessions, l.err },
		stopSession:   func(name string) error { return rec("stop %s", name) },
		deleteSession: func(name string) error { return rec("delete %s", name) },
	}
	collectTree = func() ([]*tree.Node, error) { return treeFixture(), nil }
	t.Cleanup(func() { ops, collectTree = prev, prevTree })
	return l
}

// drive presses each key and runs every command that comes back, feeding its
// message to the model, the way the Bubble Tea loop would.
func drive(m TreeModel, keys ...string) TreeModel {
	for _, k := range keys {
		next, cmd := m.Update(treeKey(k))
		m = settle(next, cmd)
	}
	return m
}

func settle(next tea.Model, cmd tea.Cmd) TreeModel {
	for cmd != nil {
		msg := cmd()
		if _, quit := msg.(tea.QuitMsg); quit {
			break
		}
		next, cmd = next.Update(msg)
	}
	return next.(TreeModel)
}

// statusText is the line above the pager.
func statusText(m TreeModel) string {
	lines := strings.Split(visible(m.View()), "\n")
	return strings.TrimSpace(lines[len(lines)-3])
}

func helpText(m TreeModel) string {
	lines := strings.Split(visible(m.View()), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// typeText sends each rune of s to the prompt.
func typeText(m TreeModel, s string) TreeModel {
	for _, r := range s {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(TreeModel)
	}
	return m
}

// The fixture's lines, top to bottom: platform, auth-service, its claude,
// billing, its claude, its shell, api-gateway, its claude.
const (
	atRepo      = 0
	atAuth      = 1
	atAuthAgent = 2
	atBilling   = 3
	atShell     = 5
)

func at(t *testing.T, line int) TreeModel {
	t.Helper()
	m := newTree(t, 24)
	for range line {
		m = pressTree(m, "down")
	}
	return m
}

func TestQuestionMarkSwapsTheFooter(t *testing.T) {
	m := newTree(t, 24)
	if !strings.Contains(helpText(m), "? manage") {
		t.Errorf("help = %q", helpText(m))
	}
	m = pressTree(m, "?")
	if helpText(m) != manageHelp {
		t.Errorf("help = %q", helpText(m))
	}
	if m = pressTree(m, "?"); helpText(m) == manageHelp {
		t.Error("? should swap back")
	}
}

func TestNewWorkspaceAsksForANameThenOpensIt(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atBilling), "n")
	if !strings.Contains(statusText(m), "new workspace in /co/billing") || helpText(m) != "enter save · esc cancel · blank gives the directory's name" {
		t.Fatalf("status %q help %q", statusText(m), helpText(m))
	}
	m = typeText(m, "spike")
	m = drive(m, "enter")
	if len(log.calls) != 1 || log.calls[0] != `create /co/billing "spike"` {
		t.Errorf("calls = %v", log.calls)
	}
	if statusText(m) != "opened a workspace in /co/billing" {
		t.Errorf("status = %q", statusText(m))
	}
}

func TestEscapeLeavesTheSessionAlone(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atBilling), "n")
	m = drive(typeText(m, "spike"), "esc")
	if len(log.calls) != 0 || statusText(m) != "left as it was" {
		t.Errorf("calls %v status %q", log.calls, statusText(m))
	}
	if m = drive(m, "down"); statusText(m) != "" {
		t.Errorf("the next key clears the notice, got %q", statusText(m))
	}
}

// A long directory is cut from the left, so the input keeps its room.
func TestALongDirectoryLeavesRoomToType(t *testing.T) {
	stubOps(t)
	m := NewTree([]*tree.Node{{Kind: tree.KindWorkspace, ID: "ws:w9", Name: "deep", WorkspaceID: "w9",
		Dir: "/var/folders/5t/qj8fdg8138xb335pd89qxm7r0000gn/T/tmp.beiZ92JnAJ/checkouts/auth-service"}}, agents.StatusAll)
	m = drive(m, "n")
	if !strings.Contains(m.ask.question, "…") || !strings.HasSuffix(m.ask.question, "checkouts/auth-service, name:") {
		t.Errorf("question = %q", m.ask.question)
	}
	if got := len([]rune(clipLeft(strings.Repeat("a", 60), 40))); got != 40 {
		t.Errorf("clipped to %d runes", got)
	}
}

func TestANodeWithNoDirectoryOpensNoWorkspace(t *testing.T) {
	log := stubOps(t)
	m := NewTree([]*tree.Node{{Kind: tree.KindWorkspace, ID: "ws:w9", Name: "scratch", WorkspaceID: "w9"}}, agents.StatusAll)
	m = drive(m, "n")
	if len(log.calls) != 0 || m.ask != nil || !strings.Contains(m.notice, "no directory") {
		t.Errorf("calls %v notice %q", log.calls, m.notice)
	}
}

func TestRenameStartsFromTheCurrentName(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atBilling), "R")
	if got := m.ask.input.Value(); got != "billing" {
		t.Fatalf("prefilled %q", got)
	}
	// Herdr refuses a blank workspace name, so the footer offers no blank.
	if helpText(m) != "enter save · esc cancel" {
		t.Errorf("help = %q", helpText(m))
	}
	m = drive(typeText(m, "-v2"), "enter")
	if len(log.calls) != 1 || log.calls[0] != `rename-ws w2 "billing-v2"` {
		t.Errorf("calls = %v", log.calls)
	}

	log.calls = nil
	m = drive(at(t, atShell), "R")
	m = drive(m, "enter")
	if len(log.calls) != 1 || log.calls[0] != `rename-pane w2:p2 ""` {
		t.Errorf("calls = %v", log.calls)
	}
}

func TestARepositoryCannotBeRenamedOrClosed(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atRepo), "R")
	if m.ask != nil || !strings.Contains(statusText(m), "takes its name from git") {
		t.Errorf("status = %q", statusText(m))
	}
	m = drive(m, "x")
	if m.ask != nil || !strings.Contains(statusText(m), "one at a time") {
		t.Errorf("status = %q", statusText(m))
	}
	if len(log.calls) != 0 {
		t.Errorf("calls = %v", log.calls)
	}
}

func TestCloseAsksFirst(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atBilling), "x")
	if !strings.Contains(statusText(m), "close workspace billing and its 2 panes? y/n") || helpText(m) != "y yes · any other key no" {
		t.Fatalf("status %q help %q", statusText(m), helpText(m))
	}
	if m = drive(m, "n"); len(log.calls) != 0 || statusText(m) != "left as it was" {
		t.Fatalf("a no must change nothing: %v", log.calls)
	}
	m = drive(m, "x", "y")
	if len(log.calls) != 1 || log.calls[0] != "close-ws w2" || statusText(m) != "closed billing" {
		t.Errorf("calls %v status %q", log.calls, statusText(m))
	}
}

func TestClosingAnAgentPaneSaysTheAgentStops(t *testing.T) {
	log := stubOps(t)
	m := drive(at(t, atAuthAgent), "x")
	if !strings.Contains(statusText(m), "close pane claude in auth-service and stop its agent? y/n") {
		t.Fatalf("status = %q", statusText(m))
	}
	drive(m, "y")
	if len(log.calls) != 1 || log.calls[0] != "close-pane w1:p1" {
		t.Errorf("calls = %v", log.calls)
	}
	m = drive(at(t, atShell), "x")
	if strings.Contains(statusText(m), "agent") {
		t.Errorf("a shell has no agent to stop: %q", statusText(m))
	}
}

// Herdr's answer reaches the status line, and the tree is not reread.
func TestAFailedChangeShowsTheError(t *testing.T) {
	log := stubOps(t)
	log.err = errors.New("herdr pane.close w1:p1: pane_not_found")
	reread := false
	collectTree = func() ([]*tree.Node, error) { reread = true; return treeFixture(), nil }
	m := drive(at(t, atAuthAgent), "x", "y")
	if m.notice != log.err.Error() || !m.noticeErr || reread {
		t.Errorf("notice %q reread %v", m.notice, reread)
	}
	if !strings.Contains(statusText(m), "pane_not_found") {
		t.Errorf("status = %q", statusText(m))
	}
}

func TestTabAndSplitNeedTheRightNode(t *testing.T) {
	log := stubOps(t)
	drive(at(t, atBilling), "t")
	drive(at(t, atShell), "v")
	drive(at(t, atAuthAgent), "s")
	want := []string{"tab w2 /co/billing", "split w2:p2 right /co/billing", "split w1:p1 down /co/auth"}
	if strings.Join(log.calls, "; ") != strings.Join(want, "; ") {
		t.Errorf("calls = %v", log.calls)
	}

	log.calls = nil
	if m := drive(at(t, atRepo), "t"); !strings.Contains(statusText(m), "open a tab") {
		t.Errorf("status = %q", statusText(m))
	}
	if m := drive(at(t, atBilling), "v"); !strings.Contains(statusText(m), "move to a pane") {
		t.Errorf("status = %q", statusText(m))
	}
	if len(log.calls) != 0 {
		t.Errorf("calls = %v", log.calls)
	}
}

func TestATabFromAPaneNamesItsWorkspace(t *testing.T) {
	stubOps(t)
	if m := drive(at(t, atShell), "t"); statusText(m) != "opened a tab in billing" {
		t.Errorf("status = %q", statusText(m))
	}
	m := NewTree(nil, agents.StatusAll)
	if got := m.spaceName(&tree.Node{Kind: tree.KindPane, WorkspaceID: "w9"}); got != "w9" {
		t.Errorf("an unknown space falls back to its ID, got %q", got)
	}
}

// Closing the line under the cursor leaves the cursor at the same height.
func TestTheCursorStaysPutWhenItsNodeGoes(t *testing.T) {
	stubOps(t)
	collectTree = func() ([]*tree.Node, error) {
		roots := treeFixture()
		billing := roots[0].Children[1]
		billing.Children = billing.Children[:1]
		return roots, nil
	}
	m := drive(at(t, atShell), "x", "y")
	if m.cursor != atShell {
		t.Errorf("cursor = %d, want %d", m.cursor, atShell)
	}
}

func TestCtrlCQuitsFromAQuestion(t *testing.T) {
	stubOps(t)
	for _, key := range []string{"n", "x"} {
		m := drive(at(t, atBilling), key)
		next, cmd := m.Update(treeKey("ctrl+c"))
		if cmd == nil || !next.(TreeModel).quitting {
			t.Errorf("%s: ctrl+c should quit", key)
		}
	}
}

func TestEditingKeysReachThePrompt(t *testing.T) {
	stubOps(t)
	m := drive(at(t, atBilling), "R")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := next.(TreeModel).ask.input.Value(); got != "billin" {
		t.Errorf("value = %q", got)
	}
}

func sessionFixture() []herdr.NamedSession {
	return []herdr.NamedSession{
		{Name: "default", Running: true, Default: true, Dir: "/h/.config/herdr", Socket: "/h/.config/herdr/herdr.sock"},
		{Name: "spike", Running: true, Dir: "/h/s/spike", Socket: "/h/s/spike/herdr.sock"},
		{Name: "old", Dir: "/h/s/old", Socket: "/h/s/old/herdr.sock"},
	}
}

func openSessions(t *testing.T) (TreeModel, *opsLog) {
	t.Helper()
	t.Setenv("HERDR_SOCKET_PATH", "/h/.config/herdr/herdr.sock")
	log := stubOps(t)
	log.sessions = sessionFixture()
	return drive(newTree(t, 24), "S"), log
}

func TestTheSessionPanelListsEverySession(t *testing.T) {
	m, _ := openSessions(t)
	got := strings.Join(shown(m), "\n")
	for _, want := range []string{"herdr sessions", "default  ● running  this one", "spike    ● running", "old      ○ stopped"} {
		if !strings.Contains(visible(m.View()), want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if helpText(m) != sessionsHelp {
		t.Errorf("help = %q", helpText(m))
	}
}

func TestTheSessionPanelShowsTheDefaultMark(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "/elsewhere.sock")
	log := stubOps(t)
	log.sessions = sessionFixture()
	m := drive(newTree(t, 24), "S")
	if !strings.Contains(visible(m.View()), "default  ● running  default") {
		t.Errorf("got\n%s", visible(m.View()))
	}
}

func TestTheCurrentSessionCannotBeStopped(t *testing.T) {
	m, log := openSessions(t)
	m = drive(m, "x")
	if m.ask != nil || !strings.Contains(statusText(m), "the switcher runs in default") {
		t.Errorf("status = %q", statusText(m))
	}
	if m = drive(m, "D"); !strings.Contains(statusText(m), "stop it first") {
		t.Errorf("status = %q", statusText(m))
	}
	if len(log.calls) != 0 {
		t.Errorf("calls = %v", log.calls)
	}
}

func TestStopThenDeleteASession(t *testing.T) {
	m, log := openSessions(t)
	m = drive(m, "down", "x")
	if !strings.Contains(statusText(m), "stop session spike and every pane in it? y/n") {
		t.Fatalf("status = %q", statusText(m))
	}
	m = drive(m, "y")
	if statusText(m) != "stopped spike" || m.sessions == nil {
		t.Errorf("status %q", statusText(m))
	}
	m = drive(m, "down", "x")
	if !strings.Contains(statusText(m), "old is already stopped") {
		t.Errorf("status = %q", statusText(m))
	}
	m = drive(m, "D", "y")
	if strings.Join(log.calls, "; ") != "stop spike; delete old" || statusText(m) != "deleted old" {
		t.Errorf("calls %v status %q", log.calls, statusText(m))
	}
}

func TestTheSessionCursorClampsAndFollowsTheList(t *testing.T) {
	m, log := openSessions(t)
	m = drive(m, "up", "down", "down", "down", "down")
	if m.sessions.cursor != 2 {
		t.Errorf("cursor = %d", m.sessions.cursor)
	}
	log.sessions = log.sessions[:1]
	if m = drive(m, "r"); m.sessions.cursor != 0 {
		t.Errorf("a shorter list pulls the cursor in, got %d", m.sessions.cursor)
	}
}

func TestTheSessionPanelClosesBackToTheTree(t *testing.T) {
	for _, key := range []string{"esc", "S"} {
		m, _ := openSessions(t)
		if m = drive(m, key); m.sessions != nil || !strings.Contains(visible(m.View()), "agents · tree") {
			t.Errorf("%s should close the panel", key)
		}
	}
	m, _ := openSessions(t)
	if _, cmd := m.Update(treeKey("q")); cmd == nil {
		t.Error("q should quit from the panel")
	}
}

func TestAFailedSessionListShowsTheError(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "/h/.config/herdr/herdr.sock")
	log := stubOps(t)
	log.err = errors.New("herdr session list: no herdr")
	m := drive(newTree(t, 24), "S")
	if !m.noticeErr || !strings.Contains(visible(m.View()), "no sessions") {
		t.Errorf("notice %q", m.notice)
	}
	// Keys on an empty panel do nothing rather than fail.
	m = drive(m, "x", "D")
	if m.ask != nil {
		t.Error("nothing to ask about")
	}
}

// A session list that arrives after the panel closed is dropped.
func TestALateSessionListIsIgnored(t *testing.T) {
	m := newTree(t, 24)
	next, _ := m.Update(sessionsMsg{list: sessionFixture()})
	if next.(TreeModel).sessions != nil {
		t.Error("the panel should stay closed")
	}
}
