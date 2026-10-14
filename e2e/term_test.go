//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
)

// The two questions Bubble Tea puts to the terminal at startup. Until both are
// answered it blocks, and a test that does not answer them reads an empty
// screen. See AGENTS.md, "Testing the terminal UI".
const (
	askBackground = "\x1b]11;?"
	askCursor     = "\x1b[6n"

	replyBackground = "\x1b]11;rgb:1e1e/1e1e/2e2e\x1b\\"
	replyCursor     = "\x1b[1;1R"
)

const (
	termCols = 110
	termRows = 32
	patience = 8 * time.Second
)

// term runs the binary on a pseudo terminal, and keeps a virtual screen of
// what it draws. Bubble Tea repaints with diffs, so only a screen model can
// say what is visible now rather than what was ever printed.
type term struct {
	t   *testing.T
	pty *os.File
	cmd *exec.Cmd

	mu     sync.Mutex
	raw    bytes.Buffer
	screen vt10x.Terminal
	asked  [2]int // answers sent: background, cursor

	exited chan struct{}
	code   int
}

// start runs the binary on a terminal of termCols by termRows.
func (e *env) start(args ...string) *term {
	e.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = e.environ()
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: termRows, Cols: termCols})
	if err != nil {
		e.t.Fatal(err)
	}
	tm := &term{
		t: e.t, pty: f, cmd: cmd,
		screen: vt10x.New(vt10x.WithSize(termCols, termRows)),
		exited: make(chan struct{}),
	}
	go tm.read()
	go func() {
		err := cmd.Wait()
		tm.code = 0
		if ee, ok := err.(*exec.ExitError); ok {
			tm.code = ee.ExitCode()
		}
		close(tm.exited)
	}()
	e.t.Cleanup(func() {
		_ = cmd.Process.Kill()
		f.Close()
	})
	return tm
}

// read feeds the screen and answers the startup questions.
func (tm *term) read() {
	buf := make([]byte, 4096)
	for {
		n, err := tm.pty.Read(buf)
		if n > 0 {
			tm.mu.Lock()
			tm.raw.Write(buf[:n])
			_, _ = tm.screen.Write(buf[:n])
			tm.answer(askBackground, replyBackground, 0)
			tm.answer(askCursor, replyCursor, 1)
			tm.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// answer replies once for each time the question appeared in the output.
func (tm *term) answer(question, reply string, slot int) {
	for tm.asked[slot] < bytes.Count(tm.raw.Bytes(), []byte(question)) {
		tm.asked[slot]++
		_, _ = tm.pty.WriteString(reply)
	}
}

// text is the screen as it reads now.
func (tm *term) text() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.screen.String()
}

// waitFor blocks until every text is on the screen.
func (tm *term) waitFor(texts ...string) {
	tm.t.Helper()
	tm.until("show "+strings.Join(texts, " and "), func(s string) bool {
		for _, x := range texts {
			if !strings.Contains(s, x) {
				return false
			}
		}
		return true
	})
}

// waitGone blocks until none of the texts is on the screen.
func (tm *term) waitGone(texts ...string) {
	tm.t.Helper()
	tm.until("hide "+strings.Join(texts, " and "), func(s string) bool {
		for _, x := range texts {
			if strings.Contains(s, x) {
				return false
			}
		}
		return true
	})
}

func (tm *term) until(what string, ok func(string) bool) {
	tm.t.Helper()
	deadline := time.Now().Add(patience)
	for time.Now().Before(deadline) {
		if ok(tm.text()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	tm.t.Fatalf("the screen did not %s. It reads:\n%s", what, tm.text())
}

// send types keys. A key is one string, so "j" and "enter" are not mixed up:
// see keys for the names.
func (tm *term) send(keys ...string) {
	tm.t.Helper()
	for _, k := range keys {
		seq, ok := keyCodes[k]
		if !ok {
			seq = k
		}
		if _, err := tm.pty.WriteString(seq); err != nil {
			tm.mu.Lock()
			raw := tm.raw.String()
			tm.mu.Unlock()
			tm.t.Fatalf("sending %q: %v\nexit code %d\nraw output %q", k, err, tm.code, raw)
		}
		// Bubble Tea reads a lone escape as the start of a sequence for a
		// moment, so a key sent straight after one can be taken as part of it.
		time.Sleep(30 * time.Millisecond)
	}
}

var keyCodes = map[string]string{
	"enter": "\r",
	"esc":   "\x1b",
	"up":    "\x1b[A",
	"down":  "\x1b[B",
	"right": "\x1b[C",
	"left":  "\x1b[D",
	"space": " ",
}

// typeText sends each character as a key.
func (tm *term) typeText(s string) {
	tm.t.Helper()
	for _, r := range s {
		tm.send(string(r))
	}
}

// exit waits for the program to end and returns its status.
func (tm *term) exit() int {
	tm.t.Helper()
	select {
	case <-tm.exited:
		return tm.code
	case <-time.After(patience):
		tm.t.Fatalf("the program did not exit. The screen reads:\n%s", tm.text())
		return -1
	}
}
