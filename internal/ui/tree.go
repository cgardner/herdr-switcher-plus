package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/paginator"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// collectTree is a package variable so tests can drive a refresh without a
// live Herdr server.
var collectTree = tree.Collect

type treeRefreshedMsg struct {
	roots []*tree.Node
	err   error
}

func refreshTree() tea.Msg {
	roots, err := collectTree()
	return treeRefreshedMsg{roots: roots, err: err}
}

// treeHelp is the footer. The tree draws its own lines rather than using the
// bubbles list, whose items are flat and whose filter reorders them.
const treeHelp = "enter jump/fold · ←/→ fold · e/c expand/collapse all · a/b/w/i/d status · r refresh · q quit"

// TreeModel is the Bubble Tea model for the tree switcher.
type TreeModel struct {
	roots     []*tree.Node
	status    agents.StatusFilter
	collapsed map[string]bool
	lines     []tree.Line
	cursor    int
	offset    int
	width     int
	height    int
	now       time.Time
	bg        lipgloss.Color
	err       error
	quitting  bool

	// Chosen is the pane the user picked. The caller focuses it after the
	// program exits, as it does for the list.
	Chosen *tree.Node
}

// NewTree builds the tree switcher with every group expanded.
func NewTree(roots []*tree.Node, status agents.StatusFilter) TreeModel {
	m := TreeModel{roots: roots, status: status, collapsed: map[string]bool{}, now: time.Now(), bg: selectionBackground()}
	m.relayout("")
	return m
}

// relayout rebuilds the visible lines and puts the cursor back on the node
// with this ID, or on the first line when that node is gone.
func (m *TreeModel) relayout(keep string) {
	m.lines = tree.Flatten(tree.Filter(m.roots, m.status), m.collapsed)
	m.cursor = 0
	for i, l := range m.lines {
		if l.Node.ID == keep {
			m.cursor = i
			break
		}
	}
	m.scroll()
}

func (m TreeModel) current() *tree.Node {
	if m.cursor < len(m.lines) {
		return m.lines[m.cursor].Node
	}
	return nil
}

func (m TreeModel) currentID() string {
	if n := m.current(); n != nil {
		return n.ID
	}
	return ""
}

// treeChrome is the lines around the tree: the title and a blank line above,
// and a blank line, the pager and the help below.
const treeChrome = 5

// body is how many tree lines fit on one page.
func (m TreeModel) body() int {
	if m.height <= 0 {
		return max(len(m.lines), 1)
	}
	return max(m.height-treeChrome, 1)
}

// scroll turns to the page that holds the cursor. The tree pages the way the
// list does, rather than scrolling a line at a time, so the pager below it can
// say which page is on screen.
func (m *TreeModel) scroll() {
	h := m.body()
	m.offset = m.cursor / h * h
}

// pager draws the list's own page dots, from the list's own styles, so the two
// views read alike. Dots too wide for the pane become "2/5", and a tree that
// fits on one page shows no pager, which is what the list does too.
func (m TreeModel) pager() string {
	p := paginator.New(paginator.WithPerPage(m.body()))
	p.Type = paginator.Dots
	p.SetTotalPages(len(m.lines))
	if p.TotalPages < 2 {
		return ""
	}
	p.Page = m.offset / p.PerPage
	styles := list.DefaultStyles()
	p.ActiveDot = styles.ActivePaginationDot.String()
	p.InactiveDot = styles.InactivePaginationDot.String()
	view := p.View()
	if m.width > 0 && lipgloss.Width(view)+2 > m.width {
		p.Type = paginator.Arabic
		view = styles.ArabicPagination.Render(p.View())
	}
	return "  " + view
}

func (m *TreeModel) move(step int) {
	m.cursor = max(min(m.cursor+step, len(m.lines)-1), 0)
	m.scroll()
}

// fold collapses or expands the group under the cursor. Folding a pane, or a
// group that is already in that state, climbs to the parent instead, which is
// what left does in every file tree.
func (m *TreeModel) fold(collapse bool) {
	n := m.current()
	if n == nil {
		return
	}
	if n.IsGroup() && len(n.Children) > 0 && m.collapsed[n.ID] != collapse {
		m.collapsed[n.ID] = collapse
		m.relayout(n.ID)
		return
	}
	if collapse {
		m.toParent()
	}
}

func (m *TreeModel) toParent() {
	depth := m.lines[m.cursor].Depth
	for i := m.cursor - 1; i >= 0; i-- {
		if m.lines[i].Depth < depth {
			m.cursor = i
			m.scroll()
			return
		}
	}
}

// Init satisfies tea.Model and starts no work, because NewTree already holds
// the tree.
func (m TreeModel) Init() tea.Cmd { return nil }

