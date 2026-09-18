package agents

import "testing"

func sample() []Row {
	return []Row{
		row("b1", "blocked", "a", 1, "2026-09-18T12:00:00Z"),
		row("w1", "working", "b", 2, "2026-09-18T11:00:00Z"),
		row("i1", "idle", "c", 3, "2026-09-18T10:00:00Z"),
		row("i2", "idle", "d", 4, "2026-09-18T09:00:00Z"),
		row("d1", "done", "e", 5, "2026-09-18T08:00:00Z"),
		row("u1", "unknown", "f", 6, "2026-09-18T07:00:00Z"),
	}
}

func TestKeepSelectsOneState(t *testing.T) {
	cases := map[StatusFilter][]string{
		StatusBlocked: {"b1"},
		StatusWorking: {"w1"},
		StatusIdle:    {"i1", "i2"},
		StatusDone:    {"d1"},
	}
	for f, want := range cases {
		got := paneOrder(f.Keep(sample()))
		if len(got) != len(want) {
			t.Errorf("%s = %v, want %v", f, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s = %v, want %v", f, got, want)
				break
			}
		}
	}
}

func TestKeepAllReturnsEverything(t *testing.T) {
	if got := StatusAll.Keep(sample()); len(got) != 6 {
		t.Errorf("StatusAll kept %d rows, want 6", len(got))
	}
}

// Herdr's picker has no key for unknown, so an unknown agent is reachable
// under "all" only. This pins that behavior rather than leaving it accidental.
func TestUnknownIsReachableOnlyUnderAll(t *testing.T) {
	for _, f := range []StatusFilter{StatusBlocked, StatusWorking, StatusIdle, StatusDone} {
		for _, r := range f.Keep(sample()) {
			if r.Agent.Status == "unknown" {
				t.Errorf("%s leaked an unknown row", f)
			}
		}
	}
	found := false
	for _, r := range StatusAll.Keep(sample()) {
		if r.Agent.Status == "unknown" {
			found = true
		}
	}
	if !found {
		t.Error("StatusAll must include unknown rows")
	}
}

func TestKeepPreservesOrder(t *testing.T) {
	rows := []Row{
		row("second", "idle", "b", 2, "2026-09-01T00:00:00Z"),
		row("skip", "working", "c", 3, "2026-09-18T00:00:00Z"),
		row("first", "idle", "a", 1, "2026-08-01T00:00:00Z"),
	}
	got := paneOrder(StatusIdle.Keep(rows))
	if len(got) != 2 || got[0] != "second" || got[1] != "first" {
		t.Errorf("Keep = %v, want [second first] in input order", got)
	}
}

// Both the picker key and the full state name must resolve, and anything else
// must clear the filter rather than hide the whole list.
func TestParseStatus(t *testing.T) {
	cases := map[string]StatusFilter{
		"b": StatusBlocked, "blocked": StatusBlocked,
		"w": StatusWorking, "working": StatusWorking,
		"i": StatusIdle, "idle": StatusIdle,
		"d": StatusDone, "done": StatusDone,
		"a": StatusAll, "": StatusAll,
		"unknown": StatusAll, "nonsense": StatusAll, "B": StatusAll,
	}
	for in, want := range cases {
		if got := ParseStatus(in); got != want {
			t.Errorf("ParseStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatusKeysMatchThePickerFooter(t *testing.T) {
	want := []string{"a", "b", "w", "i", "d"}
	if len(StatusKeys) != len(want) {
		t.Fatalf("got %d keys, want %d", len(StatusKeys), len(want))
	}
	for i, e := range StatusKeys {
		if e.Key != want[i] {
			t.Errorf("StatusKeys[%d] = %q, want %q", i, e.Key, want[i])
		}
	}
}
