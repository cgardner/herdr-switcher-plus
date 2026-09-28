package config

import (
	"fmt"
	"sort"
	"strings"
)

// Views are the ways the switcher can draw the session. The description lives
// here so the reference documentation is generated from it.
var Views = map[string]string{
	"list": "every agent on its own row, in the chosen sort order",
	"tree": "every pane, shells included, under its workspace and repository",
}

// DefaultView is the view when neither a flag, an action nor the config names
// one.
const DefaultView = "list"

// ViewNames lists the views in a stable order, for error messages and
// generated documentation.
func ViewNames() []string {
	names := make([]string, 0, len(Views))
	for name := range Views {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateView reports a view name that is not in the registry. An empty name
// is valid, and means the next source decides. A caller warns about a bad
// name and opens DefaultView, rather than refusing to open at all.
func ValidateView(name string) error {
	if _, ok := Views[name]; name != "" && !ok {
		return fmt.Errorf("view %q is not one of %s", name, strings.Join(ViewNames(), ", "))
	}
	return nil
}
