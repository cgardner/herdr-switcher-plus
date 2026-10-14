//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// panes is the pane ID column of a `--list` listing, in the order printed.
func panes(r result) []string {
	var ids []string
	for _, l := range r.lines() {
		if f := strings.Fields(l); len(f) > 1 {
			ids = append(ids, f[1])
		}
	}
	return ids
}

// The fixture's agents, oldest message last. w3:p9 is the second agent in the
// billing space.
var byRecency = []string{"w1:p1", "w2:p1", "w3:p1", "w3:p9", "w4:p1", "w5:p1", "w6:p1", "w7:p1", "w8:p1"}

func TestVersion(t *testing.T) {
	r := newEnv(t).run("--version")
	// A binary from E2E_BIN carries its real version, and a fresh build "e2e".
	want := "herdr-switcher-plus e2e"
	if os.Getenv("E2E_BIN") != "" {
		want = "herdr-switcher-plus "
	}
	if r.code != 0 || !strings.HasPrefix(strings.TrimSpace(r.stdout), strings.TrimSpace(want)) {
		t.Fatalf("exit %d, stdout %q", r.code, r.stdout)
	}
}

func TestUnknownFlagExitsWithUsage(t *testing.T) {
	r := newEnv(t).run("--no-such-flag")
	if r.code != 2 || !strings.Contains(r.stderr, "flag provided but not defined") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestListOrdersByLastMessage(t *testing.T) {
	r := newEnv(t).run("--list")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if got := panes(r); !reflect.DeepEqual(got, byRecency) {
		t.Fatalf("order %v, want %v\n%s", got, byRecency, r.stdout)
	}
	// The age column reads from the transcript, newest first.
	var ages []string
	for _, l := range r.lines() {
		ages = append(ages, strings.Fields(l)[0])
	}
	if want := []string{"now", "2m", "8m", "20m", "45m", "3h", "1d", "2d", "9d"}; !reflect.DeepEqual(ages, want) {
		t.Fatalf("ages %v, want %v", ages, want)
	}
}

// The one trap in this codebase: a transcript's modification time runs ahead of
// its last message, because Claude Code appends bookkeeping records long after
// the conversation stops. Touching the oldest transcript must not move it.
func TestListIgnoresTranscriptModificationTime(t *testing.T) {
	e := newEnv(t)
	files, _ := filepath.Glob(filepath.Join(e.home, ".claude", "projects", "*scratchpad", "*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("expected one scratchpad transcript, found %v", files)
	}
	now := time.Now()
	if err := os.Chtimes(files[0], now, now); err != nil {
		t.Fatal(err)
	}
	r := e.run("--list")
	got := panes(r)
	if got[len(got)-1] != "w8:p1" {
		t.Fatalf("a touched transcript moved: %v", got)
	}
	if age := strings.Fields(r.lines()[len(got)-1])[0]; age != "9d" {
		t.Fatalf("age %s, want 9d", age)
	}
}

func TestListSortModes(t *testing.T) {
	cases := []struct {
		mode        string
		first, last string
	}{
		{"recent", "w1:p1", "w8:p1"},
		{"oldest", "w8:p1", "w1:p1"},
		// Blocked first, and the agent still working last.
		{"attention", "w2:p1", "w1:p1"},
		{"space", "w1:p1", "w8:p1"},
	}
	for _, c := range cases {
		t.Run(c.mode, func(t *testing.T) {
			r := newEnv(t).run("--list", "--sort", c.mode)
			got := panes(r)
			if r.code != 0 || len(got) != 9 || got[0] != c.first || got[len(got)-1] != c.last {
				t.Fatalf("exit %d, order %v, want %s first and %s last", r.code, got, c.first, c.last)
			}
		})
	}
}

func TestListNameSortIsAlphabetical(t *testing.T) {
	r := newEnv(t).run("--list", "--sort", "name")
	// api-gateway, auth-service, billing twice, dotfiles, infra, notifications,
	// scratchpad, web-client: the order of the space names, not of the labels.
	want := []string{"w1:p1", "w2:p1", "w3:p1", "w3:p9", "w5:p1", "w7:p1", "w6:p1", "w8:p1", "w4:p1"}
	if got := panes(r); !reflect.DeepEqual(got, want) {
		t.Fatalf("order %v, want %v\n%s", got, want, r.stdout)
	}
}

func TestListStatusFilter(t *testing.T) {
	cases := []struct{ flag, pane string }{
		{"blocked", "w2:p1"},
		{"b", "w2:p1"}, // the picker's own key
		{"working", "w1:p1"},
		{"done", "w3:p1"},
	}
	for _, c := range cases {
		t.Run(c.flag, func(t *testing.T) {
			r := newEnv(t).run("--list", "--status", c.flag)
			if got := panes(r); !reflect.DeepEqual(got, []string{c.pane}) {
				t.Fatalf("got %v, want only %s\n%s", got, c.pane, r.stdout)
			}
		})
	}
	t.Run("idle", func(t *testing.T) {
		if got := len(panes(newEnv(t).run("--list", "--status", "idle"))); got != 6 {
			t.Fatalf("%d idle agents, want 6", got)
		}
	})
}

func TestListShowsContextThatIsNotImplied(t *testing.T) {
	r := newEnv(t).run("--list")
	for _, want := range []string{
		"api-gateway",                      // a space named for its repository stays short
		"platform ⑂ auth-service  ▸ oauth", // a linked worktree, and a pane name
		"platform ⑂ billing  ▸ Invoices",   // a tab name tells two agents apart
		"platform ⑂ billing  ▸ Reports",
		"infra-terraform / infra", // an ordinary checkout in another name
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("no %q in\n%s", want, r.stdout)
		}
	}
}

// Herdr reports no branch. The program reads it from the checkout, and a linked
// worktree keeps its HEAD behind a pointer file.
func TestListReadsTheBranchFromGit(t *testing.T) {
	e := newEnv(t)
	checkouts := filepath.Join(e.dir, "checkouts")

	// An ordinary checkout on a branch that its space name does not give away.
	head(t, filepath.Join(checkouts, "web-client", ".git", "HEAD"), "ref: refs/heads/redesign-nav\n")

	// A linked worktree: .git is a file that points at the real directory.
	gitdir := filepath.Join(e.dir, "main-repo", ".git", "worktrees", "notifications")
	head(t, filepath.Join(gitdir, "HEAD"), "ref: refs/heads/digest-batching\n")
	dot := filepath.Join(checkouts, "notifications", ".git")
	if err := os.RemoveAll(dot); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dot, []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A detached HEAD shows a short hash.
	head(t, filepath.Join(checkouts, "infra", ".git", "HEAD"), "0123456789abcdef0123456789abcdef01234567\n")

	out := e.run("--list").stdout
	for _, want := range []string{"web-client  @redesign-nav", "notifications  @digest", "infra  @01234567"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in\n%s", want, out)
		}
	}
	// main is the default branch, so api-gateway does not mention it.
	if strings.Contains(out, "@main") {
		t.Errorf("the default branch is printed:\n%s", out)
	}
}

