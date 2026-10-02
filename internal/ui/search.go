package ui

import (
	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The search narrows the tree as the user types. It keeps the tree's order,
// as the list filter keeps the sort, and it opens every group, as a status
// filter does, so that a match is never folded out of sight.

const searchHelp = "type to search · ↑/↓ move · enter jump · esc clear"

// startSearch opens the search line, holding any query already set. The input
// has no width, because a width pads it with spaces and pushes the match count
// to the far edge. CharLimit bounds it instead.
func (m *TreeModel) startSearch() {
	in := textinput.New()
	in.Prompt = "/"
	in.CharLimit = 80
	in.Cursor.SetMode(cursor.CursorStatic)
	in.SetValue(m.query)
	in.CursorEnd()
	in.Focus()
	m.search = &in
}

// setQuery applies a new query and puts the cursor on the first matching
// pane, so that enter jumps to the best match at once. Clearing the query
// keeps the cursor on its line instead, so the user lands where the search
// led them.
func (m *TreeModel) setQuery(q string) {
	if q == m.query {
		return
	}
	m.query = q
	m.collapsed = map[string]bool{}
	if q == "" {
		m.relayout(m.currentID())
		return
	}
	m.relayout("")
	for i, l := range m.lines {
		if l.Node.Kind == tree.KindPane {
			m.cursor = i
			break
		}
	}
	m.scroll()
}

// searchKey handles a key while the search line has the keyboard.
func (m TreeModel) searchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.search = nil
		m.setQuery("")
		return m, nil
	case "up", "ctrl+p":
		m.move(-1)
		return m, nil
	case "down", "ctrl+n":
		m.move(1)
		return m, nil
	case "pgup":
		m.move(-m.body())
		return m, nil
	case "pgdown":
		m.move(m.body())
		return m, nil
	case "enter":
		// The search line closes and keeps its query. On a pane, enter
		// also jumps, which is what a search for one pane is for.
		m.search = nil
		if n := m.current(); n != nil && n.Kind == tree.KindPane {
			m.Chosen, m.quitting = n, true
			return m, tea.Quit
		}
		return m, nil
	}
	in, cmd := m.search.Update(msg)
	m.search = &in
	m.setQuery(in.Value())
	return m, cmd
}

// matchCount counts the panes the search left.
func (m TreeModel) matchCount() int {
	n := 0
	for _, l := range m.lines {
		if l.Node.Kind == tree.KindPane {
			n++
		}
	}
	return n
}

// searchLine is the status line while a search is open or set: the input,
// or the query, with the number of panes it matches.
func (m TreeModel) searchLine() string {
	dim := styled(lipgloss.NewStyle(), previewColor, false, false)
	tail := dim.Render("  " + count(m.matchCount(), "pane"))
	if m.search != nil {
		return " " + m.search.View() + tail
	}
	return " " + styled(lipgloss.NewStyle(), accentColor, false, false).Render("/"+m.query) + tail + dim.Render(" · / edit · esc clear")
}
