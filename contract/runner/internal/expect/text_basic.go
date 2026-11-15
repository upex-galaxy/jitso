package expect

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"jitso-contract-runner/internal/jsonx"
)

// The plain text matchers of SPEC 7.1. route_scope lives in its own file.
func init() {
	registerText("contains", stringMatcher("contains", func(text, s string) bool { return strings.Contains(text, s) }))
	registerText("not_contains", stringMatcher("not_contains", func(text, s string) bool { return !strings.Contains(text, s) }))
	registerText("equals", stringMatcher("equals", func(text, s string) bool { return text == s }))
	registerText("regex", regexMatcher("regex", true))
	registerText("not_regex", regexMatcher("not_regex", false))
	registerText("empty", func(v any, what string) (TextMatcher, error) {
		want, err := jsonx.Bool(v, what)
		if err != nil {
			return nil, err
		}
		return emptyMatcher{want: want}, nil
	})
	registerText("line_count", func(v any, what string) (TextMatcher, error) {
		n, err := jsonx.Int(v, what)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, fmt.Errorf("%s: want 0 or more lines", what)
		}
		return lineCountMatcher{want: n}, nil
	})
}

type predicateMatcher struct {
	name string
	arg  string
	test func(text, arg string) bool
}

func (m predicateMatcher) Name() string { return m.name }

func (m predicateMatcher) Match(text string) *Mismatch {
	if m.test(text, m.arg) {
		return nil
	}
	return &Mismatch{Expected: fmt.Sprintf("%s %q", m.name, m.arg)}
}

func stringMatcher(name string, test func(text, s string) bool) func(v any, what string) (TextMatcher, error) {
	return func(v any, what string) (TextMatcher, error) {
		s, err := jsonx.String(v, what)
		if err != nil {
			return nil, err
		}
		return predicateMatcher{name: name, arg: s, test: test}, nil
	}
}

type reMatcher struct {
	name string
	re   *regexp.Regexp
	want bool
}

func (m reMatcher) Name() string { return m.name }

func (m reMatcher) Match(text string) *Mismatch {
	if m.re.MatchString(text) == m.want {
		return nil
	}
	return &Mismatch{Expected: fmt.Sprintf("%s /%s/", m.name, m.re)}
}

func regexMatcher(name string, want bool) func(v any, what string) (TextMatcher, error) {
	return func(v any, what string) (TextMatcher, error) {
		s, err := jsonx.String(v, what)
		if err != nil {
			return nil, err
		}
		re, err := CompileRegex(s, what)
		if err != nil {
			return nil, err
		}
		return reMatcher{name: name, re: re, want: want}, nil
	}
}

type emptyMatcher struct{ want bool }

func (m emptyMatcher) Name() string { return "empty" }

func (m emptyMatcher) Match(text string) *Mismatch {
	if (text == "") == m.want {
		return nil
	}
	if m.want {
		return &Mismatch{Expected: "empty text"}
	}
	return &Mismatch{Expected: "non-empty text", Actual: "(empty)"}
}

type lineCountMatcher struct{ want int }

func (m lineCountMatcher) Name() string { return "line_count" }

func (m lineCountMatcher) Match(text string) *Mismatch {
	if got := CountLines(text); got != m.want {
		return &Mismatch{Expected: fmt.Sprintf("%d lines", m.want), Actual: strconv.Itoa(got) + " lines:\n" + text}
	}
	return nil
}

// CountLines counts newline-separated lines; one trailing newline does not open a new line.
func CountLines(text string) int {
	if text == "" {
		return 0
	}
	return len(strings.Split(strings.TrimSuffix(text, "\n"), "\n"))
}
