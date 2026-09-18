package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
)

// substringFilter narrows the list by case-insensitive substring, requiring
// every space-separated term to match. Two properties matter here.
//
// It does not reorder. The bubbles default ranks by fuzzy score, which would
// override whichever sort mode the user chose, so filtering would silently
// undo sorting.
//
// It does not match loosely. Fuzzy matching spreads a term's letters across
// the whole row, so "working" matched nine of twelve agents when only one was
// working. Status and agent kind sit in the filter value precisely so that
// "/blocked" or "/codex" narrows exactly, and a fuzzy match destroys that.
//
// It reports no matched indexes. The delegate paints them onto the rendered
// title, but they index the filter value, which is a different and longer
// string carrying status, kind, cwd and message text. A stale row's title
// already holds an ANSI color sequence, and an index landing inside that
// sequence splits it and leaks "[90m" into the pane. Losing the highlight is
// the cheaper trade.
func substringFilter(term string, targets []string) []list.Rank {
	terms := strings.Fields(strings.ToLower(term))
	if len(terms) == 0 {
		ranks := make([]list.Rank, len(targets))
		for i := range targets {
			ranks[i] = list.Rank{Index: i}
		}
		return ranks
	}

	var out []list.Rank
	for i, target := range targets {
		lower := strings.ToLower(target)
		ok := true
		for _, t := range terms {
			if !strings.Contains(lower, t) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, list.Rank{Index: i})
		}
	}
	return out
}
