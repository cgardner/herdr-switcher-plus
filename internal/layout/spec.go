// Package layout describes how an agent is drawn: which tokens appear on which
// rows, and how each one is styled.
//
// The vocabulary is Herdr's own. Its agents sidebar is configured with
// `rows = [["state_icon", "workspace"], ["agent"]]`, where an entry is either a
// token name or a `{ token, fg, bold, dim, rules }` table. Copying that means
// anything learned configuring the sidebar transfers here, rather than being a
// second dialect to learn for the same job.
package layout

import (
	"fmt"
	"strings"
)

// Herdr's own limits, adopted so a layout that is legal there is legal here.
const (
	MaxRows         = 16
	MaxTokensPerRow = 16
	MaxRules        = 16
)

// Style overrides the contextual default for one token.
//
// Bold and Dim are pointers so an omitted field keeps the default while an
// explicit `false` turns the default off. Herdr states that omitted style
// fields preserve the contextual default, and a plain bool cannot express the
// difference between "unset" and "off".
type Style struct {
	Fg    string `toml:"fg"`
	Bold  *bool  `toml:"bold"`
	Dim   *bool  `toml:"dim"`
	Rules []Rule `toml:"rules"`
}

// Rule styles a token only when its value matches a condition. Rules are
// ordered and the first match wins, so a general case can follow a specific
// one.
type Rule struct {
	Equals     *string  `toml:"equals"`
	Contains   *string  `toml:"contains"`
	StartsWith *string  `toml:"starts_with"`
	Gt         *float64 `toml:"gt"`
	Lt         *float64 `toml:"lt"`

	// IgnoreCase applies to the text conditions only, matching ASCII case
	// insensitively.
	IgnoreCase bool `toml:"ignore_case"`

	Fg   string `toml:"fg"`
	Bold *bool  `toml:"bold"`
	Dim  *bool  `toml:"dim"`
}

// Token is one cell: which value to draw and how.
type Token struct {
	Name  string
	Style Style
}

// UnmarshalTOML accepts either form Herdr accepts: a bare token name, or a
// table carrying the name alongside its styling.
func (t *Token) UnmarshalTOML(v any) error {
	switch value := v.(type) {
	case string:
		t.Name = value
		return nil
	case map[string]any:
		return t.fromTable(value)
	default:
		return fmt.Errorf("a row entry must be a token name or a { token = ... } table, got %T", v)
	}
}

func (t *Token) fromTable(m map[string]any) error {
	name, ok := m["token"].(string)
	if !ok || name == "" {
		return fmt.Errorf("a token table needs a token name, for example { token = \"label\", dim = true }")
	}
	t.Name = name

	style, err := styleFrom(m)
	if err != nil {
		return fmt.Errorf("token %q: %w", name, err)
	}

	raw, ok := m["rules"]
	if ok {
		list, ok := raw.([]map[string]any)
		if !ok {
			anyList, isAny := raw.([]any)
			if !isAny {
				return fmt.Errorf("token %q: rules must be a list of tables", name)
			}
			list = make([]map[string]any, 0, len(anyList))
			for _, e := range anyList {
				table, ok := e.(map[string]any)
				if !ok {
					return fmt.Errorf("token %q: each rule must be a table", name)
				}
				list = append(list, table)
			}
		}
		if len(list) > MaxRules {
			return fmt.Errorf("token %q: %d rules exceeds the limit of %d", name, len(list), MaxRules)
		}
		for i, table := range list {
			rule, err := ruleFrom(table)
			if err != nil {
				return fmt.Errorf("token %q rule %d: %w", name, i+1, err)
			}
			style.Rules = append(style.Rules, rule)
		}
	}

	t.Style = style
	return nil
}

func styleFrom(m map[string]any) (Style, error) {
	var s Style
	if raw, ok := m["fg"]; ok {
		fg, ok := raw.(string)
		if !ok {
			return s, fmt.Errorf("fg must be a color string")
		}
		s.Fg = fg
	}
	var err error
	if s.Bold, err = optionalBool(m, "bold"); err != nil {
		return s, err
	}
	if s.Dim, err = optionalBool(m, "dim"); err != nil {
		return s, err
	}
	return s, nil
}

