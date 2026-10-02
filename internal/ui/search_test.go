package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// searchFor opens the search on the named fixture and types query. While the
// search line is open, the static cursor cell sits after the text, so the
// status line reads "/query" and a space before the match count.
func searchFor(t *testing.T, query string) TreeModel {
	t.Helper()
	return typeText(drive(namedTree(t), "/"), query)
}

func TestSearchNarrowsTheTreeAsYouType(t *testing.T) {
	m := drive(namedTree(t), "/")
	if helpText(m) != searchHelp || statusText(m) != "/   4 panes" {
		t.Fatalf("help %q status %q", helpText(m), statusText(m))
	}
	m = typeText(m, "auth-f")
	want := []string{"▾ platform", "▾ ⑂ auth-service", "auth-fix"}
	got := shown(m)
	if len(got) != 3 || !strings.HasPrefix(got[0], want[0]) || !strings.HasPrefix(got[1], want[1]) || !strings.Contains(got[2], want[2]) {
		t.Fatalf("got %v", got)
	}
	if statusText(m) != "/auth-f   1 pane" {
		t.Errorf("status = %q", statusText(m))
	}
	if !strings.Contains(cursorText(m), "auth-fix") {
		t.Errorf("the cursor goes to the first match: %q", cursorText(m))
	}
}

func TestEnterJumpsToTheMatch(t *testing.T) {
	next, cmd := searchFor(t, "psql").Update(treeKey("enter"))
	m := next.(TreeModel)
	if cmd == nil || m.Chosen == nil || m.Chosen.Pane.PaneID != "w2:p2" {
		t.Errorf("chosen %+v", m.Chosen)
	}
}

func TestEscWhileTypingClearsTheSearch(t *testing.T) {
	m := drive(searchFor(t, "psql"), "esc")
	if m.search != nil || m.query != "" || len(shown(m)) != 8 {
		t.Errorf("query %q lines %v", m.query, shown(m))
	}
	if !strings.Contains(cursorText(m), "psql billing") {
		t.Errorf("the cursor stays on the match: %q", cursorText(m))
	}
}

// enter on a group closes the search line and keeps the query, and esc then
// clears it before a second esc closes the switcher.
func TestAKeptSearchClearsBeforeTheSwitcherCloses(t *testing.T) {
	m := drive(searchFor(t, "billing"), "up", "up", "enter")
	if m.search != nil || m.query != "billing" || m.quitting {
		t.Fatalf("search open %v query %q quitting %v", m.search != nil, m.query, m.quitting)
	}
	if !strings.Contains(visible(m.View()), "agents · tree · /billing") || statusText(m) != "/billing  2 panes · / edit · esc clear" {
		t.Errorf("status %q", statusText(m))
	}
	if m = drive(m, "/"); m.search == nil || m.search.Value() != "billing" {
		t.Fatal("/ reopens the search with its query")
	}
	m = drive(m, "enter", "up", "up")
	m = drive(m, "esc")
	if m.query != "" || m.quitting {
		t.Fatalf("the first esc clears: query %q", m.query)
	}
	if _, cmd := m.Update(treeKey("esc")); cmd == nil {
		t.Error("the second esc closes the switcher")
	}
}

func TestASearchWithNoMatchSaysSo(t *testing.T) {
	m := searchFor(t, "zzz")
	if !strings.Contains(visible(m.View()), "no pane matches /zzz") || statusText(m) != "/zzz   0 panes" {
		t.Errorf("got\n%s", visible(m.View()))
	}
	if m = drive(m, "enter"); m.search != nil || m.quitting {
		t.Error("enter with nothing to jump to only closes the search line")
	}
}

// Letters are text while the search line is open, so a management key such
// as n cannot fire by accident.
func TestKeysTypeIntoTheSearch(t *testing.T) {
	log := stubOps(t)
	m := searchFor(t, "nRx")
	if m.ask != nil || len(log.calls) != 0 || m.search.Value() != "nRx" {
		t.Errorf("ask %v calls %v value %q", m.ask != nil, log.calls, m.search.Value())
	}
}

func TestTheCursorMovesWhileSearching(t *testing.T) {
	m := searchFor(t, "claude")
	start := m.cursor
	m = drive(m, "down", "pgdown", "up", "pgup")
	if m.cursor != 0 || m.search == nil {
		t.Errorf("cursor %d from %d", m.cursor, start)
	}
	for _, k := range []string{"ctrl+n", "ctrl+p"} {
		next, _ := m.Update(tea.KeyMsg{Type: map[string]tea.KeyType{"ctrl+n": tea.KeyCtrlN, "ctrl+p": tea.KeyCtrlP}[k]})
		m = next.(TreeModel)
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d", m.cursor)
	}
	// A key that edits nothing leaves the tree as it was.
	if m = drive(m, "left"); m.cursor != 0 || m.search.Value() != "claude" {
		t.Errorf("cursor %d value %q", m.cursor, m.search.Value())
	}
	if _, cmd := m.Update(treeKey("ctrl+c")); cmd == nil {
		t.Error("ctrl+c quits from the search line")
	}
}

// A notice wins the status line over a kept search, and the search comes
// back once the notice clears.
func TestANoticeTakesTheLineFromAKeptSearch(t *testing.T) {
	stubOps(t)
	m := drive(searchFor(t, "auth"), "up", "up", "enter")
	m = drive(m, "R")
	if !strings.Contains(statusText(m), "takes its name from git") {
		t.Fatalf("status = %q", statusText(m))
	}
	if m = drive(m, "down"); !strings.HasPrefix(statusText(m), "/auth") {
		t.Errorf("status = %q", statusText(m))
	}
}

// A status filter and a search narrow together.
func TestSearchWorksWithAStatusFilter(t *testing.T) {
	m := drive(namedTree(t), "d", "/")
	m = typeText(m, "claude")
	if statusText(m) != "/claude   1 pane" {
		t.Errorf("status = %q", statusText(m))
	}
}