func head(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSortModePrecedence(t *testing.T) {
	e := newEnv(t)
	first := func(r result) string { return panes(r)[0] }

	// Nothing set: recency.
	if got := first(e.run("--list")); got != "w1:p1" {
		t.Fatalf("default: %s", got)
	}
	// The remembered mode, which the last session left behind.
	if err := os.MkdirAll(e.state, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.state, "sort-mode"), []byte("oldest\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := first(e.run("--list")); got != "w8:p1" {
		t.Fatalf("remembered: %s", got)
	}
	// The environment an action sets beats the remembered mode.
	e.with("HERDR_SWITCHER_PLUS_MODE=attention")
	if got := first(e.run("--list")); got != "w2:p1" {
		t.Fatalf("environment: %s", got)
	}
	// The flag beats both.
	if got := first(e.run("--list", "--sort", "recent")); got != "w1:p1" {
		t.Fatalf("flag: %s", got)
	}
}

func TestTreeListing(t *testing.T) {
	r := newEnv(t).run("--list", "--tree")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	for _, want := range []string{
		"platform\n  ⑂ auth-service\n",        // repository, then workspace
		"[oauth] Two ways to expire",          // a pane name, in brackets
		"shell             [db] psql billing", // a pane that runs no agent
		"shell             npm run dev",
		"infra-terraform / infra",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("no %q in\n%s", want, r.stdout)
		}
	}
	// Three worktrees of one repository sit under one heading.
	heads := 0
	for _, l := range r.lines() {
		if l == "platform" {
			heads++
		}
	}
	if heads != 1 {
		t.Errorf("platform heads %d groups, want 1\n%s", heads, r.stdout)
	}
}

func TestTreeStatusFilter(t *testing.T) {
	r := newEnv(t).run("--list", "--tree", "--status", "blocked")
	if !strings.Contains(r.stdout, "auth-service") || strings.Contains(r.stdout, "billing") || strings.Contains(r.stdout, "psql") {
		t.Fatalf("a blocked filter must keep only the blocked agent:\n%s", r.stdout)
	}
}

func TestViewPrecedence(t *testing.T) {
	isTree := func(r result) bool { return strings.Contains(r.stdout, "\n  ⑂ ") }

	e := newEnv(t)
	e.writeConfig("[ui]\nview = \"tree\"\n")
	if !isTree(e.run("--list")) {
		t.Error("the config's view was ignored")
	}
	if isTree(e.run("--list", "--view", "list")) {
		t.Error("--view did not beat the config")
	}
	e.with("HERDR_SWITCHER_PLUS_VIEW=list")
	if isTree(e.run("--list")) {
		t.Error("the environment did not beat the config")
	}
	if !isTree(e.run("--list", "--tree")) {
		t.Error("--tree did not beat the environment")
	}
}

func TestUnknownViewFallsBackToTheList(t *testing.T) {
	r := newEnv(t).run("--list", "--view", "carousel")
	if r.code != 0 || !strings.Contains(r.stderr, "opening the list") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if got := panes(r); !reflect.DeepEqual(got, byRecency) {
		t.Fatalf("the list did not open: %v", got)
	}
}

