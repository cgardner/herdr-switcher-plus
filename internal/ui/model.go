// Package ui renders the agent list as a Bubble Tea program.
package ui

import (
	"strings"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The label column sizes itself to the widest label on screen, within these
// bounds. A list where nothing carries repository context stays as narrow as it
// was before, and a list full of worktrees widens only as far as the preview
// can afford.
const (
	labelMinWidth = 20
	labelMaxWidth = 38
)

// Colors are ANSI indexes so the switcher follows whatever palette the
// terminal already uses. Only the selection background is a hex value,
// because it has to match Herdr's own overlays rather than the terminal.
const (
	accentColor  = "6"
	staleColor   = "8"
	previewColor = "8"

	// On the selection bar the dim gray used elsewhere loses too much
	// contrast, so stale ages and preview text step up one level.
	selectedDimColor  = "7"
	selectedTextColor = "15"
)

// statusColors marks each agent lifecycle state. Herdr reports blocked when it
// recognizes an approval or question UI, which is the state most worth
// spotting in a long list.
var statusColors = map[string]string{
	"blocked": "1",
	"working": "3",
	"done":    "2",
	"idle":    "8",
	"unknown": "8",
}

var (
	errStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("6")).Padding(0, 1)
)

// staleAfter is when an age stops reading as recent and turns gray.
const staleAfter = 24 * time.Hour

func glyph(status string) string {
	if status == "idle" || status == "unknown" {
		return "\u25cb"
	}
	return "\u25cf"
}

func ageColorFor(r agentRow, now time.Time, selected bool) string {
	if r.HasLast && now.Sub(r.Last.At) <= staleAfter {
		return accentColor
	}
	if selected {
		return selectedDimColor
	}
	return staleColor
}

func textColorFor(selected bool) string {
	if selected {
		return selectedTextColor
	}
	return ""
}

func previewColorFor(selected bool) string {
	if selected {
		return selectedDimColor
	}
	return previewColor
}

// agentRow names the row type the delegate renders, keeping its signatures
// readable without importing the package name into every helper.
type agentRow = agents.Row

// item adapts a row to the bubbles list interface. Rendering lives in the
// delegate, which composes each line so the selection background survives
// across every styled segment.
type item struct {
	row agentRow
	now time.Time

	// labelWidth is shared by every item in one list so the status column
	// lines up down the pane.
	labelWidth int
}

// FilterValue drives the type-to-filter search. Status, agent kind, repository
// and branch are included so "/blocked", "/codex" or "/Second Brain" narrows
// the list without a separate filter control. Anything the row can display is
// searchable, which is why the repository and branch appear here even when the
// label leaves them out as implied.
func (i item) FilterValue() string {
	return strings.Join([]string{
		i.row.Space,
		i.row.Repo,
		i.row.Branch,
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

// collectRows is a package variable so tests can drive a refresh without a
// live Herdr server.
var collectRows = agents.Collect

func refresh() tea.Msg {
	rows, err := collectRows()
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
	d := delegate{selectionBg: selectionBackground()}
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
	width := labelColumnWidth(rows)
	out := make([]list.Item, len(rows))
	for i, r := range rows {
		out[i] = item{row: r, now: now, labelWidth: width}
	}
	return out
}

// labelColumnWidth measures the widest label and clamps it, so the column
// grows only as far as the rows actually need.
func labelColumnWidth(rows []agents.Row) int {
	width := labelMinWidth
	for _, r := range rows {
		if n := len([]rune(r.Label())); n > width {
			width = n
		}
	}
	if width > labelMaxWidth {
		return labelMaxWidth
	}
	return width
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
