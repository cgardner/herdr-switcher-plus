package gitref

import (
	"os"
	"path/filepath"
	"testing"
)

// checkout builds an ordinary repository whose .git is a directory.
func checkout(t *testing.T, head string) string {
	t.Helper()
	root := t.TempDir()
	git := filepath.Join(root, ".git")
	if err := os.MkdirAll(git, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(git, "HEAD"), []byte(head), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// linkedWorktree builds the shape git uses for `git worktree add`: the
// checkout's .git is a file pointing at a directory under the main repository.
// Several Herdr spaces are linked worktrees, so this is the common case.
func linkedWorktree(t *testing.T, head string) string {
	t.Helper()
	base := t.TempDir()
	real := filepath.Join(base, "main-repo", ".git", "worktrees", "feature")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "HEAD"), []byte(head), 0o600); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(base, "feature")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".git"), []byte("gitdir: "+real+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return work
}

func TestBranchFromAnOrdinaryCheckout(t *testing.T) {
	if got := Branch(checkout(t, "ref: refs/heads/main\n")); got != "main" {
		t.Errorf("got %q, want main", got)
	}
}

func TestBranchKeepsSlashesInTheName(t *testing.T) {
	if got := Branch(checkout(t, "ref: refs/heads/feature/nested-name\n")); got != "feature/nested-name" {
		t.Errorf("got %q, want feature/nested-name", got)
	}
}

func TestBranchFromALinkedWorktree(t *testing.T) {
	if got := Branch(linkedWorktree(t, "ref: refs/heads/auth-service\n")); got != "auth-service" {
		t.Errorf("got %q, want auth-service", got)
	}
}

// A detached HEAD yields a short hash. An empty result would read as "not a
// repository", which is a different thing entirely.
func TestBranchOnADetachedHeadGivesAShortHash(t *testing.T) {
	if got := Branch(checkout(t, "4302358a9b1c2d3e4f50617283940a1b2c3d4e5f\n")); got != "4302358a" {
		t.Errorf("got %q, want 4302358a", got)
	}
}

func TestBranchOnAShortHeadValueIsReturnedWhole(t *testing.T) {
	if got := Branch(checkout(t, "abc123\n")); got != "abc123" {
		t.Errorf("got %q, want abc123", got)
	}
}

// A symbolic ref outside refs/heads keeps its full name rather than being
// mistaken for a branch.
func TestBranchOnANonHeadSymbolicRef(t *testing.T) {
	if got := Branch(checkout(t, "ref: refs/tags/v1.0.0\n")); got != "refs/tags/v1.0.0" {
		t.Errorf("got %q, want the full ref", got)
	}
}

func TestBranchOnNonRepositories(t *testing.T) {
	cases := map[string]string{
		"empty path":     "",
		"absent path":    filepath.Join(t.TempDir(), "nope"),
		"no .git at all": t.TempDir(),
	}
	for name, path := range cases {
		if got := Branch(path); got != "" {
			t.Errorf("%s: got %q, want empty", name, got)
		}
	}
}

// A .git file that is not a gitdir pointer must not resolve to a branch.
func TestBranchOnAMalformedGitFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("not a pointer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Branch(root); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestBranchWhenTheWorktreePointerLeadsNowhere(t *testing.T) {
	root := t.TempDir()
	dangling := "gitdir: " + filepath.Join(t.TempDir(), "gone") + "\n"
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte(dangling), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Branch(root); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// A .git that stats as a file but cannot be read leaves the branch unknown
// rather than failing the whole switcher.
func TestBranchWhenTheGitFileIsUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	root := t.TempDir()
	dot := filepath.Join(root, ".git")
	if err := os.WriteFile(dot, []byte("gitdir: /somewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dot, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dot, 0o600) })

	if got := Branch(root); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
