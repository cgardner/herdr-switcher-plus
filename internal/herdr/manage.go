package herdr

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// The calls below change the session rather than read it. Each one goes over
// the socket, because the socket takes IDs and a null label, and the CLI takes
// neither in every case. None of them moves focus: the switcher is open while
// they run, and the user decides where to go once it closes.

// CreateWorkspace opens a workspace in dir. An empty label leaves Herdr to
// choose one, which it takes from the directory.
func CreateWorkspace(dir, label string) error {
	params := map[string]any{"cwd": dir, "focus": false}
	if label != "" {
		params["label"] = label
	}
	return wrap("workspace.create", dir, request("workspace.create", params))
}

// RenameWorkspace gives a workspace a new label. Herdr requires one, so an
// empty label is refused here rather than sent.
func RenameWorkspace(id, label string) error {
	if label == "" {
		return errors.New("a workspace needs a name")
	}
	return wrap("workspace.rename", id, request("workspace.rename", map[string]any{"workspace_id": id, "label": label}))
}

// CloseWorkspace closes a workspace and every pane in it.
func CloseWorkspace(id string) error {
	return wrap("workspace.close", id, request("workspace.close", map[string]any{"workspace_id": id}))
}

// CreateTab opens a tab in a workspace, in dir.
func CreateTab(workspaceID, dir string) error {
	return wrap("tab.create", workspaceID, request("tab.create", map[string]any{"workspace_id": workspaceID, "cwd": dir, "focus": false}))
}

// RenamePane names a pane. An empty label clears the name, which Herdr spells
// as a null label.
func RenamePane(id, label string) error {
	var value any
	if label != "" {
		value = label
	}
	return wrap("pane.rename", id, request("pane.rename", map[string]any{"pane_id": id, "label": value}))
}

// ClosePane closes a pane, and whatever runs in it.
func ClosePane(id string) error {
	return wrap("pane.close", id, request("pane.close", map[string]any{"pane_id": id}))
}

// SplitPane splits a pane. direction is "right" or "down", the two Herdr
// accepts. The new pane starts in the same directory as the old one.
func SplitPane(id, direction, dir string) error {
	params := map[string]any{"target_pane_id": id, "direction": direction, "focus": false}
	if dir != "" {
		params["cwd"] = dir
	}
	return wrap("pane.split", id, request("pane.split", params))
}

// A move keeps the pane's terminal, so an agent in it carries on with its
// conversation. The pane gets a new ID in its new workspace, and a workspace
// that loses its last pane closes.

// MovePaneToWorkspace moves a pane into a new tab of a workspace.
func MovePaneToWorkspace(id, workspaceID string) error {
	return movePane(id, map[string]any{"type": "new_tab", "workspace_id": workspaceID})
}

// MovePaneBeside moves a pane into the tab of another pane, split beside it.
// split is "right" or "down".
func MovePaneBeside(id, tabID, targetPaneID, split string) error {
	return movePane(id, map[string]any{"type": "tab", "tab_id": tabID, "target_pane_id": targetPaneID, "split": split})
}

// MovePaneToNewWorkspace moves a pane into a workspace of its own. An empty
// label leaves Herdr to choose one.
func MovePaneToNewWorkspace(id, label string) error {
	dest := map[string]any{"type": "new_workspace"}
	if label != "" {
		dest["label"] = label
	}
	return movePane(id, dest)
}

func movePane(id string, dest map[string]any) error {
	return wrap("pane.move", id, request("pane.move", map[string]any{"pane_id": id, "destination": dest, "focus": false}))
}

func wrap(method, target string, err error) error {
	if err != nil {
		return fmt.Errorf("herdr %s %s: %w", method, target, err)
	}
	return nil
}

// NamedSession is one of Herdr's named persistent sessions. Each runs its own
// server with its own socket, so the snapshot of one says nothing about the
// others.
type NamedSession struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Default bool   `json:"default"`
	Dir     string `json:"session_dir"`
	Socket  string `json:"socket_path"`
}

// Current reports whether this is the session the switcher runs in. Stopping
// that one would close the switcher, and every pane with it.
func (s NamedSession) Current() bool {
	return s.Socket != "" && filepath.Clean(s.Socket) == filepath.Clean(SocketPath())
}

// The socket API has no session methods: a session is a server, and a socket
// reaches only its own. The CLI manages them instead.

// Sessions lists every named session, running or stopped.
func Sessions() ([]NamedSession, error) {
	out, err := run(Bin(), "session", "list", "--json")
	if err != nil {
		return nil, cliError("session list", err)
	}
	var body struct {
		Sessions []NamedSession `json:"sessions"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		return nil, fmt.Errorf("parse session list: %w", err)
	}
	return body.Sessions, nil
}

// StopSession stops a running session's server.
func StopSession(name string) error {
	_, err := run(Bin(), "session", "stop", name)
	return cliError("session stop "+name, err)
}

// DeleteSession removes a stopped session.
func DeleteSession(name string) error {
	_, err := run(Bin(), "session", "delete", name)
	return cliError("session delete "+name, err)
}

// cliError keeps what Herdr printed to stderr, which says why a command
// failed where the exit status alone does not. Herdr prints a refusal as a
// JSON error, which is cut down to its message.
func cliError(command string, err error) error {
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		msg := strings.TrimSpace(string(exit.Stderr))
		var body struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(msg), &body) == nil && body.Error.Message != "" {
			msg = body.Error.Message
		}
		if msg != "" {
			return fmt.Errorf("herdr %s: %s", command, msg)
		}
	}
	return fmt.Errorf("herdr %s: %w", command, err)
}
