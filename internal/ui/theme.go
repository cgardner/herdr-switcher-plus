package ui

import (
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// defaultSelectionBg is Surface0 from catppuccin, which is Herdr's default
// theme and the color its own overlays use to mark the selected row.
//
// Herdr resolves a named theme inside the binary and exposes no API for the
// resulting palette, so a different named theme cannot be read. Someone on
// another theme sets the color explicitly, either with a [theme.custom]
// selection_bg override in the Herdr config or with the environment variable
// below.
const defaultSelectionBg = "#313244"

const selectionBgEnv = "HERDR_SWITCHER_PLUS_SELECTION_BG"

var hexColor = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// selectionBackground resolves the highlight color: the environment variable
// first, then an explicit selection_bg in the Herdr config, then the
// catppuccin default. A malformed value falls back rather than rendering a
// broken escape sequence.
func selectionBackground() lipgloss.Color {
	if v := strings.TrimSpace(os.Getenv(selectionBgEnv)); hexColor.MatchString(v) {
		return lipgloss.Color(v)
	}
	if v := configSelectionBg(); hexColor.MatchString(v) {
		return lipgloss.Color(v)
	}
	return lipgloss.Color(defaultSelectionBg)
}

var selectionBgLine = regexp.MustCompile(`(?m)^\s*selection_bg\s*=\s*"([^"]*)"`)

// configSelectionBg reads an explicit override out of the Herdr config.
//
// This reads one key with one expression rather than parsing TOML, because a
// miss is harmless: the caller validates the result and falls back to the
// default. A parser dependency would buy nothing that matters here.
func configSelectionBg() string {
	path := os.Getenv("HERDR_CONFIG_PATH")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = home + "/.config/herdr/config.toml"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	m := selectionBgLine.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}
