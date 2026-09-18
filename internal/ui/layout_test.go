package ui

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cgardner/herdr-switcher-plus/internal/agents"
	"github.com/cgardner/herdr-switcher-plus/internal/layout"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
)

func renderSpec(t *testing.T, spec layout.Spec, width int, rows ...agents.Row) []string {
	t.Helper()
	d := delegate{spec: spec, selectionBg: lipgloss.Color("#313244")}
	l := list.New(toItems(rows, spec), d, width, 20)
	l.Select(0)
	out := make([]string, len(rows))
	for i := range rows {
		var buf bytes.Buffer
		d.Render(&buf, l, i, l.Items()[i])
		out[i] = buf.String()
	}
	return out
}

func tok(name string) layout.Token { return layout.Token{Name: name} }

// The built-in look is written in the same vocabulary a user would write, so
// the default is not a special case the configuration cannot reproduce.
func TestDefaultSpecUsesOnlyKnownTokens(t *testing.T) {
	if err := DefaultSpec.Validate(KnownToken); err != nil {
		t.Fatalf("the default layout must be a valid config: %v", err)
	}
}

func TestEveryRegisteredTokenRenders(t *testing.T) {
	r := testRow("w5:p1E", "software-factory", "blocked", "a message", time.Hour)
	r.Repo, r.Branch, r.Linked = "Second Brain", "sf", true
	r.Agent.Kind, r.Agent.Cwd = "claude", "/tmp/x"

	for _, name := range TokenNames() {
		spec := layout.Spec{Rows: [][]layout.Token{{tok(name)}}}
		got := visible(renderSpec(t, spec, 60, r)[0])
		if strings.TrimSpace(got) == "" && name != "role" {
			t.Errorf("token %q rendered nothing", name)
		}
	}
}

func TestACustomSpecChangesTheRows(t *testing.T) {
	spec := layout.Spec{Rows: [][]layout.Token{{tok("state_icon"), tok("pane"), tok("agent")}}}
	got := visible(renderSpec(t, spec, 60, testRow("w9:p3", "alpha", "idle", "m", time.Minute))[0])
	if !strings.Contains(got, "w9:p3") {
		t.Errorf("pane token missing: %q", got)
	}
	if strings.Contains(got, "alpha") {
		t.Errorf("a token absent from the spec must not render: %q", got)
	}
	if n := strings.Count(got, "\n"); n != 0 {
		t.Errorf("a one-row spec should draw one line, got %d extra", n)
	}
}

func TestHeightFollowsTheSpec(t *testing.T) {
	three := layout.Spec{Rows: [][]layout.Token{{tok("age")}, {tok("label")}, {tok("message")}}}
	if got := (delegate{spec: three}).Height(); got != 3 {
		t.Errorf("Height = %d, want 3", got)
	}
	gapped := layout.Spec{Rows: [][]layout.Token{{tok("age")}}, RowGap: 2}
	if got := (delegate{spec: gapped}).Height(); got != 3 {
		t.Errorf("Height with a gap = %d, want 3", got)
	}
}

// The gap belongs to Height rather than Spacing, so the selection background
// covers the blank lines of the selected agent instead of breaking across them.
func TestRowGapDrawsBackgroundedBlankLines(t *testing.T) {
	spec := layout.Spec{Rows: [][]layout.Token{{tok("label")}}, RowGap: 1}
	lines := strings.Split(renderSpec(t, spec, 40, testRow("w1:p1", "alpha", "idle", "m", time.Minute))[0], "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !strings.Contains(lines[1], bgSequence) {
		t.Errorf("the gap line should carry the selection background: %q", lines[1])
	}
}

// A configured colour overrides the contextual default.
func TestAConfiguredForegroundApplies(t *testing.T) {
	spec := layout.Spec{Rows: [][]layout.Token{{{Name: "label", Style: layout.Style{Fg: "#ff00ff"}}}}}
	got := renderSpec(t, spec, 40, testRow("w1:p1", "alpha", "idle", "m", time.Minute))[0]
	if !strings.Contains(got, "255;0;255") {
		t.Errorf("configured colour not applied: %q", got)
	}
}

