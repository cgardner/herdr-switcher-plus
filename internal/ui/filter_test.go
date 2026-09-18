package ui

import "testing"

var targets = []string{
	"nix claude idle /Users/c/.config/infra-terraform w5:p1E rebuilt the config",
	"pricing claude blocked /Users/c/src/risk w29:p1 may I delete this branch",
	"github codex working /Users/c/src/gh w3S:p1 opening a pull request",
}

func matchedIndexes(t *testing.T, term string) []int {
	t.Helper()
	var out []int
	for _, r := range substringFilter(term, targets) {
		out = append(out, r.Index)
	}
	return out
}

func eq(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// A status term must select exactly the rows in that state. The fuzzy default
// matched nine of twelve live agents for "working", which is what this guards.
func TestSubstringFilterMatchesStatusExactly(t *testing.T) {
	if got := matchedIndexes(t, "blocked"); !eq(got, []int{1}) {
		t.Errorf("blocked = %v, want [1]", got)
	}
	if got := matchedIndexes(t, "working"); !eq(got, []int{2}) {
		t.Errorf("working = %v, want [2]", got)
	}
}

func TestSubstringFilterIsCaseInsensitive(t *testing.T) {
	if got := matchedIndexes(t, "CODEX"); !eq(got, []int{2}) {
		t.Errorf("CODEX = %v, want [2]", got)
	}
}

func TestSubstringFilterRequiresEveryTerm(t *testing.T) {
	if got := matchedIndexes(t, "claude blocked"); !eq(got, []int{1}) {
		t.Errorf("claude blocked = %v, want [1]", got)
	}
	if got := matchedIndexes(t, "codex blocked"); len(got) != 0 {
		t.Errorf("codex blocked = %v, want none", got)
	}
}

// Order must survive filtering, or the chosen sort mode is silently discarded.
func TestSubstringFilterPreservesInputOrder(t *testing.T) {
	if got := matchedIndexes(t, "claude"); !eq(got, []int{0, 1}) {
		t.Errorf("claude = %v, want [0 1] in input order", got)
	}
}

func TestSubstringFilterEmptyTermKeepsEverything(t *testing.T) {
	for _, term := range []string{"", "   "} {
		if got := matchedIndexes(t, term); !eq(got, []int{0, 1, 2}) {
			t.Errorf("%q = %v, want all", term, got)
		}
	}
}

// Matched indexes must stay empty. They index the filter value but get painted
// onto the title, and an index inside the title's ANSI color sequence splits
// it and leaks escape text into the pane.
func TestSubstringFilterReportsNoMatchedIndexes(t *testing.T) {
	for _, r := range substringFilter("infra", targets) {
		if len(r.MatchedIndexes) != 0 {
			t.Errorf("MatchedIndexes = %v, want none", r.MatchedIndexes)
		}
	}
}
