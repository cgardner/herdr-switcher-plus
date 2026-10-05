// Package tree arranges every pane in the session as a tree: the repository
// at the root, each workspace that is a checkout of it below, and the panes of
// that workspace as leaves.
//
// Herdr gives each linked worktree a workspace of its own, so the workspace
// list alone hides which spaces share a repository. Grouping on the
// repository puts them back together.
package tree

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/gitref"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
)

// Kind is what a node stands for.
type Kind int

const (
	KindRepo Kind = iota
	KindWorkspace
	KindPane
)

// Node is one line of the tree.
type Node struct {
	Kind Kind

	// ID is unique across the tree and stable across refreshes, so a
	// collapsed group stays collapsed when the snapshot is read again.
	ID string

	// Name is what the line reads: the repository, the workspace label with
	// whatever context is not already implied, or the pane's agent kind.
	Name string

	// Label is the name Herdr holds for a workspace or a pane, before Name
	// adds context to it. A pane takes its own label, else the name of its
	// agent, and has none when the user never named it. AgentNamed marks a
	// label that came from the agent, so a rename goes back the same way.
	Label      string
	AgentNamed bool

	// Linked marks a workspace that is a linked worktree.
	Linked bool

	// WorkspaceID is the Herdr workspace a workspace node stands for, and
	// the one a pane sits in. A repository node has none.
	WorkspaceID string

	// Dir is where a new workspace, tab or pane made from this node starts:
	// the checkout for a workspace, the main checkout for a repository, and
	// the working directory for a pane.
	Dir string

	// Pane is set on a pane node. Agent is set too when an agent runs in it.
	Pane  herdr.Pane
	Agent *agents.Row

	Children []*Node

	// Latest is the newest agent anywhere below this node, or on it. Agents
	// counts them, and Urgent is the status that most wants the user.
	Latest *agents.Row
	Agents int
	Urgent string

	// order breaks ties between nodes with no agent: the sidebar position
	// for a workspace, and the snapshot order for a pane.
	order int
}

// Title is how a message names the node: a pane's label when it has one, as
// that is the name the user knows it by, else Name.
func (n *Node) Title() string {
	if n.Kind == KindPane && n.Label != "" {
		return n.Label
	}
	return n.Name
}

// Display is the name the tree draws on a pane: its label, else the name of
// its tab, which the list shows too. A tab name that repeats the space draws
// nothing. Label stays as it is, because a rename acts on it and not on the tab.
func (n *Node) Display() string {
	if n.Label != "" {
		return n.Label
	}
	if r := n.Agent; r != nil && r.TabName != "" && r.TabName != r.Space {
		return r.TabName
	}
	return ""
}

// IsGroup reports whether the node can hold children.
func (n *Node) IsGroup() bool { return n.Kind != KindPane }

// The sources Collect draws on, as package variables so tests can drive it
// without a live Herdr server or a checkout on disk.
var (
	collectAgents = agents.CollectSnapshot
	resolveBranch = gitref.Branch
)

// Collect reads the live session and builds the tree.
func Collect() ([]*Node, error) {
	snap, rows, err := collectAgents()
	if err != nil {
		return nil, err
	}
	return Build(snap, rows, resolveBranch), nil
}

