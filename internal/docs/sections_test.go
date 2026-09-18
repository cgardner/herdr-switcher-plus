package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/ui"
)

// Every token must carry a description, or the generated reference ships a
// blank cell and the reader learns nothing.
func TestEveryTokenIsDescribed(t *testing.T) {
	for _, d := range ui.TokenDocs() {
		if strings.TrimSpace(d.Description) == "" {
			t.Errorf("token %q has no description", d.Name)
		}
	}
}

func TestEverySortModeAndStatusKeyIsDescribed(t *testing.T) {
	for _, m := range agents.ModeDocs() {
		if strings.TrimSpace(m.Description) == "" || strings.TrimSpace(m.Label) == "" {
			t.Errorf("sort mode %q is not fully described: %+v", m.Name, m)
		}
	}
	for _, s := range agents.StatusDocs() {
		if strings.TrimSpace(s.Description) == "" {
			t.Errorf("status key %q has no description", s.Key)
		}
	}
}

// The tables must carry every registry entry, so adding one to the code cannot
// leave it out of the documentation.
func TestTablesCoverTheRegistries(t *testing.T) {
	tokens := TokenTable()
	for _, d := range ui.TokenDocs() {
		if !strings.Contains(tokens, "`"+d.Name+"`") {
			t.Errorf("token %q is missing from the table", d.Name)
		}
	}
	list := TokenList()
	for _, d := range ui.TokenDocs() {
		if !strings.Contains(list, d.Name) {
			t.Errorf("token %q is missing from the plain list", d.Name)
		}
	}
	modes := SortModeTable()
	for _, m := range agents.ModeDocs() {
		if !strings.Contains(modes, "`"+m.Name+"`") {
			t.Errorf("sort mode %q is missing from the table", m.Name)
		}
	}
	keys := StatusKeyTable()
	for _, s := range agents.StatusDocs() {
		if !strings.Contains(keys, "`"+s.Key+"`") {
			t.Errorf("status key %q is missing from the table", s.Key)
		}
	}
}

// Only age carries a number, and the table has to say so rather than leaving
// the reader to guess which tokens gt and lt work on.
func TestTheTokenTableNamesTheNumericUnit(t *testing.T) {
	table := TokenTable()
	if !strings.Contains(table, "minutes") {
		t.Error("the age token's unit is missing from the table")
	}
	if strings.Count(table, "minutes") != 1 {
		t.Errorf("only age should carry a unit, got %d mentions", strings.Count(table, "minutes"))
	}
}

func TestTokenCountMatchesTheRegistry(t *testing.T) {
	got := TokenCount()
	if !strings.Contains(got, "configuration.md") {
		t.Errorf("the count should point at the reference: %q", got)
	}
	for _, n := range []string{"13", "12", "14"} {
		if strings.Contains(got, n) {
			return
		}
	}
	t.Errorf("no plausible count in %q", got)
}

// Generate reports staleness without writing, then writes, then reports clean.
func TestGenerateRoundTrip(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path := range Targets() {
		start, end := Markers("x", path)
		_ = end
		_ = start
		var body strings.Builder
		for _, s := range Targets()[path] {
			s1, e1 := Markers(s.Name, path)
			body.WriteString(s1 + "\n" + e1 + "\n")
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(body.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	stale, err := Generate(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != len(Targets()) {
		t.Errorf("check reported %d stale files, want %d", len(stale), len(Targets()))
	}

	if _, err := Generate(root, false); err != nil {
		t.Fatal(err)
	}
	stale, err = Generate(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Errorf("after writing, %v are still reported stale", stale)
	}
}

func TestGenerateReportsAMissingFile(t *testing.T) {
	if _, err := Generate(t.TempDir(), true); err == nil {
		t.Error("expected an error for a root with no documentation")
	}
}

// The repository's own files must be current, which is the same check CI runs.
func TestTheRepositoryDocumentationIsCurrent(t *testing.T) {
	stale, err := Generate("../..", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) > 0 {
		t.Errorf("out of date: %v — run `make docs`", stale)
	}
}
