package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/tree"
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// herdrOps is every change the tree can make to the session.
type herdrOps struct {
	createWorkspace func(dir, label string) error
	renameWorkspace func(id, label string) error
	closeWorkspace  func(id string) error
	createTab       func(workspaceID, dir string) error
	renamePane      func(id, label string) error
	closePane       func(id string) error
	splitPane       func(id, direction, dir string) error
	moveToWorkspace func(id, workspaceID string) error
	moveBeside      func(id, tabID, targetPaneID, split string) error
	moveToNew       func(id, label string) error
	sessions        func() ([]herdr.NamedSession, error)
	stopSession     func(name string) error
	deleteSession   func(name string) error
}

// ops is a package variable so tests can see each change without a live
// Herdr server.
var ops = herdrOps{
	createWorkspace: herdr.CreateWorkspace,
	renameWorkspace: herdr.RenameWorkspace,
	closeWorkspace:  herdr.CloseWorkspace,
	createTab:       herdr.CreateTab,
	renamePane:      herdr.RenamePane,
	closePane:       herdr.ClosePane,
	splitPane:       herdr.SplitPane,
	moveToWorkspace: herdr.MovePaneToWorkspace,
	moveBeside:      herdr.MovePaneBeside,
	moveToNew:       herdr.MovePaneToNewWorkspace,
	sessions:        herdr.Sessions,
	stopSession:     herdr.StopSession,
	deleteSession:   herdr.DeleteSession,
}

// manageHelp is the footer that ? swaps in, because the browsing keys already
// fill one line.
const manageHelp = "n new space · t new tab · R rename · x close · v/s split · m move · S sessions · ? back"

const moveHelp = "enter move · s move beside, split down · esc back · q quit"

const sessionsHelp = "x stop · D delete · r refresh · esc back · q quit"

// ask is a question on the status line: a name to type, or a yes or no.
// apply does the change, and done is the notice once it succeeds. blank says
// what an empty name does, in the footer, where it costs the input no width.
type ask struct {
	question string
	blank    string
	input    *textinput.Model
	apply    func(answer string) error
	done     string
}

// opDoneMsg reports a change once Herdr has answered.
type opDoneMsg struct {
	done string
	err  error
}

// perform runs a change away from the update loop, as every Herdr call must.
func perform(apply func() error, done string) tea.Cmd {
	return func() tea.Msg { return opDoneMsg{done: done, err: apply()} }
}

// sessionsMsg carries the session list.
type sessionsMsg struct {
	list []herdr.NamedSession
	err  error
}

func loadSessions() tea.Msg {
	list, err := ops.sessions()
	return sessionsMsg{list: list, err: err}
}

// sessionPanel lists Herdr's named sessions in place of the tree.
type sessionPanel struct {
	list   []herdr.NamedSession
	cursor int
}

func (p *sessionPanel) current() *herdr.NamedSession {
	if p.cursor < len(p.list) {
		return &p.list[p.cursor]
	}
	return nil
}

func (m *TreeModel) say(text string) { m.notice, m.noticeErr = text, false }

func (m *TreeModel) fail(err error) { m.notice, m.noticeErr = err.Error(), true }

// prompt asks for a name, starting from value. The cursor holds still,
// because a blinking one needs every tick routed back to the input, for no
// gain in a one-line prompt.
func (m *TreeModel) prompt(question, blank, value, done string, apply func(string) error) {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 80
	in.Cursor.SetMode(cursor.CursorStatic)
	in.Width = max(m.width-lipgloss.Width(question)-4, 10)
	in.SetValue(value)
	in.CursorEnd()
	in.Focus()
	m.ask = &ask{question: question, blank: blank, input: &in, apply: apply, done: done}
}

// confirm asks a yes or no question before a change that cannot be undone.
func (m *TreeModel) confirm(question, done string, apply func() error) {
	m.ask = &ask{question: question + " y/n", apply: func(string) error { return apply() }, done: done}
}