// Build arranges a snapshot as a tree. branch resolves a checkout path to its
// branch name, and rows are the agents already joined to their transcripts.
//
// A repository with only one workspace is not given a level of its own. The
// workspace takes the repository's place at the root, named the way the list
// names it, because a group of one adds a line and tells the reader nothing.
func Build(snap *herdr.Snapshot, rows []agents.Row, branch func(string) string) []*Node {
	byPane := make(map[string]*agents.Row, len(rows))
	for i := range rows {
		byPane[rows[i].Agent.PaneID] = &rows[i]
	}

	// An older Herdr sends no pane list. The agents are then the only panes
	// known, which still gives a usable tree.
	panes := snap.Panes
	if len(panes) == 0 {
		for _, a := range snap.Agents {
			panes = append(panes, herdr.Pane{
				PaneID: a.PaneID, TabID: a.TabID, WorkspaceID: a.WorkspaceID,
				Agent: a.Kind, Status: a.Status, Cwd: a.Cwd, Title: a.Title, Focused: a.Focused,
			})
		}
	}

	spaces := map[string]*Node{}
	var spaceOrder []string
	addSpace := func(id string) *Node {
		if n, ok := spaces[id]; ok {
			return n
		}
		label, number := snap.Workspace(id)
		n := &Node{Kind: KindWorkspace, ID: "ws:" + id, Name: label, WorkspaceID: id, order: number}
		if w := snap.FindWorkspace(id); w != nil {
			n.Label = w.Label
		}
		spaces[id] = n
		spaceOrder = append(spaceOrder, id)
		return n
	}
	for _, w := range snap.Workspaces {
		addSpace(w.WorkspaceID)
	}
	for i, p := range panes {
		n := &Node{Kind: KindPane, ID: "pane:" + p.PaneID, Pane: p, Agent: byPane[p.PaneID], Label: p.Label, WorkspaceID: p.WorkspaceID, Dir: paneDir(p), order: i}
		if n.Label == "" && n.Agent != nil && n.Agent.Agent.Name != "" {
			n.Label, n.AgentNamed = n.Agent.Agent.Name, true
		}
		n.Name = paneName(n)
		ws := addSpace(p.WorkspaceID)
		if ws.Dir == "" {
			ws.Dir = n.Dir
		}
		ws.Children = append(ws.Children, n)
	}

	var roots []*Node
	repos := map[string]*Node{}
	type member struct {
		repo  *Node
		space *Node
		w     *herdr.Workspace
	}
	var members []member
	for _, id := range spaceOrder {
		ws := spaces[id]
		w := snap.FindWorkspace(id)
		if w == nil || w.Worktree == nil {
			roots = append(roots, ws)
			continue
		}
		key := repoKey(w.Worktree)
		repo, ok := repos[key]
		if !ok {
			repo = &Node{Kind: KindRepo, ID: "repo:" + key, Name: w.Worktree.RepoName, order: ws.order}
			repos[key] = repo
			roots = append(roots, repo)
		}
		repo.Children = append(repo.Children, ws)
		members = append(members, member{repo, ws, w})
	}

	atMain := map[*Node]bool{}
	for _, m := range members {
		wt := m.w.Worktree
		label := agents.Row{Space: m.space.Name, Branch: branch(wt.CheckoutPath)}
		m.space.Linked = wt.IsLinked
		if wt.CheckoutPath != "" {
			m.space.Dir = wt.CheckoutPath
		}
		// A repository starts new work in its main checkout, and in its
		// first worktree only when no main checkout is open.
		if !atMain[m.repo] && (m.repo.Dir == "" || !wt.IsLinked) {
			m.repo.Dir, atMain[m.repo] = m.space.Dir, !wt.IsLinked
		}
		if len(m.repo.Children) == 1 {
			label.Repo, label.Linked = wt.RepoName, wt.IsLinked
		}
		m.space.Name = label.Label()
	}

	// Promote the workspace of a repository that has only one.
	for i, r := range roots {
		if r.Kind == KindRepo && len(r.Children) == 1 {
			roots[i] = r.Children[0]
		}
	}

	for _, r := range roots {
		summarize(r)
	}
	order(roots)
	return roots
}

// repoKey picks the value every checkout of one repository shares.
func repoKey(w *herdr.Worktree) string {
	switch {
	case w.RepoKey != "":
		return w.RepoKey
	case w.RepoRoot != "":
		return filepath.Clean(w.RepoRoot)
	}
	return w.RepoName
}

// paneDir is where a pane's shell stands now, which is where a split of it
// is most likely wanted.
func paneDir(p herdr.Pane) string {
	if p.ForegroundCwd != "" {
		return p.ForegroundCwd
	}
	return p.Cwd
}

// paneName is the agent kind for an agent pane, and "shell" for anything
// else. A name the user gave stays in Label, so the kind is never lost.
func paneName(n *Node) string {
	if n.Pane.Agent != "" {
		return n.Pane.Agent
	}
	if n.Agent != nil && n.Agent.Agent.Kind != "" {
		return n.Agent.Agent.Kind
	}
	return "shell"
}

// summarize fills Latest, Agents and Urgent from the node's own agent and
// every descendant.
func summarize(n *Node) {
	n.Latest, n.Agents, n.Urgent = nil, 0, ""
	if n.Agent != nil {
		n.Latest, n.Agents, n.Urgent = n.Agent, 1, n.Agent.Agent.Status
	}
	for _, c := range n.Children {
		summarize(c)
		n.Agents += c.Agents
		if c.Latest != nil && (n.Latest == nil || agents.Newer(*c.Latest, *n.Latest)) {
			n.Latest = c.Latest
		}
		if c.Agents > 0 && (n.Urgent == "" || agents.AttentionRank(c.Urgent) < agents.AttentionRank(n.Urgent)) {
			n.Urgent = c.Urgent
		}
	}
}

