package agents

import (
	"testing"
	"time"

	"github.com/cgardner/herdr-pane-sort/internal/herdr"
	"github.com/cgardner/herdr-pane-sort/internal/transcript"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func withMessage(pane, ts string) Row {
	return Row{
		Agent:   herdr.Agent{PaneID: pane},
		Last:    transcript.Message{At: at(ts)},
		HasLast: true,
	}
}

func withoutMessage(pane string, seq uint64) Row {
	return Row{Agent: herdr.Agent{PaneID: pane, StateChangeSeq: seq}}
}

func paneOrder(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Agent.PaneID
	}
	return out
}

func assertOrder(t *testing.T, rows []Row, want ...string) {
	t.Helper()
	Sort(rows)
	got := paneOrder(rows)
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestSortNewestMessageFirst(t *testing.T) {
	rows := []Row{
		withMessage("old", "2026-09-10T08:00:00Z"),
		withMessage("newest", "2026-09-18T12:00:00Z"),
		withMessage("middle", "2026-09-17T09:30:00Z"),
	}
	assertOrder(t, rows, "newest", "middle", "old")
}

// A pane with no transcript has no wall-clock age, so it must never outrank a
// timestamped row however recently its state changed.
func TestSortPutsRowsWithoutTranscriptLast(t *testing.T) {
	rows := []Row{
		withoutMessage("no-transcript", 9999),
		withMessage("ancient", "2020-01-01T00:00:00Z"),
	}
	assertOrder(t, rows, "ancient", "no-transcript")
}

func TestSortFallsBackToStateChangeSeq(t *testing.T) {
	rows := []Row{
		withoutMessage("low", 10),
		withoutMessage("high", 900),
		withoutMessage("mid", 400),
	}
	assertOrder(t, rows, "high", "mid", "low")
}

func TestAge(t *testing.T) {
	now := at("2026-09-18T12:00:00Z")
	cases := []struct {
		name string
		row  Row
		want string
	}{
		{"unknown", withoutMessage("p", 1), "—"},
		{"seconds", withMessage("p", "2026-09-18T11:59:30Z"), "now"},
		{"minutes", withMessage("p", "2026-09-18T11:15:00Z"), "45m"},
		{"hours", withMessage("p", "2026-09-18T09:00:00Z"), "3h"},
		{"days", withMessage("p", "2026-09-09T12:00:00Z"), "9d"},
		{"clock skew ahead", withMessage("p", "2026-09-18T12:05:00Z"), "now"},
	}
	for _, c := range cases {
		if got := c.row.Age(now); got != c.want {
			t.Errorf("%s: Age = %q, want %q", c.name, got, c.want)
		}
	}
}
