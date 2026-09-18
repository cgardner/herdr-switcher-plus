// Package config loads the plugin's own settings.
//
// Herdr offers no plugin storage API and states that a plugin keeps its
// configuration in HERDR_PLUGIN_CONFIG_DIR, which Herdr creates and
// `herdr plugin config-dir` prints. This reads config.toml from there.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/cgardner/herdr-switcher-plus/internal/layout"
)

// FileName is the config file inside the plugin config directory.
const FileName = "config.toml"

// Config is the whole file.
type Config struct {
	UI struct {
		layout.Spec
	} `toml:"ui"`
}

// Dir returns the directory holding the config file, preferring the one Herdr
// injects so a plugin pane and a shell invocation read the same file.
func Dir() string {
	if d := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "herdr", "plugins", "config", "cgardner.herdr-switcher-plus")
}

// Path is the config file this plugin reads.
func Path() string {
	dir := Dir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, FileName)
}

// Load reads the config, returning the zero value when no file exists.
//
// A missing file is the normal case and never an error. A malformed one is,
// because silently falling back to defaults would leave a user staring at an
// unchanged switcher with no idea why their config did nothing.
func Load(known func(string) bool) (Config, error) {
	var c Config
	path := Path()
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read %s: %w", path, err)
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if len(c.UI.Rows) > 0 {
		if err := c.UI.Spec.Validate(known); err != nil {
			return Config{}, fmt.Errorf("%s: %w", path, err)
		}
	}
	return c, nil
}
