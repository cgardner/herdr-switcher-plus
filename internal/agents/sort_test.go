package agents

import (
	"testing"

	"github.com/cgardner/herdr-pane-sort/internal/herdr"
	"github.com/cgardner/herdr-pane-sort/internal/transcript"
)

func row(pane, status, space string, number int, ts string) Row {
	r := Row{
		Agent:       herdr.Agent{PaneID: pane, Status: status},
		Space:       space,
		SpaceNumber: number,
	}
	if ts != "" {
		r.Last = transcript.Message{At: at(ts)}
		r.HasLast = true
	}
	return r
}

func check(t *testing.T, mode Mode, rows []Row, want ...string) {
	t.Helper()
	Apply(mode, rows)
	got := paneOrder(rows)
	if len(got) != len(want) {
		t.Fatalf("%s: order = %v, want %v", mode, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: order = %v, want %v", mode, got, want)
		}
	}
}

// Attention answers "where am I blocking work", so a blocked agent outranks
// everything and a working agent, which needs nobody, sinks below idle.
func TestApplyAttentionRanksStatesBeforeRecency(t *testing.T) {
	rows := []Row{
		row("working-now", "working", "a", 1, "2026-09-18T12:00:00Z"),
		row("idle-old", "idle", "b", 2, "2026-09-01T00:00:00Z"),
		row("blocked-old", "blocked", "c", 3, "2026-08-01T00:00:00Z"),
		row("done-mid", "done", "d", 4, "2026-09-10T00:00:00Z"),
		row("unknown", "unknown", "e", 5, "2026-09-18T11:00:00Z"),
	}
	check(t, ModeAttention, rows, "blocked-old", "done-mid", "idle-old", "working-now", "unknown")
}

func TestApplyAttentionBreaksTiesByRecency(t *testing.T) {
	rows := []Row{
		row("blocked-old", "blocked", "a", 1, "2026-09-01T00:00:00Z"),
		row("blocked-new", "blocked", "b", 2, "2026-09-18T00:00:00Z"),
	}
	check(t, ModeAttention, rows, "blocked-new", "blocked-old")
}

func TestApplySpaceUsesSidebarOrderThenRecency(t *testing.T) {
	rows := []Row{
		row("s3", "idle", "c", 3, "2026-09-18T12:00:00Z"),
		row("s1-old", "idle", "a", 1, "2026-09-01T00:00:00Z"),
		row("s1-new", "idle", "a", 1, "2026-09-17T00:00:00Z"),
	}
	check(t, ModeSpace, rows, "s1-new", "s1-old", "s3")
}

func TestApplyNameIsCaseInsensitive(t *testing.T) {
	rows := []Row{
		row("zebra", "idle", "Zebra", 1, "2026-09-18T12:00:00Z"),
		row("apple", "idle", "apple", 2, "2026-09-01T00:00:00Z"),
		row("Mango", "idle", "Mango", 3, "2026-09-05T00:00:00Z"),
	}
	check(t, ModeName, rows, "apple", "Mango", "zebra")
}

func TestApplyOldestReversesRecencyButKeepsUnknownsLast(t *testing.T) {
	rows := []Row{
		row("new", "idle", "a", 1, "2026-09-18T12:00:00Z"),
		row("no-transcript", "idle", "b", 2, ""),
		row("ancient", "idle", "c", 3, "2020-01-01T00:00:00Z"),
	}
	check(t, ModeOldest, rows, "ancient", "new", "no-transcript")
}

func TestApplyRecentMatchesSort(t *testing.T) {
	rows := []Row{
		row("old", "idle", "a", 1, "2026-09-01T00:00:00Z"),
		row("new", "working", "b", 2, "2026-09-18T12:00:00Z"),
	}
	check(t, ModeRecent, rows, "new", "old")
}

func TestParseModeFallsBackToRecent(t *testing.T) {
	for _, in := range []string{"", "nonsense", "  ", "RECENTLY"} {
		if got := ParseMode(in); got != ModeRecent {
			t.Errorf("ParseMode(%q) = %q, want recent", in, got)
		}
	}
	if got := ParseMode("  ATTENTION "); got != ModeAttention {
		t.Errorf("ParseMode trims and lowercases: got %q", got)
	}
}

func TestNextWrapsBothWays(t *testing.T) {
	first, last := Modes[0], Modes[len(Modes)-1]
	if got := last.Next(1); got != first {
		t.Errorf("last.Next(1) = %q, want %q", got, first)
	}
	if got := first.Next(-1); got != last {
		t.Errorf("first.Next(-1) = %q, want %q", got, last)
	}
	// An unknown mode must still yield a usable one.
	if got := Mode("bogus").Next(1); got != ModeRecent {
		t.Errorf("bogus.Next(1) = %q, want recent", got)
	}
}

func TestEveryModeHasALabel(t *testing.T) {
	for _, m := range Modes {
		if m.Label() == "" || m.Label() == string(m) {
			t.Errorf("mode %q needs a distinct human label, got %q", m, m.Label())
		}
	}
}

// An unrecognized mode must still render a label and still sort, rather than
// showing a blank title or leaving the list in input order.
func TestUnknownModeLabelsAndSortsAsRecent(t *testing.T) {
	bogus := Mode("bogus")
	if got := bogus.Label(); got != "bogus" {
		t.Errorf("Label = %q, want the raw name", got)
	}
	rows := []Row{
		row("old", "idle", "a", 1, "2026-09-01T00:00:00Z"),
		row("new", "idle", "b", 2, "2026-09-18T00:00:00Z"),
	}
	check(t, bogus, rows, "new", "old")
}

// Inside a group, two rows that both lack a transcript fall back to Herdr's
// monotonic counter rather than comparing zero timestamps.
func TestGroupedModesFallBackToStateChangeSeq(t *testing.T) {
	rows := []Row{
		withoutMessage("low", 10),
		withoutMessage("high", 900),
	}
	rows[0].Agent.Status, rows[1].Agent.Status = "idle", "idle"
	check(t, ModeAttention, rows, "high", "low")
}

// A row with a transcript outranks one without, even inside the same group.
func TestGroupedModesPreferRowsWithAMessage(t *testing.T) {
	a := withoutMessage("none", 9999)
	a.Agent.Status = "idle"
	b := row("timed", "idle", "a", 1, "2020-01-01T00:00:00Z")
	check(t, ModeAttention, []Row{a, b}, "timed", "none")
}

func TestItoaHandlesZeroAndMultipleDigits(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 42: "42", 1234: "1234"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

// Oldest-first still keeps rows without a transcript at the bottom, and orders
// them among themselves by the counter, ascending like the mode itself.
func TestOldestOrdersUntimedRowsByCounterAscending(t *testing.T) {
	rows := []Row{
		withoutMessage("high", 900),
		withoutMessage("low", 10),
		row("timed", "idle", "a", 1, "2026-09-01T00:00:00Z"),
	}
	check(t, ModeOldest, rows, "timed", "low", "high")
}
