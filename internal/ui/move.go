package ui

import (
	"strings"

	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// A move takes a pane, and the agent conversation in it, to another place in
// the session. Herdr keeps the pane's terminal, so the agent carries on.

type moveKind int

const (
	moveToNewWorkspace moveKind = iota
	moveToWorkspace
	moveBeside
)

// moveChoice is one place the pane can go.
type moveChoice struct {
	kind  moveKind
	space *tree.Node // the workspace for moveToWorkspace
	pane  *tree.Node // the pane to sit beside for moveBeside
}

// movePanel lists every place the pane can go, in place of the tree.
type movePanel struct {
	pane    *tree.Node
	from    *tree.Node // the workspace the pane leaves
	choices []moveChoice
	cursor  int
}

// movePanel lists the destinations in tree order: a new workspace, then each
// workspace, followed by the panes in it. The full tree is walked, so a
// status filter does not hide a destination.
//
// The pane's own workspace is offered as a new tab only when the pane shares
// its tab, because a pane alone in its tab is already in a tab of its own.
func (m TreeModel) movePanel(n *tree.Node) *movePanel {
	p := &movePanel{pane: n, choices: []moveChoice{{kind: moveToNewWorkspace}}}
	var walk func([]*tree.Node)
	walk = func(nodes []*tree.Node) {
		for _, c := range nodes {
			if c.Kind != tree.KindWorkspace {
				walk(c.Children)
				continue
			}
			own := c.WorkspaceID == n.WorkspaceID
			if own {
				p.from = c
			}
			if !own || sharesTab(c, n) {
				p.choices = append(p.choices, moveChoice{kind: moveToWorkspace, space: c})
			}
			for _, pane := range c.Children {
				if pane.ID != n.ID {
					p.choices = append(p.choices, moveChoice{kind: moveBeside, space: c, pane: pane})
				}
			}
		}
	}
	walk(m.roots)
	return p
}

func sharesTab(space, n *tree.Node) bool {
	for _, c := range space.Children {
		if c.ID != n.ID && c.Pane.TabID == n.Pane.TabID {
			return true
		}
	}
	return false
}

// moveKey handles a key while the move panel is open.
func (m TreeModel) moveKey(key string) (tea.Model, tea.Cmd) {
	p := m.mover
	switch key {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "m":
		m.mover = nil
	case "up", "k":
		p.cursor = max(p.cursor-1, 0)
	case "down", "j":
		p.cursor = min(p.cursor+1, len(p.choices)-1)
	case "pgup":
		p.cursor = max(p.cursor-m.body(), 0)
	case "pgdown":
		p.cursor = min(p.cursor+m.body(), len(p.choices)-1)
	case "enter", "s":
		c := p.choices[p.cursor]
		if key == "s" && c.kind != moveBeside {
			break
		}
		m.mover = nil
		return m, m.moveTo(p, c, map[string]string{"enter": "right", "s": "down"}[key])
	}
	return m, nil
}

// moveTo starts the move to one choice. A new workspace asks for its name
// first, so it starts no command until the prompt is answered.
func (m *TreeModel) moveTo(p *movePanel, c moveChoice, split string) tea.Cmd {
	id, name := p.pane.Pane.PaneID, p.pane.Title()
	switch c.kind {
	case moveToWorkspace:
		ws := c.space.WorkspaceID
		return perform(func() error { return ops.moveToWorkspace(id, ws) }, "moved "+name+" to a new tab in "+c.space.Name+p.leaving(c))
	case moveBeside:
		tab, target := c.pane.Pane.TabID, c.pane.Pane.PaneID
		return perform(func() error { return ops.moveBeside(id, tab, target, split) },
			"moved "+name+" beside "+c.pane.Title()+" in "+c.space.Name+p.leaving(c))
	}
	m.prompt("move "+name+" to a new workspace, name:", "Herdr's default", "", "moved "+name+" to a new workspace"+p.leaving(c),
		func(label string) error { return ops.moveToNew(id, label) })
	return nil
}

// leaving warns that the pane's workspace closes behind it. Herdr closes a
// workspace that loses its last pane, which is easy to miss from the tree.
func (p *movePanel) leaving(c moveChoice) string {
	if p.from == nil || len(p.from.Children) != 1 || c.space == p.from {
		return ""
	}
	return ", and " + p.from.Name + " closed with no pane left"
}

// moveLines draws the move panel's body, one page at a time.
func (m TreeModel) moveLines() []string {
	p := m.mover
	h := m.body()
	start := p.cursor / h * h
	end := min(start+h, len(p.choices))
	var out []string
	for i := start; i < end; i++ {
		c := p.choices[i]
		selected := i == p.cursor
		base := lipgloss.NewStyle()
		gutter, gutterColor := " ", ""
		if selected {
			base = base.Background(m.bg)
			gutter, gutterColor = "▌", accentColor
		}
		seg := func(text, fg string, bold bool) string { return styled(base, fg, bold, false).Render(text) }
		line := seg(gutter, gutterColor, false)
		switch c.kind {
		case moveToNewWorkspace:
			line += seg("  + a new workspace", accentColor, false)
		case moveToWorkspace:
			line += seg("  "+c.space.Name, textColorFor(selected), true) + seg("  new tab", previewColorFor(selected), false)
		case moveBeside:
			if !m.listsSpace(c.space) && (i == 0 || p.choices[i-1].space != c.space) {
				// The pane's own workspace has no line of its own when the
				// pane is alone in its tab, so its first pane names it.
				line += seg("  "+c.space.Name+"  ", textColorFor(selected), true)
			} else {
				line += seg("    ", "", false)
			}
			line += seg("beside "+pad(c.pane.Name, 8)+" ", textColorFor(selected), false)
			line += nameSeg(c.pane, func(text, fg string, bold, dim bool) string {
				return styled(base, fg, bold, dim).Render(text)
			})
			line += seg(choiceText(c.pane), previewColorFor(selected), false)
		}
		out = append(out, fill(line, m.width, base))
	}
	return out
}

// listsSpace reports whether the move panel gives this workspace a line.
func (m TreeModel) listsSpace(space *tree.Node) bool {
	for _, c := range m.mover.choices {
		if c.kind == moveToWorkspace && c.space == space {
			return true
		}
	}
	return false
}

// choiceText says what is in a pane: the last message for an agent, so one
// conversation can be told from another, and the title for a shell.
func choiceText(n *tree.Node) string {
	if r := n.Agent; r != nil {
		return strings.TrimSpace(roleMark(r.Last.Role) + previewText(*r))
	}
	return shellText(n)
}
