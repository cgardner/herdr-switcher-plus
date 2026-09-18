package transcript

import (
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
