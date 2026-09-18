// Package state remembers the chosen sort mode between openings.
//
// Herdr offers no plugin storage API, and the docs are explicit that a plugin
// keeps its own state in HERDR_PLUGIN_STATE_DIR rather than under the plugin
// root. Every failure here is silent: a forgotten sort mode is a small
// annoyance, and refusing to open the switcher over it would be worse.
package state

import (
	"os"
	"path/filepath"
	"strings"
)

const fileName = "sort-mode"

func path() string {
	dir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(cache, "herdr-switcher-plus")
	}
	return filepath.Join(dir, fileName)
}

// LoadMode returns the saved mode name, or an empty string when none is saved.
func LoadMode() string {
	p := path()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// SaveMode records the mode name for the next opening.
func SaveMode(mode string) {
	p := path()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return
	}
	_ = os.WriteFile(p, []byte(mode+"\n"), 0o600)
}