// answer routes a key to the open question.
func (m TreeModel) answer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	a := m.ask
	if msg.String() == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}
	if a.input == nil {
		m.ask = nil
		if msg.String() != "y" {
			m.say("left as it was")
			return m, nil
		}
		return m, perform(func() error { return a.apply("") }, a.done)
	}
	switch msg.String() {
	case "esc":
		m.ask = nil
		m.say("left as it was")
		return m, nil
	case "enter":
		m.ask = nil
		value := strings.TrimSpace(a.input.Value())
		return m, perform(func() error { return a.apply(value) }, a.done)
	}
	in, cmd := a.input.Update(msg)
	a.input = &in
	return m, cmd
}

// manageKey handles the keys that change the session. It reports false for
// any other key.
func (m TreeModel) manageKey(key string) (TreeModel, tea.Cmd, bool) {
	n := m.current()
	switch key {
	case "?":
		m.showManage = !m.showManage
		return m, nil, true
	case "S":
		m.sessions = &sessionPanel{}
		return m, loadSessions, true
	case "n":
		if n == nil || n.Dir == "" {
			m.say("no directory to open a workspace in")
			return m, nil, true
		}
		dir, path := clipLeft(shortenHome(n.Dir), 40), n.Dir
		m.prompt("new workspace in "+dir+", name:", "the directory's name", "",
			"opened a workspace in "+dir,
			func(label string) error { return ops.createWorkspace(path, label) })
		return m, nil, true
	case "t":
		if n == nil || n.WorkspaceID == "" {
			m.say("move to a workspace or a pane to open a tab in it")
			return m, nil, true
		}
		ws, dir := n.WorkspaceID, n.Dir
		return m, perform(func() error { return ops.createTab(ws, dir) }, "opened a tab in "+m.spaceName(n)), true
	case "R":
		m.rename(n)
		return m, nil, true
	case "x":
		m.closeNode(n)
		return m, nil, true
	case "m":
		if n == nil || n.Kind != tree.KindPane {
			m.say("move to a pane to move it")
			return m, nil, true
		}
		m.mover = m.movePanel(n)
		return m, nil, true
	case "v", "s":
		if n == nil || n.Kind != tree.KindPane {
			m.say("move to a pane to split it")
			return m, nil, true
		}
		direction := map[string]string{"v": "right", "s": "down"}[key]
		id, dir := n.Pane.PaneID, n.Dir
		return m, perform(func() error { return ops.splitPane(id, direction, dir) }, "split "+n.Name+" "+direction), true
	}
	return m, nil, false
}

func (m *TreeModel) rename(n *tree.Node) {
	switch {
	case n == nil:
	case n.Kind == tree.KindWorkspace:
		id := n.WorkspaceID
		m.prompt("rename workspace "+n.Name+":", "", n.Label, "renamed the workspace",
			func(label string) error { return ops.renameWorkspace(id, label) })
	case n.Kind == tree.KindPane:
		id := n.Pane.PaneID
		m.prompt("name pane "+n.Name+":", "no name", n.Label, "renamed the pane",
			func(label string) error { return ops.renamePane(id, label) })
	default:
		m.say("a repository takes its name from git, so rename one of its workspaces")
	}
}

func (m *TreeModel) closeNode(n *tree.Node) {
	switch {
	case n == nil:
	case n.Kind == tree.KindWorkspace:
		id := n.WorkspaceID
		m.confirm(fmt.Sprintf("close workspace %s and its %s?", n.Name, count(len(n.Children), "pane")),
			"closed "+n.Name, func() error { return ops.closeWorkspace(id) })
	case n.Kind == tree.KindPane:
		question := fmt.Sprintf("close pane %s in %s", n.Name, m.spaceName(n))
		if n.Agent != nil {
			question += " and stop its agent"
		}
		id := n.Pane.PaneID
		m.confirm(question+"?",
			"closed "+n.Name, func() error { return ops.closePane(id) })
	default:
		m.say("close a repository's workspaces one at a time")
	}
}

