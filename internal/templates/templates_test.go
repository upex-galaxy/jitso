package templates

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/upex-galaxy/jitso/internal/catalog"
)

// fixtureRoot is the contract fixture project: its allowlists and its
// instruction sections are what a template is checked against.
const fixtureRoot = "../../contract/fixtures/default"

// projectFS is a project with the fixture's allowlists and instruction
// sections and one build file, jitso/builds/<name>, holding text.
func projectFS(t *testing.T, name, text string) fstest.MapFS {
	t.Helper()
	fsys := fstest.MapFS{}
	read := func(rel string) {
		data, err := os.ReadFile(filepath.Join(fixtureRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		fsys[rel] = &fstest.MapFile{Data: data}
	}
	read("jitso/jitso.yaml")
	entries, err := os.ReadDir(filepath.Join(fixtureRoot, ".agents/instructions"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		read(".agents/instructions/" + e.Name())
	}
	fsys["jitso/builds/"+name] = &fstest.MapFile{Data: []byte(text)}
	return fsys
}

func lines(text string) []string { return strings.Split(text, "\n") }

func TestTemplateShape(t *testing.T) {
	tests := []struct {
		tier string
		keys []string
	}{
		{"worker", []string{"id", "version", "status", "tier", "role", "type", "work_kind", "description", "harness", "model", "effort", "mode_default", "agents_md", "skills_first", "mcp", "subagents", "requires", "budget"}},
		{"conductor", []string{"id", "version", "status", "tier", "role", "type", "work_kind", "description", "harness", "model", "effort", "mode_default", "agents_md", "skills_first", "mcp", "subagents", "requires", "budget"}},
		{"subagent", []string{"id", "version", "status", "tier", "description", "prompt", "model", "effort"}},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			text, err := Build(tt.tier, "my-build")
			if err != nil {
				t.Fatal(err)
			}
			raw := map[string]string{"worker": worker, "conductor": conductor, "subagent": subagent}[tt.tier]
			count := 0
			for _, l := range lines(raw) {
				if l == IDLine {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("template has %d lines %q, want exactly 1", count, IDLine)
			}
			var got []string
			for _, l := range lines(text) {
				if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, " ") {
					continue
				}
				got = append(got, l[:strings.Index(l, ":")])
			}
			if strings.Join(got, ",") != strings.Join(tt.keys, ",") {
				t.Errorf("keys = %v, want %v", got, tt.keys)
			}
			for _, want := range []string{"id: my-build", "status: draft", "tier: " + tt.tier} {
				if !hasLine(text, want) {
					t.Errorf("no line %q in\n%s", want, text)
				}
			}
			if hasLine(text, IDLine) {
				t.Errorf("the id line was not replaced")
			}
		})
	}
}

func hasLine(text, want string) bool {
	for _, l := range lines(text) {
		if l == want {
			return true
		}
	}
	return false
}

// TestTemplatesAreFullLineCommentsOnly pins the documented YAML subset
// (docs/architecture.md section 8): a comment is a whole line, a template has
// no tab, no long dash and names no real model, skill or server.
func TestTemplatesAreFullLineCommentsOnly(t *testing.T) {
	for name, text := range map[string]string{"worker": worker, "conductor": conductor, "subagent": subagent, "jitso": config} {
		t.Run(name, func(t *testing.T) {
			for _, l := range lines(text) {
				if i := strings.Index(l, " #"); i >= 0 && !strings.HasPrefix(strings.TrimSpace(l), "#") {
					t.Errorf("inline comment in %q", l)
				}
			}
			for _, bad := range []string{"\t", "\u2014", "\u2013", "claude-", "sprint-testing", "dbhub"} {
				if strings.Contains(text, bad) {
					t.Errorf("template contains %q", bad)
				}
			}
			if !strings.HasPrefix(text, "# ") {
				t.Errorf("template does not start with a header comment")
			}
			if !strings.HasSuffix(text, "\n") {
				t.Errorf("template does not end with a newline")
			}
		})
	}
}

// TestUnfilledTemplateIsNeverValid renders each template, drops it into a
// project with the fixture's allowlists and loads the catalog: the load fails
// and names each empty value.
func TestUnfilledTemplateIsNeverValid(t *testing.T) {
	tests := []struct {
		tier string
		want []string
	}{
		{"worker", []string{`model "null" is not in the allowlist`, `effort "null" is not in the allowlist`}},
		{"conductor", []string{`model "null" is not in the allowlist`, `effort "null" is not in the allowlist`}},
		{"subagent", []string{"description must be a non-empty string", "prompt must be a non-empty string", `model "null" is not in the allowlist`, `effort "null" is not in the allowlist`}},
	}
	for _, tt := range tests {
		t.Run(tt.tier, func(t *testing.T) {
			text, err := Build(tt.tier, "my-build")
			if err != nil {
				t.Fatal(err)
			}
			_, err = catalog.Load(projectFS(t, "my-build.yaml", text))
			var fe *catalog.FileError
			if !errors.As(err, &fe) {
				t.Fatalf("Load error = %v, want a *catalog.FileError: an unfilled template must not be a valid build", err)
			}
			problems := strings.Join(fe.Problems, "\n")
			for _, want := range tt.want {
				if !strings.Contains(problems, want) {
					t.Errorf("problems do not name %q:\n%s", want, problems)
				}
			}
			if strings.Contains(problems, "id must be kebab-case") || strings.Contains(problems, "tier must be") {
				t.Errorf("the template has a wrong id or tier:\n%s", problems)
			}
		})
	}
}

func TestBuildRendersTheId(t *testing.T) {
	text, err := Build("worker", "smoke-checker")
	if err != nil {
		t.Fatal(err)
	}
	if !hasLine(text, "id: smoke-checker") || hasLine(text, IDLine) {
		t.Errorf("id not rendered:\n%s", text)
	}
	if strings.Count(text, "smoke-checker") != 1 {
		t.Errorf("the id appears %d times, want 1", strings.Count(text, "smoke-checker"))
	}
}

func TestBuildRefusesWhatCannotRender(t *testing.T) {
	for _, tt := range []struct{ tier, id string }{
		{"manager", "ok"},
		{"worker", ""},
		{"worker", "Upper"},
		{"worker", "a b"},
		{"worker", "a\nstatus: approved"},
		{"worker", "../x"},
		{"worker", "1abc"},
	} {
		if _, err := Build(tt.tier, tt.id); err == nil {
			t.Errorf("Build(%q, %q) succeeded", tt.tier, tt.id)
		}
	}
}

// TestConfigSkeleton decodes the jitso.yaml skeleton with the catalog's own
// loader: a mapping at version 1 that writes no list, so every one is open.
func TestConfigSkeleton(t *testing.T) {
	cfg, err := catalog.LoadConfig(fstest.MapFS{catalog.ConfigPath: &fstest.MapFile{Data: []byte(Config())}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %v, want 1", cfg.Version)
	}
	for name, list := range map[string][]string{"models": cfg.Models, "efforts": cfg.Efforts, "mcp": cfg.MCP, "skills": cfg.Skills, "tools": cfg.Tools} {
		if !catalog.Open(list) {
			t.Errorf("the skeleton restricts %s", name)
		}
		found := false
		for _, l := range lines(Config()) {
			if strings.HasPrefix(l, "# "+name+": [") {
				found = true
			}
		}
		if !found {
			t.Errorf("no comment shows how to restrict %q", name)
		}
	}
}