// A malformed config stops the switcher. Falling back to the built-in layout
// would leave the reader looking at a pane that ignored their file.
func TestBadConfigStopsTheProgram(t *testing.T) {
	cases := []struct{ name, config, want string }{
		{"syntax", "[ui\n", "config.toml"},
		{"token", "[ui]\nrows = [[{ token = \"nonsense\" }]]\n", "nonsense"},
		{"placement", "[pane]\nplacement = \"sideways\"\n", "placement"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.writeConfig(c.config)
			r := e.run("--list")
			if r.code != 1 || !strings.Contains(r.stderr, c.want) || r.stdout != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
			}
			// --list is read by a person, so it raises no notification.
			if e.called("notification") {
				t.Error("--list raised a notification")
			}
		})
	}
}

func TestGoodConfigIsAccepted(t *testing.T) {
	e := newEnv(t)
	// The default look, spelled in the config's own vocabulary.
	e.writeConfig(`
[ui]
rows = [
  [{ token = "age" }, { token = "space" }],
  [{ token = "message", dim = true }],
]
[pane]
placement = "popup"
width = "70%"
`)
	if r := e.run("--list"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
}

// Herdr cannot be asked about its snapshot from nowhere: a failure to read it
// is the commonest failure, and the reader of --list is at a terminal.
func TestSnapshotFailureIsReported(t *testing.T) {
	e := newEnv(t).with("FAKE_HERDR_FAIL=api snapshot")
	r := e.run("--list")
	if r.code != 1 || !strings.Contains(r.stderr, "herdr api snapshot") {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
	if e.called("notification") {
		t.Error("--list raised a notification")
	}
}

func TestMissingHerdrBinaryIsReported(t *testing.T) {
	e := newEnv(t).with("HERDR_BIN_PATH=/nonexistent/herdr")
	r := e.run("--list")
	if r.code != 1 || r.stderr == "" {
		t.Fatalf("exit %d, stderr %q", r.code, r.stderr)
	}
}

func TestAgentWithoutATranscriptSortsLast(t *testing.T) {
	e := newEnv(t)
	files, _ := filepath.Glob(filepath.Join(e.home, ".claude", "projects", "*api-gateway", "*.jsonl"))
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			t.Fatal(err)
		}
	}
	got := panes(e.run("--list"))
	if len(got) != 9 || got[8] != "w1:p1" {
		t.Fatalf("an agent with no transcript must sort last: %v", got)
	}
}

// --open does no reading. The pane it opens runs the binary again, and that
// copy reads the snapshot.
func TestOpenPassesThePlacementFromTheConfig(t *testing.T) {
	e := newEnv(t)
	e.writeConfig("[pane]\nplacement = \"popup\"\nwidth = \"60%\"\nheight = \"50%\"\n")
	r := e.run("--open", "--view", "tree", "--sort", "attention")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	calls := e.calls()
	if len(calls) != 1 {
		t.Fatalf("--open made %d calls, want 1: %v", len(calls), calls)
	}
	got := strings.Join(calls[0], " ")
	for _, want := range []string{
		"plugin pane open --plugin cgardner.herdr-switcher-plus --entrypoint recent --focus",
		"--placement popup", "--width 60%", "--height 50%",
		"--env HERDR_SWITCHER_PLUS_MODE=attention", "--env HERDR_SWITCHER_PLUS_VIEW=tree",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in %q", want, got)
		}
	}
	if e.called("api") {
		t.Error("--open read the snapshot")
	}
}

func TestOpenWithoutSettingsSendsOnlyTheDefaults(t *testing.T) {
	e := newEnv(t)
	if r := e.run("--open"); r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	got := strings.Join(e.calls()[0], " ")
	for _, absent := range []string{"--placement", "--width", "--height", "--env"} {
		if strings.Contains(got, absent) {
			t.Errorf("%q travelled without a setting: %s", absent, got)
		}
	}
}

// A failure of --open has nobody to read stderr, so it becomes a notification.
// ui_busy is the common one, and it gets words of its own.
func TestOpenFailureRaisesANotification(t *testing.T) {
	e := newEnv(t).with(
		"FAKE_HERDR_FAIL=plugin pane open",
		`FAKE_HERDR_FAIL_TEXT={"error":{"code":"ui_busy","message":"a popup is open"}}`,
	)
	r := e.run("--open")
	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	notes := e.callsWith("notification", "show")
	if len(notes) != 1 {
		t.Fatalf("%d notifications, want 1: %v", len(notes), e.calls())
	}
	if !strings.Contains(strings.Join(notes[0], " "), "Another popup is already open") {
		t.Errorf("ui_busy has no words of its own: %v", notes[0])
	}
}

func TestOpenWithBadConfigNotifies(t *testing.T) {
	e := newEnv(t)
	e.writeConfig("[pane]\nplacement = \"sideways\"\n")
	if r := e.run("--open"); r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	if !e.called("notification", "show") || e.called("plugin") {
		t.Fatalf("calls: %v", e.calls())
	}
}
