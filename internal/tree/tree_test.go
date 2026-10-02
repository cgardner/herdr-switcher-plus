package tree

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/transcript"
)

func worktree(repo string, linked bool) *herdr.Worktree {
	return &herdr.Worktree{RepoName: repo, RepoKey: "/git/" + repo, CheckoutPath: "/co/" + repo, IsLinked: linked}
}

func agentRow(pane, status string, age time.Duration) agents.Row {
	return agents.Row{
		Agent:   herdr.Agent{PaneID: pane, Kind: "claude", Status: status},
		Last:    transcript.Message{At: time.Now().Add(-age), Text: "msg " + pane},
		HasLast: true,
	}
}

// fixture is a repository with three linked worktrees, a repository with one
// checkout, and a space that is no checkout at all.
func fixture() (*herdr.Snapshot, []agents.Row) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{
			{WorkspaceID: "w1", Label: "api-gateway", Number: 1, Worktree: worktree("api-gateway", false)},
			{WorkspaceID: "w2", Label: "auth-service", Number: 2, Worktree: worktree("platform", true)},
			{WorkspaceID: "w3", Label: "billing", Number: 3, Worktree: worktree("platform", true)},
			{WorkspaceID: "w4", Label: "dotfiles", Number: 4},
			{WorkspaceID: "w5", Label: "notifications", Number: 5, Worktree: worktree("platform", true)},
		},
		Panes: []herdr.Pane{
			{PaneID: "w1:p1", WorkspaceID: "w1", Agent: "claude", Status: "working"},
			{PaneID: "w1:p2", WorkspaceID: "w1", Status: "unknown", Title: "go test"},
			{PaneID: "w2:p1", WorkspaceID: "w2", Agent: "claude", Status: "blocked"},
			{PaneID: "w3:p1", WorkspaceID: "w3", Agent: "claude", Status: "done"},
			{PaneID: "w4:p1", WorkspaceID: "w4", Agent: "claude", Status: "idle"},
			{PaneID: "w5:p1", WorkspaceID: "w5", Status: "unknown"},
		},
	}
	rows := []agents.Row{
		agentRow("w1:p1", "working", time.Hour),
		agentRow("w2:p1", "blocked", time.Minute),
		agentRow("w3:p1", "done", 2*time.Hour),
		agentRow("w4:p1", "idle", 3*time.Hour),
	}
	return snap, rows
}

func noBranch(string) string { return "" }

func build() []*Node {
	snap, rows := fixture()
	return Build(snap, rows, noBranch)
}

// outline prints the tree as indented names so a test can assert on its
// shape in one comparison.
func outline(nodes []*Node, collapsed map[string]bool) string {
	var b strings.Builder
	for _, l := range Flatten(nodes, collapsed) {
		b.WriteString(strings.Repeat("  ", l.Depth) + l.Node.Name + "\n")
	}
	return b.String()
}

func TestBuildGroupsLinkedWorktreesUnderTheirRepository(t *testing.T) {
	got := outline(build(), nil)
	want := `platform
  auth-service
    claude
  billing
    claude
  notifications
    shell
api-gateway
  claude
  shell
dotfiles
  claude
`
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestBuildMarksLinkedWorktrees(t *testing.T) {
	roots := build()
	if !roots[0].Children[0].Linked {
		t.Error("a linked worktree should be marked")
	}
	if roots[1].Linked {
		t.Error("a main checkout should not be marked")
	}
}

// A single checkout names its repository the way the list does, because it
// stands at the root in the repository's place.
func TestAPromotedWorkspaceNamesItsRepository(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{{WorkspaceID: "w1", Label: "infra", Worktree: worktree("infra-terraform", false)}},
		Panes:      []herdr.Pane{{PaneID: "w1:p1", WorkspaceID: "w1"}},
	}
	roots := Build(snap, nil, noBranch)
	if len(roots) != 1 || roots[0].Kind != KindWorkspace || roots[0].Name != "infra-terraform / infra" {
		t.Errorf("got %+v", roots[0])
	}
}

func TestAWorkspaceUnderARepositoryShowsItsBranch(t *testing.T) {
	snap, rows := fixture()
	branch := func(path string) string { return "redesign-nav" }
	roots := Build(snap, rows, branch)
	if got := roots[0].Children[0].Name; got != "auth-service  @redesign-nav" {
		t.Errorf("got %q", got)
	}
}

