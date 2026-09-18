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