// A rule fires on the token's value, which is what makes state-dependent
// colouring possible without code changes.
func TestARuleAppliesOnMatch(t *testing.T) {
	equals := "blocked"
	spec := layout.Spec{Rows: [][]layout.Token{{{
		Name:  "state_text",
		Style: layout.Style{Rules: []layout.Rule{{Equals: &equals, Fg: "#ff00ff"}}},
	}}}}

	hit := renderSpec(t, spec, 40, testRow("w1:p1", "a", "blocked", "m", time.Minute))[0]
	if !strings.Contains(hit, "255;0;255") {
		t.Errorf("rule did not fire: %q", hit)
	}
	miss := renderSpec(t, spec, 40, testRow("w1:p1", "a", "idle", "m", time.Minute))[0]
	if strings.Contains(miss, "255;0;255") {
		t.Errorf("rule fired on a non-matching value: %q", miss)
	}
}

// The age token carries minutes, so a numeric rule can say "older than a day".
func TestANumericRuleReadsTheAgeInMinutes(t *testing.T) {
	gt := 1440.0
	spec := layout.Spec{Rows: [][]layout.Token{{{
		Name:  "age",
		Style: layout.Style{Rules: []layout.Rule{{Gt: &gt, Fg: "#ff00ff"}}},
	}}}}

	old := renderSpec(t, spec, 40, testRow("w1:p1", "a", "idle", "m", 48*time.Hour))[0]
	if !strings.Contains(old, "255;0;255") {
		t.Errorf("gt rule did not fire on a two-day age: %q", old)
	}
	fresh := renderSpec(t, spec, 40, testRow("w1:p1", "a", "idle", "m", time.Hour))[0]
	if strings.Contains(fresh, "255;0;255") {
		t.Errorf("gt rule fired on a one-hour age: %q", fresh)
	}
}

// Continuation rows hang under the first row's second column, so a message
// sits below the name it belongs to rather than under the age.
func TestContinuationRowsHangUnderTheSecondColumn(t *testing.T) {
	r := testRow("w1:p1", "alpha", "idle", "the message", time.Minute)
	lines := strings.Split(visible(renderSpec(t, DefaultSpec, 80, r)[0]), "\n")
	if runeIndex(lines[0], "alpha") != runeIndex(lines[1], "‹") {
		t.Errorf("continuation row is not aligned:\n%q\n%q", lines[0], lines[1])
	}
}

// An unknown token cannot reach the renderer through the config, which
// validates first, but the renderer must not panic if one ever does.
func TestAnUnknownTokenRendersNothing(t *testing.T) {
	spec := layout.Spec{Rows: [][]layout.Token{{tok("nonsense"), tok("label")}}}
	got := visible(renderSpec(t, spec, 40, testRow("w1:p1", "alpha", "idle", "m", time.Minute))[0])
	if !strings.Contains(got, "alpha") {
		t.Errorf("the rest of the row should still draw: %q", got)
	}
}

func TestNewFallsBackToTheDefaultSpec(t *testing.T) {
	m := sized(New(fixture(), agents.ModeRecent, agents.StatusAll, layout.Spec{}), 100, 24)
	if len(m.spec.Rows) != len(DefaultSpec.Rows) {
		t.Errorf("an empty spec should fall back to the default, got %+v", m.spec)
	}
}

// Age reads as a gradient from live to abandoned.
func TestAgeColourBands(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"minutes", 5 * time.Minute, freshColor},
		{"just under a day", 23 * time.Hour, freshColor},
		{"just over a day", 25 * time.Hour, recentColor},
		{"mid week", 3 * 24 * time.Hour, recentColor},
		{"just over a week", 8 * 24 * time.Hour, staleColor},
		{"months", 90 * 24 * time.Hour, staleColor},
	}
	for _, c := range cases {
		r := testRow("p", "s", "idle", "m", c.age)
		if got := ageColorFor(r, now, false); got != c.want {
			t.Errorf("%s: colour = %q, want %q", c.name, got, c.want)
		}
	}
}