func TestBuildSummarizesEachGroup(t *testing.T) {
	platform := build()[0]
	if platform.Agents != 2 {
		t.Errorf("Agents = %d, want 2", platform.Agents)
	}
	if platform.Urgent != "blocked" {
		t.Errorf("Urgent = %q, want blocked", platform.Urgent)
	}
	if platform.Latest == nil || platform.Latest.Agent.PaneID != "w2:p1" {
		t.Errorf("Latest = %+v", platform.Latest)
	}
	if notifications := platform.Children[2]; notifications.Latest != nil || notifications.Agents != 0 {
		t.Errorf("a group of shells has no agent: %+v", notifications)
	}
}

// Groups with no agent keep the sidebar's order, below every group that has
// one.
func TestGroupsWithoutAnAgentKeepSidebarOrder(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{
			{WorkspaceID: "w2", Label: "billing", Number: 2},
			{WorkspaceID: "w1", Label: "scratchpad", Number: 1},
			{WorkspaceID: "w3", Label: "dotfiles", Number: 3},
		},
		Panes: []herdr.Pane{
			{PaneID: "w2:p1", WorkspaceID: "w2"},
			{PaneID: "w1:p1", WorkspaceID: "w1"},
			{PaneID: "w3:p1", WorkspaceID: "w3", Agent: "claude"},
		},
	}
	roots := Build(snap, []agents.Row{agentRow("w3:p1", "idle", time.Hour)}, noBranch)
	var names []string
	for _, r := range roots {
		names = append(names, r.Name)
	}
	if got := strings.Join(names, " "); got != "dotfiles scratchpad billing" {
		t.Errorf("got %q", got)
	}
}

// Two agents with no transcript tie on age, and the sidebar order settles it.
func TestTiedAgentsFallBackToSidebarOrder(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{
			{WorkspaceID: "w2", Label: "billing", Number: 2},
			{WorkspaceID: "w1", Label: "scratchpad", Number: 1},
		},
		Panes: []herdr.Pane{{PaneID: "w2:p1", WorkspaceID: "w2"}, {PaneID: "w1:p1", WorkspaceID: "w1"}},
	}
	rows := []agents.Row{{Agent: herdr.Agent{PaneID: "w2:p1"}}, {Agent: herdr.Agent{PaneID: "w1:p1"}}}
	roots := Build(snap, rows, noBranch)
	if roots[0].Name != "scratchpad" {
		t.Errorf("got %q first", roots[0].Name)
	}
}

func TestAnOlderAgentSortsAfterANewerOne(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{{WorkspaceID: "w1", Label: "billing"}},
		Panes:      []herdr.Pane{{PaneID: "w1:p1", WorkspaceID: "w1"}, {PaneID: "w1:p2", WorkspaceID: "w1"}},
	}
	rows := []agents.Row{agentRow("w1:p1", "idle", time.Hour), agentRow("w1:p2", "idle", time.Minute)}
	panes := Build(snap, rows, noBranch)[0].Children
	if panes[0].Pane.PaneID != "w1:p2" {
		t.Errorf("newest should lead, got %q", panes[0].Pane.PaneID)
	}
}

// An older Herdr sends no pane list, and the agents stand in for it.
func TestBuildFallsBackToTheAgentsWithoutAPaneList(t *testing.T) {
	snap := &herdr.Snapshot{
		Workspaces: []herdr.Workspace{{WorkspaceID: "w1", Label: "billing"}},
		Agents:     []herdr.Agent{{PaneID: "w1:p1", WorkspaceID: "w1", Kind: "codex", Status: "idle"}},
	}
	roots := Build(snap, nil, noBranch)
	if got := outline(roots, nil); got != "billing\n  codex\n" {
		t.Errorf("got %q", got)
	}
}

// A pane whose workspace is missing from the snapshot still appears, under a
// group named for the raw ID.
func TestAPaneWithAnUnknownWorkspaceStillAppears(t *testing.T) {
	snap := &herdr.Snapshot{Panes: []herdr.Pane{{PaneID: "w9:p1", WorkspaceID: "w9"}}}
	if got := outline(Build(snap, nil, noBranch), nil); got != "w9\n  shell\n" {
		t.Errorf("got %q", got)
	}
}

