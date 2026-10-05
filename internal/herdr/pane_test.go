package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const panesBody = `{"id":"x","result":{"snapshot":{
  "workspaces":[{"workspace_id":"w2","label":"auth-service","number":2,
     "worktree":{"repo_name":"platform","repo_key":"/c/platform/.git","checkout_path":"/c/auth","is_linked_worktree":true}}],
  "panes":[
    {"pane_id":"w2:p1","tab_id":"w2:t1","workspace_id":"w2","agent":"claude","agent_status":"blocked",
     "cwd":"/c/auth","foreground_cwd":"/c/auth","terminal_title_stripped":"auth-service","focused":true},
    {"pane_id":"w2:p2","tab_id":"w2:t2","workspace_id":"w2","agent_status":"unknown","cwd":"/c/auth"}],
  "agents":[]}}}`

func TestParseSnapshotDecodesThePanes(t *testing.T) {
	s, err := ParseSnapshot([]byte(panesBody))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Panes) != 2 {
		t.Fatalf("got %d panes, want 2", len(s.Panes))
	}
	p := s.Panes[0]
	if p.Agent != "claude" || p.Status != "blocked" || p.TabID != "w2:t1" || !p.Focused || p.ForegroundCwd != "/c/auth" {
		t.Errorf("pane decoded wrong: %+v", p)
	}
	if s.Panes[1].Agent != "" {
		t.Errorf("a shell pane has no agent: %+v", s.Panes[1])
	}
	if got := s.FindWorkspace("w2").Worktree.RepoKey; got != "/c/platform/.git" {
		t.Errorf("repo_key = %q", got)
	}
}

// stubRequest replaces the socket call for one test.
func stubRequest(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	prev := request
	request = func(method string, params map[string]any) error {
		calls = append(calls, method+" "+fmt.Sprint(params["pane_id"]))
		return err
	}
	t.Cleanup(func() { request = prev })
	return &calls
}

func TestFocusPaneRunsWorkspaceThenTabThenTheSocket(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "herdr")
	calls := stub(t, func(string, ...string) ([]byte, error) { return nil, errors.New("tolerated") })
	sock := stubRequest(t, nil)

	if err := FocusPane(Pane{PaneID: "w2:p2", TabID: "w2:t2", WorkspaceID: "w2"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join((*calls)[0], " ") + "; " + strings.Join((*calls)[1], " "); got != "herdr workspace focus w2; herdr tab focus w2:t2" {
		t.Errorf("got %q", got)
	}
	if len(*sock) != 1 || (*sock)[0] != "pane.focus w2:p2" {
		t.Errorf("socket calls = %v", *sock)
	}
}

func TestFocusPaneReportsASocketFailure(t *testing.T) {
	stub(t, func(string, ...string) ([]byte, error) { return nil, nil })
	stubRequest(t, errors.New("pane_not_found"))
	if err := FocusPane(Pane{PaneID: "w2:p2"}); err == nil || !strings.Contains(err.Error(), "w2:p2") {
		t.Errorf("error should name the pane, got %v", err)
	}
}

func TestSocketPathPrefersTheInjectedPath(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", "/run/herdr.sock")
	if got := SocketPath(); got != "/run/herdr.sock" {
		t.Errorf("got %q", got)
	}
	t.Setenv("HERDR_SOCKET_PATH", "")
	t.Setenv("HOME", "/home/demo")
	if got := SocketPath(); got != "/home/demo/.config/herdr/herdr.sock" {
		t.Errorf("got %q", got)
	}
}

// serve listens on a real Unix socket and answers every request with reply.
// The directory comes from os.MkdirTemp rather than t.TempDir, whose path is
// long enough on macOS to exceed the 104 byte limit on a socket path.
func serve(t *testing.T, reply string) <-chan map[string]any {
	t.Helper()
	dir, err := os.MkdirTemp("", "hsp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	t.Setenv("HERDR_SOCKET_PATH", path)

	got := make(chan map[string]any, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		line, _ := bufio.NewReader(conn).ReadBytes('\n')
		var req map[string]any
		_ = json.Unmarshal(line, &req)
		got <- req
		_, _ = conn.Write([]byte(reply))
	}()
	t.Cleanup(func() { l.Close(); <-done })
	return got
}

func TestSocketRequestSendsOneLineAndAcceptsAResult(t *testing.T) {
	got := serve(t, `{"id":"herdr-switcher-plus","result":{"type":"ok"}}`+"\n")
	if err := socketRequest("pane.focus", map[string]any{"pane_id": "w2:p2"}); err != nil {
		t.Fatal(err)
	}
	req := <-got
	if req["method"] != "pane.focus" || req["params"].(map[string]any)["pane_id"] != "w2:p2" {
		t.Errorf("request = %v", req)
	}
}

func TestSocketRequestReportsTheErrorReply(t *testing.T) {
	serve(t, `{"id":"x","error":{"code":"pane_not_found","message":"pane w2:p9 not found"}}`+"\n")
	err := socketRequest("pane.focus", map[string]any{"pane_id": "w2:p9"})
	if err == nil || !strings.Contains(err.Error(), "pane_not_found") {
		t.Errorf("got %v", err)
	}
}

func TestSocketRequestRejectsAGarbageReply(t *testing.T) {
	serve(t, "not json\n")
	if err := socketRequest("pane.focus", nil); err == nil || !strings.Contains(err.Error(), "parse reply") {
		t.Errorf("got %v", err)
	}
}

// A server that closes without a newline must not leave the call hanging.
func TestSocketRequestReportsAnUnfinishedReply(t *testing.T) {
	serve(t, `{"id":"x"`)
	if err := socketRequest("pane.focus", nil); err == nil {
		t.Error("expected an error")
	}
}

func TestSocketRequestReportsNoServer(t *testing.T) {
	t.Setenv("HERDR_SOCKET_PATH", filepath.Join(t.TempDir(), "absent"))
	if err := socketRequest("pane.focus", nil); err == nil {
		t.Error("expected an error")
	}
}

func TestNotifyShowsATitleAndBody(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "herdr")
	calls := stub(t, func(string, ...string) ([]byte, error) { return nil, errors.New("dropped") })
	Notify("switcher+", "Another popup is already open.")
	want := "herdr notification show switcher+ --body Another popup is already open."
	if len(*calls) != 1 || strings.Join((*calls)[0], " ") != want {
		t.Errorf("calls = %v", *calls)
	}
}

func TestPaneLabelFindsTheNameOrNothing(t *testing.T) {
	s := &Snapshot{Panes: []Pane{{PaneID: "w1:p1", Label: "api"}, {PaneID: "w1:p2"}}}
	if got := s.PaneLabel("w1:p1"); got != "api" {
		t.Errorf("got %q", got)
	}
	if got := s.PaneLabel("w1:p2"); got != "" {
		t.Errorf("unnamed pane gave %q", got)
	}
	if got := s.PaneLabel("w9:p9"); got != "" {
		t.Errorf("missing pane gave %q", got)
	}
}

func TestTabLabelIgnoresANumberedTab(t *testing.T) {
	s := &Snapshot{Tabs: []Tab{{TabID: "t1", Label: "Bear Den", Number: 4}, {TabID: "t2", Label: "1", Number: 4}}}
	if got := s.TabLabel("t1"); got != "Bear Den" {
		t.Errorf("got %q", got)
	}
	if got := s.TabLabel("t2"); got != "" {
		t.Errorf("numbered tab gave %q", got)
	}
	if got := s.TabLabel("t9"); got != "" {
		t.Errorf("missing tab gave %q", got)
	}
}
