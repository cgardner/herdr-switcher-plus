package ui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-pane-sort/internal/agents"
	"github.com/cgardner/herdr-pane-sort/internal/herdr"
	"github.com/cgardner/herdr-pane-sort/internal/transcript"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The test terminal must be told it has color, or lipgloss strips every
// sequence and the background assertions cannot mean anything.
func init() { lipgloss.SetColorProfile(termenv.TrueColor) }

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visible(s string) string { return ansi.ReplaceAllString(s, "") }

func testRow(pane, space, status, text string, age time.Duration) agents.Row {
	return agents.Row{
		Agent:   herdr.Agent{PaneID: pane, Status: status, Title: "term title"},
		Space:   space,
		Last:    transcript.Message{At: time.Now().Add(-age), Role: "assistant", Text: text},
		HasLast: true,
	}
}

func render(t *testing.T, width int, selected int, rows ...agents.Row) []string {
	t.Helper()
	d := delegate{selectionBg: lipgloss.Color("#313244")}
	l := list.New(toItems(rows), d, width, 20)
	l.Select(selected)

	out := make([]string, len(rows))
	for i := range rows {
		var buf bytes.Buffer
		d.Render(&buf, l, i, l.Items()[i])
		out[i] = buf.String()
	}
	return out
}

const bgSequence = "48;2;48;50;68" // #313244 as truecolor

// Every segment must carry the background. A foreground style emits its own
// reset, so a single unstyled segment would punch a hole in the bar.
func TestRenderSelectedRowIsFullyBackgrounded(t *testing.T) {
	got := render(t, 80, 0, testRow("w1:p1", "nix", "idle", "hello", time.Minute))[0]
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, bgSequence) {
			t.Fatalf("line lacks the selection background: %q", line)
		}
		// A reset that is not immediately followed by the background again
		// would end the bar early.
		for _, part := range strings.Split(line, "\x1b[0m")[1:] {
			if part != "" && !strings.HasPrefix(part, "\x1b[") {
				t.Errorf("unstyled text after a reset breaks the bar: %q", part)
			}
		}
	}
}

func TestRenderUnselectedRowHasNoBackground(t *testing.T) {
	rows := []agents.Row{
		testRow("w1:p1", "nix", "idle", "first", time.Minute),
		testRow("w2:p1", "docs", "idle", "second", time.Hour),
	}
	got := render(t, 80, 0, rows...)[1]
	if strings.Contains(got, bgSequence) {
		t.Errorf("unselected row must not be highlighted: %q", got)
	}
}

// The bar has to reach the right edge, or the highlight looks ragged.
func TestRenderPadsBothLinesToTheFullWidth(t *testing.T) {
	for _, width := range []int{40, 80, 120} {
		got := render(t, width, 0, testRow("w1:p1", "nix", "idle", "short", time.Minute))[0]
		for _, line := range strings.Split(got, "\n") {
			if n := lipgloss.Width(line); n != width {
				t.Errorf("width %d: line rendered %d columns: %q", width, n, visible(line))
			}
		}
	}
}

func TestRenderTruncatesOverlongText(t *testing.T) {
	long := strings.Repeat("abcdefghij ", 40)
	got := render(t, 60, 0, testRow("w1:p1", "nix", "idle", long, time.Minute))[0]
	for _, line := range strings.Split(got, "\n") {
		if n := lipgloss.Width(line); n > 60 {
			t.Errorf("line overflowed to %d columns", n)
		}
	}
}

// A zero width happens before the first WindowSizeMsg arrives. Rendering then
// must produce nothing rather than a negative-length pad.
func TestRenderAtZeroWidthWritesNothing(t *testing.T) {
	d := delegate{selectionBg: lipgloss.Color("#313244")}
	l := list.New(toItems([]agents.Row{testRow("w1:p1", "nix", "idle", "x", time.Minute)}), d, 0, 0)
	var buf bytes.Buffer
	d.Render(&buf, l, 0, l.Items()[0])
	if buf.Len() != 0 {
		t.Errorf("expected no output at zero width, got %q", buf.String())
	}
}

func TestRenderSelectionBarOnlyOnTheSelectedRow(t *testing.T) {
	rows := []agents.Row{
		testRow("w1:p1", "nix", "idle", "first", time.Minute),
		testRow("w2:p1", "docs", "idle", "second", time.Hour),
	}
	out := render(t, 80, 1, rows...)
	if strings.Contains(visible(out[0]), "▌") {
		t.Error("unselected row must not show the selection bar")
	}
	if !strings.Contains(visible(out[1]), "▌") {
		t.Error("selected row must show the selection bar")
	}
}

func TestRenderShowsAgeSpaceAndStatus(t *testing.T) {
	got := visible(render(t, 100, 0, testRow("w1:p1", "risk-levels", "blocked", "may I delete", 90*time.Minute))[0])
	for _, want := range []string{"1h", "risk-levels", "blocked", "may I delete"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered row is missing %q: %q", want, got)
		}
	}
}

func TestRenderIgnoresAForeignListItem(t *testing.T) {
	d := delegate{selectionBg: lipgloss.Color("#313244")}
	l := list.New(nil, d, 80, 20)
	var buf bytes.Buffer
	d.Render(&buf, l, 0, foreignItem{})
	if buf.Len() != 0 {
		t.Errorf("expected no output for an unknown item type, got %q", buf.String())
	}
}

type foreignItem struct{}

func (foreignItem) FilterValue() string { return "" }

func TestDelegateGeometry(t *testing.T) {
	d := delegate{}
	if d.Height() != 2 {
		t.Errorf("Height = %d, want 2", d.Height())
	}
	if d.Spacing() != 0 {
		t.Errorf("Spacing = %d, want 0", d.Spacing())
	}
	if d.Update(nil, nil) != nil {
		t.Error("Update must issue no command")
	}
}

func TestPreviewTextFallsBackThroughMessageThenTitle(t *testing.T) {
	r := testRow("w1:p1", "nix", "idle", "the message", time.Minute)
	if got := previewText(r); got != "the message" {
		t.Errorf("got %q, want the message", got)
	}
	r.Last.Text = ""
	if got := previewText(r); got != "term title" {
		t.Errorf("got %q, want term title", got)
	}
	r.Agent.Title = ""
	if got := previewText(r); got != "(no message found)" {
		t.Errorf("got %q, want the placeholder", got)
	}
}

func TestRoleMark(t *testing.T) {
	cases := map[string]string{"user": "› ", "assistant": "‹ ", "": "  ", "system": "  "}
	for role, want := range cases {
		if got := roleMark(role); got != want {
			t.Errorf("roleMark(%q) = %q, want %q", role, got, want)
		}
	}
}
