package catalog

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

var testConfig = Config{
	Models:  []string{"claude-haiku-5-5", "claude-sonnet-5-5", "123"},
	Efforts: []string{"low", "medium", "high"},
	MCP:     []string{"dbhub", "openapi"},
	Skills:  []string{"sprint-testing"},
	Tools:   []string{"Read", "Bash"},
}

var testRouteIDs = []string{"code-quickref", "ticket-work"}

const validBuild = `id: haiku-explorer
version: 1
status: approved
role: operator
type: explorer
work_kind: repo-exploration
description: Read-only search.
harness: claude
model: claude-haiku-5-5
effort: low
mode_default: direct
agents_md: exclude
route_scope: [none]
skills_first: []
tools: [Read, Bash]
mcp: []
requires: []
budget: { tokens: 250000, usd_list_cap: 0.50 }
success:
  - { check: answer_delivered }
`

// with replaces the top-level key an edit names, with its indented lines (or
// appends the edit), then decodes the result. An edit "key:-" removes the key.
func with(t *testing.T, edits ...string) any {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(validBuild, "\n"), "\n")
	for _, edit := range edits {
		key, _, _ := strings.Cut(edit, ":")
		replaced := false
		for i, l := range lines {
			if strings.HasPrefix(l, key+":") {
				end := i + 1
				for end < len(lines) && strings.HasPrefix(lines[end], "  ") {
					end++
				}
				var repl []string
				if !strings.HasPrefix(edit, key+":-") {
					repl = []string{edit}
				}
				lines = append(lines[:i], append(repl, lines[end:]...)...)
				replaced = true
				break
			}
		}
		if !replaced {
			lines = append(lines, edit)
		}
	}
	raw, err := decodeYAML([]byte(strings.Join(lines, "\n") + "\n"))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return raw
}

