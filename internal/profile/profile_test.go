package profile

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/upex-galaxy/jitso/internal/catalog"
)

const config = "version: 1\nmodels: [claude-haiku-5-5, claude-sonnet-5-5]\nefforts: [low, medium]\nmcp: [dbhub]\nskills: [sprint-testing]\ntools: [Read, Bash]\nfields:\n  ticket_key: '^[A-Z][A-Z0-9]*-[0-9]+$'\n  env: '^[a-z][a-z0-9_-]*$'\n"

func build(id string, extra ...string) string {
	lines := map[string]string{
		"id": id, "version": "1", "status": "approved", "role": "operator", "type": "explorer", "harness": "claude",
		"model": "claude-haiku-5-5", "effort": "low", "agents_md": "exclude", "route_scope": "[none]", "skills_first": "[]",
		"mcp": "[]", "requires": "[]", "budget": "{ tokens: 1000, usd_list_cap: 0.5 }",
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

func load(t *testing.T, extra ...string) *catalog.Catalog {
	t.Helper()
	fsys := fstest.MapFS{
		"jitso/jitso.yaml":                          {Data: []byte(config)},
		".agents/instructions/agent-ticket-work.md": {Data: []byte("---\nid: ticket-work\n---\n")},
		".agents/instructions/agent-code.md":        {Data: []byte("---\nid: code-quickref\n---\n")},
		"jitso/builds/explorer.yaml":                {Data: []byte(build("explorer"))},
		"jitso/builds/draft.yaml":                   {Data: []byte(build("draft", "status: candidate"))},
		"jitso/builds/plan.yaml":                    {Data: []byte(build("plan", "tier: subagent", "prompt: You plan."))},
		"jitso/builds/worker.yaml":                  {Data: []byte(build("worker", "subagents: [plan]", "model: claude-sonnet-5-5", "effort: medium", "mcp: [dbhub]", "route_scope: [ticket-work, code-quickref]"))},
		"jitso/builds/tester.yaml":                  {Data: []byte(build("tester", "requires: [ticket_key, env]"))},
		"jitso/builds/mixed-scope.yaml":             {Data: []byte(build("mixed-scope", "route_scope: [none, ticket-work]"))},
	}
	for i := 0; i+1 < len(extra); i += 2 {
		fsys["jitso/builds/"+extra[i]] = &fstest.MapFile{Data: []byte(extra[i+1])}
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
		{"ok", Request{Build: "explorer"}, ""},
		{"unknown", Request{Build: "wizard"}, `unknown build "wizard"`},
		{"not approved", Request{Build: "draft"}, `build "draft" is candidate, not approved`},
		{"subagent tier", Request{Build: "plan"}, `build "plan" has tier subagent: it is not launched alone, a worker lists it in its subagents`},
		{"none mixed", Request{Build: "mixed-scope"}, `"none" cannot be combined with route-scope ids`},
		{"unknown field", Request{Build: "explorer", Fields: fields("ticket_key", "PRJ-1")}, `unknown field "ticket_key" for build "explorer"`},
		{"invalid field", Request{Build: "tester", Fields: fields("ticket_key", "prj-1", "env", "x")}, "field ticket_key has an invalid value"},
		{"trailing newline is invalid", Request{Build: "tester", Fields: fields("ticket_key", "PRJ-1\n", "env", "x")}, "field ticket_key has an invalid value"},
		{"missing in requires order", Request{Build: "tester"}, "missing ticket_key"},
		{"second missing", Request{Build: "tester", Fields: fields("ticket_key", "PRJ-1")}, "missing env"},
		{"index-like names come first", Request{Build: "tester", Fields: fields("ticket_key", "bad", "7", "x")}, `unknown field "7" for build "tester"`},
		{"all given", Request{Build: "tester", Fields: fields("env", "staging", "ticket_key", "PRJ-1")}, ""},
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

// A worker may launch only with approved subagents (defect N-11). The extra
// file replaces the `plan` subagent of the base catalog.
func TestResolveSubagentStatus(t *testing.T) {
	tests := []struct {
		status string
		reason string
	}{
		{"approved", ""},
		{"candidate", `build "worker" lists subagent "plan", which is candidate, not approved`},
		{"retired", `build "worker" lists subagent "plan", which is retired, not approved`},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			cat := load(t, "plan.yaml", build("plan", "tier: subagent", "prompt: You plan.", "status: "+tt.status))
			_, err := Resolve(Request{Build: "worker"}, cat)
			if tt.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != "JITSO REJECTED: "+tt.reason {
				t.Fatalf("got %v, want %q", err, tt.reason)
			}
			// A worker with no subagents is unaffected.
			if _, err := Resolve(Request{Build: "explorer"}, cat); err != nil {
				t.Errorf("explorer: %v", err)
			}
			problems := LaunchProblems(mustFind(t, cat, "worker"), cat)
			var want []string
			if tt.reason != "" {
				want = []string{`subagent "plan" is ` + tt.status + ", not approved"}
			}
			if !reflect.DeepEqual(problems, want) {
				t.Errorf("LaunchProblems %q, want %q", problems, want)
			}
		})
	}
}

func mustFind(t *testing.T, cat *catalog.Catalog, id string) *catalog.Build {
	t.Helper()
	b, ok := cat.Find(id)
	if !ok {
		t.Fatalf("build %s not found", id)
	}
	return b
}

func TestResolveProfile(t *testing.T) {
	p, err := Resolve(Request{Build: "worker"}, load(t))
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

// An id whose nearest file is invalid for this project is rejected with the
// file and its problems, not reported as unknown (defect N-12).
func TestResolveInvalidHere(t *testing.T) {
	cat := load(t)
	cat.Skipped = []catalog.Skipped{
		{ID: "future", Layer: catalog.LayerDraft, Display: ".jitso/drafts/future.yaml", Problems: []string{`model "x" is not in the allowlist`, `effort "y" is not in the allowlist`}},
		{ID: "", Layer: catalog.LayerDraft, Display: ".jitso/drafts/noid.yaml", Problems: []string{"id must be kebab-case"}},
	}
	_, err := Resolve(Request{Build: "future"}, cat)
	want := `JITSO REJECTED: build "future" from draft .jitso/drafts/future.yaml is invalid here: model "x" is not in the allowlist; effort "y" is not in the allowlist`
	if err == nil || err.Error() != want {
		t.Errorf("got %v\nwant %s", err, want)
	}
	if _, err := Resolve(Request{Build: "nobody"}, cat); err == nil || err.Error() != `JITSO REJECTED: unknown build "nobody"` {
		t.Errorf("unknown: %v", err)
	}
	if _, err := Resolve(Request{Build: ""}, cat); err == nil || err.Error() != `JITSO REJECTED: unknown build ""` {
		t.Errorf("empty id must not match a file without an id: %v", err)
	}
}

// TestResolveFieldWithoutPattern: a field with no entry in the fields key of
// jitso.yaml gets the generic check, a non-empty value on one line.
func TestResolveFieldWithoutPattern(t *testing.T) {
	fsys := fstest.MapFS{
		"jitso/jitso.yaml":           {Data: []byte("version: 1\nmodels: [claude-haiku-5-5]\nefforts: [low]\n")},
		".agents/instructions/x.txt": {Data: []byte("")},
		"jitso/builds/painter.yaml":  {Data: []byte(build("painter", "requires: [color]", "route_scope: null"))},
	}
	cat, err := catalog.Load(fsys)
	if err != nil {
		t.Fatal(err)
	}
	for value, reason := range map[string]string{"any text": "", "": "field color has an invalid value", "a\nb": "field color has an invalid value"} {
		_, err := Resolve(Request{Build: "painter", Fields: fields("color", value)}, cat)
		if reason == "" && err != nil || reason != "" && (err == nil || !strings.Contains(err.Error(), reason)) {
			t.Errorf("value %q: got %v, want %q", value, err, reason)
		}
	}
}
