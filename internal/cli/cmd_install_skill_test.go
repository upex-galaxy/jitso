package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ids is the random source of the backup ids: the first draw is a3f9, and a
// collision with an earlier backup of the same second draws the next pair.
var ids = []byte{0xa3, 0xf9, 0x10, 0x20, 0x11, 0x21, 0x12, 0x22, 0x13, 0x23, 0x14, 0x24}

// installRun runs `jitso install ...` in cwd with the given HOME, standard
// input and terminal answer.
func installRun(t *testing.T, cwd, home, stdin string, terminal bool, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	env := map[string]string{}
	if home != "" {
		env["HOME"] = home
	}
	code := Run(append([]string{"install"}, args...), System{
		Stdin:           strings.NewReader(stdin),
		Stdout:          &out,
		Stderr:          &errOut,
		LookupEnv:       func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		Getwd:           func() (string, error) { return cwd, nil },
		Now:             func() time.Time { return time.Date(2026, 11, 15, 7, 34, 55, 0, time.UTC) },
		Rand:            bytes.NewReader(ids),
		StdinIsTerminal: func() bool { return terminal },
	})
	return code, out.String(), errOut.String()
}

func existsAt(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func TestInstallSkillUsageAnswers(t *testing.T) {
	cwd := t.TempDir()
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{"no word", nil, 2, "", "jitso install: missing what to install (skill)\n\nUsage: jitso install skill --scope user|project"},
		{"unknown word", []string{"plugin"}, 2, "", "jitso install: unknown subcommand \"plugin\"\n\nUsage: jitso install skill --scope user|project"},
		{"--help", []string{"--help"}, 0, installSkillUsage + "\n", ""},
		{"-h", []string{"-h"}, 0, installSkillUsage + "\n", ""},
		{"skill --help", []string{"skill", "--help"}, 0, installSkillUsage + "\n", ""},
		{"no scope", []string{"skill"}, 2, "", "JITSO REJECTED: install skill needs --scope user or --scope project\n"},
		{"bad scope", []string{"skill", "--scope", "team"}, 2, "", "JITSO REJECTED: --scope must be user or project\n"},
		{"scope without a value", []string{"skill", "--scope"}, 2, "", "JITSO REJECTED: --scope needs a value\n"},
		{"scope followed by a flag", []string{"skill", "--scope", "--yes"}, 2, "", "JITSO REJECTED: --scope needs a value\n"},
		{"a stray word", []string{"skill", "--scope", "user", "extra"}, 2, "", "JITSO REJECTED: unknown argument \"extra\"\n"},
		{"a flag of another command", []string{"skill", "--scope", "user", "--absolute"}, 2, "", "JITSO REJECTED: unknown argument \"--absolute\"\n"},
		{"the equals spelling", []string{"skill", "--scope=user"}, 2, "", "JITSO REJECTED: unknown argument \"--scope=user\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := installRun(t, cwd, filepath.Join(cwd, "home"), "", false, tt.args...)
			if code != tt.code || out != tt.stdout || !strings.HasPrefix(errOut, tt.stderr) {
				t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
			if existsAt(cwd, "home") || existsAt(cwd, ".agents") {
				t.Error("a usage error wrote something")
			}
		})
	}
}