// order sorts every level newest message first, the ordering this plugin
// exists for. Nodes with no agent keep the sidebar's own order below them.
func order(nodes []*Node) {
	sort.SliceStable(nodes, func(i, j int) bool { return before(nodes[i], nodes[j]) })
	for _, n := range nodes {
		order(n.Children)
	}
}

func before(a, b *Node) bool {
	switch {
	case a.Latest != nil && b.Latest != nil:
		if agents.Newer(*a.Latest, *b.Latest) {
			return true
		}
		if agents.Newer(*b.Latest, *a.Latest) {
			return false
		}
	case a.Latest != nil:
		return true
	case b.Latest != nil:
		return false
	}
	return a.order < b.order
}

// Filter keeps the agent panes in one status and the groups that lead to
// them. It returns a copy, so the full tree survives to be filtered again.
func Filter(nodes []*Node, status agents.StatusFilter) []*Node {
	if status == agents.StatusAll {
		return nodes
	}
	var out []*Node
	for _, n := range nodes {
		if n.Kind == KindPane {
			if n.Agent != nil && n.Agent.Agent.Status == string(status) {
				out = append(out, n)
			}
			continue
		}
		if kids := Filter(n.Children, status); len(kids) > 0 {
			c := *n
			c.Children = kids
			summarize(&c)
			out = append(out, &c)
		}
	}
	return out
}

// Search keeps the panes that match a query and the groups that lead to
// them, the way Filter does for a status. It returns a copy.
//
// It matches as the list filter does: case-insensitive substrings, with every
// space-separated term required. A pane's text is its own name, kind, status,
// terminal title and last message, plus the names of every group above it, so
// that a repository, a workspace or a branch finds all of its panes. An empty
// query keeps everything.
func Search(nodes []*Node, query string) []*Node {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nodes
	}
	return search(nodes, terms, "")
}

func search(nodes []*Node, terms []string, above string) []*Node {
	var out []*Node
	for _, n := range nodes {
		if n.Kind == KindPane {
			if matches(above+" "+searchText(n), terms) {
				out = append(out, n)
			}
			continue
		}
		if kids := search(n.Children, terms, above+" "+n.Name+" "+n.Label); len(kids) > 0 {
			c := *n
			c.Children = kids
			summarize(&c)
			out = append(out, &c)
		}
	}
	return out
}

// searchText is what a pane offers a search.
func searchText(n *Node) string {
	parts := []string{n.Name, n.Display(), n.Pane.Title, n.Pane.Status}
	if r := n.Agent; r != nil {
		parts = append(parts, r.Agent.Status, r.Last.Text)
	}
	return strings.Join(parts, " ")
}

func matches(text string, terms []string) bool {
	lower := strings.ToLower(text)
	for _, t := range terms {
		if !strings.Contains(lower, t) {
			return false
		}
	}
	return true
}

// Line is one visible row: a node and how deep it sits.
type Line struct {
	Node  *Node
	Depth int
}

// Flatten lists the nodes a reader can see, skipping the children of every
// collapsed group.
func Flatten(nodes []*Node, collapsed map[string]bool) []Line {
	var out []Line
	var walk func([]*Node, int)
	walk = func(ns []*Node, depth int) {
		for _, n := range ns {
			out = append(out, Line{Node: n, Depth: depth})
			if !collapsed[n.ID] {
				walk(n.Children, depth+1)
			}
		}
	}
	walk(nodes, 0)
	return out
}

// GroupsOf lists the ID of every group of one kind that has children, for
// folding one level of the tree. A group with no children cannot fold, so it
// is left out, or it would always count as open.
func GroupsOf(nodes []*Node, kind Kind) []string {
	var out []string
	for _, n := range nodes {
		if n.Kind == kind && len(n.Children) > 0 {
			out = append(out, n.ID)
		}
		out = append(out, GroupsOf(n.Children, kind)...)
	}
	return out
}

// Groups lists the ID of every group in the tree, for collapsing them all.
func Groups(nodes []*Node) []string {
	var out []string
	for _, n := range nodes {
		if n.IsGroup() {
			out = append(out, n.ID)
			out = append(out, Groups(n.Children)...)
		}
	}
	return out
}
