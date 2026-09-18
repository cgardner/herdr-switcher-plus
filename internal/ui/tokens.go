package ui

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
)

// cell is what one token draws for one agent: the text, an optional number for
// the gt and lt rules, and the color the switcher would use if the config says
// nothing.
type cell struct {
	text     string
	number   *float64
	fg       string
	dim      bool
	alignEnd bool
}

// tokenFunc produces a cell. selected is passed because several tokens dim
// differently on the selection bar, where the usual gray loses contrast.
type tokenFunc func(r agents.Row, now time.Time, selected bool) cell

func num(f float64) *float64 { return &f }

// tokens is the registry. Every name here is configurable in a row, and any
// name absent from it is rejected at load time with the row number.
var tokens = map[string]tokenFunc{
	// age carries a number so a rule can say { gt = 1440, dim = true } to mean
	// "older than a day". Minutes is the unit because it is the finest scale
	// the text itself shows.
	"age": func(r agents.Row, now time.Time, selected bool) cell {
		c := cell{text: r.Age(now), fg: ageColorFor(r, now, selected), alignEnd: true}
		if r.HasLast {
			c.number = num(now.Sub(r.Last.At).Minutes())
		}
		return c
	},
	"label": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: r.Label(), fg: textColorFor(selected)}
	},
	"space": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: r.Space, fg: textColorFor(selected)}
	},
	"repo": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: r.Repo, fg: textColorFor(selected)}
	},
	"branch": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: r.Branch, fg: textColorFor(selected)}
	},
	"state_icon": func(r agents.Row, _ time.Time, _ bool) cell {
		return cell{text: glyph(r.Agent.Status), fg: statusColors[r.Agent.Status]}
	},
	"state_text": func(r agents.Row, _ time.Time, _ bool) cell {
		return cell{text: r.Agent.Status, fg: statusColors[r.Agent.Status]}
	},
	"agent": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: r.Agent.Kind, fg: previewColorFor(selected)}
	},
	"pane": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: r.Agent.PaneID, fg: previewColorFor(selected)}
	},
	"cwd": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: shortenHome(r.Agent.Cwd), fg: previewColorFor(selected)}
	},
	"terminal_title": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: r.Agent.Title, fg: previewColorFor(selected)}
	},
	"role": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: strings.TrimSpace(roleMark(r.Last.Role)), fg: previewColorFor(selected)}
	},
	"message": func(r agents.Row, _ time.Time, selected bool) cell {
		return cell{text: previewText(r), fg: previewColorFor(selected), dim: true}
	},
}

// knownToken reports whether a name may appear in a row.
func knownToken(name string) bool {
	_, ok := tokens[name]
	return ok
}

// tokenNames lists every token, for the error message a bad config earns.
func tokenNames() []string {
	names := make([]string, 0, len(tokens))
	for name := range tokens {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// shortenHome keeps a path readable in a column by writing the home directory
// as a tilde, the way a shell prompt does.
func shortenHome(path string) string {
	if home == "" || !strings.HasPrefix(path, home) {
		return path
	}
	return "~" + strings.TrimPrefix(path, home)
}

// home is resolved once, because every row with a cwd token needs it.
var home = userHome()

func userHome() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// KnownToken reports whether a token name may appear in a configured row. The
// config package validates against it so an unknown name is reported with the
// row number rather than silently drawing nothing.
func KnownToken(name string) bool { return knownToken(name) }

// TokenNames lists every configurable token.
func TokenNames() []string { return tokenNames() }
