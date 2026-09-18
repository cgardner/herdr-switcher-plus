package transcript

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The bookkeeping entries below are the ones observed in live Claude Code
// transcripts. They carry no timestamp and must never be mistaken for a
// message, because they are what makes the file modification time useless.
const bookkeeping = `{"type":"file-history-snapshot","v":1}
{"type":"artifact-autoreact-ledger","sessionId":"x"}
{"type":"bridge-session","v":1}
{"type":"atis-latch"}
{"type":"ai-title","title":"something"}
`

func writeTranscript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadIgnoresBookkeepingAfterLastMessage(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":"first"}}
{"type":"assistant","timestamp":"2026-09-16T21:06:25.505Z","message":{"role":"assistant","content":[{"type":"text","text":"the real last message"}]}}
` + bookkeeping

	got, ok := Read(writeTranscript(t, body))
	if !ok {
		t.Fatal("expected a message")
	}
	want := time.Date(2026, 9, 16, 21, 6, 25, 505000000, time.UTC)
	if !got.At.Equal(want) {
		t.Errorf("At = %v, want %v", got.At, want)
	}
	if got.Role != "assistant" {
		t.Errorf("Role = %q, want assistant", got.Role)
	}
	if got.Text != "the real last message" {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestReadSkipsEntriesWithoutTimestamp(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":"kept"}}
{"type":"user","message":{"role":"user","content":"no timestamp, must be skipped"}}
`
	got, ok := Read(writeTranscript(t, body))
	if !ok {
		t.Fatal("expected a message")
	}
	if got.Text != "kept" {
		t.Errorf("Text = %q, want kept", got.Text)
	}
}

func TestReadReturnsFalseWhenNoMessages(t *testing.T) {
	if _, ok := Read(writeTranscript(t, bookkeeping)); ok {
		t.Error("expected no message from a bookkeeping-only transcript")
	}
}

// A transcript larger than the tail window must still resolve, which exercises
// the full-read fallback and the discarded partial first line.
func TestReadFallsBackToFullReadBeyondTailWindow(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":"early and only"}}` + "\n")
	filler := `{"type":"artifact-autoreact-ledger","pad":"` + strings.Repeat("x", 4096) + `"}` + "\n"
	for b.Len() < tailBytes+(1<<16) {
		b.WriteString(filler)
	}

	got, ok := Read(writeTranscript(t, b.String()))
	if !ok {
		t.Fatal("expected the full-read fallback to find the message")
	}
	if got.Text != "early and only" {
		t.Errorf("Text = %q", got.Text)
	}
}

func TestReadPrefersTextBlockThenToolUse(t *testing.T) {
	body := `{"type":"assistant","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash"}]}}`
	got, ok := Read(writeTranscript(t, body))
	if !ok {
		t.Fatal("expected a message")
	}
	if got.Text != "[Bash]" {
		t.Errorf("Text = %q, want [Bash]", got.Text)
	}
}

