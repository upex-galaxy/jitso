package profile

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/upex-galaxy/jitso/internal/catalog"
)

const config = "version: 1\nmodels: [claude-haiku-5-5, claude-sonnet-5-5]\nefforts: [low, medium]\nmcp: [dbhub]\nskills: [sprint-testing]\ntools: [Read, Bash]\n"

func build(id string, extra ...string) string {
	lines := map[string]string{
		"id": id, "version": "1", "status": "approved", "role": "operator", "type": "explorer", "harness": "claude",
		"model": "claude-haiku-5-5", "effort": "low", "agents_md": "exclude", "route_scope": "[none]", "skills_first": "[]",
		"mcp": "[]", "requires": "[]", "budget": "{ tokens: 1000, usd_list_cap: 0.5 }", "success": "[{ check: answer_delivered }]",
		"description": "d",
	}
	for _, e := range extra {
		k, v, _ := strings.Cut(e, ": ")
		lines[k] = v
	}
	var b strings.Builder
	for k, v := range lines {
		b.WriteString(k + ": " + v + "\n")
	}
	return b.String()
}

func load(t *testing.T) *catalog.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"jitso/jitso.yaml":                          {Data: []byte(config)},
		".agents/instructions/agent-ticket-work.md": {Data: []byte("---\nid: ticket-work\n---\n")},
		".agents/instructions/agent-code.md":        {Data: []byte("---\nid: code-quickref\n---\n")},
		"jitso/presets/explorer.yaml":               {Data: []byte(build("explorer"))},
		"jitso/presets/draft.yaml":                  {Data: []byte(build("draft", "status: candidate"))},
		"jitso/presets/plan.yaml":                   {Data: []byte(build("plan", "tier: subagent", "prompt: You plan."))},
		"jitso/presets/worker.yaml":                 {Data: []byte(build("worker", "subagents: [plan]", "model: claude-sonnet-5-5", "effort: medium", "mcp: [dbhub]", "route_scope: [ticket-work, code-quickref]"))},
		"jitso/presets/tester.yaml":                 {Data: []byte(build("tester", "requires: [ticket_key, env]"))},
		"jitso/presets/mixed-scope.yaml":            {Data: []byte(build("mixed-scope", "route_scope: [none, ticket-work]"))},
	}
	cat, err := catalog.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func fields(pairs ...string) Fields {
	var f Fields
	for i := 0; i+1 < len(pairs); i += 2 {
		f.Set(pairs[i], pairs[i+1])
	}
	return f
}

func TestResolve(t *testing.T) {
	cat := load(t)
	tests := []struct {
		name   string
		req    Request
		reason string
	}{
		{"ok", Request{Preset: "explorer"}, ""},
		{"unknown", Request{Preset: "wizard"}, `unknown preset "wizard"`},
		{"not approved", Request{Preset: "draft"}, `preset "draft" is candidate, not approved`},
		{"subagent tier", Request{Preset: "plan"}, `preset "plan" has tier subagent: it is not launched alone, a worker lists it in its subagents`},
		{"none mixed", Request{Preset: "mixed-scope"}, `"none" cannot be combined with route-scope ids`},
		{"unknown field", Request{Preset: "explorer", Fields: fields("ticket_key", "PRJ-1")}, `unknown field "ticket_key" for preset "explorer"`},
		{"invalid field", Request{Preset: "tester", Fields: fields("ticket_key", "prj-1", "env", "x")}, "field ticket_key has an invalid value"},
		{"trailing newline is invalid", Request{Preset: "tester", Fields: fields("ticket_key", "PRJ-1\n", "env", "x")}, "field ticket_key has an invalid value"},
		{"missing in requires order", Request{Preset: "tester"}, "missing ticket_key"},
		{"second missing", Request{Preset: "tester", Fields: fields("ticket_key", "PRJ-1")}, "missing env"},
		{"index-like names come first", Request{Preset: "tester", Fields: fields("ticket_key", "bad", "7", "x")}, `unknown field "7" for preset "tester"`},
		{"all given", Request{Preset: "tester", Fields: fields("env", "staging", "ticket_key", "PRJ-1")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.req, cat)
			if tt.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || err.Error() != "JITSO REJECTED: "+tt.reason {
				t.Fatalf("got %v, want %q", err, tt.reason)
			}
		})
	}
}

func TestResolveProfile(t *testing.T) {
	p, err := Resolve(Request{Preset: "worker"}, load(t))
	if err != nil {
		t.Fatal(err)
	}
	if p.Model != "claude-sonnet-5-5" || p.Effort != "medium" || !reflect.DeepEqual(p.MCP, []string{"dbhub"}) ||
		!reflect.DeepEqual(p.RouteScope, []string{"ticket-work", "code-quickref"}) {
		t.Errorf("profile %+v", p)
	}
	if len(p.Subagents) != 1 || p.Subagents[0].ID != "plan" || p.Subagents[0].Prompt != "You plan." {
		t.Errorf("subagents %+v", p.Subagents)
	}
}

func TestFieldsKeys(t *testing.T) {
	f := fields("b", "1", "10", "x", "a", "2", "2", "y", "b", "3", "01", "z", "4294967295", "w")
	if got, want := f.Keys(), []string{"2", "10", "b", "a", "01", "4294967295"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys %q, want %q", got, want)
	}
	if f.Get("b") != "3" || !f.Has("a") || f.Has("c") || f.Len() != 6 {
		t.Error("get, has or len")
	}
}
