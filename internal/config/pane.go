package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// PluginID is this plugin's registered id. It is the name `herdr plugin` needs
// to open a pane, so it lives here rather than being repeated at each call.
const PluginID = "cgardner.herdr-switcher-plus"

// Entrypoint is the pane declared in herdr-plugin.toml.
const Entrypoint = "recent"

// Placements are the surfaces Herdr can open a plugin pane on.
//
// Herdr owns where each one sits and offers no anchor or position parameter,
// so choosing between them is the whole of the control available. A popup is
// session-modal and centres on the pane area rather than the window, which is
// why someone might prefer one of the others.
var Placements = map[string]string{
	"popup":   "session-modal, centred on the pane area, sized by width and height",
	"overlay": "a temporary zoom filling the tab",
	"zoomed":  "the tab, zoomed to one pane",
	"split":   "a new split beside the current pane",
	"tab":     "a new tab",
}

// PlacementNames lists the placements in a stable order, for error messages
// and generated documentation.
func PlacementNames() []string {
	names := make([]string, 0, len(Placements))
	for name := range Placements {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// popupSize matches what Herdr accepts: whole terminal cells, or a percentage
// from 1 to 100. Rejecting anything else here means a typo is reported with
// the file name rather than arriving as an opaque CLI error.
var popupSize = regexp.MustCompile(`^([0-9]{1,5}|(100|[1-9][0-9]?)%)$`)

// Pane is how the switcher's own pane is opened.
type Pane struct {
	Placement string `toml:"placement"`
	Width     string `toml:"width"`
	Height    string `toml:"height"`
}

// Validate reports the first problem, naming the key so the message points at
// the line to fix.
func (p Pane) Validate() error {
	if p.Placement != "" {
		if _, ok := Placements[p.Placement]; !ok {
			return fmt.Errorf("placement %q is not one of %s",
				p.Placement, strings.Join(PlacementNames(), ", "))
		}
	}
	for key, value := range map[string]string{"width": p.Width, "height": p.Height} {
		if value != "" && !popupSize.MatchString(value) {
			return fmt.Errorf("%s %q must be a number of cells or a percentage such as \"80%%\"", key, value)
		}
	}
	return nil
}

// OpenArgs builds the `herdr plugin pane open` arguments.
//
// Width and height are passed only when set, because Herdr applies the
// manifest's own values otherwise, and a placement that is not a popup ignores
// them entirely. The mode and the view travel as environment variables,
// because the pane entrypoint has one fixed command.
func (p Pane) OpenArgs(mode, view string) []string {
	args := []string{"plugin", "pane", "open", "--plugin", PluginID, "--entrypoint", Entrypoint, "--focus"}
	if p.Placement != "" {
		args = append(args, "--placement", p.Placement)
	}
	if p.Width != "" {
		args = append(args, "--width", p.Width)
	}
	if p.Height != "" {
		args = append(args, "--height", p.Height)
	}
	if mode != "" {
		args = append(args, "--env", "HERDR_SWITCHER_PLUS_MODE="+mode)
	}
	if view != "" {
		args = append(args, "--env", "HERDR_SWITCHER_PLUS_VIEW="+view)
	}
	return args
}
