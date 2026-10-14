//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// focusCalls is the CLI calls a jump makes, in order. A jump to an agent
// focuses its workspace and tab first, so the target shows when it is in a
// space that is off screen, and `agent focus` comes last.
func focusCalls(e *env) []string {
	var out []string
	for _, c := range e.calls() {
		switch {
		case len(c) >= 3 && (c[0] == "workspace" || c[0] == "tab" || c[0] == "agent") && c[1] == "focus":
			out = append(out, strings.Join(c, " "))
		}
	}
	return out
}

func TestListRendersEveryAgent(t *testing.T) {
	tm := newEnv(t).start()
	tm.waitFor("agents · last message", "9 items")
	tm.waitFor(
		"api-gateway", "platform ⑂ auth-service  ▸ oauth", "platform ⑂ billing  ▸ Invoices",
		"web-client  @redesign-nav", "infra-terraform / infra", "scratchpad",
		"Rate limiter is in.", "Which do you want?", "Done for now.",
	)
	tm.send("q")
	if code := tm.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestListEnterJumpsToTheFirstAgent(t *testing.T) {
	e := newEnv(t)
	tm := e.start()
	tm.waitFor("9 items")
	tm.send("enter")
	if code := tm.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := []string{"workspace focus w1", "tab focus w1:t1", "agent focus w1:p1"}
	if got := focusCalls(e); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

func TestListArrowKeysMoveTheSelection(t *testing.T) {
	e := newEnv(t)
	tm := e.start()
	tm.waitFor("9 items")
	tm.send("down", "down", "j", "down", "up") // the fourth row: billing's Reports tab
	tm.send("enter")
	tm.exit()
	want := []string{"workspace focus w3", "tab focus w3:t3", "agent focus w3:p9"}
	if got := focusCalls(e); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

func TestListQuitKeysJumpNowhere(t *testing.T) {
	for name, key := range map[string]string{"q": "q", "esc": "esc", "ctrl+c": "\x03"} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			tm := e.start()
			tm.waitFor("9 items")
			tm.send(key)
			if code := tm.exit(); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if got := focusCalls(e); len(got) != 0 {
				t.Fatalf("a quit moved focus: %q", got)
			}
		})
	}
}

func TestListFilterNarrowsBySubstring(t *testing.T) {
	e := newEnv(t)
	tm := e.start()
	tm.waitFor("9 items")
	tm.send("/")
	tm.typeText("reports")
	tm.waitFor("1 item", "Monthly report")
	tm.waitGone("api-gateway", "scratchpad")
	tm.send("enter", "enter") // apply the filter, then jump
	tm.exit()
	if got := focusCalls(e); got[len(got)-1] != "agent focus w3:p9" {
		t.Fatalf("calls %q", got)
	}
}

func TestListFilterCancels(t *testing.T) {
	tm := newEnv(t).start()
	tm.waitFor("9 items")
	tm.send("/")
	tm.typeText("billing")
	tm.waitGone("scratchpad")
	tm.send("esc")
	tm.waitFor("scratchpad", "9 items")
}

func TestListStatusKeys(t *testing.T) {
	tm := newEnv(t).start()
	tm.waitFor("9 items")

	tm.send("b")
	tm.waitFor("auth-service")
	tm.waitGone("api-gateway", "scratchpad")

	tm.send("w")
	tm.waitFor("api-gateway")
	tm.waitGone("auth-service")

	tm.send("d")
	tm.waitFor("billing  ▸ Invoices")
	tm.waitGone("billing  ▸ Reports")

	tm.send("i")
	tm.waitFor("scratchpad")
	tm.waitGone("api-gateway")

	tm.send("a")
	tm.waitFor("api-gateway", "scratchpad", "9 items")
}

// The sort key changes the order on screen, and the choice is remembered for
// the next opening, which is what the state directory is for.
func TestListSortKeyCyclesAndIsRemembered(t *testing.T) {
	e := newEnv(t)
	tm := e.start()
	tm.waitFor("agents · last message")
	tm.send("s")
	tm.waitFor("agents · needs attention")
	tm.waitGone("agents · last message")
	tm.send("q")
	tm.exit()

	b, err := os.ReadFile(filepath.Join(e.state, "sort-mode"))
	if err != nil || strings.TrimSpace(string(b)) != "attention" {
		t.Fatalf("remembered %q, %v", b, err)
	}
	if got := panes(e.run("--list")); got[0] != "w2:p1" {
		t.Fatalf("the next opening did not use the remembered mode: %v", got)
	}

	// S steps back through the modes.
	tm = e.start()
	tm.waitFor("agents · needs attention")
	tm.send("S")
	tm.waitFor("agents · last message")
	tm.send("q")
	tm.exit()
}

