package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cgardner/herdr-switcher-plus/internal/config"
)

const examplePath = "../../example-config.toml"

// uncommentedBlocks pulls each commented-out `rows = [...]` example out of the
// shipped example config.
func uncommentedBlocks(t *testing.T, src string) []string {
	t.Helper()
	var blocks []string
	var cur []string
	collecting := false

	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if !collecting && strings.HasPrefix(trimmed, "# rows = [") {
			collecting = true
		}
		if !collecting {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			collecting = false
			cur = nil
			continue
		}
		cur = append(cur, strings.TrimPrefix(strings.TrimPrefix(trimmed, "#"), " "))
		if strings.TrimSpace(cur[len(cur)-1]) == "]" {
			blocks = append(blocks, strings.Join(cur, "\n"))
			collecting = false
			cur = nil
		}
	}
	return blocks
}

// Every example in the shipped config must load and validate. Documentation
// that drifts out of step with the token registry is worse than none, because
// a user copies it and gets an error.
func TestShippedExamplesAreValid(t *testing.T) {
	src, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}

	blocks := uncommentedBlocks(t, string(src))
	if len(blocks) < 4 {
		t.Fatalf("found %d examples, expected the file to carry several", len(blocks))
	}

	for i, block := range blocks {
		dir := t.TempDir()
		t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)
		body := "[ui]\n" + block + "\n"
		if err := os.WriteFile(filepath.Join(dir, config.FileName), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := defaultConfig()
		if err != nil {
			t.Errorf("example %d does not load:\n%s\nerror: %v", i+1, body, err)
			continue
		}
		if len(cfg.UI.Rows) == 0 {
			t.Errorf("example %d parsed to no rows:\n%s", i+1, body)
		}
	}
}

// The active, uncommented layout in the example file must match the built-in
// default, so the shipped file is a faithful starting point rather than a
// silent change of behaviour.
func TestTheActiveExampleLoads(t *testing.T) {
	src, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, config.FileName), src, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := defaultConfig()
	if err != nil {
		t.Fatalf("the shipped example must load as written: %v", err)
	}
	if len(cfg.UI.Rows) != 2 {
		t.Fatalf("got %d rows, want the two of the built-in layout", len(cfg.UI.Rows))
	}
	want := []string{"age", "label", "state_icon", "state_text"}
	for i, name := range want {
		if cfg.UI.Rows[0][i].Name != name {
			t.Errorf("row 1 token %d = %q, want %q", i, cfg.UI.Rows[0][i].Name, name)
		}
	}
}
