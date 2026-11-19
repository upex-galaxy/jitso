package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/catalog"
)

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadSkill(t *testing.T) {
	t.Run("files and hash", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"SKILL.md": "x\n", "references/a.md": "a\n"})
		s, err := readSkill(dir)
		if err != nil || s == nil {
			t.Fatalf("%v %v", s, err)
		}
		var rels []string
		for _, f := range s.files {
			rels = append(rels, f.rel)
		}
		if !reflect.DeepEqual(rels, []string{"SKILL.md", "references/a.md"}) {
			t.Errorf("files %v", rels)
		}
		if s.sha256 != treeHash([]packageFile{{rel: "references/a.md", data: []byte("a\n")}, {rel: "SKILL.md", data: []byte("x\n")}}) {
			t.Error("the hash does not depend on the walk order")
		}
		if len(s.sha256) != 64 {
			t.Errorf("hash %q", s.sha256)
		}
	})
	t.Run("file cap", func(t *testing.T) {
		dir := t.TempDir()
		files := map[string]string{}
		for i := 0; i <= MaxSkillFiles; i++ {
			files[fmt.Sprintf("p/%04d.md", i)] = ""
		}
		writeTree(t, dir, files)
		if s, err := readSkill(dir); err != nil || s != nil {
			t.Errorf("want too large, got %v %v", s, err)
		}
	})
	t.Run("byte cap", func(t *testing.T) {
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{"SKILL.md": strings.Repeat("x", MaxSkillBytes)})
		if s, err := readSkill(dir); err != nil || s == nil {
			t.Fatalf("exactly the cap is carried: %v %v", s, err)
		}
		writeTree(t, dir, map[string]string{"extra.md": "y"})
		if s, err := readSkill(dir); err != nil || s != nil {
			t.Errorf("one byte over is refused, got %v %v", s, err)
		}
	})
	t.Run("a link to a folder inside is left out", func(t *testing.T) {
		dir, other := t.TempDir(), t.TempDir()
		writeTree(t, dir, map[string]string{"SKILL.md": "x"})
		writeTree(t, other, map[string]string{"secret.md": "s"})
		if err := os.Symlink(other, filepath.Join(dir, "elsewhere")); err != nil {
			t.Skip(err)
		}
		s, err := readSkill(dir)
		if err != nil || len(s.files) != 1 {
			t.Errorf("got %v %v", s, err)
		}
	})
}

func TestSessionSkillNames(t *testing.T) {
	got := SessionSkillNames([]string{"sprint-testing", "user:humanizer"})
	if !reflect.DeepEqual(got, []string{"sprint-testing", "humanizer"}) {
		t.Errorf("got %v", got)
	}
}

func TestPlanPackage(t *testing.T) {
	bin := t.TempDir()
	writeTree(t, bin, map[string]string{"jitso": ""})
	binPath, _ := filepath.EvalSymlinks(filepath.Join(bin, "jitso"))
	self := func() (string, error) { return binPath, nil }
	registered := `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "jitso hook guard"}]},
{"matcher": "mcp__.*orca.*__.*", "hooks": [{"type": "command", "command": "jitso hook guard"}]}],
"SessionEnd": [{"hooks": [{"type": "command", "command": "jitso hook guard"}]}]}}`
	home := t.TempDir()
	writeTree(t, home, map[string]string{".claude/skills/jitso/SKILL.md": "user copy\n"})
	user := catalog.LoadUser(home)
	tests := []struct {
		name       string
		files      map[string]string
		build      catalog.Build
		guard      string
		jitsoSkill bool
		userSkills int
	}{
		{"worker in a bare project carries the guard", nil, catalog.Build{Tier: catalog.TierWorker}, "'" + binPath + "' hook guard", false, 0},
		{"project registers the guard", map[string]string{".claude/settings.json": registered}, catalog.Build{Tier: catalog.TierWorker}, "", false, 0},
		{"conductor gets the skill", map[string]string{".claude/settings.json": registered}, catalog.Build{Tier: catalog.TierConductor}, "", true, 0},
		{"a worker that names jitso gets it", map[string]string{".claude/settings.json": registered}, catalog.Build{Tier: catalog.TierWorker, SkillsFirst: []string{"jitso"}}, "", true, 0},
		{"a project copy wins", map[string]string{".claude/settings.json": registered, ".claude/skills/jitso/SKILL.md": "x"}, catalog.Build{Tier: catalog.TierConductor}, "", false, 0},
		{"the user's copy replaces the embedded one", map[string]string{".claude/settings.json": registered}, catalog.Build{Tier: catalog.TierConductor, SkillsFirst: []string{"user:jitso"}}, "", false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checkout := t.TempDir()
			writeTree(t, checkout, tt.files)
			b := tt.build
			pkg, err := planPackage(&b, user, checkout, self)
			if err != nil {
				t.Fatal(err)
			}
			if pkg.GuardCommand != tt.guard || pkg.JitsoSkill != tt.jitsoSkill || len(pkg.Skills) != tt.userSkills {
				t.Errorf("got guard %q, jitso %v, %d user skills", pkg.GuardCommand, pkg.JitsoSkill, len(pkg.Skills))
			}
		})
	}
}
