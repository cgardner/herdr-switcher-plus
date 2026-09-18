package state

import (
	"os"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	SaveMode("attention")
	if got := LoadMode(); got != "attention" {
		t.Errorf("LoadMode = %q, want attention", got)
	}
}

func TestLoadModeEmptyWhenNothingSaved(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	if got := LoadMode(); got != "" {
		t.Errorf("LoadMode = %q, want empty", got)
	}
}

// A state directory that cannot be created must not stop the switcher, so the
// save is silent and the load simply reports nothing.
func TestUnwritableStateDirIsSilent(t *testing.T) {
	f := t.TempDir() + "/not-a-dir"
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_PLUGIN_STATE_DIR", f)
	SaveMode("attention")
	if got := LoadMode(); got != "" {
		t.Errorf("LoadMode = %q, want empty", got)
	}
}

// With no HERDR_PLUGIN_STATE_DIR the mode still persists, under the user cache
// directory, so the switcher remembers itself when run outside Herdr.
func TestFallsBackToTheUserCacheDirectory(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", "")
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)

	SaveMode("oldest")
	if got := LoadMode(); got != "oldest" {
		t.Errorf("LoadMode = %q, want oldest", got)
	}
}

func TestSaveTrimsTrailingWhitespaceOnLoad(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", t.TempDir())
	SaveMode("space")
	if got := LoadMode(); got != "space" {
		t.Errorf("LoadMode = %q, want space with no newline", got)
	}
}

// With neither a state directory nor a home directory there is nowhere to
// write. That must stay silent, because losing a remembered sort mode is a far
// smaller problem than refusing to open the switcher.
func TestNoWritableLocationIsSilent(t *testing.T) {
	t.Setenv("HERDR_PLUGIN_STATE_DIR", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	SaveMode("attention")
	if got := LoadMode(); got != "" {
		t.Errorf("LoadMode = %q, want empty", got)
	}
}
