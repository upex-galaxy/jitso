package spec

import (
	"errors"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	vars := Vars{"PROJECT": "/p", "LAUNCH_ID": "L1", "v": "{{PROJECT}}"}
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"{{PROJECT}}/x", "/p/x"},
		{"{{PROJECT}}{{LAUNCH_ID}}", "/pL1"},
		{"literal {{{{PROJECT}}", "literal {{PROJECT}}"},
		{"{{{{", "{{"},
		{"a}}b", "a}}b"},
		{"{{v}}", "{{PROJECT}}"}, // values are inserted verbatim, never resolved again
		{"", ""},
	}
	for _, c := range cases {
		got, err := Resolve(c.in, vars)
		if err != nil || got != c.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
}

func TestResolveErrors(t *testing.T) {
	var unknown *UnknownPlaceholder
	if _, err := Resolve("{{NOPE}}", Vars{}); !errors.As(err, &unknown) || unknown.Name != "NOPE" {
		t.Errorf("unknown placeholder: got %v", err)
	}
	for _, in := range []string{"{{", "x {{PROJECT", "{{a b}}", "{{1x}}", `{{"k":1}}`} {
		if _, err := Resolve(in, Vars{"PROJECT": "/p"}); err == nil || !strings.Contains(err.Error(), "{{{{") {
			t.Errorf("Resolve(%q): want an error that names the {{{{ escape, got %v", in, err)
		}
	}
}

func TestResolveTree(t *testing.T) {
	tree := map[string]any{"{{K}}/a": []any{"{{K}}", true, nil}}
	got, err := ResolveTree(tree, Vars{"K": "k"})
	if err != nil {
		t.Fatal(err)
	}
	list := got.(map[string]any)["k/a"].([]any)
	if list[0] != "k" || list[1] != true || list[2] != nil {
		t.Errorf("ResolveTree = %v", got)
	}
	if _, err := ResolveTree(map[string]any{"{{A}}": 1, "{{B}}": 2}, Vars{"A": "x", "B": "x"}); err == nil {
		t.Error("two keys resolving to the same name must be an error")
	}
}
