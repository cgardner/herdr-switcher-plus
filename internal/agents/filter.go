package agents

// StatusFilter narrows the list to one agent state.
//
// The keys mirror Herdr's own workspace picker, whose footer reads
// "filter a/b/w/i/d". Matching it keeps one set of muscle memory across both
// overlays.
type StatusFilter string

const (
	// StatusAll disables the filter.
	StatusAll StatusFilter = ""

	StatusBlocked StatusFilter = "blocked"
	StatusWorking StatusFilter = "working"
	StatusIdle    StatusFilter = "idle"
	StatusDone    StatusFilter = "done"
)

// StatusKeys maps each picker key to the state it selects, in footer order.
var StatusKeys = []struct {
	Key    string
	Status StatusFilter
}{
	{"a", StatusAll},
	{"b", StatusBlocked},
	{"w", StatusWorking},
	{"i", StatusIdle},
	{"d", StatusDone},
}

// ParseStatus resolves a status name or a single picker key. Anything
// unrecognized clears the filter, so a bad flag shows everything rather than
// hiding the list.
func ParseStatus(s string) StatusFilter {
	for _, e := range StatusKeys {
		if s == e.Key || s == string(e.Status) {
			return e.Status
		}
	}
	return StatusAll
}

// Label names the active filter for the pane title.
func (f StatusFilter) Label() string {
	if f == StatusAll {
		return "all"
	}
	return string(f)
}

// Keep returns the rows matching the filter.
//
// Herdr's picker offers no key for the unknown state, and matching that means
// an unknown agent is reachable only under "all". That is deliberate: unknown
// means Herdr could not classify the pane, so it belongs in no state's list.
func (f StatusFilter) Keep(rows []Row) []Row {
	if f == StatusAll {
		return rows
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		if r.Agent.Status == string(f) {
			out = append(out, r)
		}
	}
	return out
}
