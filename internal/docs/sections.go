package docs

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/ui"
)

// Targets maps each documentation file to the sections generated into it.
//
// The token reference lives in docs/configuration.md alone. The README points
// at it rather than carrying a second copy, which is the duplication this
// package exists to remove.
func Targets() map[string][]Section {
	return map[string][]Section{
		"README.md": {
			{Name: "sort-modes", Body: SortModeTable()},
			{Name: "status-keys", Body: StatusKeyTable()},
		},
		filepath.Join("docs", "configuration.md"): {
			{Name: "tokens", Body: TokenTable()},
		},
		"example-config.toml": {
			{Name: "tokens", Body: Comment(TokenList())},
		},
		"AGENTS.md": {
			{Name: "token-count", Body: TokenCount()},
		},
	}
}

// Generate splices every section into every target under root. It returns the
// files that were out of date, and writes them unless check is set.
func Generate(root string, check bool) (stale []string, err error) {
	for path, sections := range Targets() {
		changed, err := Apply(filepath.Join(root, path), sections, !check)
		if err != nil {
			return nil, err
		}
		if changed {
			stale = append(stale, path)
		}
	}
	return stale, nil
}

// TokenTable renders the token reference.
func TokenTable() string {
	rows := make([][]string, 0, len(ui.TokenDocs()))
	for _, t := range ui.TokenDocs() {
		numeric := "—"
		if t.Numeric != "" {
			numeric = t.Numeric
		}
		rows = append(rows, []string{"`" + t.Name + "`", t.Description, numeric})
	}
	return Table([]string{"token", "shows", "number for `gt` and `lt`"}, rows)
}

// TokenList is the plain form for a TOML comment, where a markdown table would
// read badly.
func TokenList() string {
	var b strings.Builder
	width := 0
	for _, t := range ui.TokenDocs() {
		if n := len(t.Name); n > width {
			width = n
		}
	}
	for _, t := range ui.TokenDocs() {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, t.Name, t.Description)
		if t.Numeric != "" {
			fmt.Fprintf(&b, "  %-*s  gt and lt compare against this, in %s\n", width, "", t.Numeric)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// SortModeTable renders the sort-mode reference.
func SortModeTable() string {
	rows := make([][]string, 0, len(agents.ModeDocs()))
	for _, m := range agents.ModeDocs() {
		rows = append(rows, []string{"`" + m.Name + "`", m.Label, m.Description})
	}
	return Table([]string{"mode", "orders by", "answers"}, rows)
}

// StatusKeyTable renders the status-filter reference.
func StatusKeyTable() string {
	rows := make([][]string, 0, len(agents.StatusDocs()))
	for _, s := range agents.StatusDocs() {
		rows = append(rows, []string{"`" + s.Key + "`", s.Shows, s.Description})
	}
	return Table([]string{"key", "shows", "meaning"}, rows)
}

// TokenCount states how many tokens exist, so a stale number in prose becomes
// a CI failure rather than a small lie.
func TokenCount() string {
	return fmt.Sprintf("There are %d tokens, listed in `docs/configuration.md`.", len(ui.TokenDocs()))
}
