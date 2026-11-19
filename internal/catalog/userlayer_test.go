package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func writeHome(t *testing.T, files map[string]string) string {
	t.Helper()
	home := t.TempDir()
	for rel, text := range files {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

func TestLoadUser(t *testing.T) {
	t.Run("no home is an empty layer", func(t *testing.T) {
		if u := LoadUser(""); u.Err != nil || u.Skills != nil {
			t.Errorf("got %+v", u)
		}
	})
	t.Run("missing file is an empty layer", func(t *testing.T) {
		if u := LoadUser(t.TempDir()); u.Err != nil || u.Skills != nil {
			t.Errorf("got %+v", u)
		}
	})
	t.Run("comments only is an empty layer", func(t *testing.T) {
		if u := LoadUser(writeHome(t, map[string]string{".jitso/jitso.yaml": "# nothing\n"})); u.Err != nil {
			t.Errorf("got %v", u.Err)
		}
	})
	t.Run("lists", func(t *testing.T) {
		u := LoadUser(writeHome(t, map[string]string{".jitso/jitso.yaml": "version: 1\nskills: [humanizer]\nmcp: [engram]\nliteral_env: [engram/MODE]\nmodels: [x]\n"}))
		if u.Err != nil || !reflect.DeepEqual(u.Skills, []string{"humanizer"}) || !reflect.DeepEqual(u.MCP, []string{"engram"}) || !reflect.DeepEqual(u.LiteralEnv, []string{"engram/MODE"}) {
			t.Errorf("got %+v", u)
		}
	})
	for name, text := range map[string]string{"syntax": "skills: [a\n", "not a mapping": "- a\n"} {
		t.Run(name, func(t *testing.T) {
			u := LoadUser(writeHome(t, map[string]string{".jitso/jitso.yaml": text}))
			if fail.Code(u.Err) != fail.CodeEnv || !strings.HasPrefix(fail.Text(u.Err), "jitso: ~/.jitso/jitso.yaml is invalid: ") {
				t.Errorf("got %v", u.Err)
			}
		})
	}
}

func TestSplitScope(t *testing.T) {
	for in, want := range map[string][3]string{
		"humanizer":      {"", "humanizer", "false"},
		"user:humanizer": {"user", "humanizer", "true"},
		"a:b:c":          {"a", "b:c", "true"},
		"user:":          {"user", "", "true"},
	} {
		scope, rest, scoped := SplitScope(in)
		if got := [3]string{scope, rest, map[bool]string{true: "true", false: "false"}[scoped]}; got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
	if name, ok := UserName("user:engram"); !ok || name != "engram" {
		t.Errorf("UserName: %q %v", name, ok)
	}
	if _, ok := UserName("cloud:Slack"); ok {
		t.Error("cloud: is not a user name")
	}
}

func TestValidateScopedNames(t *testing.T) {
	home := writeHome(t, map[string]string{
		".jitso/jitso.yaml":                      "skills: [humanizer, nofile, sprint-testing, local]\nmcp: [engram, undefined, dbhub]\n",
		".claude/skills/humanizer/SKILL.md":      "x",
		".claude/skills/sprint-testing/SKILL.md": "x",
		".claude/skills/local/SKILL.md":          "x",
		".claude.json":                           `{"mcpServers": {"engram": {"command": "e"}, "dbhub": {"command": "d"}}}`,
	})
	cfg := testConfig
	cfg.User = LoadUser(home)
	cfg.ProjectSkillDirs = []string{"local"}
	tests := []struct {
		edit string
		want []string
	}{
		{"skills_first: [sprint-testing, user:humanizer]", nil},
		{"mcp: [dbhub, user:engram]", nil},
		{"skills_first: [user:nope]", []string{`skill "user:nope" is not in the user allowlist (~/.jitso/jitso.yaml)`}},
		{"skills_first: [user:nofile]", []string{`user skill "nofile" has no SKILL.md under ~/.claude/skills/nofile`}},
		{"skills_first: [user:sprint-testing]", []string{`user skill "sprint-testing" has the name of a project skill`}},
		{"skills_first: [user:local]", []string{`user skill "local" has the name of a project skill`}},
		{"skills_first: [team:x]", []string{`skill "team:x": unknown scope (use user:)`}},
		{"mcp: [user:nope]", []string{`mcp "user:nope" is not in the user allowlist (~/.jitso/jitso.yaml)`}},
		{"mcp: [user:undefined]", []string{`user mcp "undefined" is not defined in ~/.claude.json`}},
		{"mcp: [user:dbhub]", []string{`user mcp "dbhub" has the name of a project mcp server`}},
		{"mcp: [cloud:Slack]", []string{`mcp "cloud:Slack": cloud connectors cannot be given to a launched session yet`}},
		{"mcp: [team:x]", []string{`mcp "team:x": unknown scope (use user:)`}},
	}
	for _, tt := range tests {
		t.Run(tt.edit, func(t *testing.T) {
			if got := ValidateBuild(with(t, tt.edit), cfg, testRouteIDs); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
	t.Run("a broken user file is the problem of a user name", func(t *testing.T) {
		broken := testConfig
		broken.User = LoadUser(writeHome(t, map[string]string{".jitso/jitso.yaml": "- a\n"}))
		got := ValidateBuild(with(t, "skills_first: [user:humanizer]"), broken, testRouteIDs)
		if want := []string{"~/.jitso/jitso.yaml is invalid: the top level is not a mapping"}; !reflect.DeepEqual(got, want) {
			t.Errorf("got %q", got)
		}
		if got := ValidateBuild(with(t, "skills_first: [sprint-testing]"), broken, testRouteIDs); got != nil {
			t.Errorf("a plain name ignores it, got %q", got)
		}
	})
}
