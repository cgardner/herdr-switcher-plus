package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// delegate renders one agent as two full-width lines and marks the selected
// one with a background bar, the way Herdr's own overlays do.
//
// It composes every segment itself rather than styling strings up front. A
// foreground style emits its own reset at the end of each segment, which would
// clear the row background part way across the line, so each segment has to
// carry the background as well as its own color.
type delegate struct {
	selectionBg lipgloss.Color
}

// Height is two because each agent occupies a title line and a preview line.
func (d delegate) Height() int { return 2 }

// Spacing is zero so the list stays dense enough to scan at a glance.
func (d delegate) Spacing() int { return 0 }

// Update satisfies list.ItemDelegate. The delegate holds no state.
func (d delegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render writes the two lines for one agent.
func (d delegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok {
		return
	}

	width := m.Width()
	if width <= 0 {
		return
	}
	selected := index == m.Index()

	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(d.selectionBg)
	}
	seg := func(color string) lipgloss.Style {
		if color == "" {
			return base
		}
		return base.Foreground(lipgloss.Color(color))
	}

	// The selection bar replaces the plain gutter, so the marked row reads as
	// one block rather than two loosely related lines.
	gutter := "  "
	gutterColor := ""
	if selected {
		gutter = "▌ "
		gutterColor = accentColor
	}

	age := it.row.Age(it.now)
	ageColor := ageColorFor(it.row, it.now, selected)
	status := it.row.Agent.Status

	title := seg(gutterColor).Render(gutter) +
		seg(ageColor).Render(padLeft(age, 4)) +
		base.Render("  ") +
		seg(textColorFor(selected)).Render(pad(it.row.Space, spaceWidth)) +
		base.Render(" ") +
		seg(statusColors[status]).Render(glyph(status)+" "+status)

	preview := seg(gutterColor).Render(gutter) +
		base.Render(strings.Repeat(" ", 5)) +
		seg(previewColorFor(selected)).Render(roleMark(it.row.Last.Role)+previewText(it.row))

	fmt.Fprint(w, fill(title, width, base)+"\n"+fill(preview, width, base))
}

// fill pads a composed line out to the pane width so the background reaches
// the right edge, and truncates anything that overflows.
func fill(line string, width int, base lipgloss.Style) string {
	if w := lipgloss.Width(line); w < width {
		return line + base.Render(strings.Repeat(" ", width-w))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func previewText(r agentRow) string {
	if t := r.Last.Text; t != "" {
		return t
	}
	if t := r.Agent.Title; t != "" {
		return t
	}
	return "(no message found)"
}

func roleMark(role string) string {
	switch role {
	case "user":
		return "› "
	case "assistant":
		return "‹ "
	}
	return "  "
}
