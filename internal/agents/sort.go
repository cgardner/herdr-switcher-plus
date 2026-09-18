package agents

import (
	"sort"
	"strings"
)

// Mode is one ordering the switcher offers.
type Mode string

const (
	// ModeRecent puts the newest message on top. This is the default and the
	// reason the plugin exists.
	ModeRecent Mode = "recent"

	// ModeAttention is a needs-you-first queue. It answers "where am I
	// blocking work?" rather than "what did I touch last".
	ModeAttention Mode = "attention"

	// ModeSpace groups by Herdr space in the sidebar's own order, which keeps
	// rows where muscle memory expects them.
	ModeSpace Mode = "space"

	// ModeName orders spaces alphabetically, giving a stable list that does
	// not move as agents work.
	ModeName Mode = "name"

	// ModeOldest reverses recency to surface stale sessions worth closing.
	ModeOldest Mode = "oldest"
)

// Modes is the cycle order presented in the UI.
var Modes = []Mode{ModeRecent, ModeAttention, ModeSpace, ModeName, ModeOldest}

// Label is the short name shown in the pane title.
func (m Mode) Label() string {
	switch m {
	case ModeRecent:
		return "last message"
	case ModeAttention:
		return "needs attention"
	case ModeSpace:
		return "space order"
	case ModeName:
		return "space name"
	case ModeOldest:
		return "oldest first"
	}
	return string(m)
}

// ParseMode resolves a mode name, falling back to ModeRecent so a stale saved
// value or a bad flag never leaves the switcher unusable.
func ParseMode(s string) Mode {
	m := Mode(strings.TrimSpace(strings.ToLower(s)))
	for _, known := range Modes {
		if m == known {
			return m
		}
	}
	return ModeRecent
}

// Next returns the mode after m, wrapping at the end. A step of -1 walks back.
func (m Mode) Next(step int) Mode {
	for i, known := range Modes {
		if known == m {
			n := (i + step) % len(Modes)
			if n < 0 {
				n += len(Modes)
			}
			return Modes[n]
		}
	}
	return ModeRecent
}

// attentionRank orders states by how much they want the user.
//
// blocked waits on an answer, so nothing outranks it. done is finished work
// nobody has looked at. idle is ready for the next instruction. working needs
// no one. unknown means Herdr could not classify the pane, which proves
// nothing, so it sits last.
func attentionRank(status string) int {
	switch status {
	case "blocked":
		return 0
	case "done":
		return 1
	case "idle":
		return 2
	case "working":
		return 3
	default:
		return 4
	}
}

// newer reports whether i carries a more recent message than j. A row with no
// transcript has no wall-clock age and never outranks one that has.
func newer(i, j Row) bool {
	if i.HasLast != j.HasLast {
		return i.HasLast
	}
	if i.HasLast {
		return i.Last.At.After(j.Last.At)
	}
	return i.Agent.StateChangeSeq > j.Agent.StateChangeSeq
}

// Apply orders rows in place for the given mode.
func Apply(mode Mode, rows []Row) {
	switch mode {
	case ModeAttention:
		sortBy(rows, func(a, b Row) (bool, bool) {
			ra, rb := attentionRank(a.Agent.Status), attentionRank(b.Agent.Status)
			return ra < rb, ra == rb
		})
	case ModeSpace:
		sortBy(rows, func(a, b Row) (bool, bool) {
			return a.SpaceNumber < b.SpaceNumber, a.SpaceNumber == b.SpaceNumber
		})
	case ModeName:
		sortBy(rows, func(a, b Row) (bool, bool) {
			la, lb := strings.ToLower(a.Space), strings.ToLower(b.Space)
			return la < lb, la == lb
		})
	case ModeOldest:
		// Rows with no transcript stay at the bottom here too, because an
		// unknown age is not evidence of being stale.
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := rows[i], rows[j]
			if a.HasLast != b.HasLast {
				return a.HasLast
			}
			if a.HasLast {
				return a.Last.At.Before(b.Last.At)
			}
			return a.Agent.StateChangeSeq < b.Agent.StateChangeSeq
		})
	default:
		Sort(rows)
	}
}

// sortBy applies a primary comparison and breaks every tie by recency, so
// every mode still reads newest-first inside a group.
func sortBy(rows []Row, primary func(a, b Row) (less bool, equal bool)) {
	sort.SliceStable(rows, func(i, j int) bool {
		less, equal := primary(rows[i], rows[j])
		if !equal {
			return less
		}
		return newer(rows[i], rows[j])
	})
}

// ModeDoc is one row of the generated sort-mode reference.
type ModeDoc struct {
	Name        string
	Label       string
	Description string
}

// modeDescriptions says what each ordering answers. It lives beside the modes
// so the reference documentation is generated rather than restated, and a new
// mode cannot ship with a stale sentence.
var modeDescriptions = map[Mode]string{
	ModeRecent:    "where was I?",
	ModeAttention: "where am I blocking work?",
	ModeSpace:     "match the sidebar's own order",
	ModeName:      "a list that does not move as agents work",
	ModeOldest:    "what can I close?",
}

// ModeDocs returns every sort mode in cycle order.
func ModeDocs() []ModeDoc {
	out := make([]ModeDoc, 0, len(Modes))
	for _, m := range Modes {
		out = append(out, ModeDoc{Name: string(m), Label: m.Label(), Description: modeDescriptions[m]})
	}
	return out
}
