package agents

import "testing"

func labelled(space, repo, branch string, linked bool) Row {
	return Row{Space: space, Repo: repo, Branch: branch, Linked: linked}
}

// The whole point of the rule: say nothing when the space name already implies
// the repository and the branch.
func TestLabelStaysSilentWhenNothingIsSurprising(t *testing.T) {
	cases := map[string]Row{
		"repo matches the space":   labelled("rstk-otel-router", "rstk-otel-router", "main", false),
		"branch matches the space": labelled("badger", "badger", "badger", false),
		"default branch main":      labelled("cm", "cm", "main", false),
		"default branch master":    labelled("cm", "cm", "master", false),
		"no repository at all":     labelled("zmk-config", "", "", false),
	}
	for name, r := range cases {
		if got := r.Label(); got != r.Space {
			t.Errorf("%s: Label = %q, want the bare space %q", name, got, r.Space)
		}
	}
}

// A linked worktree is marked differently from an ordinary checkout that simply
// sits in a differently named directory.
func TestLabelDistinguishesALinkedWorktree(t *testing.T) {
	linked := labelled("software-factory", "Second Brain", "software-factory", true)
	if got := linked.Label(); got != "Second Brain ⑂ software-factory" {
		t.Errorf("got %q", got)
	}
	plain := labelled("nix", "nix-darwin", "main", false)
	if got := plain.Label(); got != "nix-darwin / nix" {
		t.Errorf("got %q", got)
	}
}

// The case the rule exists to catch: a space whose name hides its branch.
func TestLabelShowsABranchThatTheSpaceNameHides(t *testing.T) {
	r := labelled("scouts-scraping", "scouts-scraping", "scoutbook-plus-api", false)
	if got := r.Label(); got != "scouts-scraping  @scoutbook-plus-api" {
		t.Errorf("got %q", got)
	}
}

func TestLabelCombinesRepositoryAndBranch(t *testing.T) {
	r := labelled("notes", "Second Brain", "draft-42", true)
	if got := r.Label(); got != "Second Brain ⑂ notes  @draft-42" {
		t.Errorf("got %q", got)
	}
}

// An unresolved branch must not print an empty marker.
func TestLabelOmitsAnUnresolvedBranch(t *testing.T) {
	r := labelled("github", "Second Brain", "", true)
	if got := r.Label(); got != "Second Brain ⑂ github" {
		t.Errorf("got %q", got)
	}
}

// A detached HEAD is not a default branch, so its short hash shows.
func TestLabelShowsADetachedHead(t *testing.T) {
	r := labelled("research", "research", "4302358a", false)
	if got := r.Label(); got != "research  @4302358a" {
		t.Errorf("got %q", got)
	}
}