func TestPaneNameFallsBackToTheRowKind(t *testing.T) {
	n := &Node{Agent: &agents.Row{Agent: herdr.Agent{Kind: "codex"}}}
	if got := paneName(n); got != "codex" {
		t.Errorf("got %q", got)
	}
}

func TestRepoKeyPrefersTheGitDirectory(t *testing.T) {
	cases := []struct {
		w    herdr.Worktree
		want string
	}{
		{herdr.Worktree{RepoKey: "/git/platform", RepoRoot: "/co/platform/"}, "/git/platform"},
		{herdr.Worktree{RepoRoot: "/co/platform/"}, "/co/platform"},
		{herdr.Worktree{RepoName: "platform"}, "platform"},
	}
	for _, c := range cases {
		if got := repoKey(&c.w); got != c.want {
			t.Errorf("repoKey(%+v) = %q, want %q", c.w, got, c.want)
		}
	}
}

func TestFilterKeepsMatchingPanesAndTheirGroups(t *testing.T) {
	roots := build()
	got := outline(Filter(roots, agents.StatusBlocked), nil)
	if got != "platform\n  auth-service\n    claude\n" {
		t.Errorf("got %q", got)
	}
	if platform := Filter(roots, agents.StatusBlocked)[0]; platform.Agents != 1 {
		t.Errorf("a filtered group recounts its agents, got %d", platform.Agents)
	}
	if roots[0].Agents != 2 {
		t.Error("filtering must not change the full tree")
	}
}

func TestFilterAllReturnsTheTreeUnchanged(t *testing.T) {
	roots := build()
	if got := Filter(roots, agents.StatusAll); len(got) != len(roots) || got[0] != roots[0] {
		t.Error("the all filter should return the same tree")
	}
}

func TestFlattenSkipsCollapsedChildren(t *testing.T) {
	roots := build()
	got := outline(roots, map[string]bool{roots[0].ID: true, roots[1].ID: true})
	if got != "platform\napi-gateway\ndotfiles\n  claude\n" {
		t.Errorf("got %q", got)
	}
}

func TestGroupsListsEveryGroup(t *testing.T) {
	if got := len(Groups(build())); got != 6 {
		t.Errorf("got %d groups, want 6", got)
	}
}

func TestIsGroup(t *testing.T) {
	if (&Node{Kind: KindPane}).IsGroup() || !(&Node{Kind: KindRepo}).IsGroup() {
		t.Error("only repositories and workspaces are groups")
	}
}

func TestCollectBuildsFromTheLiveSources(t *testing.T) {
	ca, rb := collectAgents, resolveBranch
	t.Cleanup(func() { collectAgents, resolveBranch = ca, rb })
	collectAgents = func() (*herdr.Snapshot, []agents.Row, error) {
		snap, rows := fixture()
		return snap, rows, nil
	}
	resolveBranch = noBranch

	roots, err := Collect()
	if err != nil || len(roots) != 3 {
		t.Fatalf("got %d roots, %v", len(roots), err)
	}
}

func TestCollectReportsAFailure(t *testing.T) {
	ca := collectAgents
	t.Cleanup(func() { collectAgents = ca })
	collectAgents = func() (*herdr.Snapshot, []agents.Row, error) { return nil, nil, errors.New("no server") }
	if _, err := Collect(); err == nil {
		t.Fatal("expected an error")
	}
}

// find returns the node with this ID anywhere in the tree.
func find(nodes []*Node, id string) *Node {
	for _, n := range nodes {
		if n.ID == id {
			return n
		}
		if f := find(n.Children, id); f != nil {
			return f
		}
	}
	return nil
}

// Every node an action can start from says where new work begins.
func TestBuildGivesEachNodeAWorkspaceAndADirectory(t *testing.T) {
	snap, rows := fixture()
	snap.Workspaces[2].Worktree.CheckoutPath = "/co/billing"
	snap.Panes[1].Cwd, snap.Panes[1].ForegroundCwd = "/co/api-gateway", "/co/api-gateway/cmd"
	snap.Panes[4].Cwd = "/home/dotfiles"
	roots := Build(snap, rows, noBranch)

	cases := []struct{ id, ws, dir string }{
		{"ws:w3", "w3", "/co/billing"},
		{"ws:w4", "w4", "/home/dotfiles"},
		{"pane:w1:p2", "w1", "/co/api-gateway/cmd"},
		{"pane:w4:p1", "w4", "/home/dotfiles"},
		{"repo:/git/platform", "", "/co/platform"},
	}
	for _, c := range cases {
		n := find(roots, c.id)
		if n == nil {
			t.Fatalf("no node %s", c.id)
		}
		if n.WorkspaceID != c.ws || n.Dir != c.dir {
			t.Errorf("%s: workspace %q dir %q, want %q %q", c.id, n.WorkspaceID, n.Dir, c.ws, c.dir)
		}
	}
}

