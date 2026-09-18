package herdr

import (
	"errors"
	"strings"
	"testing"
)

// stub replaces the command runner for one test and restores it afterwards.
func stub(t *testing.T, fn func(string, ...string) ([]byte, error)) *[][]string {
	t.Helper()
	var calls [][]string
	prev := run
	run = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string{name}, args...))
		return fn(name, args...)
	}
	t.Cleanup(func() { run = prev })
	return &calls
}

const body = `{"id":"x","result":{"snapshot":{
  "focused_pane_id":"w5:p1",
  "workspaces":[{"workspace_id":"w5","label":"infra","number":1},{"workspace_id":"w9","label":"","number":4}],
  "agents":[{"agent":"claude","agent_status":"idle","pane_id":"w5:p1","tab_id":"w5:t1","workspace_id":"w5",
             "cwd":"/src/nix","terminal_title_stripped":"infra","state_change_seq":400,
             "agent_session":{"kind":"id","value":"uuid-1"}}]}}}`

func TestParseSnapshot(t *testing.T) {
	s, err := ParseSnapshot([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(s.Agents))
	}
	a := s.Agents[0]
	if a.PaneID != "w5:p1" || a.Kind != "claude" || a.Status != "idle" {
		t.Errorf("agent decoded wrong: %+v", a)
	}
	if a.Session.Value != "uuid-1" {
		t.Errorf("session value = %q, want uuid-1", a.Session.Value)
	}
	if a.StateChangeSeq != 400 {
		t.Errorf("state_change_seq = %d, want 400", a.StateChangeSeq)
	}
	if s.FocusedPaneID != "w5:p1" {
		t.Errorf("focused pane = %q", s.FocusedPaneID)
	}
}

func TestParseSnapshotRejectsGarbage(t *testing.T) {
	if _, err := ParseSnapshot([]byte("not json")); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "parse snapshot") {
		t.Errorf("error should name the step: %v", err)
	}
}

func TestWorkspaceResolvesLabelAndNumber(t *testing.T) {
	s, _ := ParseSnapshot([]byte(body))
	if l, n := s.Workspace("w5"); l != "infra" || n != 1 {
		t.Errorf("got (%q,%d), want (nix,1)", l, n)
	}
}

// A workspace with an empty label falls back to its ID but keeps its real
// position, so space-order sorting still places it correctly.
func TestWorkspaceWithBlankLabelKeepsItsNumber(t *testing.T) {
	s, _ := ParseSnapshot([]byte(body))
	if l, n := s.Workspace("w9"); l != "w9" || n != 4 {
		t.Errorf("got (%q,%d), want (w9,4)", l, n)
	}
}

// An unknown workspace must sort after every known one, never before.
func TestWorkspaceUnknownSortsLast(t *testing.T) {
	s, _ := ParseSnapshot([]byte(body))
	l, n := s.Workspace("nope")
	if l != "nope" {
		t.Errorf("label = %q, want the raw id", l)
	}
	if _, known := s.Workspace("w5"); n <= known {
		t.Errorf("unknown number %d must exceed a known one", n)
	}
}

func TestBinPrefersTheInjectedPath(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "/opt/herdr")
	if got := Bin(); got != "/opt/herdr" {
		t.Errorf("Bin = %q, want /opt/herdr", got)
	}
	t.Setenv("HERDR_BIN_PATH", "")
	if got := Bin(); got != "herdr" {
		t.Errorf("Bin = %q, want herdr", got)
	}
}

func TestLoadCallsApiSnapshot(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "/opt/herdr")
	calls := stub(t, func(string, ...string) ([]byte, error) { return []byte(body), nil })
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Agents) != 1 {
		t.Errorf("got %d agents, want 1", len(s.Agents))
	}
	want := []string{"/opt/herdr", "api", "snapshot"}
	if len(*calls) != 1 || strings.Join((*calls)[0], " ") != strings.Join(want, " ") {
		t.Errorf("calls = %v, want %v", *calls, want)
	}
}

