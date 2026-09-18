package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkersFollowTheFileType(t *testing.T) {
	start, end := Markers("tokens", "README.md")
	if start != "<!-- BEGIN GENERATED: tokens -->" || end != "<!-- END GENERATED: tokens -->" {
		t.Errorf("markdown markers wrong: %q %q", start, end)
	}
	start, end = Markers("tokens", "example-config.toml")
	if start != "# BEGIN GENERATED: tokens" || end != "# END GENERATED: tokens" {
		t.Errorf("toml markers wrong: %q %q", start, end)
	}
}

func TestSpliceReplacesOnlyTheMarkedRegion(t *testing.T) {
	before := "keep me\n<!-- BEGIN GENERATED: t -->\nold\nlines\n<!-- END GENERATED: t -->\nkeep me too\n"
	got, err := Splice(before, "t", "README.md", "fresh\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "keep me\n") || !strings.Contains(got, "keep me too\n") {
		t.Errorf("surrounding prose was disturbed:\n%s", got)
	}
	if strings.Contains(got, "old") {
		t.Errorf("the old body survived:\n%s", got)
	}
	if !strings.Contains(got, "fresh") {
		t.Errorf("the new body is missing:\n%s", got)
	}
}

func TestSpliceIsIdempotent(t *testing.T) {
	src := "<!-- BEGIN GENERATED: t -->\n<!-- END GENERATED: t -->\n"
	once, err := Splice(src, "t", "README.md", "body\n")
	if err != nil {
		t.Fatal(err)
	}
	twice, err := Splice(once, "t", "README.md", "body\n")
	if err != nil {
		t.Fatal(err)
	}
	if once != twice {
		t.Errorf("running twice changed the file:\n%q\n%q", once, twice)
	}
}

func TestSpliceAddsAMissingTrailingNewline(t *testing.T) {
	got, _ := Splice("<!-- BEGIN GENERATED: t -->\n<!-- END GENERATED: t -->\n", "t", "README.md", "no newline")
	if !strings.Contains(got, "no newline\n<!-- END") {
		t.Errorf("body and closing marker ran together:\n%q", got)
	}
}

func TestSpliceWithAnEmptyBody(t *testing.T) {
	got, err := Splice("<!-- BEGIN GENERATED: t -->\nold\n<!-- END GENERATED: t -->\n", "t", "README.md", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "old") {
		t.Errorf("an empty body should clear the region:\n%q", got)
	}
}

// A missing marker is an error, not a silent no-op. A generator that quietly
// writes nothing produces documentation nobody notices has stopped updating.
func TestSpliceReportsMissingMarkers(t *testing.T) {
	if _, err := Splice("nothing here", "t", "README.md", "x"); err == nil {
		t.Error("a missing opening marker should fail")
	}
	if _, err := Splice("<!-- BEGIN GENERATED: t -->\n", "t", "README.md", "x"); err == nil {
		t.Error("a missing closing marker should fail")
	}
}

func TestApplyReportsChangeWithoutWritingInCheckMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	before := "<!-- BEGIN GENERATED: t -->\n<!-- END GENERATED: t -->\n"
	if err := os.WriteFile(path, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}

	changed, err := Apply(path, []Section{{Name: "t", Body: "new\n"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("expected the file to be reported as stale")
	}
	after, _ := os.ReadFile(path)
	if string(after) != before {
		t.Error("check mode must not write")
	}

	if _, err := Apply(path, []Section{{Name: "t", Body: "new\n"}}, true); err != nil {
		t.Fatal(err)
	}
	after, _ = os.ReadFile(path)
	if !strings.Contains(string(after), "new") {
		t.Error("write mode should update the file")
	}

	changed, err = Apply(path, []Section{{Name: "t", Body: "new\n"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("a second run should report no change")
	}
}

func TestApplyOnAMissingFile(t *testing.T) {
	if _, err := Apply(filepath.Join(t.TempDir(), "absent.md"), nil, true); err == nil {
		t.Error("expected an error")
	}
}

func TestApplyPropagatesASpliceError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("no markers"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(path, []Section{{Name: "t", Body: "x"}}, true); err == nil {
		t.Error("expected an error")
	}
}

func TestTable(t *testing.T) {
	got := Table([]string{"a", "b"}, [][]string{{"1", "2"}})
	want := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestComment(t *testing.T) {
	got := Comment("one\n\ntwo\n")
	if got != "# one\n#\n# two" {
		t.Errorf("got %q", got)
	}
}

// A file that cannot be written reports the error rather than claiming
// success, so a read-only checkout fails loudly instead of silently skipping.
func TestApplyReportsAWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the permission bits this test relies on")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("<!-- BEGIN GENERATED: t -->\n<!-- END GENERATED: t -->\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if err := os.Remove(path); err == nil {
		t.Skip("this filesystem ignores directory permissions")
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}

	changed, err := Apply(path, []Section{{Name: "t", Body: "new\n"}}, true)
	if !changed {
		t.Error("the file should still be reported as changed")
	}
	if err == nil {
		t.Error("expected a write error")
	}
}
