package catalog

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// Config is jitso/jitso.yaml: the allowlists every build is checked against,
// plus what the project holds, which an open list is checked against.
//
// A list is open when it is nil: its key is absent, null or [] (and every list
// is open when the file is missing). An open list restricts nothing, but what
// a build names must still exist. Any other list restricts to what it holds.
type Config struct {
	// Version is Number(version): NaN when it is missing or not a number.
	Version float64
	Models  []string
	Efforts []string
	MCP     []string
	Skills  []string
	Tools   []string
	// Fields are the patterns of the fields a build may require, in the order
	// of the fields key. Without the key there are none.
	Fields []FieldRule
	// User is the user allowlist that user:<name> items resolve in.
	User User
	// ProjectSkillDirs are the names of the skill folders the project holds
	// under .agents/skills and .claude/skills, sorted: what an open skills
	// list allows, and names a user skill may not take.
	ProjectSkillDirs []string
	// ProjectServers is the mcpServers table of the project's .mcp.json: what
	// an open mcp list allows.
	ProjectServers map[string]map[string]any
}

// Open reports whether the allowlist restricts nothing.
func Open(list []string) bool { return list == nil }

// allowsValue reports whether a model, effort or tool name is allowed: one of
// a restricting list, or any usable text when the list is open.
func allowsValue(list []string, v string) bool {
	if Open(list) {
		return usableText(v)
	}
	return slices.Contains(list, v)
}

// usableText is a text an open list accepts: not blank and with no control
// character.
func usableText(v string) bool {
	return strings.TrimSpace(v) != "" && !HasControl(v)
}

// AllowsModel reports whether a build may name the model.
func (c Config) AllowsModel(v string) bool { return allowsValue(c.Models, v) }

// AllowsEffort reports whether a build may name the effort.
func (c Config) AllowsEffort(v string) bool { return allowsValue(c.Efforts, v) }

// AllowsMCP reports whether a build may name the project server: one of the
// mcp list, or, when it is open, one .mcp.json defines.
func (c Config) AllowsMCP(name string) bool {
	if Open(c.MCP) {
		_, ok := c.ProjectServers[name]
		return ok
	}
	return slices.Contains(c.MCP, name)
}

// OwnsSkill reports whether name is a skill of the project, so that no user
// skill may take it: one the skills list names or a skill folder it holds.
func (c Config) OwnsSkill(name string) bool {
	return slices.Contains(c.Skills, name) || slices.Contains(c.ProjectSkillDirs, name)
}

// OwnsMCP reports whether name is a server of the project, so that no user
// server may take it: one of the mcp list, or, when it is open, one .mcp.json
// defines.
func (c Config) OwnsMCP(name string) bool { return c.AllowsMCP(name) }

// FieldRule is one entry of the fields key: a field name and the pattern its
// value must match, compiled to match the whole value.
type FieldRule struct {
	Name    string
	Pattern *regexp.Regexp
	// Source is the pattern as the fields key writes it.
	Source string
}

// FieldPattern is the pattern of the named field, and false for a field with
// no entry in the fields key. A field with no pattern gets only ValidFieldValue.
func (c Config) FieldPattern(name string) (*regexp.Regexp, bool) {
	for _, f := range c.Fields {
		if f.Name == name {
			return f.Pattern, true
		}
	}
	return nil, false
}

// ValidFieldValue is the value check of a field with no pattern: not empty and
// no control character (U+0000 to U+001F, U+007F).
func ValidFieldValue(v string) bool {
	return v != "" && !HasControl(v)
}

// HasControl reports whether v holds a control character (U+0000 to U+001F,
// U+007F). No field value may: a launch writes each value on one line of
// brief.md, so a value with a line break could start a heading or a list
// there.
func HasControl(v string) bool {
	for _, r := range v {
		if unicode.IsControl(r) && (r < 0x20 || r == 0x7f) {
			return true
		}
	}
	return false
}

// allowlist reads one list key. Absent, null and [] are open (nil). A list
// with items is read through String(). Any other value restricts to nothing,
// so a mistake never lifts a restriction.
func allowlist(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		if len(x) == 0 {
			return nil
		}
		return jsconv.Strings(x)
	}
	if v == jsconv.Undefined {
		return nil
	}
	return []string{}
}

// configFrom reads the allowlists from a decoded jitso.yaml the way
// JavaScript's String() reads them (see allowlist). A top level that is not a
// mapping is an error. The second result lists every problem of the fields
// key, in key order.
func configFrom(raw any) (Config, []string, bool) {
	m, ok := raw.(*Map)
	if !ok {
		return Config{}, nil, false
	}
	fields, problems := fieldsFrom(m)
	return Config{
		Version: jsNumber(m.Value("version")),
		Models:  allowlist(m.Value("models")),
		Efforts: allowlist(m.Value("efforts")),
		MCP:     allowlist(m.Value("mcp")),
		Skills:  allowlist(m.Value("skills")),
		Tools:   allowlist(m.Value("tools")),
		Fields:  fields,
	}, problems, true
}

// fieldsFrom reads the optional fields key: a mapping of field name to a Go
// regexp that must match the whole value. An absent or null key is no rules.
func fieldsFrom(m *Map) ([]FieldRule, []string) {
	v, present := m.Get("fields")
	if !present || v == nil {
		return nil, nil
	}
	fm, ok := v.(*Map)
	if !ok {
		return nil, []string{"fields must be a mapping of field name to pattern"}
	}
	var rules []FieldRule
	var problems []string
	for _, name := range fm.Keys() {
		if !FieldName.MatchString(name) {
			problems = append(problems, `field name "`+name+`" must be lowercase letters and _`)
			continue
		}
		text, isString := fm.Value(name).(string)
		if !isString || text == "" {
			problems = append(problems, `field "`+name+`" pattern must be a non-empty string`)
			continue
		}
		if _, err := regexp.Compile(text); err != nil {
			problems = append(problems, `field "`+name+`" pattern does not compile: `+err.Error())
			continue
		}
		rules = append(rules, FieldRule{Name: name, Pattern: regexp.MustCompile(`^(?:` + text + `)$`), Source: text})
	}
	return rules, problems
}

// jsNumber is a close port of JavaScript Number(v) for decoded YAML values.
func jsNumber(v any) float64 {
	switch x := v.(type) {
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	case float64:
		return x
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return math.NaN()
	default:
		return math.NaN()
	}
}
