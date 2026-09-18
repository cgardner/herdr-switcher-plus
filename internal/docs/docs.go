// Package docs renders the reference tables that the registries already
// define, and splices them into files between markers.
//
// The token list, the sort modes and the status keys were each written out by
// hand in two or three places. Every one of those copies was a chance for the
// documentation to drift from the code, and a reader who copies a stale token
// name gets an error. Generating them leaves one source of truth: the registry
// itself.
//
// Only the marked regions are rewritten, so the prose around them stays
// hand-written where judgement belongs.
package docs

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// Section is one generated block, named by the marker that encloses it.
type Section struct {
	Name string
	Body string
}

// Markers returns the opening and closing lines for a section in a file, using
// the comment syntax the file's extension implies.
func Markers(name, path string) (start, end string) {
	if strings.HasSuffix(path, ".toml") {
		return "# BEGIN GENERATED: " + name, "# END GENERATED: " + name
	}
	return "<!-- BEGIN GENERATED: " + name + " -->", "<!-- END GENERATED: " + name + " -->"
}

// Splice replaces the marked region with body, returning the new content.
//
// A missing marker is an error rather than a silent no-op: a generator that
// quietly writes nothing produces documentation nobody notices has stopped
// updating.
func Splice(content, name, path, body string) (string, error) {
	start, end := Markers(name, path)
	from := strings.Index(content, start)
	if from < 0 {
		return "", fmt.Errorf("%s: no %q marker", path, start)
	}
	to := strings.Index(content[from:], end)
	if to < 0 {
		return "", fmt.Errorf("%s: %q has no closing marker", path, start)
	}
	to += from

	var b strings.Builder
	b.WriteString(content[:from])
	b.WriteString(start)
	b.WriteString("\n")
	if body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	b.WriteString(content[to:])
	return b.String(), nil
}

// Apply splices every section into a file. It reports whether the file changed,
// which is what lets a check fail on stale documentation without rewriting it.
func Apply(path string, sections []Section, write bool) (changed bool, err error) {
	original, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	updated := string(original)
	for _, s := range sections {
		updated, err = Splice(updated, s.Name, path, s.Body)
		if err != nil {
			return false, err
		}
	}
	if updated == string(original) {
		return false, nil
	}
	if write {
		if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
			return true, err
		}
	}
	return true, nil
}

// Table renders a markdown table. Every column is present in every row, so a
// ragged row is a programming error rather than a rendering one.
func Table(headers []string, rows [][]string) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "| %s |\n", strings.Join(headers, " | "))
	fmt.Fprintf(&b, "|%s\n", strings.Repeat("---|", len(headers)))
	for _, row := range rows {
		fmt.Fprintf(&b, "| %s |\n", strings.Join(row, " | "))
	}
	return b.String()
}

// Comment prefixes every line so a markdown body can sit inside a TOML file.
func Comment(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = "#"
		} else {
			lines[i] = "# " + line
		}
	}
	return strings.Join(lines, "\n")
}
