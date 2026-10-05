// Package herdr wraps the parts of the Herdr CLI this plugin needs.
//
// Herdr exposes no separate plugin SDK: the CLI is the API. Every call here
// shells out to the binary named by HERDR_BIN_PATH, which Herdr injects into
// plugin processes so the plugin always reaches the server that launched it.
package herdr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
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

	// Name is what `herdr agent rename` set. Herdr reports it here only,
	// and not on the pane, whose own name is Pane.Label.
	Name string `json:"name"`
}

// Worktree describes the git checkout behind a workspace. It is absent for a
// workspace that is not a git checkout at all.
//
// Herdr reports no branch here. Resolving one means reading the checkout, which
// internal/gitref does.
//
// RepoKey is the repository's git directory, which every checkout of one
// repository shares. RepoRoot is not a safe key: Herdr reports it with a
// trailing slash for some checkouts and without one for others.
type Worktree struct {
	RepoName     string `json:"repo_name"`
	RepoKey      string `json:"repo_key"`
	RepoRoot     string `json:"repo_root"`
	CheckoutPath string `json:"checkout_path"`
	IsLinked     bool   `json:"is_linked_worktree"`
}

// Workspace is a Herdr workspace, which the UI calls a space.
type Workspace struct {
	WorkspaceID string    `json:"workspace_id"`
	Label       string    `json:"label"`
	Number      int       `json:"number"`
	Worktree    *Worktree `json:"worktree"`
}

// Pane is one terminal pane, whether or not an agent runs in it. Agent is
// empty for a plain shell, and Status is then "unknown".
type Pane struct {
	PaneID        string `json:"pane_id"`
	TabID         string `json:"tab_id"`
	WorkspaceID   string `json:"workspace_id"`
	Agent         string `json:"agent"`
	Status        string `json:"agent_status"`
	Cwd           string `json:"cwd"`
	ForegroundCwd string `json:"foreground_cwd"`
	Title         string `json:"terminal_title_stripped"`
	Focused       bool   `json:"focused"`

	// Label is the name the user gave the pane, and is empty until they
	// give one.
	Label string `json:"label"`
}

// Tab is one tab of a workspace. Herdr gives an unnamed tab a number as its
// label.
type Tab struct {
	TabID  string `json:"tab_id"`
	Label  string `json:"label"`
	Number int    `json:"number"`
}

// Snapshot is the subset of session.snapshot this plugin reads.
type Snapshot struct {
	Agents        []Agent     `json:"agents"`
	Panes         []Pane      `json:"panes"`
	Workspaces    []Workspace `json:"workspaces"`
	Tabs          []Tab       `json:"tabs"`
	FocusedPaneID string      `json:"focused_pane_id"`
}

// PaneLabel returns the name the user gave a pane, or an empty string when the
// pane has none or is missing from the snapshot.
func (s *Snapshot) PaneLabel(id string) string {
	for _, p := range s.Panes {
		if p.PaneID == id {
			return p.Label
		}
	}
	return ""
}

// TabLabel returns the name the user gave a tab, or an empty string when the
// tab is unnamed or missing from the snapshot. Herdr keeps the number an
// unnamed tab was born with, even after tabs close, so any numeric label is
// treated as no name.
func (s *Snapshot) TabLabel(id string) string {
	for _, t := range s.Tabs {
		if t.TabID == id {
			if _, err := strconv.Atoi(t.Label); err == nil {
				return ""
			}
			return t.Label
		}
	}
	return ""
}

// FindWorkspace returns the workspace with this ID, or nil.
func (s *Snapshot) FindWorkspace(id string) *Workspace {
	for i := range s.Workspaces {
		if s.Workspaces[i].WorkspaceID == id {
			return &s.Workspaces[i]
		}
	}
	return nil
}

// Workspace resolves a workspace ID to its display label and sidebar position.
// A workspace missing from the snapshot falls back to the raw ID, and sorts
// after every known space.
func (s *Snapshot) Workspace(id string) (label string, number int) {
	w := s.FindWorkspace(id)
	if w == nil {
		return id, 1 << 30
	}
	if w.Label != "" {
		return w.Label, w.Number
	}
	return id, w.Number
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

// Notify raises a Herdr notification. It needs no free popup slot, so it can
// say why a popup failed to open. It is best effort: a notice that cannot be
// shown has nowhere better to go, so its failure is dropped.
func Notify(title, body string) {
	_, _ = run(Bin(), "notification", "show", title, "--body", body)
}

// FocusPane moves the user to any pane, including one that runs no agent.
//
// `agent focus` rejects a plain shell with agent_not_found, and the CLI has
// no command that focuses a pane by ID. The pane.focus socket method does, so
// the last step goes over the socket. Workspace and tab focus run first for
// the same reason as in Focus.
func FocusPane(p Pane) error {
	bin := Bin()
	_, _ = run(bin, "workspace", "focus", p.WorkspaceID)
	_, _ = run(bin, "tab", "focus", p.TabID)
	if err := request("pane.focus", map[string]any{"pane_id": p.PaneID}); err != nil {
		return fmt.Errorf("herdr pane.focus %s: %w", p.PaneID, err)
	}
	return nil
}

// request sends one call over the Herdr socket. It is a package variable so
// tests can observe the call without a live server.
var request = socketRequest

// SocketPath resolves the Herdr socket. Herdr injects HERDR_SOCKET_PATH into
// every pane it starts, and the default covers a pane that lost it.
func SocketPath() string {
	if p := os.Getenv("HERDR_SOCKET_PATH"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}

// socketTimeout bounds a socket call, so a server that stops answering cannot
// hold the switcher open after the user picked a pane.
const socketTimeout = 2 * time.Second

// socketRequest writes one JSON request and reads one reply. The protocol is
// newline-delimited JSON: a request carries an id, a method and params, and
// the reply carries either a result or an error with a code and a message.
func socketRequest(method string, params map[string]any) error {
	conn, err := net.DialTimeout("unix", SocketPath(), socketTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(socketTimeout))

	// Marshal cannot fail on the strings, booleans and nils that params hold.
	req, _ := json.Marshal(map[string]any{"id": "herdr-switcher-plus", "method": method, "params": params})
	// A failed write needs no check of its own: the read that follows fails
	// too, and reports it.
	_, _ = conn.Write(append(req, '\n'))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}
	var reply struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &reply); err != nil {
		return fmt.Errorf("parse reply: %w", err)
	}
	if reply.Error != nil {
		return fmt.Errorf("%s: %s", reply.Error.Code, reply.Error.Message)
	}
	return nil
}
