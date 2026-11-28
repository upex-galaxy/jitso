package launch

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/catalog"
)

// worker is a worker build of the fixture with one line replaced or removed.
func worker(t *testing.T, id string, edits map[string]string) string {
	t.Helper()
	base, err := os.ReadFile("../../contract/fixtures/default/jitso/builds/haiku-explorer.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(base)
	text = replaceLine(text, "id: haiku-explorer", "id: "+id)
	for from, to := range edits {
		text = replaceLine(text, from, to)
	}
	return text
}

// replaceLine replaces the line that starts with prefix; an empty replacement
// removes the line.
func replaceLine(text, prefix, to string) string {
	var out []string
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		if strings.HasPrefix(line, prefix) {
			if to == "" {
				continue
			}
			line = to
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n") + "\n"
}

func TestProblems(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../contract/fixtures/default")); err != nil {
		t.Fatal(err)
	}
	subagent := "id: cand\nversion: 1\nstatus: candidate\ntier: subagent\ndescription: d\nprompt: p\nmodel: claude-haiku-5-5\neffort: low\n"
	builds := map[string]string{
		"cand.yaml":         subagent,
		"fine.yaml":         worker(t, "fine", nil),
		"no-budget.yaml":    worker(t, "no-budget", map[string]string{"budget:": ""}),
		"odd-harness.yaml":  worker(t, "odd-harness", map[string]string{"harness:": "harness: other"}),
		"no-harness.yaml":   worker(t, "no-harness", map[string]string{"harness:": ""}),
		"lists-cand.yaml":   worker(t, "lists-cand", map[string]string{"mcp:": "subagents: [cand, plan]\nmcp: []"}),
		"every-rule.yaml":   worker(t, "every-rule", map[string]string{"harness:": "harness: other", "mcp:": "subagents: [cand]\nmcp: []"}),
		"pi-fine.yaml":      worker(t, "pi-fine", map[string]string{"harness:": "harness: pi"}),
		"pi-asks.yaml":      worker(t, "pi-asks", map[string]string{"harness:": "harness: pi", "mcp:": "subagents: [cand]\nmcp: [dbhub]"}),
		"codex-asks.yaml":   worker(t, "codex-asks", map[string]string{"harness:": "harness: codex"}),
		"codex-cloud.yaml":  worker(t, "codex-cloud", map[string]string{"harness:": "harness: codex", "tools:": "", "mcp:": "mcp: [dbhub, 'cloud:Atlas']"}),
		"pi-cloud.yaml":     worker(t, "pi-cloud", map[string]string{"harness:": "harness: pi", "mcp:": "mcp: ['cloud:Atlas']"}),
		"claude-cloud.yaml": worker(t, "claude-cloud", map[string]string{"mcp:": "mcp: [dbhub, 'cloud:Atlas']"}),
		"opencode-ok.yaml":  worker(t, "opencode-ok", map[string]string{"harness:": "harness: opencode", "agents_md:": "agents_md: full", "tools:": ""}),
	}
	for name, text := range builds {
		if err := os.WriteFile(filepath.Join(root, "jitso", "builds", name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res, err := catalog.Scan(os.DirFS(root), "")
	if err != nil {
		t.Fatal(err)
	}
	cat := res.Tolerant()

	tests := []struct {
		id   string
		want []string
	}{
		{"haiku-explorer", nil},
		{"fine", nil},
		{"cand", nil}, // a subagent-tier build needs no harness
		{"plan", nil},
		{"no-budget", nil}, // the budget is optional
		{"odd-harness", []string{`harness "other" has no adapter`}},
		{"no-harness", nil}, // no harness key is the default one
		{"lists-cand", []string{`subagent "cand" is candidate, not approved`}},
		// the order of the lines: harness, then the subagents
		{"every-rule", []string{
			`harness "other" has no adapter`,
			`subagent "cand" is candidate, not approved`,
		}},
		{"pi-fine", nil}, // pi honors tools and agents_md: exclude
		// the support lines come before the subagent status lines
		{"pi-asks", []string{
			`harness "pi" cannot give a session subagents`,
			`harness "pi" cannot give a session MCP servers`,
			`subagent "cand" is candidate, not approved`,
		}},
		{"codex-asks", []string{`harness "codex" cannot limit the tools of a session`}},
		{"opencode-ok", nil},
		{"claude-cloud", nil},
		{"codex-cloud", []string{`harness "codex" cannot give a session the claude.ai connector "cloud:Atlas"`}},
		{"pi-cloud", []string{`harness "pi" cannot give a session MCP servers`}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			f := res.Winner(tt.id)
			if f == nil {
				t.Fatalf("no file with id %s", tt.id)
			}
			b := f.Build()
			if b == nil {
				t.Fatalf("%s is not valid: %v", tt.id, f.Problems)
			}
			if got := Problems(b, cat); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Problems(%s) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

// Launch and Problems agree on the harness: a build that Problems accepts has
// an adapter for Run.
func TestAdapterOfAgreesWithAdapters(t *testing.T) {
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../contract/fixtures/default")); err != nil {
		t.Fatal(err)
	}
	res, err := catalog.Scan(os.DirFS(root), "")
	if err != nil {
		t.Fatal(err)
	}
	b := res.Winner("haiku-explorer").Build()
	a, name, ok := adapterOf(b)
	if !ok || name != "claude" || a == nil {
		t.Fatalf("adapterOf = %v, %q, %v", a, name, ok)
	}
}
