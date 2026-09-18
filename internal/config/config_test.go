package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func known(name string) bool {
	switch name {
	case "age", "label", "state_icon", "state_text", "message", "role":
		return true
	}
	return false
}

func withConfig(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// No config file is the normal case for most users, so it is never an error.
func TestLoadWithNoFile(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", t.TempDir())
	c, err := Load(known)
	if err != nil {
		t.Fatalf("a missing config must not fail: %v", err)
	}
	if len(c.UI.Rows) != 0 {
		t.Errorf("expected no rows, got %+v", c.UI.Rows)
	}
}

func TestLoadReadsRows(t *testing.T) {
	withConfig(t, `
[ui]
row_gap = 1
rows = [
  ["age", { token = "label", bold = true }],
  ["message"],
]
`)
	c, err := Load(known)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.UI.Rows) != 2 {
		t.Fatalf("got %d rows", len(c.UI.Rows))
	}
	if c.UI.RowGap != 1 {
		t.Errorf("row_gap = %d, want 1", c.UI.RowGap)
	}
	if c.UI.Rows[0][1].Name != "label" {
		t.Errorf("second token = %q", c.UI.Rows[0][1].Name)
	}
}

// Malformed TOML stops the switcher. Falling back to the built-in layout would
// leave the user staring at an unchanged pane with no idea why.
func TestLoadRejectsMalformedToml(t *testing.T) {
	withConfig(t, "[ui\nrows = ")
	if _, err := Load(known); err == nil {
		t.Fatal("expected an error")
	}
}

// A validation failure names the file and the problem, so the message points
// at the line to fix.
func TestLoadReportsAnUnknownToken(t *testing.T) {
	withConfig(t, `[ui]`+"\n"+`rows = [["age", "nonsense"]]`)
	_, err := Load(known)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "nonsense") || !strings.Contains(err.Error(), FileName) {
		t.Errorf("error should name the token and the file, got %v", err)
	}
}

func TestLoadRejectsAnEmptyRow(t *testing.T) {
	withConfig(t, `[ui]`+"\n"+`rows = [[]]`)
	if _, err := Load(known); err == nil {
		t.Error("an empty row should be rejected")
	}
}

// A file with settings but no rows is valid: it leaves the layout alone.
func TestLoadWithoutRowsIsValid(t *testing.T) {
	withConfig(t, "[ui]\nrow_gap = 2\n")
	c, err := Load(known)
	if err != nil {
		t.Fatal(err)
	}
	if c.UI.RowGap != 2 || len(c.UI.Rows) != 0 {
		t.Errorf("got %+v", c.UI)
	}
}

func TestDirPrefersTheInjectedPath(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "/injected")
	if got := Dir(); got != "/injected" {
		t.Errorf("Dir = %q", got)
	}
}

// Outside a Herdr pane the directory is derived from home, so running the
// binary from a shell reads the same file.
func TestDirFallsBackToHome(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")
	t.Setenv("HOME", "/home/c")
	want := "/home/c/.config/herdr/plugins/config/cgardner.herdr-switcher-plus"
	if got := Dir(); got != want {
		t.Errorf("Dir = %q, want %q", got, want)
	}
	if got := Path(); got != filepath.Join(want, FileName) {
		t.Errorf("Path = %q", got)
	}
}

func TestLoadWithNoResolvableDirectory(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", "")
	t.Setenv("HOME", "")
	if Path() != "" {
		t.Fatal("expected no path")
	}
	if _, err := Load(known); err != nil {
		t.Errorf("an unresolvable directory must not fail: %v", err)
	}
}

// A directory where the file should be is a read error, not a missing file.
func TestLoadReportsAReadError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_CONFIG_DIR", dir)
	if err := os.MkdirAll(filepath.Join(dir, FileName), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(known); err == nil {
		t.Error("expected a read error")
	}
}
