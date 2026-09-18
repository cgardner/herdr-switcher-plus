// Package gitref resolves the branch a checkout sits on.
//
// Herdr's session snapshot reports a workspace's repository name and whether it
// is a linked worktree, but never its branch. Asking Herdr means one
// `worktree list` call per repository, because that command scopes to the
// calling pane. Reading the checkout directly answers every workspace at once,
// and measured across a live session it resolved 14 of 17 in about 2 ms.
package gitref

import (
	"os"
	"path/filepath"
	"strings"
)

const refPrefix = "ref: refs/heads/"

// shortSHA is how much of a detached HEAD to show.
const shortSHA = 8

// Branch returns the branch checked out at path, or an empty string when the
// path is not a checkout or cannot be read. A detached HEAD yields a short
// commit hash instead, because an empty result would read as "no repository".
func Branch(checkout string) string {
	if checkout == "" {
		return ""
	}
	head, err := os.ReadFile(filepath.Join(gitDir(checkout), "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(head))
	if rest, ok := strings.CutPrefix(line, refPrefix); ok {
		return rest
	}
	// A symbolic ref outside refs/heads, such as a tag, keeps its full name.
	if rest, ok := strings.CutPrefix(line, "ref: "); ok {
		return rest
	}
	if len(line) > shortSHA {
		return line[:shortSHA]
	}
	return line
}

// gitDir resolves the directory holding HEAD.
//
// In an ordinary checkout `.git` is that directory. In a linked worktree it is
// a file holding `gitdir: <path>`, pointing at the worktree's own directory
// under the main repository. Several Herdr spaces are linked worktrees of one
// repository, so this branch is the common case rather than an edge case.
func gitDir(checkout string) string {
	dot := filepath.Join(checkout, ".git")
	info, err := os.Stat(dot)
	if err != nil || info.IsDir() {
		return dot
	}
	b, err := os.ReadFile(dot)
	if err != nil {
		return dot
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir:")
	if !ok {
		return dot
	}
	return strings.TrimSpace(rest)
}