func TestClean(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<command-name>/install</command-name>\n  install", "/install install"},
		{"line one\n\n   line two", "line one line two"},
		{"<system-reminder>hidden</system-reminder>ok", "hidden ok"},
		{"plain", "plain"},
		{"", ""},
	}
	for _, c := range cases {
		if got := clean(c.in); got != c.want {
			t.Errorf("clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCleanTruncatesLongText(t *testing.T) {
	got := clean(strings.Repeat("a", previewRunes+50))
	if r := []rune(got); len(r) != previewRunes+1 {
		t.Errorf("length = %d, want %d", len(r), previewRunes+1)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("expected an ellipsis suffix")
	}
}

func TestIndexInMapsUuidToPath(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"-Users-c-src-a", "-Users-c-platform-b"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	a := filepath.Join(root, "-Users-c-src-a", "uuid-one.jsonl")
	b := filepath.Join(root, "-Users-c-platform-b", "uuid-two.jsonl")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A non-transcript file in the same directory must be ignored.
	if err := os.WriteFile(filepath.Join(root, "-Users-c-src-a", "notes.md"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	idx := IndexIn(root)
	if len(idx) != 2 {
		t.Fatalf("indexed %d entries, want 2: %v", len(idx), idx)
	}
	if idx["uuid-one"] != a {
		t.Errorf("uuid-one = %q, want %q", idx["uuid-one"], a)
	}
	// A project directory whose name contains a space must still be indexed,
	// because the UUID is the key and the directory name is never parsed.
	if idx["uuid-two"] != b {
		t.Errorf("uuid-two = %q, want %q", idx["uuid-two"], b)
	}
}

func TestIndexInOnAMissingDirectoryIsEmpty(t *testing.T) {
	if idx := IndexIn(filepath.Join(t.TempDir(), "absent")); len(idx) != 0 {
		t.Errorf("want an empty index, got %v", idx)
	}
}

func TestIndexUsesTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "uuid-home.jsonl")
	if err := os.WriteFile(p, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Index()["uuid-home"]; got != p {
		t.Errorf("Index()[uuid-home] = %q, want %q", got, p)
	}
}

func TestReadOnAMissingFileReturnsFalse(t *testing.T) {
	if _, ok := Read(filepath.Join(t.TempDir(), "gone.jsonl")); ok {
		t.Error("expected false for a missing transcript")
	}
}

func TestReadHandlesStringContent(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":"plain string"}}`
	got, ok := Read(writeTranscript(t, body))
	if !ok || got.Text != "plain string" {
		t.Errorf("got %+v, ok=%v", got, ok)
	}
}

func TestReadSkipsUnparsableLinesAndBadTimestamps(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":"good"}}
{"type":"assistant","timestamp":"not a time","message":{"role":"assistant","content":"bad stamp"}}
this line is not json at all
`
	got, ok := Read(writeTranscript(t, body))
	if !ok || got.Text != "good" {
		t.Errorf("got %+v, ok=%v", got, ok)
	}
}

func TestReadWithUnreadableMessageBodyYieldsEmptyPreview(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":"not an object"}`
	got, ok := Read(writeTranscript(t, body))
	if !ok {
		t.Fatal("the entry still carries a usable timestamp")
	}
	if got.Text != "" {
		t.Errorf("Text = %q, want empty", got.Text)
	}
}

func TestReadWithNoTextBlockAndNoToolUseYieldsEmptyPreview(t *testing.T) {
	body := `{"type":"assistant","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"thinking"}]}}`
	got, ok := Read(writeTranscript(t, body))
	if !ok || got.Text != "" {
		t.Errorf("got %+v, ok=%v", got, ok)
	}
}

// A text block that cleans down to nothing must not win over a later tool use.
func TestReadPrefersAToolNameOverAnEmptyTextBlock(t *testing.T) {
	body := `{"type":"assistant","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"   "},{"type":"tool_use","name":"Read"}]}}`
	got, _ := Read(writeTranscript(t, body))
	if got.Text != "[Read]" {
		t.Errorf("Text = %q, want [Read]", got.Text)
	}
}

func TestScanOnEmptyInput(t *testing.T) {
	if _, ok := scan(nil); ok {
		t.Error("expected false for empty input")
	}
}

// A malformed glob pattern must yield an empty index, not a panic.
func TestIndexInWithAnUnparsablePattern(t *testing.T) {
	if idx := IndexIn("["); len(idx) != 0 {
		t.Errorf("want an empty index, got %v", idx)
	}
}

// With no home directory there is nowhere to look, so the index is empty.
func TestIndexWithNoHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	if idx := Index(); len(idx) != 0 {
		t.Errorf("want an empty index, got %v", idx)
	}
}

func TestPreviewWithNonArrayNonStringContent(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":42}}`
	got, ok := Read(writeTranscript(t, body))
	if !ok || got.Text != "" {
		t.Errorf("got %+v ok=%v, want an empty preview", got, ok)
	}
}

// An unterminated tag must not spin or truncate the rest of the text away.
func TestCleanWithAnUnterminatedTag(t *testing.T) {
	if got := clean("before <unclosed tag"); got != "before <unclosed tag" {
		t.Errorf("got %q", got)
	}
}

// failingFile lets the Stat and Seek guards in readTail be exercised.
type failingFile struct {
	statErr error
	seekErr error
}

func (failingFile) Read([]byte) (int, error) { return 0, io.EOF }
func (failingFile) Close() error             { return nil }
func (f failingFile) Seek(int64, int) (int64, error) {
	return 0, f.seekErr
}
func (f failingFile) Stat() (os.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return statOf(0), nil
}

type statOf int64

func (s statOf) Name() string     { return "fake" }
func (s statOf) Size() int64      { return int64(s) }
func (statOf) Mode() os.FileMode  { return 0 }
func (statOf) ModTime() time.Time { return time.Time{} }
func (statOf) IsDir() bool        { return false }
func (statOf) Sys() any           { return nil }

func stubOpenTail(t *testing.T, f tailFile, err error) {
	t.Helper()
	prev := openTail
	openTail = func(string) (tailFile, error) { return f, err }
	t.Cleanup(func() { openTail = prev })
}

// When the tail cannot be read the full-read fallback still finds the message,
// so a Stat or Seek failure costs speed rather than correctness.
func TestReadFallsBackWhenStatFails(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":"still found"}}`
	path := writeTranscript(t, body)
	stubOpenTail(t, failingFile{statErr: errors.New("stat failed")}, nil)

	got, ok := Read(path)
	if !ok || got.Text != "still found" {
		t.Errorf("got %+v ok=%v", got, ok)
	}
}

func TestReadFallsBackWhenSeekFails(t *testing.T) {
	body := `{"type":"user","timestamp":"2026-09-16T10:00:00.000Z","message":{"role":"user","content":"still found"}}`
	path := writeTranscript(t, body)
	stubOpenTail(t, failingFile{seekErr: errors.New("seek failed")}, nil)

	got, ok := Read(path)
	if !ok || got.Text != "still found" {
		t.Errorf("got %+v ok=%v", got, ok)
	}
}

// A path that is a directory opens cleanly and then fails to read. The result
// must be no message rather than a panic.
func TestReadOnADirectory(t *testing.T) {
	if _, ok := Read(t.TempDir()); ok {
		t.Error("a directory holds no message")
	}
}