func TestLoadWrapsACommandFailure(t *testing.T) {
	stub(t, func(string, ...string) ([]byte, error) { return nil, errors.New("socket gone") })
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "herdr api snapshot") {
		t.Errorf("error should name the command, got %v", err)
	}
}

// Workspace and tab focus run first so the jump lands even when the target
// sits in a space that is not on screen.
func TestFocusRunsWorkspaceThenTabThenAgent(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "herdr")
	calls := stub(t, func(string, ...string) ([]byte, error) { return nil, nil })
	err := Focus(Agent{PaneID: "w5:p1", TabID: "w5:t1", WorkspaceID: "w5"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"herdr workspace focus w5",
		"herdr tab focus w5:t1",
		"herdr agent focus w5:p1",
	}
	if len(*calls) != 3 {
		t.Fatalf("got %d calls, want 3: %v", len(*calls), *calls)
	}
	for i, w := range want {
		if got := strings.Join((*calls)[i], " "); got != w {
			t.Errorf("call %d = %q, want %q", i, got, w)
		}
	}
}

// The first two steps are best effort. Only the agent focus decides success,
// so a missing workspace must not abort the jump.
func TestFocusToleratesWorkspaceAndTabFailures(t *testing.T) {
	stub(t, func(_ string, args ...string) ([]byte, error) {
		if args[0] == "agent" {
			return nil, nil
		}
		return nil, errors.New("no such target")
	})
	if err := Focus(Agent{PaneID: "w5:p1"}); err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

func TestFocusReportsAnAgentFocusFailure(t *testing.T) {
	stub(t, func(_ string, args ...string) ([]byte, error) {
		if args[0] == "agent" {
			return nil, errors.New("pane closed")
		}
		return nil, nil
	})
	err := Focus(Agent{PaneID: "w5:p1"})
	if err == nil || !strings.Contains(err.Error(), "w5:p1") {
		t.Errorf("error should name the pane, got %v", err)
	}
}

const worktreeBody = `{"id":"x","result":{"snapshot":{
  "workspaces":[
    {"workspace_id":"w5","label":"infra","number":1,
     "worktree":{"repo_name":"infra-terraform","repo_root":"/c/infra-terraform/",
                 "checkout_path":"/c/infra-terraform/","is_linked_worktree":false}},
    {"workspace_id":"w3R","label":"auth-service","number":2,
     "worktree":{"repo_name":"platform","checkout_path":"/c/sf","is_linked_worktree":true}},
    {"workspace_id":"w19","label":"dotfiles","number":3}],
  "agents":[]}}}`

func TestParseSnapshotDecodesTheWorktree(t *testing.T) {
	s, err := ParseSnapshot([]byte(worktreeBody))
	if err != nil {
		t.Fatal(err)
	}
	w := s.FindWorkspace("w5")
	if w == nil || w.Worktree == nil {
		t.Fatal("expected a worktree on w5")
	}
	if w.Worktree.RepoName != "infra-terraform" || w.Worktree.IsLinked {
		t.Errorf("decoded wrong: %+v", w.Worktree)
	}
	if w.Worktree.CheckoutPath != "/c/infra-terraform/" {
		t.Errorf("checkout path = %q", w.Worktree.CheckoutPath)
	}
}

func TestParseSnapshotMarksALinkedWorktree(t *testing.T) {
	s, _ := ParseSnapshot([]byte(worktreeBody))
	w := s.FindWorkspace("w3R")
	if w.Worktree == nil || !w.Worktree.IsLinked {
		t.Errorf("w3R should be a linked worktree: %+v", w.Worktree)
	}
}

// A workspace that is not a git checkout carries no worktree at all, so the
// field stays nil rather than becoming a zero-valued struct.
func TestWorkspaceWithoutAWorktreeIsNil(t *testing.T) {
	s, _ := ParseSnapshot([]byte(worktreeBody))
	if w := s.FindWorkspace("w19"); w == nil || w.Worktree != nil {
		t.Errorf("w19 should have no worktree, got %+v", w)
	}
}

func TestFindWorkspaceMissesCleanly(t *testing.T) {
	s, _ := ParseSnapshot([]byte(worktreeBody))
	if got := s.FindWorkspace("nope"); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}
