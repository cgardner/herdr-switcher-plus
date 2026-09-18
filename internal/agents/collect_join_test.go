package agents

import (
	"errors"
	"testing"
	"time"

	"github.com/cgardner/herdr-pane-sort/internal/herdr"
	"github.com/cgardner/herdr-pane-sort/internal/transcript"
)

// stubSources replaces the three inputs Collect joins and restores them after
// the test.
func stubSources(t *testing.T,
	snap *herdr.Snapshot, snapErr error,
	idx map[string]string,
	read func(string) (transcript.Message, bool),
) {
	t.Helper()
	ls, it, rt := loadSnapshot, indexTranscripts, readTranscript
	loadSnapshot = func() (*herdr.Snapshot, error) { return snap, snapErr }
	indexTranscripts = func() map[string]string { return idx }
	readTranscript = read
	t.Cleanup(func() { loadSnapshot, indexTranscripts, readTranscript = ls, it, rt })
}

func agent(pane, ws, session string) herdr.Agent {
	return herdr.Agent{
		PaneID:      pane,
		WorkspaceID: ws,
		Status:      "idle",
		Session:     herdr.Session{Kind: "id", Value: session},
	}
}

func TestCollectJoinsTranscriptsAndSortsByRecency(t *testing.T) {
	snap := &herdr.Snapshot{
		Agents: []herdr.Agent{
			agent("w1:p1", "w1", "old-uuid"),
			agent("w2:p1", "w2", "new-uuid"),
		},
		Workspaces: []herdr.Workspace{
			{WorkspaceID: "w1", Label: "alpha", Number: 1},
			{WorkspaceID: "w2", Label: "beta", Number: 2},
		},
	}
	times := map[string]string{
		"/t/old.jsonl": "2026-09-01T00:00:00Z",
		"/t/new.jsonl": "2026-09-18T00:00:00Z",
	}
	stubSources(t, snap, nil,
		map[string]string{"old-uuid": "/t/old.jsonl", "new-uuid": "/t/new.jsonl"},
		func(p string) (transcript.Message, bool) {
			ts, _ := time.Parse(time.RFC3339, times[p])
			return transcript.Message{At: ts, Role: "assistant", Text: "hello"}, true
		})

	rows, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	if got := paneOrder(rows); got[0] != "w2:p1" || got[1] != "w1:p1" {
		t.Errorf("order = %v, want newest first", got)
	}
	if rows[0].Space != "beta" || rows[0].SpaceNumber != 2 {
		t.Errorf("space not resolved: %+v", rows[0])
	}
	if !rows[0].HasLast || rows[0].Last.Text != "hello" {
		t.Errorf("message not attached: %+v", rows[0])
	}
}

// An agent whose transcript is absent still appears, marked as having no
// message, so a non-Claude agent is never silently dropped.
func TestCollectKeepsAgentsWithNoTranscript(t *testing.T) {
	snap := &herdr.Snapshot{Agents: []herdr.Agent{
		agent("w1:p1", "w1", "missing-uuid"),
		{PaneID: "w2:p1", WorkspaceID: "w2", Status: "idle"},
	}}
	stubSources(t, snap, nil, map[string]string{}, func(string) (transcript.Message, bool) {
		t.Fatal("no transcript should be read")
		return transcript.Message{}, false
	})

	rows, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.HasLast {
			t.Errorf("%s should carry no message", r.Agent.PaneID)
		}
	}
}

// An empty session value must never match an index entry keyed by "".
func TestCollectIgnoresABlankSessionValue(t *testing.T) {
	snap := &herdr.Snapshot{Agents: []herdr.Agent{{PaneID: "w1:p1", Status: "idle"}}}
	stubSources(t, snap, nil, map[string]string{"": "/t/wrong.jsonl"},
		func(string) (transcript.Message, bool) {
			t.Fatal("a blank session must not resolve to a transcript")
			return transcript.Message{}, false
		})
	rows, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].HasLast {
		t.Error("blank session must produce no message")
	}
}

// A transcript that exists but holds no message leaves the row unmarked.
func TestCollectHandlesAnUnreadableTranscript(t *testing.T) {
	snap := &herdr.Snapshot{Agents: []herdr.Agent{agent("w1:p1", "w1", "uuid")}}
	stubSources(t, snap, nil, map[string]string{"uuid": "/t/x.jsonl"},
		func(string) (transcript.Message, bool) { return transcript.Message{}, false })
	rows, _ := Collect()
	if rows[0].HasLast {
		t.Error("expected HasLast false")
	}
}

func TestCollectPropagatesASnapshotError(t *testing.T) {
	stubSources(t, nil, errors.New("socket gone"), nil, nil)
	if _, err := Collect(); err == nil {
		t.Fatal("expected the snapshot error to surface")
	}
}

func TestCollectOnAnEmptySession(t *testing.T) {
	stubSources(t, &herdr.Snapshot{}, nil, nil, nil)
	rows, err := Collect()
	if err != nil || len(rows) != 0 {
		t.Errorf("rows = %v, err = %v", rows, err)
	}
}

func TestStatusFilterLabel(t *testing.T) {
	if got := StatusAll.Label(); got != "all" {
		t.Errorf("StatusAll.Label = %q, want all", got)
	}
	if got := StatusBlocked.Label(); got != "blocked" {
		t.Errorf("StatusBlocked.Label = %q, want blocked", got)
	}
}