// An unresolved age is treated as the oldest: not knowing when a session last
// spoke is not evidence that it spoke recently.
func TestAnUnknownAgeIsTreatedAsStale(t *testing.T) {
	if got := ageColorFor(agents.Row{}, time.Now(), false); got != staleColor {
		t.Errorf("colour = %q, want the stale band", got)
	}
}

// hasSGR reports whether an SGR attribute is set anywhere in the rendered
// text.
//
// Two things make a substring search wrong. Attributes ride alongside colours
// as "\x1b[1;97;48;2;48;50;68m", so looking for "\x1b[1m" misses them. And an
// extended colour spells itself "48;2;R;G;B", where the 2 is the truecolor
// marker rather than the faint attribute, so its arguments have to be skipped
// or every backgrounded cell looks faint.
func hasSGR(rendered, attr string) bool {
	for _, seq := range sgrPattern.FindAllStringSubmatch(rendered, -1) {
		params := strings.Split(seq[1], ";")
		for i := 0; i < len(params); i++ {
			switch params[i] {
			case "38", "48", "58": // foreground, background, underline colour
				if i+1 < len(params) && params[i+1] == "2" {
					i += 4 // 2;R;G;B
				} else if i+1 < len(params) && params[i+1] == "5" {
					i += 2 // 5;N
				}
				continue
			}
			if params[i] == attr {
				return true
			}
		}
	}
	return false
}

var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// A configured dim or bold reaches the rendered cell.
func TestConfiguredDimAndBoldApply(t *testing.T) {
	yes, no := true, false
	r := testRow("w1:p1", "alpha", "idle", "m", time.Minute)

	bold := layout.Spec{Rows: [][]layout.Token{{{Name: "label", Style: layout.Style{Bold: &yes}}}}}
	if got := renderSpec(t, bold, 40, r)[0]; !hasSGR(got, "1") {
		t.Errorf("bold not applied: %q", got)
	}

	// The message is dim by default, so switching it off proves an explicit
	// false overrides the contextual default rather than being ignored.
	plain := layout.Spec{Rows: [][]layout.Token{{{Name: "message"}}}}
	if got := renderSpec(t, plain, 40, r)[0]; !hasSGR(got, "2") {
		t.Fatalf("precondition: the message should be faint by default: %q", got)
	}
	undim := layout.Spec{Rows: [][]layout.Token{{{Name: "message", Style: layout.Style{Dim: &no}}}}}
	if got := renderSpec(t, undim, 40, r)[0]; hasSGR(got, "2") {
		t.Errorf("dim = false should have removed the faint attribute: %q", got)
	}
}

func TestWidthAtOutsideTheSpecIsZero(t *testing.T) {
	it := item{widths: [][]int{{3, 5}}}
	for _, c := range [][2]int{{0, 0}, {0, 1}} {
		if it.widthAt(c[0], c[1]) == 0 {
			t.Errorf("widthAt(%d,%d) should be a real width", c[0], c[1])
		}
	}
	for _, c := range [][2]int{{1, 0}, {0, 9}, {5, 5}} {
		if got := it.widthAt(c[0], c[1]); got != 0 {
			t.Errorf("widthAt(%d,%d) = %d, want 0", c[0], c[1], got)
		}
	}
}

// A path under the home directory is written with a tilde, the way a shell
// prompt does, so it fits a column.
func TestCwdTokenShortensTheHomeDirectory(t *testing.T) {
	if home == "" {
		t.Skip("no home directory resolved in this environment")
	}
	r := testRow("w1:p1", "alpha", "idle", "m", time.Minute)
	r.Agent.Cwd = home + "/src/thing"
	spec := layout.Spec{Rows: [][]layout.Token{{tok("cwd")}}}
	got := visible(renderSpec(t, spec, 60, r)[0])
	if !strings.Contains(got, "~/src/thing") {
		t.Errorf("got %q, want a tilde path", got)
	}

	r.Agent.Cwd = "/elsewhere/thing"
	got = visible(renderSpec(t, spec, 60, r)[0])
	if !strings.Contains(got, "/elsewhere/thing") {
		t.Errorf("a path outside home should be untouched: %q", got)
	}
}

// Without a home directory there is nothing to shorten, and a cwd renders
// whole rather than failing.
func TestUserHomeWithoutAHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	if got := userHome(); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