func TestListStartsInTheRequestedSortAndStatus(t *testing.T) {
	tm := newEnv(t).start("--sort", "oldest", "--status", "idle")
	tm.waitFor("scratchpad")
	tm.waitGone("api-gateway", "auth-service")
	tm.send("q")
}

func TestListRefreshReadsTheSessionAgain(t *testing.T) {
	e := newEnv(t)
	tm := e.start()
	tm.waitFor("● blocked")

	snap := filepath.Join(e.dir, "snapshot.json")
	b, _ := os.ReadFile(snap)
	if err := os.WriteFile(snap, []byte(strings.ReplaceAll(string(b), `"blocked"`, `"idle"`)), 0o644); err != nil {
		t.Fatal(err)
	}
	tm.send("r")
	tm.waitGone("● blocked")
	tm.send("q")
}

func TestListFocusFailureNotifies(t *testing.T) {
	e := newEnv(t).with("FAKE_HERDR_FAIL=agent focus")
	tm := e.start()
	tm.waitFor("9 items")
	tm.send("enter")
	if code := tm.exit(); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	notes := e.callsWith("notification", "show")
	if len(notes) != 1 || !strings.Contains(strings.Join(notes[0], " "), "agent focus w1:p1") {
		t.Fatalf("notifications: %v", notes)
	}
}

// With no terminal to draw on, the failure to read the snapshot has only one
// place to go.
func TestUnreadableSnapshotRaisesANotification(t *testing.T) {
	e := newEnv(t).with("FAKE_HERDR_FAIL=api snapshot")
	r := e.run()
	if r.code != 1 || !strings.Contains(r.stderr, "herdr api snapshot") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if !e.called("notification", "show", "switcher+ could not open") {
		t.Fatalf("calls: %v", e.calls())
	}
}

