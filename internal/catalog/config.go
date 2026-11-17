package catalog

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// Config is jitso/jitso.yaml: the allowlists every build and every request
// override is checked against.
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
}

// FieldRule is one entry of the fields key: a field name and the pattern its
// value must match, compiled to match the whole value.
type FieldRule struct {
	Name    string
	Pattern *regexp.Regexp
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
	if v == "" {
		return false
	}
	for _, r := range v {
		if unicode.IsControl(r) && (r < 0x20 || r == 0x7f) {
			return false
		}
	}
	return true
}

// configFrom reads the allowlists from a decoded jitso.yaml the way the
// oracle's loadConfig does: each list through String(), anything that is not
// a list is an empty list. A top level that is not a mapping is an error (the
// oracle crashes on it). The second result lists every problem of the fields
// key, in key order.
func configFrom(raw any) (Config, []string, bool) {
	m, ok := raw.(*Map)
	if !ok {
		return Config{}, nil, false
	}
	fields, problems := fieldsFrom(m)
	return Config{
		Version: jsNumber(m.Value("version")),
		Models:  jsconv.Strings(m.Value("models")),
		Efforts: jsconv.Strings(m.Value("efforts")),
		MCP:     jsconv.Strings(m.Value("mcp")),
		Skills:  jsconv.Strings(m.Value("skills")),
		Tools:   jsconv.Strings(m.Value("tools")),
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
		rules = append(rules, FieldRule{Name: name, Pattern: regexp.MustCompile(`^(?:` + text + `)$`)})
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
