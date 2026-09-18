package layout

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func parse(t *testing.T, body string) (Spec, error) {
	t.Helper()
	var wrapper struct {
		Spec
	}
	err := toml.Unmarshal([]byte(body), &wrapper)
	return wrapper.Spec, err
}

func mustParse(t *testing.T, body string) Spec {
	t.Helper()
	s, err := parse(t, body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return s
}

func known(name string) bool {
	switch name {
	case "age", "label", "state_icon", "state_text", "message", "role":
		return true
	}
	return false
}

// A row entry may be a bare name or a styling table, which is the shape
// Herdr's own sidebar config uses.
func TestBothTokenFormsParse(t *testing.T) {
	s := mustParse(t, `rows = [["age", { token = "label", fg = "#89b4fa", bold = true }]]`)
	if len(s.Rows) != 1 || len(s.Rows[0]) != 2 {
		t.Fatalf("got %+v", s.Rows)
	}
	if s.Rows[0][0].Name != "age" || s.Rows[0][0].Style.Fg != "" {
		t.Errorf("bare token wrong: %+v", s.Rows[0][0])
	}
	label := s.Rows[0][1]
	if label.Name != "label" || label.Style.Fg != "#89b4fa" {
		t.Errorf("table token wrong: %+v", label)
	}
	if label.Style.Bold == nil || !*label.Style.Bold {
		t.Errorf("bold not parsed: %+v", label.Style)
	}
}

// An omitted bold or dim keeps the contextual default, while an explicit false
// turns it off. A plain bool could not tell those apart.
func TestOmittedAndExplicitBoolsDiffer(t *testing.T) {
	s := mustParse(t, `rows = [[{ token = "message", dim = false }, { token = "age" }]]`)
	if d := s.Rows[0][0].Style.Dim; d == nil || *d {
		t.Errorf("explicit false should be recorded, got %v", d)
	}
	if d := s.Rows[0][1].Style.Dim; d != nil {
		t.Errorf("omitted dim should stay unset, got %v", *d)
	}
}

func TestRulesParse(t *testing.T) {
	s := mustParse(t, `
rows = [[{ token = "state_text", rules = [
  { equals = "blocked", fg = "#ff6188", bold = true },
  { contains = "work", fg = "#f9e2af" },
] }]]`)
	rules := s.Rows[0][0].Style.Rules
	if len(rules) != 2 {
		t.Fatalf("got %d rules", len(rules))
	}
	if rules[0].Equals == nil || *rules[0].Equals != "blocked" || rules[0].Fg != "#ff6188" {
		t.Errorf("first rule wrong: %+v", rules[0])
	}
	if rules[1].Contains == nil || *rules[1].Contains != "work" {
		t.Errorf("second rule wrong: %+v", rules[1])
	}
}

func TestNumericRulesParse(t *testing.T) {
	s := mustParse(t, `rows = [[{ token = "age", rules = [{ gt = 1440, dim = true }, { lt = 60.5, fg = "#a6e3a1" }] }]]`)
	rules := s.Rows[0][0].Style.Rules
	if rules[0].Gt == nil || *rules[0].Gt != 1440 {
		t.Errorf("gt wrong: %+v", rules[0])
	}
	if rules[1].Lt == nil || *rules[1].Lt != 60.5 {
		t.Errorf("lt wrong: %+v", rules[1])
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"a number is not a token":            `rows = [[42]]`,
		"a table needs a name":               `rows = [[{ fg = "#fff" }]]`,
		"a rule needs a condition":           `rows = [[{ token = "age", rules = [{ fg = "#fff" }] }]]`,
		"fg must be a string":                `rows = [[{ token = "age", fg = 3 }]]`,
		"bold must be a boolean":             `rows = [[{ token = "age", bold = "yes" }]]`,
		"a numeric condition wants a number": `rows = [[{ token = "age", rules = [{ gt = "soon" }] }]]`,
	}
	for name, body := range cases {
		if _, err := parse(t, body); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// The error has to name the token, or a long config gives no clue where to
// look.
func TestParseErrorNamesTheToken(t *testing.T) {
	_, err := parse(t, `rows = [[{ token = "state_text", rules = [{ fg = "#fff" }] }]]`)
	if err == nil || !strings.Contains(err.Error(), "state_text") {
		t.Errorf("error should name the token, got %v", err)
	}
}

func TestValidateRejectsUnknownTokens(t *testing.T) {
	s := mustParse(t, `rows = [["age", "nonsense"]]`)
	err := s.Validate(known)
	if err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("got %v", err)
	}
	if !strings.Contains(err.Error(), "row 1") {
		t.Errorf("error should name the row, got %v", err)
	}
}

func TestValidateEnforcesHerdrsLimits(t *testing.T) {
	tooManyRows := Spec{Rows: make([][]Token, MaxRows+1)}
	for i := range tooManyRows.Rows {
		tooManyRows.Rows[i] = []Token{{Name: "age"}}
	}
	if err := tooManyRows.Validate(known); err == nil || !strings.Contains(err.Error(), "rows") {
		t.Errorf("row limit not enforced: %v", err)
	}

	wide := make([]Token, MaxTokensPerRow+1)
	for i := range wide {
		wide[i] = Token{Name: "age"}
	}
	if err := (Spec{Rows: [][]Token{wide}}).Validate(known); err == nil {
		t.Error("token limit not enforced")
	}
}

func TestValidateRejectsEmptyAndNegative(t *testing.T) {
	if err := (Spec{}).Validate(known); err == nil {
		t.Error("an empty spec should be rejected")
	}
	if err := (Spec{Rows: [][]Token{{}}}).Validate(known); err == nil {
		t.Error("an empty row should be rejected")
	}
	if err := (Spec{Rows: [][]Token{{{Name: "age"}}}, RowGap: -1}).Validate(known); err == nil {
		t.Error("a negative row_gap should be rejected")
	}
}

func TestValidateAcceptsAGoodSpec(t *testing.T) {
	s := mustParse(t, `rows = [["age", "label"], ["message"]]`)
	if err := s.Validate(known); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func str(s string) *string { return &s }
func f(v float64) *float64 { return &v }

func TestRuleMatching(t *testing.T) {
	cases := []struct {
		name   string
		rule   Rule
		text   string
		number *float64
		want   bool
	}{
		{"equals hit", Rule{Equals: str("idle")}, "idle", nil, true},
		{"equals miss", Rule{Equals: str("idle")}, "working", nil, false},
		{"equals is case sensitive", Rule{Equals: str("Idle")}, "idle", nil, false},
		{"ignore_case", Rule{Equals: str("Idle"), IgnoreCase: true}, "idle", nil, true},
		{"contains", Rule{Contains: str("ork")}, "working", nil, true},
		{"starts_with hit", Rule{StartsWith: str("Second")}, "platform", nil, true},
		{"starts_with miss", Rule{StartsWith: str("Brain")}, "platform", nil, false},
		{"gt hit", Rule{Gt: f(1440)}, "3d", f(4320), true},
		{"gt miss", Rule{Gt: f(1440)}, "2h", f(120), false},
		{"lt hit", Rule{Lt: f(60)}, "5m", f(5), true},
		{"a numeric rule never fires without a number", Rule{Gt: f(1)}, "label", nil, false},
		{"no condition never fires", Rule{}, "anything", f(1), false},
	}
	for _, c := range cases {
		if got := c.rule.Matches(c.text, c.number); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Rules are ordered and the first match wins, so a specific case can precede a
// general one.
func TestResolveTakesTheFirstMatchingRule(t *testing.T) {
	s := Style{Fg: "#base", Rules: []Rule{
		{Equals: str("blocked"), Fg: "#red"},
		{Contains: str("l"), Fg: "#never"},
	}}
	if got := s.Resolve("blocked", nil).Fg; got != "#red" {
		t.Errorf("got %q, want #red", got)
	}
}

func TestResolveKeepsTheTokenStyleWhenNoRuleFires(t *testing.T) {
	yes := true
	s := Style{Fg: "#base", Bold: &yes, Rules: []Rule{{Equals: str("other"), Fg: "#red"}}}
	got := s.Resolve("idle", nil)
	if got.Fg != "#base" || got.Bold == nil || !*got.Bold {
		t.Errorf("got %+v", got)
	}
}

// A rule overrides only the fields it names, leaving the rest of the token
// style intact.
func TestResolveOverridesOnlyNamedFields(t *testing.T) {
	yes := true
	s := Style{Fg: "#base", Bold: &yes, Rules: []Rule{{Equals: str("idle"), Fg: "#dim"}}}
	got := s.Resolve("idle", nil)
	if got.Fg != "#dim" {
		t.Errorf("fg = %q, want #dim", got.Fg)
	}
	if got.Bold == nil || !*got.Bold {
		t.Error("bold should survive a rule that does not mention it")
	}
}

func TestMoreParseErrors(t *testing.T) {
	cases := map[string]string{
		"rules must be a list":         `rows = [[{ token = "age", rules = "nope" }]]`,
		"each rule must be a table":    `rows = [[{ token = "age", rules = ["nope"] }]]`,
		"dim must be a boolean":        `rows = [[{ token = "age", dim = "yes" }]]`,
		"a rule's dim must be boolean": `rows = [[{ token = "age", rules = [{ equals = "x", dim = 1 }] }]]`,
		"ignore_case must be boolean":  `rows = [[{ token = "age", rules = [{ equals = "x", ignore_case = "yes" }] }]]`,
		"equals must be a string":      `rows = [[{ token = "age", rules = [{ equals = 3 }] }]]`,
	}
	for name, body := range cases {
		if _, err := parse(t, body); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestIgnoreCaseParses(t *testing.T) {
	s := mustParse(t, `rows = [[{ token = "state_text", rules = [{ equals = "IDLE", ignore_case = true, fg = "#fff" }] }]]`)
	if r := s.Rows[0][0].Style.Rules[0]; !r.IgnoreCase {
		t.Errorf("ignore_case not parsed: %+v", r)
	}
}

// Herdr caps rules at 16, and the cap is enforced while parsing as well as in
// Validate, because a spec built in code never passes through the parser.
func TestTooManyRulesAreRejectedWhileParsing(t *testing.T) {
	var b strings.Builder
	b.WriteString(`rows = [[{ token = "age", rules = [`)
	for i := 0; i <= MaxRules; i++ {
		b.WriteString(`{ equals = "x", fg = "#fff" },`)
	}
	b.WriteString(`] }]]`)
	if _, err := parse(t, b.String()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("got %v", err)
	}
}

func TestValidateRejectsTooManyRules(t *testing.T) {
	equals := "x"
	rules := make([]Rule, MaxRules+1)
	for i := range rules {
		rules[i] = Rule{Equals: &equals}
	}
	spec := Spec{Rows: [][]Token{{{Name: "age", Style: Style{Rules: rules}}}}}
	if err := spec.Validate(known); err == nil || !strings.Contains(err.Error(), "rules") {
		t.Errorf("got %v", err)
	}
}

// A rule may switch bold or dim without naming a colour.
func TestResolveAppliesBoldAndDimFromARule(t *testing.T) {
	yes, no := true, false
	s := Style{Rules: []Rule{{Equals: str("idle"), Bold: &yes, Dim: &no}}}
	got := s.Resolve("idle", nil)
	if got.Bold == nil || !*got.Bold {
		t.Error("bold from the rule was not applied")
	}
	if got.Dim == nil || *got.Dim {
		t.Error("dim from the rule was not applied")
	}
}
