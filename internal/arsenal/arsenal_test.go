package arsenal

import (
	"reflect"
	"strings"
	"testing"
)

func TestSummary(t *testing.T) {
	long := strings.Repeat("word ", 40)
	for in, want := range map[string]string{
		"Remove AI patterns. Use when writing.": "Remove AI patterns.",
		"No period at all":                      "No period at all",
		"  spread\n  over   lines. Then more.":  "spread over lines.",
		"v1.2 is a version. Second.":            "v1.2 is a version.",
		long:                                    strings.TrimRight(long[:117], " ") + "...",
		"":                                      "",
	} {
		if got := Summary(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestDescription(t *testing.T) {
	for in, want := range map[string]string{
		"---\nname: a\ndescription: Plain text.\n---\n":            "Plain text.",
		"---\ndescription: \"Quoted \\\"x\\\".\"\n---\n":           `Quoted "x".`,
		"---\ndescription: 'It''s quoted.'\n---\n":                 "It's quoted.",
		"---\ndescription: >\n  Folded\n  text.\nname: a\n---\n":   "Folded text.",
		"---\nname: a\n---\ndescription: after the front matter\n": "",
		"no front matter\ndescription: x\n":                        "",
	} {
		if got := Description(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestTransport(t *testing.T) {
	for want, def := range map[string]map[string]any{
		"sse":     {"type": "sse", "url": "u"},
		"stdio":   {"command": "c"},
		"http":    {"url": "u"},
		"unknown": {},
	} {
		if got := Transport(def); got != want {
			t.Errorf("%v: got %q", def, got)
		}
	}
}

func TestLines(t *testing.T) {
	inv := Inventory{
		Allowed:    []Item{{"model", "m", ""}, {"skill", "user:humanizer", "Human."}},
		Candidates: []Item{{"mcp", "x", "stdio, not allowlisted"}},
	}
	want := []string{"model    m", "skill    user:humanizer  Human."}
	if got := inv.Lines(false); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
	want = []string{"model    m", "skill    user:humanizer  Human.", "mcp      x               stdio, not allowlisted"}
	if got := inv.Lines(true); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}
