package herdr

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// recordRequests replaces the socket call and keeps each method with its
// params encoded, so a test can compare one string.
func recordRequests(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	prev := request
	request = func(method string, params map[string]any) error {
		b, _ := json.Marshal(params)
		calls = append(calls, method+" "+string(b))
		return err
	}
	t.Cleanup(func() { request = prev })
	return &calls
}

func TestManageSendsTheSocketCalls(t *testing.T) {
	cases := []struct {
		name string
		do   func() error
		want string
	}{
		{"create", func() error { return CreateWorkspace("/c/auth", "") }, `workspace.create {"cwd":"/c/auth","focus":false}`},
		{"create named", func() error { return CreateWorkspace("/c/auth", "spike") }, `workspace.create {"cwd":"/c/auth","focus":false,"label":"spike"}`},
		{"rename", func() error { return RenameWorkspace("w2", "billing") }, `workspace.rename {"label":"billing","workspace_id":"w2"}`},
		{"close", func() error { return CloseWorkspace("w2") }, `workspace.close {"workspace_id":"w2"}`},
		{"tab", func() error { return CreateTab("w2", "/c/auth") }, `tab.create {"cwd":"/c/auth","focus":false,"workspace_id":"w2"}`},
		{"name pane", func() error { return RenamePane("w2:p1", "tests") }, `pane.rename {"label":"tests","pane_id":"w2:p1"}`},
		{"clear pane", func() error { return RenamePane("w2:p1", "") }, `pane.rename {"label":null,"pane_id":"w2:p1"}`},
		{"close pane", func() error { return ClosePane("w2:p1") }, `pane.close {"pane_id":"w2:p1"}`},
		{"split", func() error { return SplitPane("w2:p1", "down", "/c/auth") }, `pane.split {"cwd":"/c/auth","direction":"down","focus":false,"target_pane_id":"w2:p1"}`},
		{"move to space", func() error { return MovePaneToWorkspace("w2:p1", "w3") }, `pane.move {"destination":{"type":"new_tab","workspace_id":"w3"},"focus":false,"pane_id":"w2:p1"}`},
		{"move beside", func() error { return MovePaneBeside("w2:p1", "w3:t1", "w3:p1", "right") }, `pane.move {"destination":{"split":"right","tab_id":"w3:t1","target_pane_id":"w3:p1","type":"tab"},"focus":false,"pane_id":"w2:p1"}`},
		{"move out", func() error { return MovePaneToNewWorkspace("w2:p1", "") }, `pane.move {"destination":{"type":"new_workspace"},"focus":false,"pane_id":"w2:p1"}`},
		{"move out named", func() error { return MovePaneToNewWorkspace("w2:p1", "spike") }, `pane.move {"destination":{"label":"spike","type":"new_workspace"},"focus":false,"pane_id":"w2:p1"}`},
		{"split here", func() error { return SplitPane("w2:p1", "right", "") }, `pane.split {"direction":"right","focus":false,"target_pane_id":"w2:p1"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			calls := recordRequests(t, nil)
			if err := c.do(); err != nil {
				t.Fatal(err)
			}
			if len(*calls) != 1 || (*calls)[0] != c.want {
				t.Errorf("calls = %v, want %s", *calls, c.want)
			}
		})
	}
}

func TestManageNamesTheTargetInAFailure(t *testing.T) {
	recordRequests(t, errors.New("pane_not_found: gone"))
	err := ClosePane("w2:p9")
	if err == nil || !strings.Contains(err.Error(), "pane.close w2:p9") || !strings.Contains(err.Error(), "pane_not_found") {
		t.Errorf("got %v", err)
	}
}

func TestRenameWorkspaceRefusesAnEmptyName(t *testing.T) {
	calls := recordRequests(t, nil)
	if err := RenameWorkspace("w2", ""); err == nil {
		t.Error("expected an error")
	}
	if len(*calls) != 0 {
		t.Errorf("nothing should reach the socket: %v", *calls)
	}
}

const sessionsBody = `{"sessions":[
  {"default":true,"name":"default","running":true,"session_dir":"/h/.config/herdr","socket_path":"/h/.config/herdr/herdr.sock"},
  {"default":false,"name":"spike","running":false,"session_dir":"/h/.config/herdr/sessions/spike","socket_path":"/h/.config/herdr/sessions/spike/herdr.sock"}]}`

func TestSessionsDecodesTheList(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "herdr")
	t.Setenv("HERDR_SOCKET_PATH", "/h/.config/herdr/herdr.sock")
	calls := stub(t, func(string, ...string) ([]byte, error) { return []byte(sessionsBody), nil })
	got, err := Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0], " ") != "herdr session list --json" {
		t.Errorf("calls = %v", *calls)
	}
	if len(got) != 2 || got[0].Name != "default" || !got[0].Running || !got[0].Default || got[1].Running {
		t.Fatalf("decoded wrong: %+v", got)
	}
	if !got[0].Current() || got[1].Current() {
		t.Errorf("only the session behind this socket is current: %+v", got)
	}
	if (NamedSession{}).Current() {
		t.Error("a session with no socket is not current")
	}
}

func TestSessionsReportsFailures(t *testing.T) {
	stub(t, func(string, ...string) ([]byte, error) { return nil, errors.New("no herdr") })
	if _, err := Sessions(); err == nil || !strings.Contains(err.Error(), "session list") {
		t.Errorf("got %v", err)
	}
	stub(t, func(string, ...string) ([]byte, error) { return []byte("not json"), nil })
	if _, err := Sessions(); err == nil || !strings.Contains(err.Error(), "parse session list") {
		t.Errorf("got %v", err)
	}
}

func TestStopAndDeleteRunTheCLI(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "herdr")
	calls := stub(t, func(string, ...string) ([]byte, error) { return nil, nil })
	if err := StopSession("spike"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteSession("spike"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join((*calls)[0], " ") + "; " + strings.Join((*calls)[1], " "); got != "herdr session stop spike; herdr session delete spike" {
		t.Errorf("got %q", got)
	}
}

// Herdr says why a command failed on stderr, which the error must carry.
func TestCLIErrorKeepsWhatHerdrPrinted(t *testing.T) {
	stub(t, func(string, ...string) ([]byte, error) {
		return nil, &exec.ExitError{Stderr: []byte("session spike is running\n")}
	})
	if err := DeleteSession("spike"); err == nil || err.Error() != "herdr session delete spike: session spike is running" {
		t.Errorf("got %v", err)
	}
	stub(t, func(string, ...string) ([]byte, error) {
		return nil, &exec.ExitError{Stderr: []byte(`{"error":{"code":"session_delete_failed","message":"session spike is running; stop it before deleting"}}`)}
	})
	if err := DeleteSession("spike"); err == nil || err.Error() != "herdr session delete spike: session spike is running; stop it before deleting" {
		t.Errorf("got %v", err)
	}
	stub(t, func(string, ...string) ([]byte, error) { return nil, &exec.ExitError{} })
	if err := StopSession("spike"); err == nil || !strings.Contains(err.Error(), "session stop spike") {
		t.Errorf("got %v", err)
	}
}
