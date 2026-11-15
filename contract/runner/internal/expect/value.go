package expect

import (
	"fmt"
	"regexp"

	"jitso-contract-runner/internal/jsonx"
)

// ValueMatcher tests one JSON value found at a path: equal to an expected value, matching a
// regex (against its string form), or absent.
type ValueMatcher struct {
	mode string // "equals", "regex", "absent", "keys_exact" or "keys_include"
	want any
	re   *regexp.Regexp
	keys []string
}

// Test reports whether the value (present or not) satisfies the matcher.
func (m ValueMatcher) Test(v any, present bool) (ok bool, expected, actual string) {
	actual = "(absent)"
	if present {
		actual = jsonx.Compact(v)
	}
	switch m.mode {
	case "absent":
		return !present, "absent", actual
	case "regex":
		return present && m.re.MatchString(jsonx.StringForm(v)), fmt.Sprintf("regex /%s/", m.re), actual
	case "keys_exact", "keys_include":
		expected = fmt.Sprintf("an object whose keys %s %s", map[string]string{"keys_exact": "are exactly", "keys_include": "include"}[m.mode], jsonx.Compact(m.keys))
		obj, isObj := v.(map[string]any)
		if !present || !isObj {
			return false, expected, actual
		}
		got := jsonx.SortedKeys(obj)
		actual = "keys " + jsonx.Compact(got)
		for _, k := range m.keys {
			if _, ok := obj[k]; !ok {
				return false, expected, actual
			}
		}
		return m.mode == "keys_include" || len(got) == len(m.keys), expected, actual
	}
	return present && jsonx.Equal(v, m.want), jsonx.Compact(m.want), actual
}

// ParseFieldValue reads the registry/env form: an object with the single key `regex` or
// `absent` is a matcher; any other value is the expected value itself.
func ParseFieldValue(v any, what string) (ValueMatcher, error) {
	if obj, ok := v.(map[string]any); ok && len(obj) == 1 {
		if _, ok := obj["regex"]; ok {
			return parseMatcherKey(obj, "regex", what)
		}
		if _, ok := obj["absent"]; ok {
			return parseMatcherKey(obj, "absent", what)
		}
	}
	return ValueMatcher{mode: "equals", want: v}, nil
}

func parseMatcherKey(obj map[string]any, key, what string) (ValueMatcher, error) {
	switch key {
	case "regex":
		s, err := jsonx.String(obj[key], what+".regex")
		if err != nil {
			return ValueMatcher{}, err
		}
		re, err := CompileRegex(s, what+".regex")
		if err != nil {
			return ValueMatcher{}, err
		}
		return ValueMatcher{mode: "regex", re: re}, nil
	case "absent":
		b, err := jsonx.Bool(obj[key], what+".absent")
		if err != nil {
			return ValueMatcher{}, err
		}
		if !b {
			return ValueMatcher{}, fmt.Errorf("%s.absent: only true is meaningful", what)
		}
		return ValueMatcher{mode: "absent"}, nil
	case "keys_exact", "keys_include":
		keys, err := jsonx.Strings(obj[key], what+"."+key)
		if err != nil {
			return ValueMatcher{}, err
		}
		seen := map[string]bool{}
		for _, k := range keys {
			if seen[k] {
				return ValueMatcher{}, fmt.Errorf("%s.%s: %q is listed twice", what, key, k)
			}
			seen[k] = true
		}
		return ValueMatcher{mode: key, keys: keys}, nil
	}
	return ValueMatcher{mode: "equals", want: obj[key]}, nil
}

// jsonCheckKinds are the matcher keys of a JSON check.
var jsonCheckKinds = []string{"equals", "regex", "absent", "keys_exact", "keys_include"}

// JSONCheck is one `{path, equals|regex|absent|keys_exact|keys_include}` entry (SPEC 7.2).
// An empty path is the document root.
type JSONCheck struct {
	Path    string
	Matcher ValueMatcher
}

// ParseJSONChecks reads a list of JSON checks.
func ParseJSONChecks(v any, what string) ([]JSONCheck, error) {
	list, err := jsonx.List(v, what)
	if err != nil {
		return nil, err
	}
	out := make([]JSONCheck, len(list))
	for i, item := range list {
		at := fmt.Sprintf("%s[%d]", what, i)
		obj, err := jsonx.Object(item, at)
		if err != nil {
			return nil, err
		}
		if err := jsonx.Keys(obj, at, append([]string{"path"}, jsonCheckKinds...)...); err != nil {
			return nil, err
		}
		path, err := jsonx.String(obj["path"], at+".path")
		if err != nil {
			return nil, err
		}
		var keys []string
		for _, k := range jsonCheckKinds {
			if _, ok := obj[k]; ok {
				keys = append(keys, k)
			}
		}
		if len(keys) != 1 {
			return nil, fmt.Errorf("%s: give exactly one of %v", at, jsonCheckKinds)
		}
		m, err := parseMatcherKey(obj, keys[0], at)
		if err != nil {
			return nil, err
		}
		out[i] = JSONCheck{Path: path, Matcher: m}
	}
	return out, nil
}

// evalJSON applies checks to a decoded document.
func evalJSON(what string, checks []JSONCheck, doc any) []Failure {
	var out []Failure
	for _, c := range checks {
		v, present := jsonx.Lookup(doc, c.Path)
		if ok, expected, actual := c.Matcher.Test(v, present); !ok {
			path := c.Path
			if path == "" {
				path = "(root)"
			}
			out = append(out, Failure{What: fmt.Sprintf("%s %s", what, path), Expected: expected, Actual: actual})
		}
	}
	return out
}
