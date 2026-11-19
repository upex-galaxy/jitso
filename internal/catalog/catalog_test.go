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
		{"success is an unknown key and is ignored", []string{"success: [{ key: '{x}' }]"}, nil},
		{"route scope empty is none", []string{"route_scope: []"}, nil},
		{"route scope missing is none", []string{"route_scope:-"}, nil},
		{"route scope unknown", []string{"route_scope: [ticket-work, nope]"}, []string{`route_scope id "nope" is not an instruction section`}},
		{"required field without a fields entry", []string{"requires: [ticket_key, color]"}, nil},
		{"required field not a field name", []string{"requires: [Color, env2, a-b]"}, []string{
			`required field "Color" is not a field name`, `required field "env2" is not a field name`, `required field "a-b" is not a field name`,
		}},
		{"tier worker", []string{"tier: worker"}, nil},
		{"tier conductor", []string{"tier: conductor"}, nil},
		{"tier unknown", []string{"tier: boss"}, []string{"tier must be one of worker, conductor, subagent"}},
		{"tier null", []string{"tier: null"}, []string{"tier must be one of worker, conductor, subagent"}},
		{"subagent tier ok", []string{"tier: subagent", "prompt: You plan."}, nil},
		{"subagent tier without prompt", []string{"tier: subagent"}, []string{"prompt must be a non-empty string in a subagent-tier build"}},
		{"subagent tier blank prompt", []string{"tier: subagent", "prompt: '  '"}, []string{"prompt must be a non-empty string in a subagent-tier build"}},
		{"subagent tier without description", []string{"tier: subagent", "prompt: p", "description:-"}, []string{"description must be a non-empty string in a subagent-tier build"}},
		{"subagent tier listing subagents", []string{"tier: subagent", "prompt: p", "subagents: [plan]"}, []string{"a subagent-tier build cannot list subagents"}},
		{"conductor needs no route_scope", []string{"tier: conductor", "route_scope:-"}, nil},
		{"conductor may list subagents and skills", []string{"tier: conductor", "skills_first: [sprint-testing]", "mcp: [dbhub]", "requires: [ticket_key]", "subagents: [plan]"}, nil},
		{"subagent tier needs no launch-only field", []string{"tier: subagent", "prompt: p", "route_scope:-", "budget:-", "mode_default:-", "agents_md:-", "harness:-"}, nil},
		{"subagent tier needs no list field at all", []string{"tier: subagent", "prompt: p", "skills_first:-", "mcp:-", "requires:-"}, nil},
		{"subagent tier route_scope is checked when present", []string{"tier: subagent", "prompt: p", "route_scope: [nope]"}, []string{`route_scope id "nope" is not an instruction section`}},
		{"subagent tier empty route_scope is none", []string{"tier: subagent", "prompt: p", "route_scope: []"}, nil},
		{"subagent tier skills_first must be empty", []string{"tier: subagent", "prompt: p", "skills_first: [sprint-testing]"}, []string{"skills_first must be empty in a subagent-tier build"}},
		{"subagent tier mcp must be empty", []string{"tier: subagent", "prompt: p", "mcp: [dbhub]"}, []string{"mcp must be empty in a subagent-tier build"}},
		{"subagent tier requires must be empty", []string{"tier: subagent", "prompt: p", "requires: [ticket_key]"}, []string{"requires must be empty in a subagent-tier build"}},
		{"subagent tier unknown mcp reports both", []string{"tier: subagent", "prompt: p", "mcp: [evil]"}, []string{`mcp "evil" is not in the allowlist`, "mcp must be empty in a subagent-tier build"}},
		{"subagent tier lists in order", []string{"tier: subagent", "prompt: p", "skills_first: [sprint-testing]", "mcp: [dbhub]", "requires: [ticket_key]", "subagents: [plan]"}, []string{
			"skills_first must be empty in a subagent-tier build", "mcp must be empty in a subagent-tier build",
			"requires must be empty in a subagent-tier build", "a subagent-tier build cannot list subagents",
		}},
		{"unknown tier value keeps the worker rules", []string{"tier: boss", "route_scope: [nope]"}, []string{`route_scope id "nope" is not an instruction section`, "tier must be one of worker, conductor, subagent"}},
		{"several problems in order", []string{"id: X", "model: gpt", "route_scope: [nope]"}, []string{
			"id must be kebab-case", `model "gpt" is not in the allowlist`, `route_scope id "nope" is not an instruction section`,
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
}

// A subagent-tier build with only the fields claude needs loads, resolves
// from its worker and is never listed (defect N-11).
func TestLoadLeanSubagent(t *testing.T) {
	lean := "id: lean\nversion: 1\nstatus: approved\ntier: subagent\ndescription: Helper.\nprompt: You help.\nmodel: claude-haiku-5-5\neffort: low\ntools: [Read]\n"
	worker := strings.Replace(validBuild, "tools: [Read, Bash]", "tools: [Read, Bash]\nsubagents: [lean]", 1)
	cat, err := Load(testFS(map[string]string{"lean.yaml": lean, "w.yaml": worker}))
	if err != nil {
		t.Fatal(err)
	}
	w, _ := cat.Find("haiku-explorer")
	subs := cat.SubagentsOf(w)
	if len(subs) != 1 || subs[0].ID != "lean" || subs[0].Prompt != "You help." || len(subs[0].RouteScope) != 0 || subs[0].Budget.Set {
		t.Errorf("subagents %+v", subs)
	}
	if l := cat.Launchable(); len(l) != 1 || l[0].ID != "haiku-explorer" {
		t.Errorf("launchable %+v", l)
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
		{"subagent with mcp", map[string]string{"p.yaml": strings.Replace(sub("plan"), "route_scope:", "mcp: [dbhub]\nroute_scope:", 1)},
			`build jitso/builds/p.yaml is invalid: mcp "dbhub" is not in the allowlist; mcp must be empty in a subagent-tier build`},
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
	cfg, _, ok := configFrom(raw)
	if !ok || cfg.Version != 1 || !reflect.DeepEqual(cfg.Models, []string{"123", "a"}) || len(cfg.Efforts) != 0 {
		t.Errorf("config %+v", cfg)
	}
}

func TestConfigFields(t *testing.T) {
	tests := []struct {
		name, yaml string
		problems   []string
		names      []string
	}{
		{"absent", "version: 1\n", nil, nil},
		{"null", "fields:\n", nil, nil},
		{"empty mapping", "fields: {}\n", nil, nil},
		{"two patterns", "fields:\n  ticket_key: '^[A-Z]+-[0-9]+$'\n  env: '^[a-z]+$'\n", nil, []string{"ticket_key", "env"}},
		{"list", "fields: [a]\n", []string{"fields must be a mapping of field name to pattern"}, nil},
		{"string", "fields: abc\n", []string{"fields must be a mapping of field name to pattern"}, nil},
		{"bad names", "fields:\n  Ticket: x\n  env2: x\n  a-b: x\n", []string{
			`field name "Ticket" must be lowercase letters and _`, `field name "env2" must be lowercase letters and _`, `field name "a-b" must be lowercase letters and _`,
		}, nil},
		{"not a string", "fields:\n  a: 5\n  b: ''\n  c:\n", []string{
			`field "a" pattern must be a non-empty string`, `field "b" pattern must be a non-empty string`, `field "c" pattern must be a non-empty string`,
		}, nil},
		{"does not compile", "fields:\n  a: '['\n", []string{"field \"a\" pattern does not compile: error parsing regexp: missing closing ]: `[`"}, nil},
		{"problems in key order", "fields:\n  b: '['\n  A: x\n  c: ''\n", []string{
			"field \"b\" pattern does not compile: error parsing regexp: missing closing ]: `[`",
			`field name "A" must be lowercase letters and _`,
			`field "c" pattern must be a non-empty string`,
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := decodeYAML([]byte(tt.yaml))
			if err != nil {
				t.Fatal(err)
			}
			cfg, problems, ok := configFrom(raw)
			if !ok || !reflect.DeepEqual(problems, tt.problems) {
				t.Fatalf("problems %q, want %q", problems, tt.problems)
			}
			var names []string
			for _, f := range cfg.Fields {
				names = append(names, f.Name)
			}
			if !reflect.DeepEqual(names, tt.names) {
				t.Errorf("fields %v, want %v", names, tt.names)
			}
		})
	}
}

// TestFieldPatternMatchesTheWholeValue pins the wrap the loader puts around a
// pattern: an unanchored pattern still has to match the whole value.
func TestFieldPatternMatchesTheWholeValue(t *testing.T) {
	raw, _ := decodeYAML([]byte("fields:\n  a: 'PRJ-[0-9]+'\n  b: '^[a-z]+$'\n"))
	cfg, _, _ := configFrom(raw)
	a, ok := cfg.FieldPattern("a")
	if !ok || !a.MatchString("PRJ-1") || a.MatchString("xPRJ-1") || a.MatchString("PRJ-1x") {
		t.Errorf("pattern a: %v %v", a, ok)
	}
	b, _ := cfg.FieldPattern("b")
	if !b.MatchString("abc") || b.MatchString("abc\n") {
		t.Errorf("pattern b must reject a trailing newline")
	}
	if _, ok := cfg.FieldPattern("c"); ok {
		t.Errorf("c has no pattern")
	}
}

func TestValidFieldValue(t *testing.T) {
	for v, want := range map[string]bool{"any text": true, "x": true, "": false, "a\nb": false, "a\x00b": false, "a\tb": false, "a\x7fb": false, "caf\u00e9": true} {
		if got := ValidFieldValue(v); got != want {
			t.Errorf("ValidFieldValue(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestLoadRejectsBadFields(t *testing.T) {
	fsys := testFS(nil)
	fsys["jitso/jitso.yaml"] = &fstest.MapFile{Data: []byte("version: 1\nfields:\n  Bad: x\n")}
	_, err := Load(fsys)
	if err == nil || err.Error() != `jitso/jitso.yaml is invalid: field name "Bad" must be lowercase letters and _` {
		t.Errorf("got %v", err)
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

func TestDecodeYAMLScopedNamesArePlainScalars(t *testing.T) {
	raw, err := decodeYAML([]byte("skills_first: [git-flow-master, user:humanizer]\nmcp:\n  - user:engram\n  - cloud:Slack\n"))
	if err != nil {
		t.Fatal(err)
	}
	m := raw.(*Map)
	if v, _ := m.Get("skills_first"); !reflect.DeepEqual(v, []any{"git-flow-master", "user:humanizer"}) {
		t.Errorf("skills_first = %#v", v)
	}
	if v, _ := m.Get("mcp"); !reflect.DeepEqual(v, []any{"user:engram", "cloud:Slack"}) {
		t.Errorf("mcp = %#v", v)
	}
}