func TestValidateBuild(t *testing.T) {
	tests := []struct {
		name  string
		edits []string
		want  []string
	}{
		{"valid", nil, nil},
		{"model as a number is coerced", []string{"model: 123"}, nil},
		{"model not allowlisted", []string{"model: gpt"}, []string{`model "gpt" is not in the allowlist`}},
		{"model float", []string{"model: 0.50"}, []string{`model "0.5" is not in the allowlist`}},
		{"model missing", []string{"model:-"}, []string{`model "undefined" is not in the allowlist`}},
		{"model null", []string{"model: null"}, []string{`model "null" is not in the allowlist`}},
		{"model list", []string{"model: [a, b]"}, []string{`model "a,b" is not in the allowlist`}},
		{"model mapping", []string{"model: {a: 1}"}, []string{`model "[object Object]" is not in the allowlist`}},
		{"model bool", []string{"model: true"}, []string{`model "true" is not in the allowlist`}},
		{"effort", []string{"effort: max"}, []string{`effort "max" is not in the allowlist`}},
		{"id not kebab", []string{"id: Haiku"}, []string{"id must be kebab-case"}},
		{"id number", []string{"id: 7"}, []string{"id must be kebab-case"}},
		{"status", []string{"status: draft"}, []string{"status must be one of candidate, approved, retired"}},
		{"status missing", []string{"status:-"}, []string{"status must be one of candidate, approved, retired"}},
		{"skill", []string{"skills_first: [sprint-testing, nope]"}, []string{`skill "nope" is not in the allowlist`}},
		{"skills scalar is an empty list", []string{"skills_first: nope"}, nil},
		{"mcp", []string{"mcp: [dbhub, evil]"}, []string{`mcp "evil" is not in the allowlist`}},
		{"tool", []string{"tools: [Read, Write]"}, []string{`tool "Write" is not in the allowlist`}},
		{"tool number", []string{"tools: [1]"}, []string{`tool "1" is not in the allowlist`}},
		{"subagents ok", []string{"subagents: [plan, code]"}, nil},
		{"subagents not kebab", []string{"subagents: [Plan]"}, []string{"subagents must be a list of kebab-case names"}},
		{"subagents scalar", []string{"subagents: plan"}, []string{"subagents must be a list of kebab-case names"}},
		{"subagents null", []string{"subagents: null"}, []string{"subagents must be a list of kebab-case names"}},
		{"subagents number item", []string{"subagents: [1]"}, []string{"subagents must be a list of kebab-case names"}},
		{"route scope empty", []string{"route_scope: []"}, []string{"route_scope is empty (use [none])"}},
		{"route scope missing", []string{"route_scope:-"}, []string{"route_scope is empty (use [none])"}},
		{"route scope unknown", []string{"route_scope: [ticket-work, nope]"}, []string{`route_scope id "nope" is not an instruction section`}},
		{"required field without pattern", []string{"requires: [ticket_key, color]"}, []string{`required field "color" has no pattern`}},
		{"success empty", []string{"success: []"}, []string{"success must list at least one check"}},
		{"success missing", []string{"success:-"}, []string{"success must list at least one check"}},
		{"success unknown check", []string{"success: [{ check: magic }]"}, []string{`success check "magic" is unknown`}},
		{"success entry without check", []string{"success: [{ key: x }]"}, []string{`success check "undefined" is unknown`}},
		{"success string entry", []string{"success: [answer_delivered]"}, []string{`success check "undefined" is unknown`}},
		{"success placeholder", []string{"success: [{ check: jira_comment_on, key: '{ticket_key}' }]"}, []string{"success placeholder {ticket_key} is not a required field"}},
		{"success placeholder required", []string{"requires: [ticket_key]", "success: [{ check: jira_comment_on, key: '{ticket_key}' }]"}, nil},
		{"success placeholders in key order", []string{"success: [{ check: jira_comment_on, b: '{env}', a: '{ticket_key}' }]"}, []string{
			"success placeholder {env} is not a required field",
			"success placeholder {ticket_key} is not a required field",
		}},
		{"success string entry characters", []string{"success: ['{x}']"}, []string{`success check "undefined" is unknown`}},
		{"tier worker", []string{"tier: worker"}, nil},
		{"tier conductor", []string{"tier: conductor"}, nil},
		{"tier unknown", []string{"tier: boss"}, []string{"tier must be one of worker, conductor, subagent"}},
		{"tier null", []string{"tier: null"}, []string{"tier must be one of worker, conductor, subagent"}},
		{"subagent tier ok", []string{"tier: subagent", "prompt: You plan."}, nil},
		{"subagent tier without prompt", []string{"tier: subagent"}, []string{"prompt must be a non-empty string in a subagent-tier build"}},
		{"subagent tier blank prompt", []string{"tier: subagent", "prompt: '  '"}, []string{"prompt must be a non-empty string in a subagent-tier build"}},
		{"subagent tier without description", []string{"tier: subagent", "prompt: p", "description:-"}, []string{"description must be a non-empty string in a subagent-tier build"}},
		{"subagent tier listing subagents", []string{"tier: subagent", "prompt: p", "subagents: [plan]"}, []string{"a subagent-tier build cannot list subagents"}},
		{"several problems in order", []string{"id: X", "model: gpt", "route_scope: []"}, []string{
			"id must be kebab-case", `model "gpt" is not in the allowlist`, "route_scope is empty (use [none])",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateBuild(with(t, tt.edits...), testConfig, testRouteIDs)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestValidateBuildNotAMapping(t *testing.T) {
	for _, src := range []string{"", "- a\n", "just text\n", "7\n"} {
		raw, err := decodeYAML([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if got := ValidateBuild(raw, testConfig, testRouteIDs); !reflect.DeepEqual(got, []string{"build is not a mapping"}) {
			t.Errorf("%q: got %q", src, got)
		}
	}
}

func TestDecodeYAML(t *testing.T) {
	raw, err := decodeYAML([]byte("# comment\nb: 1\na: [x, 'y', 0.50]\nc: { d: ~ }\nd: 2026-11-15\ne: yes\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := raw.(*Map)
	if got := m.Keys(); !reflect.DeepEqual(got, []string{"b", "a", "c", "d", "e"}) {
		t.Errorf("keys %v", got)
	}
	if v, _ := m.Get("a"); !reflect.DeepEqual(v, []any{"x", "y", 0.5}) {
		t.Errorf("a = %#v", v)
	}
	if v, _ := m.Get("d"); v != "2026-11-15" {
		t.Errorf("a timestamp stays a string, got %#v", v)
	}
	if v, _ := m.Get("e"); v != "yes" {
		t.Errorf("yes stays a string, got %#v", v)
	}
	c, _ := m.Get("c")
	if v, ok := c.(*Map).Get("d"); !ok || v != nil {
		t.Errorf("c.d = %#v %v", v, ok)
	}

	for name, src := range map[string]string{
		"duplicate key": "a: 1\na: 2\n",
		"two documents": "a: 1\n---\nb: 2\n",
		"syntax":        "a: [1, 2\n",
	} {
		if _, err := decodeYAML([]byte(src)); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

const subagentBuild = `id: %s
version: 1
status: approved
tier: subagent
description: Helper.
prompt: You help.
model: claude-haiku-5-5
effort: low
route_scope: [none]
success:
  - { check: answer_delivered }
`

func testFS(builds map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{
		"jitso/jitso.yaml": {Data: []byte("version: 1\nmodels: [claude-haiku-5-5]\nefforts: [low]\nmcp: []\nskills: []\ntools: [Read, Bash]\n")},
		".agents/instructions/agent-code-quickref.md": {Data: []byte("---\nid: code-quickref\n---\n")},
		".agents/instructions/agent-x.md":             {Data: []byte("no id here\n")},
		".agents/instructions/other.md":               {Data: []byte("id: ignored\n")},
		"jitso/builds/notes.txt":                      {Data: []byte("ignored")},
	}
	for name, body := range builds {
		fsys["jitso/builds/"+name] = &fstest.MapFile{Data: []byte(body)}
	}
	return fsys
}

func sub(id string) string { return strings.Replace(subagentBuild, "%s", id, 1) }

func TestLoad(t *testing.T) {
	worker := strings.Replace(validBuild, "tools: [Read, Bash]", "tools: [Read, Bash]\nsubagents: [plan]", 1)
	cat, err := Load(testFS(map[string]string{"b-worker.yaml": worker, "a-plan.yaml": sub("plan")}))
	if err != nil {
		t.Fatal(err)
	}
	if got := cat.RouteIDs; !reflect.DeepEqual(got, []string{"code-quickref"}) {
		t.Errorf("route ids %v", got)
	}
	if len(cat.Builds) != 2 || cat.Builds[0].File != "a-plan.yaml" || cat.Builds[1].File != "b-worker.yaml" {
		t.Fatalf("builds not sorted by file: %+v", cat.Builds)
	}
	w, ok := cat.Find("haiku-explorer")
	if !ok {
		t.Fatal("worker not found")
	}
	if w.Tier != TierWorker || !w.ToolsSet || !reflect.DeepEqual(w.Tools, []string{"Read", "Bash"}) {
		t.Errorf("worker fields: %+v", w)
	}
	if !w.Budget.Set || w.Budget.Tokens != 250000 || w.Budget.USDListCap != 0.5 {
		t.Errorf("budget %+v", w.Budget)
	}
	if subs := cat.SubagentsOf(w); len(subs) != 1 || subs[0].Prompt != "You help." {
		t.Errorf("subagents %+v", subs)
	}
	if got := cat.Summary(); got != "- haiku-explorer: claude-haiku-5-5 low. Read-only search. requires: nothing" {
		t.Errorf("summary %q", got)
	}
}

func TestLoadErrors(t *testing.T) {
	worker := func(subs string) string {
		return strings.Replace(validBuild, "tools: [Read, Bash]", "tools: [Read, Bash]\nsubagents: "+subs, 1)
	}
	bad := strings.Replace(validBuild, "model: claude-haiku-5-5", "model: gpt", 1)
	tests := []struct {
		name   string
		builds map[string]string
		want   string
	}{
		{"invalid build", map[string]string{"broken.yaml": bad}, `build jitso/builds/broken.yaml is invalid: model "gpt" is not in the allowlist`},
		{"first invalid in file order", map[string]string{"z.yaml": bad, "a.yaml": strings.Replace(bad, "id: haiku-explorer", "id: X", 1)},
			`build jitso/builds/a.yaml is invalid: id must be kebab-case; model "gpt" is not in the allowlist`},
		{"unknown subagent", map[string]string{"w.yaml": worker("[nonexistent-helper]")}, `build jitso/builds/w.yaml is invalid: subagent "nonexistent-helper" is not a subagent-tier build`},
		{"worker-tier subagent", map[string]string{"w.yaml": worker("[plan, other]"), "p.yaml": sub("plan"),
			"o.yaml": strings.Replace(validBuild, "id: haiku-explorer", "id: other", 1)},
			`build jitso/builds/w.yaml is invalid: subagent "other" is not a subagent-tier build`},
		{"bad yaml", map[string]string{"y.yaml": "a: [1\n"}, "build jitso/builds/y.yaml is invalid: "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(testFS(tt.builds))
			var fe *FileError
			if !errors.As(err, &fe) {
				t.Fatalf("want a FileError, got %v", err)
			}
			if !strings.HasPrefix(err.Error(), tt.want) {
				t.Errorf("got  %s\nwant %s", err, tt.want)
			}
		})
	}
}

func TestLoadMissingFiles(t *testing.T) {
	fsys := testFS(nil)
	delete(fsys, "jitso/jitso.yaml")
	if _, err := Load(fsys); err == nil {
		t.Error("missing jitso.yaml must fail")
	}
	fsys = testFS(nil)
	fsys["jitso/jitso.yaml"] = &fstest.MapFile{Data: []byte("- a\n")}
	if _, err := Load(fsys); err == nil || err.Error() != "jitso/jitso.yaml is invalid: the top level is not a mapping" {
		t.Errorf("not a mapping: %v", err)
	}
}

func TestConfigCoercion(t *testing.T) {
	raw, _ := decodeYAML([]byte("version: 1\nmodels: [123, a]\nefforts: low\n"))
	cfg, ok := configFrom(raw)
	if !ok || cfg.Version != 1 || !reflect.DeepEqual(cfg.Models, []string{"123", "a"}) || len(cfg.Efforts) != 0 {
		t.Errorf("config %+v", cfg)
	}
}

func TestSummaryLine(t *testing.T) {
	tests := []struct {
		name  string
		edits []string
		want  string
	}{
		{"nothing", nil, "- haiku-explorer: claude-haiku-5-5 low. Read-only search. requires: nothing"},
		{"fields", []string{"requires: [ticket_key, env]"}, "- haiku-explorer: claude-haiku-5-5 low. Read-only search. requires: ticket_key, env"},
		{"missing description", []string{"description:-"}, "- haiku-explorer: claude-haiku-5-5 low. undefined requires: nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := buildFrom("x.yaml", with(t, tt.edits...).(*Map))
			if got := SummaryLine(&b); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// TestDefaultFixture loads the contract's default fixture: the four
// launchable builds are listed, the three subagent-tier builds are not.
func TestDefaultFixture(t *testing.T) {
	const dir = "../../contract/fixtures/default"
	if _, err := os.Stat(dir); err != nil {
		t.Skip("contract fixture not found")
	}
	cat, err := Load(os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, b := range cat.Launchable() {
		ids = append(ids, b.ID)
	}
	if want := []string{"haiku-explorer", "sonnet-automator", "sonnet-conductor", "sonnet-tester"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("launchable %v, want %v", ids, want)
	}
	for _, id := range []string{"plan", "code", "verifier"} {
		b, ok := cat.Find(id)
		if !ok || b.Tier != TierSubagent || b.Launchable() {
			t.Errorf("%s: %+v", id, b)
		}
	}
}
