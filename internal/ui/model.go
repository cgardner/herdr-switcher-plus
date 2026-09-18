// Package ui renders the agent list as a Bubble Tea program.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/cgardner/herdr-pane-sort/internal/agents"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const spaceWidth = 22

var (
	ageStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	staleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	previewStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	titleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
)

// statusStyles colors each agent lifecycle state. Herdr reports blocked when
// it recognizes an approval or question UI, which is the state most worth
// spotting in a long list.
var statusStyles = map[string]lipgloss.Style{
	"blocked": lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	"working": lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	"done":    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	"idle":    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	"unknown": lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
}

func glyph(status string) string {
	if status == "idle" || status == "unknown" {
		return "○"
	}
	return "●"
}

// item adapts a row to the bubbles list interface.
type item struct {
	row agents.Row
	now time.Time
}

func (i item) Title() string {
	age := i.row.Age(i.now)
	style := ageStyle
	if !i.row.HasLast || i.now.Sub(i.row.Last.At) > 24*time.Hour {
		style = staleStyle
	}
	st := statusStyles[i.row.Agent.Status]
	space := pad(i.row.Space, spaceWidth)
	return fmt.Sprintf("%s  %s %s %s",
		style.Render(padLeft(age, 4)),
		space,
		st.Render(glyph(i.row.Agent.Status)),
		st.Render(i.row.Agent.Status))
}

func (i item) Description() string {
	text := i.row.Last.Text
	if text == "" {
		text = i.row.Agent.Title
	}
	if text == "" {
		text = "(no message found)"
	}
	who := "  "
	if i.row.Last.Role == "user" {
		who = "› "
	} else if i.row.Last.Role == "assistant" {
		who = "‹ "
	}
	return previewStyle.Render(strings.Repeat(" ", 6) + who + text)
}

// FilterValue drives the type-to-filter search. Status and agent kind are
// included so "/blocked" or "/codex" narrows the list without a separate
// filter control.
func (i item) FilterValue() string {
	return strings.Join([]string{
		i.row.Space,
		i.row.Agent.Status,
		i.row.Agent.Kind,
		i.row.Agent.Title,
		i.row.Agent.Cwd,
		i.row.Agent.PaneID,
		i.row.Last.Text,
	}, " ")
}

func pad(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

func padLeft(s string, w int) string {
	if r := []rune(s); len(r) < w {
		return strings.Repeat(" ", w-len(r)) + s
	}
	return s
}

type refreshedMsg struct {
	rows []agents.Row
	err  error
}

func refresh() tea.Msg {
	rows, err := agents.Collect()
	return refreshedMsg{rows: rows, err: err}
}

// Model is the Bubble Tea model for the switcher.
type Model struct {
	list     list.Model
	rows     []agents.Row
	mode     agents.Mode
	status   agents.StatusFilter
	err      error
	quitting bool

	// Chosen is set when the user picks a row, and the caller focuses that
	// pane after the program exits so the jump never fights the alt screen.
	Chosen *agents.Row

	// Mode is the sort the user left the switcher in, which the caller saves.
	Mode agents.Mode
}

// title names the sort, and the status filter too when one is active, so an
// empty list always explains itself.
func title(mode agents.Mode, status agents.StatusFilter) string {
	t := "agents · " + mode.Label()
	if status != agents.StatusAll {
		t += " · " + status.Label()
	}
	return t
}

// New builds the switcher from an initial set of rows, a starting mode and a
// starting status filter.
func New(rows []agents.Row, mode agents.Mode, status agents.StatusFilter) Model {
	d := list.NewDefaultDelegate()
	d.SetSpacing(0)
	ordered := append([]agents.Row(nil), rows...)
	agents.Apply(mode, ordered)
	l := list.New(toItems(status.Keep(ordered)), d, 0, 0)
	l.Title = title(mode, status)
	l.Styles.Title = titleStyle
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Filter = substringFilter
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "jump")),
			key.NewBinding(key.WithKeys("s"), key.WithHelp("s/S", "sort")),
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a/b/w/i/d", "status")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		}
	}
	return Model{list: l, rows: ordered, mode: mode, status: status, Mode: mode}
}

// reorder re-sorts the rows already in hand.
//
// keepCursor decides where the view lands, and the two cases genuinely differ.
// A refresh must not move the user, so it holds the cursor on the same agent.
// Changing sort mode is a request to see a new ordering, so it returns to the
// top: holding the cursor there scrolls the viewport away from the rows the
// new mode just promoted, which is the opposite of what was asked for.
func (m *Model) reorder(mode agents.Mode, keepCursor bool) {
	var selected string
	if keepCursor {
		if it, ok := m.list.SelectedItem().(item); ok {
			selected = it.row.Agent.PaneID
		}
	}
	m.mode, m.Mode = mode, mode
	agents.Apply(mode, m.rows)
	visible := m.status.Keep(m.rows)
	m.list.SetItems(toItems(visible))
	m.list.Title = title(mode, m.status)

	m.list.Select(0)
	if selected == "" {
		return
	}
	for i, r := range visible {
		if r.Agent.PaneID == selected {
			m.list.Select(i)
			return
		}
	}
}

// setStatus applies a status filter. It returns to the top for the same reason
// a sort change does: the request is to see a different set, not to keep
// looking at the row that happened to be under the cursor.
func (m *Model) setStatus(status agents.StatusFilter) {
	m.status = status
	m.reorder(m.mode, false)
}

func toItems(rows []agents.Row) []list.Item {
	now := time.Now()
	out := make([]list.Item, len(rows))
	for i, r := range rows {
		out[i] = item{row: r, now: now}
	}
	return out
}

// Init satisfies tea.Model and starts no work, because New already holds rows.
func (m Model) Init() tea.Cmd { return nil }

// Update handles input and refresh results.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case refreshedMsg:
		m.err = msg.err
		if msg.err == nil {
			m.rows = msg.rows
			m.reorder(m.mode, true)
		}
		return m, nil

	case tea.KeyMsg:
		// While the filter input is open every key belongs to the list.
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "enter":
			if it, ok := m.list.SelectedItem().(item); ok {
				row := it.row
				m.Chosen = &row
				m.quitting = true
				return m, tea.Quit
			}
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "r":
			return m, refresh
		case "s":
			m.reorder(m.mode.Next(1), false)
			return m, nil
		case "S":
			m.reorder(m.mode.Next(-1), false)
			return m, nil
		case "a", "b", "w", "i", "d":
			m.setStatus(agents.ParseStatus(msg.String()))
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the list, or the error when the snapshot could not be read.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.err != nil {
		return errStyle.Render("could not read the Herdr snapshot: "+m.err.Error()) + "\n"
	}
	return m.list.View()
}