// spaceName is the tree's name for the workspace a node sits in.
func (m TreeModel) spaceName(n *tree.Node) string {
	if n.Kind == tree.KindWorkspace {
		return n.Name
	}
	for _, l := range m.lines {
		if l.Node.Kind == tree.KindWorkspace && l.Node.WorkspaceID == n.WorkspaceID {
			return l.Node.Name
		}
	}
	return n.WorkspaceID
}

// clipLeft keeps the end of a path, which names the place, and marks what it
// dropped.
func clipLeft(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return "…" + string(r[len(r)-width+1:])
}

func count(n int, word string) string {
	if n != 1 {
		word += "s"
	}
	return strconv.Itoa(n) + " " + word
}

// sessionKey handles a key while the session panel is open.
func (m TreeModel) sessionKey(key string) (tea.Model, tea.Cmd) {
	p := m.sessions
	s := p.current()
	switch key {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "S":
		m.sessions = nil
		return m, refreshTree
	case "up", "k":
		p.cursor = max(p.cursor-1, 0)
	case "down", "j":
		p.cursor = max(min(p.cursor+1, len(p.list)-1), 0)
	case "r":
		return m, loadSessions
	case "x":
		switch {
		case s == nil:
		case s.Current():
			m.say("the switcher runs in " + s.Name + ", so stop it from a terminal")
		case !s.Running:
			m.say(s.Name + " is already stopped")
		default:
			name := s.Name
			m.confirm("stop session "+name+" and every pane in it?", "stopped "+name,
				func() error { return ops.stopSession(name) })
		}
	case "D":
		switch {
		case s == nil:
		case s.Running:
			m.say(s.Name + " is running, so stop it first")
		default:
			name := s.Name
			m.confirm("delete session "+name+"?", "deleted "+name,
				func() error { return ops.deleteSession(name) })
		}
	}
	return m, nil
}

// statusLine is the line above the pager: the open question, else the last
// notice.
func (m TreeModel) statusLine() string {
	if a := m.ask; a != nil {
		line := " " + styled(lipgloss.NewStyle(), accentColor, true, false).Render(a.question)
		if a.input != nil {
			line += " " + a.input.View()
		}
		return line
	}
	if m.notice == "" {
		return ""
	}
	if m.noticeErr {
		return " " + errStyle.Render(m.notice)
	}
	return " " + styled(lipgloss.NewStyle(), previewColor, false, false).Render(m.notice)
}

// help is the footer for whatever has the keyboard.
func (m TreeModel) help() string {
	switch {
	case m.ask != nil && m.ask.input != nil:
		if m.ask.blank != "" {
			return "enter save · esc cancel · blank gives " + m.ask.blank
		}
		return "enter save · esc cancel"
	case m.ask != nil:
		return "y yes · any other key no"
	case m.sessions != nil:
		return sessionsHelp
	case m.mover != nil:
		return moveHelp
	case m.showManage:
		return manageHelp
	}
	return treeHelp
}

// sessionLines draws the session panel's body.
func (m TreeModel) sessionLines() []string {
	p := m.sessions
	if len(p.list) == 0 {
		return []string{"  " + styled(lipgloss.NewStyle(), previewColor, false, false).Render("no sessions")}
	}
	width := 0
	for _, s := range p.list {
		width = max(width, len([]rune(s.Name)))
	}
	var out []string
	for i, s := range p.list {
		selected := i == p.cursor
		base := lipgloss.NewStyle()
		gutter, gutterColor := " ", ""
		if selected {
			base = base.Background(m.bg)
			gutter, gutterColor = "▌", accentColor
		}
		seg := func(text, fg string) string { return styled(base, fg, false, false).Render(text) }
		state, color := "○ stopped", previewColor
		if s.Running {
			state, color = "● running", statusColors["working"]
		}
		line := seg(gutter, gutterColor) + seg("  "+pad(s.Name, width)+"  ", textColorFor(selected))
		line += seg(state, color)
		mark := ""
		switch {
		case s.Current():
			mark = "  this one"
		case s.Default:
			mark = "  default"
		}
		line += seg(pad(mark, 11), accentColor)
		line += seg("  "+shortenHome(s.Dir), previewColorFor(selected))
		out = append(out, fill(line, m.width, base))
	}
	return out
}
