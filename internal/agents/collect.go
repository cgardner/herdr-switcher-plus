// Package agents joins the Herdr session snapshot to the Claude Code
// transcripts on disk and orders the result by last message.
package agents

import (
	"sort"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/gitref"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/transcript"
)

// Row is one agent ready for display.
//
// HasLast is false when no transcript matched the pane, which happens for
// agent kinds other than Claude Code and for a session whose transcript is
// gone. Such a row still appears, ordered after every timestamped row.
type Row struct {
	Agent herdr.Agent

	// Space and SpaceNumber mirror the sidebar: the label the user reads and
	// the position Herdr keeps spaces in.
	Space       string
	SpaceNumber int

	// Repo, Linked and Branch describe the git checkout behind the space.
	// Several spaces are often linked worktrees of one repository, which the
	// space name alone does not reveal.
	Repo   string
	Linked bool
	Branch string

	// PaneName is the name the user gave the pane, empty until they give
	// one. It tells apart sessions that share a space.
	PaneName string

	// TabName is the name of the tab holding the pane, empty for a tab left
	// with its number. Sessions are often told apart by tab rather than pane.
	TabName string

	Last    transcript.Message
	HasLast bool
}

// The three sources Collect draws on, as package variables so tests can drive
// the join without a live Herdr server or a transcript directory on disk.
var (
	loadSnapshot     = herdr.Load
	indexTranscripts = transcript.Index
	readTranscript   = transcript.Read
	resolveBranch    = gitref.Branch
)

// Collect reads the live snapshot and returns every agent, newest message
// first. Callers reorder with Apply rather than calling Collect again, so
// changing sort mode never re-reads a transcript.
func Collect() ([]Row, error) {
	_, rows, err := CollectSnapshot()
	return rows, err
}

// CollectSnapshot is Collect that also returns the snapshot the rows came
// from. The tree needs the panes and workspaces that run no agent, and reading
// the snapshot a second time could disagree with the first.
func CollectSnapshot() (*herdr.Snapshot, []Row, error) {
	snap, err := loadSnapshot()
	if err != nil {
		return nil, nil, err
	}
	idx := indexTranscripts()

	rows := make([]Row, 0, len(snap.Agents))
	for _, a := range snap.Agents {
		label, number := snap.Workspace(a.WorkspaceID)
		r := Row{Agent: a, Space: label, SpaceNumber: number, PaneName: snap.PaneLabel(a.PaneID), TabName: snap.TabLabel(a.TabID)}
		if w := snap.FindWorkspace(a.WorkspaceID); w != nil && w.Worktree != nil {
			r.Repo = w.Worktree.RepoName
			r.Linked = w.Worktree.IsLinked
			r.Branch = resolveBranch(w.Worktree.CheckoutPath)
		}
		if p, ok := idx[a.Session.Value]; ok && a.Session.Value != "" {
			r.Last, r.HasLast = readTranscript(p)
		}
		rows = append(rows, r)
	}
	Sort(rows)
	return snap, rows, nil
}

// Sort orders rows by newest message first. Rows with no transcript fall to
// the bottom, ordered among themselves by Herdr's monotonic state-change
// counter, which ranks recent transitions without giving a wall-clock age.
func Sort(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.HasLast != b.HasLast {
			return a.HasLast
		}
		if a.HasLast {
			return a.Last.At.After(b.Last.At)
		}
		return a.Agent.StateChangeSeq > b.Agent.StateChangeSeq
	})
}

// Age renders the time since the last message in a compact fixed form. An
// unknown age reads as a dash so a missing transcript never looks recent.
func (r Row) Age(now time.Time) string {
	if !r.HasLast {
		return "—"
	}
	d := now.Sub(r.Last.At)
	switch {
	case d < 0:
		return "now"
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return itoa(int(d.Hours())) + "h"
	default:
		return itoa(int(d.Hours()/24)) + "d"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// defaultBranches never earn a place on the row. Sitting on the default branch
// is the unremarkable case, so naming it would cost width on most rows while
// telling the reader nothing.
var defaultBranches = map[string]bool{"main": true, "master": true}

// Name is what the user called this session: the pane name, else the tab name.
func (r Row) Name() string {
	if r.PaneName != "" {
		return r.PaneName
	}
	return r.TabName
}

// Label names the row: the space, plus whatever context is not already implied
// by it.
//
// The rule is to print only what is surprising. A space named after its own
// repository repeats itself if the repository is shown, and several of them on
// screen crowd out the message preview while adding nothing. So the repository
// appears only when it differs from the space name, and the branch only when it
// differs from the space name and is not the default.
//
// What survives that filter is the genuinely ambiguous case: a space that is a
// linked worktree of some other project, or one whose name hides which branch
// it sits on.
func (r Row) Label() string {
	label := r.Space

	if r.Repo != "" && r.Repo != r.Space {
		sep := "/"
		if r.Linked {
			sep = "⑂"
		}
		label = r.Repo + " " + sep + " " + label
	}

	if r.Branch != "" && r.Branch != r.Space && !defaultBranches[r.Branch] {
		label += "  @" + r.Branch
	}

	// A name is a choice the user made to tell sessions apart, so it is
	// always surprising and always shown, unless it repeats the space name.
	if n := r.Name(); n != "" && n != r.Space {
		label += "  ▸ " + n
	}
	return label
}