func TestListShowsAnEmptySession(t *testing.T) {
	e := newEnv(t)
	empty := `{"id":"x","result":{"snapshot":{"agents":[],"workspaces":[],"tabs":[],"panes":[]}}}`
	if err := os.WriteFile(filepath.Join(e.dir, "snapshot.json"), []byte(empty), 0o644); err != nil {
		t.Fatal(err)
	}
	tm := e.start()
	tm.waitFor("No items")
	tm.send("q")
	if code := tm.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

// The layout config is Herdr's own sidebar vocabulary. Two rows of two tokens
// each draw two lines for every agent, and a token left out draws nothing.
func TestListDrawsTheLayoutFromTheConfig(t *testing.T) {
	e := newEnv(t)
	e.writeConfig(`
[ui]
rows = [
  [{ token = "age" }, { token = "space", bold = true }, { token = "state_text" }],
  [{ token = "message" }],
]
`)
	tm := e.start()
	tm.waitFor("api-gateway", "working", "Rate limiter is in.")
	// The default look has a state icon and a role mark, and this layout names
	// neither.
	tm.waitGone("●", "‹")
	tm.send("q")
}

func TestListHonoursAColourRule(t *testing.T) {
	e := newEnv(t).with("COLORTERM=truecolor")
	e.writeConfig(`
[ui]
rows = [[{ token = "space" }, { token = "age", rules = [{ gt = 60, fg = "#ff6188" }] }]]
`)
	tm := e.start()
	tm.waitFor("api-gateway", "9d")
	tm.send("q")
	tm.exit()
	// Truecolor spells #ff6188 as the sequence 38-2-255-97-136. Only the ages over an hour
	// can carry it, so a missing sequence means the rule did not apply.
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if !strings.Contains(tm.raw.String(), "38;2;255;97;136") {
		t.Error("an age over an hour was not drawn in the rule's colour")
	}
}

// ---- the tree ----

func TestTreeRenders(t *testing.T) {
	tm := newEnv(t).start("--tree")
	tm.waitFor(
		"agents · tree", "▾ platform", "▾ ⑂ auth-service", "▾ ⑂ billing", "▾ ⑂ notifications",
		"· shell    db  psql billing", "· shell    npm run dev", "Done for now.",
	)
	tm.send("q")
	if code := tm.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestTreeIsWhatTheConfigAsksFor(t *testing.T) {
	e := newEnv(t)
	e.writeConfig("[ui]\nview = \"tree\"\n")
	tm := e.start()
	tm.waitFor("agents · tree")
	tm.send("q")
}

func TestTreeEnterOnAnAgentUsesAgentFocus(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("down", "enter") // the first line is the workspace, the next is its agent
	tm.exit()
	want := []string{"workspace focus w1", "tab focus w1:t1", "agent focus w1:p1"}
	if got := focusCalls(e); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls %q, want %q", got, want)
	}
}

// A shell is no agent, and `agent focus` rejects it, so the jump goes over the
// socket instead.
func TestTreeEnterOnAShellFocusesOverTheSocket(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("down", "down", "enter")
	if code := tm.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
	got := e.socket.requests("pane.focus")
	if len(got) != 1 || got[0].Params["pane_id"] != "w1:p2" {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	for _, c := range focusCalls(e) {
		if strings.HasPrefix(c, "agent") {
			t.Fatalf("a shell went through agent focus: %q", c)
		}
	}
}

func TestTreeShellJumpFailureNotifies(t *testing.T) {
	e := newEnv(t)
	e.socket.failing("pane.focus", "pane_not_found")
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("down", "down", "enter")
	if code := tm.exit(); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	notes := e.callsWith("notification", "show")
	if len(notes) != 1 || !strings.Contains(strings.Join(notes[0], " "), "pane_not_found") {
		t.Fatalf("notifications: %v", notes)
	}
}

func TestTreeFoldKeys(t *testing.T) {
	tm := newEnv(t).start("--tree")
	tm.waitFor("Rate limiter is in.", "psql billing")

	tm.send("enter") // on the first group: fold it
	tm.waitGone("Rate limiter is in.")
	tm.waitFor("psql billing")
	tm.send("space") // open it again
	tm.waitFor("Rate limiter is in.")

	tm.send("c") // fold everything
	tm.waitGone("Rate limiter is in.", "psql billing", "▾")
	tm.waitFor("▸ platform", "▸ scratchpad")
	tm.send("e") // open everything
	tm.waitFor("Rate limiter is in.", "psql billing", "Done for now.")

	tm.send("W") // fold the workspaces and keep the repository
	tm.waitGone("▾ ⑂ billing", "psql billing")
	tm.waitFor("▾ platform")
	tm.send("W")
	tm.waitFor("psql billing")

	tm.send("T") // fold the repositories
	tm.waitGone("▾ ⑂ billing")
	tm.waitFor("▸ platform")
	tm.send("q")
}

func TestTreeMovementKeys(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("G", "enter") // the last line is scratchpad's agent
	tm.exit()
	if got := focusCalls(e); got[len(got)-1] != "agent focus w8:p1" {
		t.Fatalf("calls %q", got)
	}
}

func TestTreeStatusKeys(t *testing.T) {
	tm := newEnv(t).start("--tree")
	tm.waitFor("platform")
	tm.send("b")
	tm.waitFor("auth-service", "Which do you want?")
	tm.waitGone("billing", "psql billing", "scratchpad")
	tm.send("a")
	tm.waitFor("psql billing", "scratchpad")
	tm.send("q")
}

func TestTreeSearch(t *testing.T) {
	tm := newEnv(t).start("--tree")
	tm.waitFor("platform")
	tm.send("/")
	tm.typeText("psql")
	tm.waitFor("psql billing", "billing")
	tm.waitGone("npm run dev", "scratchpad")
	tm.send("esc") // the first esc clears the search, and keeps the switcher open
	tm.waitFor("npm run dev", "scratchpad")
	tm.send("q")
	if code := tm.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

func TestTreeManageHelpToggles(t *testing.T) {
	tm := newEnv(t).start("--tree")
	tm.waitFor("a/b/w/i/d status")
	tm.send("?")
	tm.waitFor("n new space", "R rename", "x close")
	tm.send("?")
	tm.waitFor("a/b/w/i/d status")
	tm.send("q")
}

// ---- changes the tree makes to the session ----

func TestTreeRenamesAWorkspace(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("R") // the cursor starts on the api-gateway workspace
	tm.waitFor("rename workspace api-gateway:")
	tm.typeText("-edge")
	tm.send("enter")
	tm.waitFor("renamed the workspace")

	got := e.socket.requests("workspace.rename")
	want := map[string]any{"workspace_id": "w1", "label": "api-gateway-edge"}
	if len(got) != 1 || !reflect.DeepEqual(got[0].Params, want) {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	tm.send("q")
}

func TestTreeRenamesAPane(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("down", "down", "R") // the shell in api-gateway, which has no name
	tm.waitFor("name pane")
	tm.typeText("tests")
	tm.send("enter")
	tm.waitFor("renamed the pane")

	got := e.socket.requests("pane.rename")
	want := map[string]any{"pane_id": "w1:p2", "label": "tests"}
	if len(got) != 1 || !reflect.DeepEqual(got[0].Params, want) {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	tm.send("q")
}

func TestTreeRenameCanBeCancelled(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("R")
	tm.waitFor("rename workspace")
	tm.typeText("x")
	tm.send("esc")
	tm.waitFor("left as it was")
	if got := e.socket.seen(); len(got) != 0 {
		t.Fatalf("a cancelled rename sent %+v", got)
	}
	tm.send("q")
}

func TestTreeShowsWhyAChangeFailed(t *testing.T) {
	e := newEnv(t)
	e.socket.failing("workspace.rename", "workspace_not_found")
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("R")
	tm.typeText("x")
	tm.send("enter")
	tm.waitFor("workspace_not_found")
	tm.send("q")
	if code := tm.exit(); code != 0 {
		t.Fatalf("exit %d", code)
	}
}

// Closing is the one change that cannot be undone, so it asks, and only a y
// answers yes.
func TestTreeClosesAPaneOnlyAfterConfirming(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")

	tm.send("down", "down", "x")
	tm.waitFor("close pane", "y/n")
	tm.send("n")
	tm.waitFor("left as it was")
	if got := e.socket.requests("pane.close"); len(got) != 0 {
		t.Fatalf("a refused close was sent: %+v", got)
	}

	tm.send("x")
	tm.waitFor("y/n")
	tm.send("y")
	tm.waitFor("closed")
	got := e.socket.requests("pane.close")
	if len(got) != 1 || got[0].Params["pane_id"] != "w1:p2" {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	tm.send("q")
}

func TestTreeClosesAWorkspaceAfterConfirming(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("x")
	tm.waitFor("close workspace api-gateway and its 2 panes?")
	tm.send("y")
	tm.waitFor("closed api-gateway")
	got := e.socket.requests("workspace.close")
	if len(got) != 1 || got[0].Params["workspace_id"] != "w1" {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	tm.send("q")
}

func TestTreeSplitsAPane(t *testing.T) {
	cases := []struct{ key, direction string }{{"v", "right"}, {"s", "down"}}
	for _, c := range cases {
		t.Run(c.direction, func(t *testing.T) {
			e := newEnv(t)
			tm := e.start("--tree")
			tm.waitFor("platform")
			tm.send("down", c.key)
			tm.waitFor("split")
			got := e.socket.requests("pane.split")
			if len(got) != 1 || got[0].Params["target_pane_id"] != "w1:p1" || got[0].Params["direction"] != c.direction {
				t.Fatalf("socket requests: %+v", e.socket.seen())
			}
			// Splitting does not move the user: the new pane opens unfocused.
			if got[0].Params["focus"] != false {
				t.Errorf("the split took focus: %+v", got[0].Params)
			}
			tm.send("q")
		})
	}
}

func TestTreeOpensATab(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("t")
	tm.waitFor("opened a tab in api-gateway")
	got := e.socket.requests("tab.create")
	if len(got) != 1 || got[0].Params["workspace_id"] != "w1" {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	tm.send("q")
}

func TestTreeOpensAWorkspaceInADirectory(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("n")
	tm.waitFor("new workspace in")
	tm.typeText("scratch")
	tm.send("enter")
	tm.waitFor("opened a workspace")
	got := e.socket.requests("workspace.create")
	if len(got) != 1 || got[0].Params["label"] != "scratch" ||
		got[0].Params["cwd"] != filepath.Join(e.dir, "checkouts", "api-gateway") {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	tm.send("q")
}

func TestTreeMovesAPane(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("down", "down", "m") // the shell in api-gateway
	tm.waitFor("move shell from api-gateway to", "+ a new workspace")
	tm.send("enter") // the first target is a new workspace, and it asks for a name
	tm.waitFor("move shell to a new workspace, name:")
	tm.typeText("side")
	tm.send("enter")
	tm.waitFor("moved shell to a new workspace")
	got := e.socket.requests("pane.move")
	if len(got) != 1 || got[0].Params["pane_id"] != "w1:p2" {
		t.Fatalf("socket requests: %+v", e.socket.seen())
	}
	if dest, _ := got[0].Params["destination"].(map[string]any); dest["type"] != "new_workspace" || dest["label"] != "side" {
		t.Fatalf("destination: %+v", got[0].Params)
	}
	tm.send("q")
}

func TestTreeSessionPanel(t *testing.T) {
	e := newEnv(t)
	tm := e.start("--tree")
	tm.waitFor("platform")
	tm.send("S")
	tm.waitFor("default", "this one", "spike", "release-0.2", "stopped")

	// The session the switcher runs in cannot be stopped from inside it.
	tm.send("x")
	tm.waitFor("the switcher runs in default")

	tm.send("down", "x") // spike, which is running
	tm.waitFor("stop session spike and every pane in it? y/n")
	tm.send("y")
	tm.waitFor("stopped spike")
	if !e.called("session", "stop", "spike") {
		t.Fatalf("calls: %v", e.calls())
	}

	tm.send("D") // delete a running one: refused
	tm.waitFor("spike is running, so stop it first")
	tm.send("down", "D")
	tm.waitFor("delete session release-0.2? y/n")
	tm.send("y")
	tm.waitFor("deleted release-0.2")
	if !e.called("session", "delete", "release-0.2") {
		t.Fatalf("calls: %v", e.calls())
	}

	tm.send("esc")
	tm.waitFor("agents · tree")
	tm.send("q")
}
