package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSelectionBackgroundDefaultsToCatppuccin(t *testing.T) {
	t.Setenv(selectionBgEnv, "")
	t.Setenv("HERDR_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.toml"))
	if got := selectionBackground(); got != lipgloss.Color(defaultSelectionBg) {
		t.Errorf("got %q, want %q", got, defaultSelectionBg)
	}
}

func TestSelectionBackgroundFromEnvironment(t *testing.T) {
	t.Setenv(selectionBgEnv, "  #445566  ")
	if got := selectionBackground(); got != lipgloss.Color("#445566") {
		t.Errorf("got %q, want #445566", got)
	}
}

func TestSelectionBackgroundAcceptsShortHex(t *testing.T) {
	t.Setenv(selectionBgEnv, "#abc")
	if got := selectionBackground(); got != lipgloss.Color("#abc") {
		t.Errorf("got %q, want #abc", got)
	}
}

// A malformed value must fall back rather than emit a broken escape sequence.
func TestSelectionBackgroundRejectsMalformedValues(t *testing.T) {
	t.Setenv("HERDR_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.toml"))
	for _, bad := range []string{"red", "313244", "#12", "#gggggg", "rgb(1,2,3)", ""} {
		t.Setenv(selectionBgEnv, bad)
		if got := selectionBackground(); got != lipgloss.Color(defaultSelectionBg) {
			t.Errorf("%q gave %q, want the default", bad, got)
		}
	}
}

func TestSelectionBackgroundFromConfig(t *testing.T) {
	t.Setenv(selectionBgEnv, "")
	t.Setenv("HERDR_CONFIG_PATH", writeConfig(t, `
[theme.custom]
accent = "#f5c2e7"
selection_bg = "#112233"
`))
	if got := selectionBackground(); got != lipgloss.Color("#112233") {
		t.Errorf("got %q, want #112233", got)
	}
}

// The environment wins, so a one-off override needs no config edit.
func TestEnvironmentBeatsConfig(t *testing.T) {
	t.Setenv(selectionBgEnv, "#aabbcc")
	t.Setenv("HERDR_CONFIG_PATH", writeConfig(t, "[theme.custom]\nselection_bg = \"#112233\"\n"))
	if got := selectionBackground(); got != lipgloss.Color("#aabbcc") {
		t.Errorf("got %q, want #aabbcc", got)
	}
}

func TestConfigSelectionBgOnAConfigWithoutTheKey(t *testing.T) {
	t.Setenv("HERDR_CONFIG_PATH", writeConfig(t, "[theme]\nname = \"catppuccin\"\n"))
	if got := configSelectionBg(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestConfigSelectionBgOnAMissingFile(t *testing.T) {
	t.Setenv("HERDR_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.toml"))
	if got := configSelectionBg(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// With no HERDR_CONFIG_PATH the lookup falls back to the home directory.
func TestConfigSelectionBgFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERDR_CONFIG_PATH", "")
	dir := filepath.Join(home, ".config", "herdr")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("[theme.custom]\nselection_bg = \"#654321\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := configSelectionBg(); got != "#654321" {
		t.Errorf("got %q, want #654321", got)
	}
}

// With no config path and no home directory the lookup finds nothing and the
// default color applies.
func TestConfigSelectionBgWithNoHomeDirectory(t *testing.T) {
	t.Setenv("HERDR_CONFIG_PATH", "")
	t.Setenv("HOME", "")
	if got := configSelectionBg(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
