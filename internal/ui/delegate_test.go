package ui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/herdr"
	"github.com/cgardner/herdr-switcher-plus/internal/transcript"
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
	d := delegate{spec: DefaultSpec, selectionBg: lipgloss.Color("#313244")}
	l := list.New(toItems(rows, DefaultSpec), d, width, 20)
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
	d := delegate{spec: DefaultSpec, selectionBg: lipgloss.Color("#313244")}
	l := list.New(toItems([]agents.Row{testRow("w1:p1", "nix", "idle", "x", time.Minute)}, DefaultSpec), d, 0, 0)
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
	d := delegate{spec: DefaultSpec, selectionBg: lipgloss.Color("#313244")}
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
	d := delegate{spec: DefaultSpec}
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

func worktreeRow(pane, space, repo, branch string, linked bool) agents.Row {
	r := testRow(pane, space, "idle", "a message", time.Minute)
	r.Repo, r.Branch, r.Linked = repo, branch, linked
	return r
}

func TestRenderShowsTheWorktreeLabel(t *testing.T) {
	got := visible(render(t, 100, 0,
		worktreeRow("w1:p1", "software-factory", "Second Brain", "software-factory", true))[0])
	if !strings.Contains(got, "Second Brain ⑂ software-factory") {
		t.Errorf("rendered row is missing the worktree label: %q", got)
	}
}

func TestRenderOmitsAnImpliedRepository(t *testing.T) {
	got := visible(render(t, 100, 0,
		worktreeRow("w1:p1", "badger", "badger", "main", false))[0])
	if strings.Count(got, "badger") != 1 {
		t.Errorf("the space name should appear once, not twice: %q", got)
	}
}

// Repository and branch stay searchable even on rows whose label leaves them
// out as implied.
func TestFilterValueCarriesRepositoryAndBranch(t *testing.T) {
	r := worktreeRow("w1:p1", "badger", "scouts-scraping", "scoutbook-plus-api", true)
	got := item{row: r, now: time.Now()}.FilterValue()
	for _, want := range []string{"scouts-scraping", "scoutbook-plus-api"} {
		if !strings.Contains(got, want) {
			t.Errorf("FilterValue is missing %q: %q", want, got)
		}
	}
}

// The gap between the selection bar and the age is one column for the widest
// age on screen. A fixed-width age column padded shorter ages out further,
// which is what made the bar look detached from the row.
func TestSelectionBarSitsOneColumnFromTheWidestAge(t *testing.T) {
	rows := []agents.Row{
		testRow("w1:p1", "alpha", "idle", "m", 30*time.Second), // "now", 3 wide
		testRow("w2:p1", "beta", "idle", "m", 2*time.Hour),     // "2h",  2 wide
	}
	lines := strings.Split(visible(render(t, 90, 0, rows...)[0]), "\n")
	if got := lines[0]; !strings.HasPrefix(got, "▌ now") {
		t.Errorf("widest age should sit one column from the bar, got %q", got[:12])
	}
}

// runeIndex reports where a substring starts in columns rather than bytes. The
// selection bar is three bytes wide and one column, so strings.Index would
// report a selected row as further right than it renders.
func runeIndex(line, sub string) int {
	at := strings.Index(line, sub)
	if at < 0 {
		return -1
	}
	return len([]rune(line[:at]))
}

// A narrower age is right-aligned into the same column, so the label column
// still lines up down the pane.
func TestShorterAgesStayRightAligned(t *testing.T) {
	rows := []agents.Row{
		testRow("w1:p1", "alpha", "idle", "m", 30*time.Second),
		testRow("w2:p1", "beta", "idle", "m", 2*time.Hour),
	}
	out := render(t, 90, 0, rows...)
	first := strings.Split(visible(out[0]), "\n")[0]
	second := strings.Split(visible(out[1]), "\n")[0]
	if runeIndex(first, "alpha") != runeIndex(second, "beta") {
		t.Errorf("label column is ragged:\n%q\n%q", first, second)
	}
}

// A four-character age widens the column rather than colliding with the bar.
func TestAWideAgeKeepsItsSeparatingSpace(t *testing.T) {
	old := testRow("w1:p1", "ancient", "idle", "m", 400*24*time.Hour)
	line := strings.Split(visible(render(t, 90, 0, old)[0]), "\n")[0]
	if !strings.HasPrefix(line, "▌ 400d") {
		t.Errorf("got %q, want the bar, one space, then 400d", line[:12])
	}
}

// The preview line indents to the label column, so the message sits under the
// name it belongs to whatever the age column measures.
func TestPreviewIndentsToTheLabelColumn(t *testing.T) {
	r := testRow("w1:p1", "alpha", "idle", "the message", time.Minute)
	lines := strings.Split(visible(render(t, 90, 0, r)[0]), "\n")
	if runeIndex(lines[0], "alpha") != runeIndex(lines[1], "‹") {
		t.Errorf("preview marker is not under the label:\n%q\n%q", lines[0], lines[1])
	}
}