func ruleFrom(m map[string]any) (Rule, error) {
	var r Rule
	style, err := styleFrom(m)
	if err != nil {
		return r, err
	}
	r.Fg, r.Bold, r.Dim = style.Fg, style.Bold, style.Dim

	if raw, ok := m["ignore_case"]; ok {
		b, ok := raw.(bool)
		if !ok {
			return r, fmt.Errorf("ignore_case must be true or false")
		}
		r.IgnoreCase = b
	}
	for key, dst := range map[string]**string{
		"equals": &r.Equals, "contains": &r.Contains, "starts_with": &r.StartsWith,
	} {
		if raw, ok := m[key]; ok {
			s, ok := raw.(string)
			if !ok {
				return r, fmt.Errorf("%s must be a string", key)
			}
			*dst = &s
		}
	}
	for key, dst := range map[string]**float64{"gt": &r.Gt, "lt": &r.Lt} {
		if raw, ok := m[key]; ok {
			f, err := toFloat(raw)
			if err != nil {
				return r, fmt.Errorf("%s: %w", key, err)
			}
			*dst = &f
		}
	}
	if !r.hasCondition() {
		return r, fmt.Errorf("needs one of equals, contains, starts_with, gt or lt")
	}
	return r, nil
}

func (r Rule) hasCondition() bool {
	return r.Equals != nil || r.Contains != nil || r.StartsWith != nil || r.Gt != nil || r.Lt != nil
}

func optionalBool(m map[string]any, key string) (*bool, error) {
	raw, ok := m[key]
	if !ok {
		return nil, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return nil, fmt.Errorf("%s must be true or false", key)
	}
	return &b, nil
}

func toFloat(v any) (float64, error) {
	switch n := v.(type) {
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	default:
		return 0, fmt.Errorf("must be a number, got %T", v)
	}
}

// Spec is a whole layout: the rows of tokens, and the blank lines between
// agents.
type Spec struct {
	Rows   [][]Token `toml:"rows"`
	RowGap int       `toml:"row_gap"`
}

// Validate reports the first problem with a spec, naming the row and token so
// the message points at the line to fix.
func (s Spec) Validate(known func(string) bool) error {
	if len(s.Rows) == 0 {
		return fmt.Errorf("rows must list at least one row of tokens")
	}
	if len(s.Rows) > MaxRows {
		return fmt.Errorf("%d rows exceeds the limit of %d", len(s.Rows), MaxRows)
	}
	if s.RowGap < 0 {
		return fmt.Errorf("row_gap must not be negative")
	}
	for i, row := range s.Rows {
		if len(row) == 0 {
			return fmt.Errorf("row %d is empty", i+1)
		}
		if len(row) > MaxTokensPerRow {
			return fmt.Errorf("row %d has %d tokens, which exceeds the limit of %d", i+1, len(row), MaxTokensPerRow)
		}
		for _, tok := range row {
			if !known(tok.Name) {
				return fmt.Errorf("row %d: unknown token %q", i+1, tok.Name)
			}
			if len(tok.Style.Rules) > MaxRules {
				return fmt.Errorf("row %d: token %q has %d rules, which exceeds the limit of %d",
					i+1, tok.Name, len(tok.Style.Rules), MaxRules)
			}
		}
	}
	return nil
}

// Matches reports whether a rule fires for a token's value. Text conditions
// read the rendered text and numeric ones the token's numeric value, which is
// absent for most tokens.
func (r Rule) Matches(text string, number *float64) bool {
	fold := func(s string) string {
		if r.IgnoreCase {
			return strings.ToLower(s)
		}
		return s
	}
	switch {
	case r.Equals != nil:
		return fold(text) == fold(*r.Equals)
	case r.Contains != nil:
		return strings.Contains(fold(text), fold(*r.Contains))
	case r.StartsWith != nil:
		return strings.HasPrefix(fold(text), fold(*r.StartsWith))
	case r.Gt != nil:
		return number != nil && *number > *r.Gt
	case r.Lt != nil:
		return number != nil && *number < *r.Lt
	}
	return false
}

// Resolve folds the token's style and the first matching rule into one set of
// overrides. An unset field means "keep the contextual default", which is what
// lets a config restyle one token without restating the whole theme.
func (s Style) Resolve(text string, number *float64) Style {
	out := Style{Fg: s.Fg, Bold: s.Bold, Dim: s.Dim}
	for _, rule := range s.Rules {
		if !rule.Matches(text, number) {
			continue
		}
		if rule.Fg != "" {
			out.Fg = rule.Fg
		}
		if rule.Bold != nil {
			out.Bold = rule.Bold
		}
		if rule.Dim != nil {
			out.Dim = rule.Dim
		}
		break
	}
	return out
}
