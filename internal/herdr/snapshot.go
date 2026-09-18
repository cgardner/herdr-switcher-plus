// Package herdr wraps the parts of the Herdr CLI this plugin needs.
//
// Herdr exposes no separate plugin SDK: the CLI is the API. Every call here
// shells out to the binary named by HERDR_BIN_PATH, which Herdr injects into
// plugin processes so the plugin always reaches the server that launched it.
package herdr

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// run executes a Herdr CLI command and returns its stdout. It is a package
// variable so tests can drive the parsing and sequencing without a live
// Herdr server.
var run = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// Bin resolves the Herdr binary to call.
func Bin() string {
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		return p
	}
	return "herdr"
}

// Session identifies the agent conversation running in a pane. For Claude Code
// panes Kind is "id" and Value is the session UUID, which also names the
// transcript file on disk.
type Session struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// Agent is one recognized coding agent occupying a pane.
//
// The snapshot carries no timestamp for an agent. StateChangeSeq is a
// monotonic counter that orders state transitions but yields no wall-clock
// age, so it only serves as a fallback ordering for agents with no transcript.
type Agent struct {
	Kind           string  `json:"agent"`
	Status         string  `json:"agent_status"`
	Cwd            string  `json:"cwd"`
	Focused        bool    `json:"focused"`
	PaneID         string  `json:"pane_id"`
	TabID          string  `json:"tab_id"`
	WorkspaceID    string  `json:"workspace_id"`
	Title          string  `json:"terminal_title_stripped"`
	StateChangeSeq uint64  `json:"state_change_seq"`
	Session        Session `json:"agent_session"`
}

// Workspace is a Herdr workspace, which the UI calls a space.
type Workspace struct {
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Number      int    `json:"number"`
}

// Snapshot is the subset of session.snapshot this plugin reads.
type Snapshot struct {
	Agents        []Agent     `json:"agents"`
	Workspaces    []Workspace `json:"workspaces"`
	FocusedPaneID string      `json:"focused_pane_id"`
}

// Workspace resolves a workspace ID to its display label and sidebar position.
// A workspace missing from the snapshot falls back to the raw ID, and sorts
// after every known space.
func (s *Snapshot) Workspace(id string) (label string, number int) {
	for _, w := range s.Workspaces {
		if w.WorkspaceID == id {
			if w.Label != "" {
				return w.Label, w.Number
			}
			return id, w.Number
		}
	}
	return id, 1 << 30
}

type envelope struct {
	Result struct {
		Snapshot Snapshot `json:"snapshot"`
	} `json:"result"`
}

// Load reads the live session snapshot.
func Load() (*Snapshot, error) {
	out, err := run(Bin(), "api", "snapshot")
	if err != nil {
		return nil, fmt.Errorf("herdr api snapshot: %w", err)
	}
	return ParseSnapshot(out)
}

// ParseSnapshot decodes a session.snapshot response body.
func ParseSnapshot(body []byte) (*Snapshot, error) {
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		return nil, fmt.Errorf("parse snapshot: %w", err)
	}
	return &e.Result.Snapshot, nil
}

// Focus moves the user to the pane hosting a. Workspace and tab focus run
// first so the jump also lands when the target sits in a space that is not on
// screen. Failures on those two steps are tolerated because the final agent
// focus is the one that matters.
func Focus(a Agent) error {
	bin := Bin()
	_, _ = run(bin, "workspace", "focus", a.WorkspaceID)
	_, _ = run(bin, "tab", "focus", a.TabID)
	if _, err := run(bin, "agent", "focus", a.PaneID); err != nil {
		return fmt.Errorf("herdr agent focus %s: %w", a.PaneID, err)
	}
	return nil
}