// Update handles input and refresh results.
func (m TreeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()

	case treeRefreshedMsg:
		m.err = msg.err
		if msg.err == nil {
			m.roots, m.now = msg.roots, time.Now()
			m.relayout(m.currentID())
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.move(-m.body())
		case "pgdown":
			m.move(m.body())
		case "home", "g":
			m.move(-len(m.lines))
		case "end", "G":
			m.move(len(m.lines))
		case "left", "h":
			m.fold(true)
		case "right", "l":
			m.fold(false)
		case " ":
			if n := m.current(); n != nil {
				m.fold(!m.collapsed[n.ID])
			}
		case "enter":
			n := m.current()
			if n == nil {
				break
			}
			if n.IsGroup() {
				m.fold(!m.collapsed[n.ID])
				break
			}
			m.Chosen, m.quitting = n, true
			return m, tea.Quit
		case "e":
			keep := m.currentID()
			m.collapsed = map[string]bool{}
			m.relayout(keep)
		case "c":
			// The cursor climbs to its root, the only line still visible
			// once every group is closed.
			keep := ""
			if m.cursor < len(m.lines) {
				keep = m.rootOf(m.cursor)
			}
			for _, id := range tree.Groups(m.roots) {
				m.collapsed[id] = true
			}
			m.relayout(keep)
		case "r":
			return m, refreshTree
		case "a", "b", "w", "i", "d":
			// A filter asks to see the panes in that state, so it opens
			// every group rather than leave a match folded out of sight.
			m.status = agents.ParseStatus(msg.String())
			m.collapsed = map[string]bool{}
			m.relayout(m.currentID())
		}
	}
	return m, nil
}

func (m TreeModel) rootOf(i int) string {
	for ; i > 0 && m.lines[i].Depth > 0; i-- {
	}
	return m.lines[i].Node.ID
}

// View renders the title, the visible slice of the tree and the footer.
func (m TreeModel) View() string {
	if m.quitting {
		return ""
	}
	if m.err != nil {
		return errStyle.Render("could not read the Herdr snapshot: "+m.err.Error()) + "\n"
	}
	title := "agents · tree"
	if m.status != agents.StatusAll {
		title += " · " + m.status.Label()
	}
	out := []string{" " + titleStyle.Render(title), ""}

	if len(m.lines) == 0 {
		out = append(out, "  "+styled(lipgloss.NewStyle(), previewColor, false, false).Render("nothing to show"))
	}
	end := min(m.offset+m.body(), len(m.lines))
	for i := m.offset; i < end; i++ {
		out = append(out, m.renderLine(m.lines[i], i == m.cursor))
	}
	for len(out) < m.height-3 {
		out = append(out, "")
	}
	out = append(out, "", m.pager())
	out = append(out, " "+styled(lipgloss.NewStyle(), previewColor, false, false).Render(treeHelp))
	return strings.Join(out, "\n")
}

// renderLine composes one line segment by segment, attaching the selection
// background to each, for the same reason the list delegate does.
func (m TreeModel) renderLine(l tree.Line, selected bool) string {
	base := lipgloss.NewStyle()
	gutter, gutterColor := " ", ""
	if selected {
		base = base.Background(m.bg)
		gutter, gutterColor = "▌", accentColor
	}
	seg := func(text, fg string, bold, dim bool) string {
		return styled(base, fg, bold, dim).Render(text)
	}

	n := l.Node
	line := seg(gutter, gutterColor, false, false) + seg(" "+strings.Repeat("  ", l.Depth), "", false, false)

	if n.IsGroup() {
		arrow := "▾"
		if m.collapsed[n.ID] {
			arrow = "▸"
		}
		if len(n.Children) == 0 {
			arrow = " "
		}
		name := n.Name
		if n.Linked {
			name = "⑂ " + name
		}
		line += seg(arrow+" ", accentColor, false, false) + seg(name, textColorFor(selected), n.Kind == tree.KindRepo, false)
		line += m.summary(n, selected, seg)
		return fill(line, m.width, base)
	}

	// A pane: age, state and kind in the order the list uses, then the
	// message preview.
	if r := n.Agent; r != nil {
		status := r.Agent.Status
		line += seg(padLeft(r.Age(m.now), 3)+" ", ageColorFor(*r, m.now, selected), false, false)
		line += seg(glyph(status)+" ", statusColors[status], false, false)
		line += seg(pad(status, 7)+" ", statusColors[status], false, false)
		line += seg(n.Name+"  ", textColorFor(selected), false, false)
		line += seg(roleMark(r.Last.Role)+previewText(*r), previewColorFor(selected), false, false)
		return fill(line, m.width, base)
	}
	line += seg("    · ", previewColorFor(selected), false, false)
	line += seg(pad(n.Name, 8)+" ", previewColorFor(selected), false, false)
	line += seg(shellText(n), previewColorFor(selected), false, false)
	return fill(line, m.width, base)
}

// summary is what a closed group shows after its name: the status that most
// wants the user, the newest age below it, and the agent count. An open group
// shows none of it, because its panes say the same on the lines below.
func (m TreeModel) summary(n *tree.Node, selected bool, seg func(string, string, bool, bool) string) string {
	if n.Latest == nil || !m.collapsed[n.ID] {
		return ""
	}
	out := seg("  ", "", false, false)
	out += seg(glyph(n.Urgent)+" ", statusColors[n.Urgent], false, false)
	out += seg(n.Latest.Age(m.now), ageColorFor(*n.Latest, m.now, selected), false, false)
	word := " agents"
	if n.Agents == 1 {
		word = " agent"
	}
	return out + seg("  "+strconv.Itoa(n.Agents)+word, previewColorFor(selected), false, false)
}

// shellText describes a pane with no agent: its terminal title, or the
// directory it sits in when the title is blank.
func shellText(n *tree.Node) string {
	if n.Pane.Title != "" {
		return n.Pane.Title
	}
	cwd := n.Pane.ForegroundCwd
	if cwd == "" {
		cwd = n.Pane.Cwd
	}
	return shortenHome(cwd)
}
