// Package transcript reads Claude Code session transcripts to find when each
// session last exchanged a real message.
//
// The file modification time is NOT a usable answer. Claude Code keeps
// appending bookkeeping records to a transcript long after the conversation
// stops: artifact-autoreact-ledger, bridge-session, file-history-snapshot,
// mode, atis-latch and others. Measured on a live session, mtime reported a
// 31-minute age for a conversation whose last message was 52 hours old, and it
// ranked one session 2nd that belonged in 9th place. Only entries of type
// "user" or "assistant" carry a timestamp, and only those are messages.
package transcript

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// tailBytes is how much of the end of a transcript gets scanned before falling
// back to a full read. Transcripts here run to about 2.5 MB, and the last
// message sits within this window in every observed case.
const tailBytes = 256 * 1024

// previewRunes caps the stored message preview. The UI truncates further to
// fit the pane width.
const previewRunes = 300

// Index maps a session UUID to its transcript path. Building it walks the
// project directories and reads no file contents, so it stays cheap even with
// several hundred transcripts on disk.
func Index() map[string]string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(home, ".claude", "projects", "*", "*.jsonl"))
	if err != nil {
		return nil
	}
	idx := make(map[string]string, len(matches))
	for _, p := range matches {
		idx[strings.TrimSuffix(filepath.Base(p), ".jsonl")] = p
	}
	return idx
}

// Message is the newest real message found in a transcript.
type Message struct {
	At   time.Time
	Role string
	Text string
}

type entry struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type payload struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type block struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

// Read returns the newest user or assistant message in the transcript at path.
// It scans the tail first and re-reads the whole file only when the tail holds
// no message, which keeps the common case to a single small read.
func Read(path string) (Message, bool) {
	if m, ok := scan(readTail(path)); ok {
		return m, true
	}
	whole, err := os.ReadFile(path)
	if err != nil {
		return Message{}, false
	}
	return scan(whole)
}

func readTail(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil
	}
	start := info.Size() - tailBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, 0); err != nil {
		return nil
	}
	buf := make([]byte, info.Size()-start)
	n, _ := f.Read(buf)
	return buf[:n]
}

// scan walks lines from the end so the first match is the newest message. A
// partial first line from a mid-file seek fails to parse and gets skipped.
func scan(data []byte) (Message, bool) {
	if len(data) == 0 {
		return Message{}, false
	}
	lines := strings.Split(string(data), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var e entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		if e.Type != "user" && e.Type != "assistant" {
			continue
		}
		if e.Timestamp == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339, e.Timestamp)
		if err != nil {
			continue
		}
		return Message{At: at, Role: e.Type, Text: preview(e.Message)}, true
	}
	return Message{}, false
}

// preview pulls readable text out of a message body. Content arrives either as
// a plain string or as an array of typed blocks, and a turn may open with a
// tool call rather than prose.
func preview(raw json.RawMessage) string {
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(p.Content, &s); err == nil {
		return clean(s)
	}
	var blocks []block
	if err := json.Unmarshal(p.Content, &blocks); err != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Type == "text" {
			if t := clean(b.Text); t != "" {
				return t
			}
		}
	}
	for _, b := range blocks {
		if b.Type == "tool_use" && b.Name != "" {
			return "[" + b.Name + "]"
		}
	}
	return ""
}

// clean strips the XML-ish wrappers Claude Code puts around slash commands,
// system reminders and pasted content, then collapses whitespace so a preview
// stays on one line.
func clean(s string) string {
	for {
		open := strings.Index(s, "<")
		if open < 0 {
			break
		}
		close := strings.Index(s[open:], ">")
		if close < 0 {
			break
		}
		s = s[:open] + " " + s[open+close+1:]
	}
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > previewRunes {
		s = string(r[:previewRunes]) + "…"
	}
	return s
}
