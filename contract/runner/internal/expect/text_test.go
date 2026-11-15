package expect

import (
	"reflect"
	"testing"

	"jitso-contract-runner/internal/jsonx"
)

func matcher(t *testing.T, js string) TextMatcher {
	t.Helper()
	v, err := jsonx.Decode([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseTextMatcher(v, "m")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTextMatchers(t *testing.T) {
	cases := []struct {
		matcher, text string
		ok            bool
	}{
		{`"abc"`, "xabcx", true},
		{`"abc"`, "ab", false},
		{`{"contains":"b"}`, "abc", true},
		{`{"not_contains":"b"}`, "abc", false},
		{`{"not_contains":"z"}`, "abc", true},
		{`{"equals":"abc"}`, "abc", true},
		{`{"equals":"abc"}`, "abc\n", false},
		{`{"regex":"^a.c$"}`, "abc", true},
		{`{"regex":"^b$"}`, "a\nb\nc", false}, // multiline is off by default
		{`{"regex":"(?m)^b$"}`, "a\nb\nc", true},
		{`{"not_regex":"\\d"}`, "abc", true},
		{`{"not_regex":"\\d"}`, "a1", false},
		{`{"empty":true}`, "", true},
		{`{"empty":true}`, " ", false},
		{`{"empty":false}`, "x", true},
		{`{"line_count":0}`, "", true},
		{`{"line_count":2}`, "a\nb\n", true},
		{`{"line_count":2}`, "a\nb", true},
		{`{"line_count":3}`, "a\n\nb", true},
	}
	for _, c := range cases {
		got := matcher(t, c.matcher).Match(c.text) == nil
		if got != c.ok {
			t.Errorf("%s on %q: got %v, want %v", c.matcher, c.text, got, c.ok)
		}
	}
}

func TestTextMatcherShape(t *testing.T) {
	for _, js := range []string{`{"contains":"a","equals":"b"}`, `{}`, `{"includes":"a"}`, `{"regex":"("}`, `{"line_count":-1}`, `3`} {
		v, _ := jsonx.Decode([]byte(js))
		if _, err := ParseTextMatcher(v, "m"); err == nil {
			t.Errorf("%s: want a parse error", js)
		}
	}
}

func TestRouteScopeParsing(t *testing.T) {
	cases := []struct {
		text  string
		ids   []string
		found bool
	}{
		// today's TS card: no colon, so the router does not see it (defect S1)
		{"subagents unrestricted (harness default) | ROUTE-SCOPE none", nil, false},
		{"subagents x | ROUTE-SCOPE code-quickref,ticket-work", nil, false},
		{"ROUTE-SCOPE: none", []string{}, true},
		{"ROUTE-SCOPE:", []string{}, true},
		{"ROUTE-SCOPE: code-quickref, ticket-work.", []string{"code-quickref", "ticket-work"}, true},
		{"ROUTE-SCOPE: a;. , b;", []string{"a", "b"}, true},
		{"route-scope: A", []string{"A"}, true},
		{"Read the brief. ROUTE-SCOPE: a", []string{"a"}, true},
		{"x | ROUTE-SCOPE: a", nil, false}, // must open a line or follow a sentence
		{"> - ROUTE-SCOPE: a", []string{"a"}, true},
		{"line one\nROUTE-SCOPE: a, none\nmore\n* ROUTE-SCOPE: b", []string{"a", "b"}, true},
		{"ROUTE-SCOPE: a\nb, c", []string{"a"}, true}, // a capture stops at the newline
	}
	for _, c := range cases {
		ids, found := RouteScope(c.text)
		if found != c.found || (c.found && !reflect.DeepEqual(ids, c.ids)) {
			t.Errorf("RouteScope(%q) = %q, %v; want %q, %v", c.text, ids, found, c.ids, c.found)
		}
	}
}

func TestRouteScopeMatcher(t *testing.T) {
	empty := matcher(t, `{"route_scope":[]}`)
	if empty.Match("ROUTE-SCOPE none") == nil {
		t.Error("route_scope [] must fail when no ROUTE-SCOPE: line exists")
	}
	if empty.Match("ROUTE-SCOPE: none") != nil {
		t.Error("route_scope [] must pass on ROUTE-SCOPE: none")
	}
	two := matcher(t, `{"route_scope":["code-quickref","ticket-work"]}`)
	if two.Match("ROUTE-SCOPE: code-quickref,ticket-work") != nil {
		t.Error("route_scope must pass on the same ids in order")
	}
	if two.Match("ROUTE-SCOPE: ticket-work,code-quickref") == nil {
		t.Error("route_scope must fail on a different order")
	}
}
