package config

import (
	"strings"
	"testing"
)

func args(p Pane, mode string) string { return strings.Join(p.OpenArgs(mode), " ") }

// With nothing configured the arguments carry no placement or size, so Herdr
// applies the manifest's own values and today's behaviour is unchanged.
func TestOpenArgsWithoutConfiguration(t *testing.T) {
	got := args(Pane{}, "")
	want := "plugin pane open --plugin " + PluginID + " --entrypoint " + Entrypoint + " --focus"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestOpenArgsCarriesEveryConfiguredField(t *testing.T) {
	got := args(Pane{Placement: "overlay", Width: "70%", Height: "40"}, "")
	for _, want := range []string{"--placement overlay", "--width 70%", "--height 40"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q is missing from %q", want, got)
		}
	}
}

// A sort mode travels as an environment variable, because a manifest pane has
// one fixed command and no way to take an argument.
func TestOpenArgsCarriesTheSortMode(t *testing.T) {
	if got := args(Pane{}, "attention"); !strings.Contains(got, "--env HERDR_SWITCHER_PLUS_MODE=attention") {
		t.Errorf("mode not passed: %q", got)
	}
	if got := args(Pane{}, ""); strings.Contains(got, "--env") {
		t.Errorf("no mode should mean no --env: %q", got)
	}
}

func TestValidatePlacement(t *testing.T) {
	for name := range Placements {
		if err := (Pane{Placement: name}).Validate(); err != nil {
			t.Errorf("placement %q should be accepted: %v", name, err)
		}
	}
	if err := (Pane{}).Validate(); err != nil {
		t.Errorf("an empty pane section is valid: %v", err)
	}
	err := (Pane{Placement: "floating"}).Validate()
	if err == nil {
		t.Fatal("expected an error")
	}
	// The message has to list the alternatives, or a typo leaves the reader
	// guessing what is allowed.
	for _, name := range PlacementNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error should list %q: %v", name, err)
		}
	}
}

func TestValidateSizes(t *testing.T) {
	good := []string{"80%", "1%", "100%", "40", "0", "65535"}
	for _, v := range good {
		if err := (Pane{Width: v, Height: v}).Validate(); err != nil {
			t.Errorf("%q should be accepted: %v", v, err)
		}
	}
	bad := []string{"80 %", "101%", "0%", "eighty", "80px", "-4", "%80", "1000000"}
	for _, v := range bad {
		if err := (Pane{Width: v}).Validate(); err == nil {
			t.Errorf("width %q should be rejected", v)
		}
		if err := (Pane{Height: v}).Validate(); err == nil {
			t.Errorf("height %q should be rejected", v)
		}
	}
}

// The error names which key is wrong, since width and height fail the same way.
func TestValidateNamesTheFailingKey(t *testing.T) {
	if err := (Pane{Height: "tall"}).Validate(); err == nil || !strings.Contains(err.Error(), "height") {
		t.Errorf("got %v", err)
	}
}

func TestPlacementsAreDescribed(t *testing.T) {
	for name, desc := range Placements {
		if strings.TrimSpace(desc) == "" {
			t.Errorf("placement %q has no description", name)
		}
	}
	if len(PlacementNames()) != len(Placements) {
		t.Error("PlacementNames must list every placement")
	}
}