func TestInstallSkillHomeNotSet(t *testing.T) {
	code, out, errOut := installRun(t, t.TempDir(), "", "", false, "skill", "--scope", "user", "--yes")
	if code != 3 || out != "" || errOut != "jitso: HOME is not set\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestInstallSkillProjectOutsideGit(t *testing.T) {
	if _, err := os.Stat("/dev/null"); err != nil {
		t.Skip()
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	code, out, errOut := installRun(t, dir, filepath.Join(dir, "home"), "", false, "skill", "--scope", "project", "--yes")
	if code != 3 || out != "" || !strings.Contains(errOut, "is not inside a git checkout") {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestInstallSkillUserScope(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	// From a folder that is no checkout: user scope needs no project.
	cwd := filepath.Join(home, "elsewhere")
	os.MkdirAll(cwd, 0o755)

	code, out, errOut := installRun(t, cwd, home, "", false, "skill", "--scope", "user", "--dry-run")
	if code != 0 || errOut != "" || !strings.Contains(out, "new file ~/.agents/skills/jitso/SKILL.md") || !strings.HasSuffix(out, "jitso install skill: dry run, nothing written\n") {
		t.Fatalf("dry run: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if existsAt(home, ".agents") || existsAt(home, ".claude") || existsAt(home, ".jitso") {
		t.Fatal("a dry run wrote")
	}

	code, out, errOut = installRun(t, cwd, home, "", false, "skill", "--scope", "user", "--yes")
	if code != 0 || errOut != "" || !strings.Contains(out, "created ~/.agents/skills/jitso/SKILL.md\n") || !strings.Contains(out, "linked ~/.claude/skills/jitso -> ../../.agents/skills/jitso\n") || !strings.HasSuffix(out, "jitso install skill: done\n") {
		t.Fatalf("install: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if strings.Contains(out, "note: commit") {
		t.Errorf("the commit note is for project scope only: %q", out)
	}
	if !strings.Contains(out, "note: sessions that jitso launches load project skills only; run jitso install skill --scope project in each project\n") {
		t.Errorf("the user scope note is missing: %q", out)
	}
	if !existsAt(home, ".agents/skills/jitso/.jitso-skill.json") || !existsAt(home, ".claude/skills/jitso/SKILL.md") {
		t.Fatal("the skill is not where Codex and Claude Code read it")
	}

	// A hand edit in HOME: refused, then forced with a backup under ~/.jitso.
	skillMD := filepath.Join(home, ".agents/skills/jitso/SKILL.md")
	os.WriteFile(skillMD, []byte("mine\n"), 0o644)
	code, out, errOut = installRun(t, cwd, home, "", false, "skill", "--scope", "user", "--yes")
	if code != 2 || !strings.Contains(errOut, "the jitso skill in ~/.agents/skills/jitso was edited by hand (SKILL.md)") || !strings.Contains(out, "--- a/~/.agents/skills/jitso/SKILL.md") {
		t.Fatalf("edit: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	code, out, errOut = installRun(t, cwd, home, "", false, "skill", "--scope", "user", "--yes", "--force")
	if code != 0 || !strings.Contains(out, "backed up ~/.agents/skills/jitso/SKILL.md to ~/.jitso/backups/20261115T073455Z-a3f9/.agents/skills/jitso/SKILL.md\n") {
		t.Fatalf("force: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if b, _ := os.ReadFile(filepath.Join(home, ".jitso/backups/20261115T073455Z-a3f9/.agents/skills/jitso/SKILL.md")); string(b) != "mine\n" {
		t.Errorf("backup holds %q", b)
	}

	code, out, errOut = installRun(t, cwd, home, "", false, "skill", "--scope", "user", "--remove", "--yes")
	if code != 0 || !strings.Contains(out, "unlinked ~/.claude/skills/jitso\n") {
		t.Fatalf("remove: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if existsAt(home, ".agents") || existsAt(home, ".claude") {
		t.Error("remove left folders behind")
	}
	code, out, _ = installRun(t, cwd, home, "", false, "skill", "--scope", "user", "--remove", "--yes")
	if code != 0 || out != "jitso install skill: nothing to do (the jitso skill is not installed at user scope)\n" {
		t.Errorf("second remove: exit %d, stdout %q", code, out)
	}
}

func TestInstallSkillProjectConfirmation(t *testing.T) {
	root := gitProject(t)
	home := filepath.Join(root, "no-home")
	args := []string{"skill", "--scope", "project"}

	// No terminal, no --yes: preview, refusal, nothing written.
	code, out, errOut := installRun(t, root, home, "y\n", false, args...)
	if code != 2 || !strings.Contains(out, "new file .agents/skills/jitso/SKILL.md") || !strings.Contains(errOut, "run it again with --yes") || existsAt(root, ".agents/skills") {
		t.Fatalf("no terminal: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	// A terminal that answers no, or nothing: refused, nothing written.
	for _, answer := range []string{"n\n", "\n", ""} {
		code, out, errOut = installRun(t, root, home, answer, true, args...)
		if code != 2 || !strings.HasSuffix(out, "Apply these changes? [y/N] ") || errOut != "JITSO REJECTED: not applied; nothing written\n" || existsAt(root, ".agents/skills") {
			t.Fatalf("answer %q: exit %d, stdout %q, stderr %q", answer, code, out, errOut)
		}
	}
	// A terminal that answers yes: applied, and the untracked paths get the commit note.
	code, out, errOut = installRun(t, root, home, "YES\n", true, args...)
	if code != 0 || errOut != "" || !strings.Contains(out, "created .agents/skills/jitso/SKILL.md\n") ||
		!strings.HasSuffix(out, "note: commit .agents/skills/jitso and .claude/skills/jitso so other clones get the skill\njitso install skill: done\n") {
		t.Fatalf("yes: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	// A dry run with --yes asks nothing and writes nothing.
	code, out, _ = installRun(t, root, home, "", false, "skill", "--scope", "project", "--remove", "--dry-run", "--yes")
	if code != 0 || !strings.HasSuffix(out, "jitso install skill: dry run, nothing written\n") || !existsAt(root, ".agents/skills/jitso/SKILL.md") {
		t.Fatalf("remove dry run: exit %d, stdout %q", code, out)
	}
}

// From a linked worktree the skill goes into that checkout, and the backups
// into the primary's .jitso.
func TestInstallSkillLinkedWorktree(t *testing.T) {
	root := gitProject(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git %v: %v: %s", args, err, out)
		}
	}
	git("-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "x")
	wt, _ := filepath.EvalSymlinks(t.TempDir())
	wt = filepath.Join(wt, "w")
	git("worktree", "add", "-q", "-b", "w", wt)
	wt, _ = filepath.EvalSymlinks(wt)

	home := filepath.Join(root, "no-home")
	if code, out, errOut := installRun(t, wt, home, "", false, "skill", "--scope", "project", "--yes"); code != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if !existsAt(wt, ".agents/skills/jitso/SKILL.md") || existsAt(root, ".agents/skills") || existsAt(root, ".claude") {
		t.Fatal("the skill is not in the worktree alone")
	}
	os.WriteFile(filepath.Join(wt, ".agents/skills/jitso/SKILL.md"), []byte("mine\n"), 0o644)
	code, out, errOut := installRun(t, wt, home, "", false, "skill", "--scope", "project", "--yes", "--force")
	if code != 0 {
		t.Fatalf("force: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if !existsAt(root, ".jitso/backups/20261115T073455Z-a3f9/.agents/skills/jitso/SKILL.md") || existsAt(wt, ".jitso") {
		t.Errorf("the backup is not in the primary's .jitso: %q", out)
	}
}