// A repository starts new work in its main checkout when one is open, even
// when a linked worktree comes first in the sidebar.
func TestARepositoryPrefersItsMainCheckout(t *testing.T) {
	snap, rows := fixture()
	snap.Workspaces[1].Worktree.CheckoutPath = "/co/auth"
	snap.Workspaces[2].Worktree.CheckoutPath, snap.Workspaces[2].Worktree.IsLinked = "/co/main", false
	snap.Workspaces[4].Worktree.CheckoutPath = "/co/notify"
	if got := find(Build(snap, rows, noBranch), "repo:/git/platform").Dir; got != "/co/main" {
		t.Errorf("dir = %q, want the main checkout", got)
	}
}

// A name the user gave sits beside the agent kind rather than replacing it.
// The pane's own label wins over the agent's name.
func TestAPaneKeepsItsKindAndCarriesItsName(t *testing.T) {
	snap, rows := fixture()
	snap.Panes[0].Label = "reviewer"
	rows[0].Agent.Name = "ignored"
	rows[1].Agent.Name = "auth-fix"
	roots := Build(snap, rows, noBranch)

	cases := []struct {
		id, name, label, title string
		fromAgent              bool
	}{
		{"pane:w1:p1", "claude", "reviewer", "reviewer", false},
		{"pane:w2:p1", "claude", "auth-fix", "auth-fix", true},
		{"pane:w1:p2", "shell", "", "shell", false},
	}
	for _, c := range cases {
		n := find(roots, c.id)
		if n.Name != c.name || n.Label != c.label || n.Title() != c.title || n.AgentNamed != c.fromAgent {
			t.Errorf("%s: name %q label %q title %q agent %v", c.id, n.Name, n.Label, n.Title(), n.AgentNamed)
		}
	}
	if ws := find(roots, "ws:w4"); ws.Title() != ws.Name {
		t.Errorf("a workspace's title is its name, got %q", ws.Title())
	}
}

// A group with no children cannot fold, so it never counts as open.
func TestGroupsOfListsOneKindWithChildren(t *testing.T) {
	roots := build()
	roots = append(roots, &Node{Kind: KindWorkspace, ID: "ws:empty"})
	if got := strings.Join(GroupsOf(roots, KindRepo), " "); got != "repo:/git/platform" {
		t.Errorf("repos = %q", got)
	}
	got := GroupsOf(roots, KindWorkspace)
	if len(got) != 5 || strings.Contains(strings.Join(got, " "), "ws:empty") {
		t.Errorf("workspaces = %v", got)
	}
}

func TestSearchKeepsMatchingPanesAndTheirGroups(t *testing.T) {
	snap, rows := fixture()
	snap.Panes[2].Label = "token-expiry"
	roots := Build(snap, rows, noBranch)

	cases := []struct{ query, want string }{
		{"", outline(roots, nil)},
		{"TOKEN", "platform\n  auth-service\n    claude\n"},
		{"go test", "api-gateway\n  shell\n"},
		{"platform shell", "platform\n  notifications\n    shell\n"},
		{"msg w3:p1", "platform\n  billing\n    claude\n"},
		{"done", "platform\n  billing\n    claude\n"},
		{"dotfiles", "dotfiles\n  claude\n"},
		{"nothing-like-this", ""},
	}
	for _, c := range cases {
		if got := outline(Search(roots, c.query), nil); got != c.want {
			t.Errorf("%q: got\n%swant\n%s", c.query, got, c.want)
		}
	}
	// The copy leaves the full tree whole, with fresh summaries.
	if got := len(find(roots, "repo:/git/platform").Children); got != 3 {
		t.Errorf("the search changed the tree: %d workspaces", got)
	}
	if got := Search(roots, "token")[0].Agents; got != 1 {
		t.Errorf("a searched group counts only its matches, got %d", got)
	}
}
