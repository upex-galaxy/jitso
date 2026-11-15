package expect

import (
	"fmt"
	"regexp"

	"jitso-contract-runner/internal/jsonx"
)

// TextMatcher is one entry of a text matcher list (SPEC 7.1).
type TextMatcher interface {
	Name() string
	// Match returns nil when the text satisfies the matcher.
	Match(text string) *Mismatch
}

// Mismatch explains a failed text matcher. Actual, when set, replaces the whole text in the report.
type Mismatch struct {
	Expected string
	Actual   string
}

var textMatchers = map[string]func(v any, what string) (TextMatcher, error){}

// registerText adds a text matcher kind, keyed by its JSON name.
func registerText(name string, build func(v any, what string) (TextMatcher, error)) {
	textMatchers[name] = build
}

// ParseTextMatchers reads a list of text matchers. A plain string is `{"contains": s}`.
func ParseTextMatchers(v any, what string) ([]TextMatcher, error) {
	list, err := jsonx.List(v, what)
	if err != nil {
		return nil, err
	}
	out := make([]TextMatcher, len(list))
	for i, item := range list {
		m, err := ParseTextMatcher(item, fmt.Sprintf("%s[%d]", what, i))
		if err != nil {
			return nil, err
		}
		out[i] = m
	}
	return out, nil
}

// ParseTextMatcher reads one matcher: a string, or an object with exactly one key.
func ParseTextMatcher(v any, what string) (TextMatcher, error) {
	if s, ok := v.(string); ok {
		return textMatchers["contains"](s, what)
	}
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return nil, err
	}
	if len(obj) != 1 {
		return nil, fmt.Errorf("%s: a text matcher has exactly one key, got %d (kinds: %v)", what, len(obj), jsonx.SortedKeys(textMatchers))
	}
	for key, arg := range obj {
		build, ok := textMatchers[key]
		if !ok {
			return nil, fmt.Errorf("%s: unknown text matcher %q (kinds: %v)", what, key, jsonx.SortedKeys(textMatchers))
		}
		return build(arg, what+"."+key)
	}
	return nil, nil
}

// CompileRegex compiles an RE2 pattern, naming the place in the case on error.
func CompileRegex(pattern, what string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%s: bad regex %q: %v", what, pattern, err)
	}
	return re, nil
}
