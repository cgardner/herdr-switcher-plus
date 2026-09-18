package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/layout"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// DefaultSpec is the layout used when the config says nothing. It is written
// in the same vocabulary a user would write, so the built-in look is not a
// special case the configuration cannot reproduce.
var DefaultSpec = layout.Spec{
	Rows: [][]layout.Token{
		{{Name: "age"}, {Name: "label"}, {Name: "state_icon"}, {Name: "state_text"}},
		{{Name: "role"}, {Name: "message"}},
	},
}

// delegate draws each agent according to a layout spec and marks the selected
// one with a background bar, the way Herdr's own overlays do.
//
// It composes every cell itself rather than styling strings up front. A
// foreground style emits its own reset at the end of each segment, which would
// clear the row background part way across the line, so each cell has to carry
// the background as well as its own color.
type delegate struct {
	spec        layout.Spec
	selectionBg lipgloss.Color
}

// Height is the rows in the spec plus the configured gap between agents.
func (d delegate) Height() int { return len(d.spec.Rows) + d.spec.RowGap }

// Spacing stays zero because the gap belongs to Height, which keeps the
// selection background covering the blank lines of the selected agent.
func (d delegate) Spacing() int { return 0 }

// Update satisfies list.ItemDelegate. The delegate holds no state.
func (d delegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render writes one agent's lines.
func (d delegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok {
		return
	}
	width := m.Width()
	if width <= 0 || len(d.spec.Rows) == 0 {
		return
	}
	selected := index == m.Index()

	base := lipgloss.NewStyle()
	if selected {
		base = base.Background(d.selectionBg)
	}

	// The selection bar replaces the plain gutter, so the marked row reads as
	// one block rather than several loosely related lines. It is one column
	// wide and carries a single separating space.
	gutter, gutterColor := " ", ""
	if selected {
		gutter, gutterColor = "▌", accentColor
	}

	lines := make([]string, 0, d.Height())
	for ri, row := range d.spec.Rows {
		line := styled(base, gutterColor, false, false).Render(gutter) + base.Render(" ")

		// Rows after the first hang under the first row's second column, so a
		// message sits below the name it belongs to rather than under the age.
		if ri > 0 {
			line += base.Render(strings.Repeat(" ", it.indent))
		}

		for ti, tok := range row {
			if ti > 0 {
				line += base.Render(" ")
			}
			line += d.cell(base, it, tok, ri, ti, selected)
		}
		lines = append(lines, fill(line, width, base))
	}
	for i := 0; i < d.spec.RowGap; i++ {
		lines = append(lines, fill("", width, base))
	}
	fmt.Fprint(w, strings.Join(lines, "\n"))
}

// cell renders one token, folding the contextual default with whatever the
// config overrides.
func (d delegate) cell(base lipgloss.Style, it item, tok layout.Token, ri, ti int, selected bool) string {
	def, ok := tokens[tok.Name]
	if !ok {
		return ""
	}
	c := def.render(it.row, it.now, selected)
	style := tok.Style.Resolve(c.text, c.number)

	fg := c.fg
	if style.Fg != "" {
		fg = style.Fg
	}
	bold := style.Bold != nil && *style.Bold
	dim := c.dim
	if style.Dim != nil {
		dim = *style.Dim
	}

	text := c.text
	if w := it.widthAt(ri, ti); w > 0 {
		if c.alignEnd {
			text = padLeft(text, w)
		} else if ti < len(d.spec.Rows[ri])-1 {
			// The last cell on a line runs to the edge rather than being
			// padded, so a long message is not cut short by its column.
			text = pad(text, w)
		}
	}
	return styled(base, fg, bold, dim).Render(text)
}

// styled derives a cell style from the row base, keeping the selection
// background attached so the bar survives across every segment.
func styled(base lipgloss.Style, fg string, bold, dim bool) lipgloss.Style {
	s := base
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if bold {
		s = s.Bold(true)
	}
	if dim {
		s = s.Faint(true)
	}
	return s
}

// fill pads a composed line out to the pane width so the background reaches
// the right edge, and truncates anything that overflows.
func fill(line string, width int, base lipgloss.Style) string {
	if w := lipgloss.Width(line); w < width {
		return line + base.Render(strings.Repeat(" ", width-w))
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(line)
}

func previewText(r agents.Row) string {
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

// measure computes the column widths for a spec across every agent on screen,
// plus the indent that continuation rows hang at.
func measure(spec layout.Spec, rows []agents.Row, now time.Time) (widths [][]int, indent int) {
	widths = make([][]int, len(spec.Rows))
	for ri, row := range spec.Rows {
		widths[ri] = make([]int, len(row))
		for ti, tok := range row {
			def, ok := tokens[tok.Name]
			if !ok {
				continue
			}
			for _, row := range rows {
				// selected is false here: no token changes width when it is
				// selected, only its color.
				if n := len([]rune(def.render(row, now, false).text)); n > widths[ri][ti] {
					widths[ri][ti] = n
				}
			}
		}
	}
	if len(widths) > 0 && len(widths[0]) > 0 {
		indent = widths[0][0] + 1
	}
	return widths, indent
}
