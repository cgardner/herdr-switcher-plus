// Package agents joins the Herdr session snapshot to the Claude Code
// transcripts on disk and orders the result by last message.
package agents

import (
	"sort"
	"time"

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

	Last    transcript.Message
	HasLast bool
}

// The three sources Collect draws on, as package variables so tests can drive
// the join without a live Herdr server or a transcript directory on disk.
var (
	loadSnapshot     = herdr.Load
	indexTranscripts = transcript.Index
	readTranscript   = transcript.Read
)

// Collect reads the live snapshot and returns every agent, newest message
// first. Callers reorder with Apply rather than calling Collect again, so
// changing sort mode never re-reads a transcript.
func Collect() ([]Row, error) {
	snap, err := loadSnapshot()
	if err != nil {
		return nil, err
	}
	idx := indexTranscripts()

	rows := make([]Row, 0, len(snap.Agents))
	for _, a := range snap.Agents {
		label, number := snap.Workspace(a.WorkspaceID)
		r := Row{Agent: a, Space: label, SpaceNumber: number}
		if p, ok := idx[a.Session.Value]; ok && a.Session.Value != "" {
			r.Last, r.HasLast = readTranscript(p)
		}
		rows = append(rows, r)
	}
	Sort(rows)
	return rows, nil
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
