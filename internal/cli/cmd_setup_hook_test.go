package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/settingsjson"
)

const setupCanonical = `{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      },
      {
        "matcher": "mcp__.*orca.*__.*",
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      }
    ],
    "SessionEnd": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "jitso hook guard"
          }
        ]
      }
    ]
  }
}
`

const setupUserSeed = `{
  "env": {
    "API_KEY": "ENV-CANARY-9"
  },
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "./notify.sh TOKEN-CANARY-7"
          }
        ]
      }
    ]
  }
}
`

// setupRepo is a fresh git checkout in a temp folder (real path).
func setupRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v: %s", err, out)
	}
	return root
}

// setupGit runs git in dir with a fixed identity and no signing.
func setupGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// setupBinary is a stand-in for the running executable: a real file, so its
// path resolves.
func setupBinary(t *testing.T) string {
	t.Helper()
	if path, ok := setupBinaries[t.Name()]; ok {
		return path
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "jitso")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	setupBinaries[t.Name()] = path
	return path
}

// setupBinaries keeps one stand-in per test, so two runs of a test see the
// same executable path.
var setupBinaries = map[string]string{}

// setupRun runs `jitso <args>` from dir, with the given standard input and
// whether it is a terminal. The jitso word is reported as not on PATH unless
// onPath is set through setupOnPath.
func setupRun(t *testing.T, dir, stdin string, terminal bool, args ...string) (int, string, string) {
	t.Helper()
	return setupRunHome(t, dir, "", stdin, terminal, args...)
}

// setupRunHome is setupRun with $HOME set to home ("" leaves it unset).
func setupRunHome(t *testing.T, dir, home, stdin string, terminal bool, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	exe := setupBinary(t)
	var seed byte
	code := Run(args, System{
		Stdin:     strings.NewReader(stdin),
		Stdout:    &out,
		Stderr:    &errOut,
		LookupEnv: func(k string) (string, bool) { return home, k == "HOME" && home != "" },
		Getwd:     func() (string, error) { return dir, nil },
		Executable: func() (string, error) {
			return exe, nil
		},
		Now:             func() time.Time { return time.Date(2026, 11, 15, 7, 34, 55, 0, time.UTC) },
		Rand:            readerFunc(func(p []byte) (int, error) { seed++; p[0], p[1] = seed, 0xf9; return 2, nil }),
		StdinIsTerminal: func() bool { return terminal },
	})
	return code, out.String(), errOut.String()
}

type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

// setupPatch replaces the PATH lookup and the git-tracked question for one test.
func setupPatch(t *testing.T, onPath, tracked bool) {
	t.Helper()
	oldLook, oldTracked := setupLookPath, setupGitTracked
	t.Cleanup(func() { setupLookPath, setupGitTracked = oldLook, oldTracked })
	setupLookPath = func(file string) (string, error) {
		if onPath {
			return "/usr/local/bin/" + file, nil
		}
		return "", errors.New("not found")
	}
	setupGitTracked = func(string, string) bool { return tracked }
}

func setupWrite(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupRead(root, rel string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	return string(data), err == nil
}

func TestSetupUsageAnswers(t *testing.T) {
	root := setupRepo(t)
	const head = "Usage: jitso setup hook [--scope project|user] [--absolute] [--remove] [--dry-run] [--yes]"
	noValue := "JITSO REJECTED: --scope needs a value\n"
	badValue := "JITSO REJECTED: --scope must be user or project\n"
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string // a prefix; "" means empty
		stderr string // a prefix; "" means empty
	}{
		{"missing", []string{"setup"}, 2, "", "jitso setup: missing what to set up (hook, instructions)\n\n" + head},
		{"help", []string{"setup", "--help"}, 0, head, ""},
		{"short help", []string{"setup", "-h"}, 0, head, ""},
		{"hook help", []string{"setup", "hook", "--help"}, 0, head, ""},
		{"hook short help", []string{"setup", "hook", "-h"}, 0, head, ""},
		{"unknown subcommand", []string{"setup", "hooks"}, 2, "", "jitso setup: unknown subcommand \"hooks\"\n\n" + head},
		{"scope without a value", []string{"setup", "hook", "--scope"}, 2, "", noValue},
		{"scope with a bad value", []string{"setup", "hook", "--scope", "elsewhere"}, 2, "", badValue},
		{"scope equals", []string{"setup", "hook", "--scope=user"}, 2, "", "JITSO REJECTED: unknown argument \"--scope=user\"\n"},
		{"a stray word", []string{"setup", "hook", "extra"}, 2, "", "JITSO REJECTED: unknown argument \"extra\"\n"},
		{"an unknown flag", []string{"setup", "hook", "--bogus"}, 2, "", "JITSO REJECTED: unknown argument \"--bogus\"\n"},
		{"a flag of another command", []string{"setup", "hook", "--force"}, 2, "", "JITSO REJECTED: unknown argument \"--force\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := setupRun(t, root, "", false, tt.args...)
			if code != tt.code || !strings.HasPrefix(out, tt.stdout) || (tt.stdout == "") != (out == "") ||
				!strings.HasPrefix(errOut, tt.stderr) || (tt.stderr == "") != (errOut == "") {
				t.Errorf("exit %d\nstdout %q\nstderr %q", code, out, errOut)
			}
			if _, ok := setupRead(root, ".claude/settings.json"); ok {
				t.Error("a usage answer wrote the file")
			}
		})
	}
}

func TestSetupHookCreatesTheFileAndWarnsAboutPath(t *testing.T) {
	root := setupRepo(t)
	setupPatch(t, false, false)
	code, out, errOut := setupRun(t, root, "", false, "setup", "hook", "--yes")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if got, _ := setupRead(root, ".claude/settings.json"); got != setupCanonical {
		t.Errorf("file:\n%s", got)
	}
	for _, want := range []string{
		"--- /dev/null\n+++ b/.claude/settings.json\n",
		"created .claude/settings.json\n",
		"note: commit .claude/settings.json so linked worktrees and other clones get the guard\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if !strings.HasSuffix(out, "jitso setup hook: done\n") {
		t.Errorf("stdout does not end with the done line:\n%s", out)
	}
	if want := "jitso: warning: jitso is not on PATH, so the hook cannot start until it is (the guard then lets every call through)\n"; errOut != want {
		t.Errorf("stderr %q", errOut)
	}
	if _, ok := setupRead(root, ".jitso/backups/.gitignore"); ok {
		t.Error("a new file made a backup folder")
	}
}

func TestSetupHookNoWarningWhenOnPathAndNoNoteWhenTracked(t *testing.T) {
	root := setupRepo(t)
	setupPatch(t, true, true)
	code, out, errOut := setupRun(t, root, "", false, "setup", "hook", "--yes")
	if code != 0 || errOut != "" || strings.Contains(out, "note:") {
		t.Errorf("exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
}

func TestSetupHookDryRunAndConfirmation(t *testing.T) {
	setupPatch(t, true, false)
	root := setupRepo(t)

	// A dry run writes nothing, with or without --yes.
	for _, args := range [][]string{{"setup", "hook", "--dry-run"}, {"setup", "hook", "--dry-run", "--yes"}} {
		code, out, errOut := setupRun(t, root, "", false, args...)
		if code != 0 || errOut != "" || !strings.HasSuffix(out, "jitso setup hook: dry run, nothing written\n") {
			t.Fatalf("%v: exit %d\nstdout %q\nstderr %q", args, code, out, errOut)
		}
		if _, err := os.Stat(filepath.Join(root, ".claude")); err == nil {
			t.Fatalf("%v wrote .claude", args)
		}
	}

	// No --yes and no terminal: the preview, then a refusal.
	code, out, errOut := setupRun(t, root, "y\n", false, "setup", "hook")
	want := "JITSO REJECTED: this changes files: read the preview above, then run it again with --yes, or in a terminal to answer the prompt; nothing written\n"
	if code != 2 || errOut != want || !strings.Contains(out, "+++ b/.claude/settings.json") || strings.Contains(out, "created") {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}

	// A terminal: n declines, y applies.
	code, out, errOut = setupRun(t, root, "n\n", true, "setup", "hook")
	if code != 2 || errOut != "JITSO REJECTED: not applied; nothing written\n" || !strings.Contains(out, "Apply these changes? [y/N] ") {
		t.Fatalf("n: exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
	if _, ok := setupRead(root, ".claude/settings.json"); ok {
		t.Fatal("a declined prompt wrote the file")
	}
	code, out, errOut = setupRun(t, root, "yes\n", true, "setup", "hook")
	if code != 0 || errOut != "" || !strings.Contains(out, "created .claude/settings.json") {
		t.Fatalf("y: exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
}

func TestSetupHookPreviewNeverPrintsTheUsersLines(t *testing.T) {
	setupPatch(t, true, true)
	root := setupRepo(t)
	setupWrite(t, root, ".claude/settings.json", setupUserSeed)
	code, out, errOut := setupRun(t, root, "", false, "setup", "hook", "--dry-run")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	for _, secret := range []string{"TOKEN-CANARY-7", "ENV-CANARY-9", "API_KEY", "notify.sh"} {
		if strings.Contains(out, secret) {
			t.Errorf("the preview printed %q:\n%s", secret, out)
		}
	}
	if !strings.Contains(out, "@@") || !strings.Contains(out, `+        "matcher": "mcp__.*orca.*__.*",`) {
		t.Errorf("the preview is not a diff of the additions:\n%s", out)
	}
}

func TestSetupHookMergeBackupSecondRunAndRemove(t *testing.T) {
	setupPatch(t, true, true)
	root := setupRepo(t)
	setupWrite(t, root, ".claude/settings.json", setupUserSeed)
	setupWrite(t, root, ".claude/settings.local.json", "not json LOCAL-SECRET")

	code, out, errOut := setupRun(t, root, "", false, "setup", "hook", "--yes")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	re := regexp.MustCompile(`backed up \.claude/settings\.json to (\.jitso/backups/20261115T073455Z-[0-9a-f]{4}/\.claude/settings\.json)\nwrote \.claude/settings\.json\n`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("stdout:\n%s", out)
	}
	if saved, _ := setupRead(root, m[1]); saved != setupUserSeed {
		t.Errorf("the backup differs from the old file:\n%s", saved)
	}
	merged, _ := setupRead(root, ".claude/settings.json")
	if !strings.Contains(merged, `"env": {`) || strings.Index(merged, `"Stop"`) > strings.Index(merged, `"PreToolUse"`) {
		t.Errorf("the user's keys moved:\n%s", merged)
	}
	if !settingsjson.State([]byte(merged)).Registered() {
		t.Errorf("the guard is not registered:\n%s", merged)
	}
	if gi, _ := setupRead(root, ".jitso/backups/.gitignore"); gi != "*\n" {
		t.Errorf("backups .gitignore = %q", gi)
	}

	// Second run: nothing to do, nothing new in the backups.
	code, out, errOut = setupRun(t, root, "", false, "setup", "hook", "--yes")
	if code != 0 || errOut != "" || out != "jitso setup hook: nothing to do (the guard is already registered in .claude/settings.json)\n" {
		t.Fatalf("second run: exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
	if after, _ := setupRead(root, ".claude/settings.json"); after != merged {
		t.Error("the second run changed the file")
	}
	entries, _ := os.ReadDir(filepath.Join(root, ".jitso", "backups"))
	if len(entries) != 2 { // .gitignore and the one backup
		t.Errorf("backups folder has %d entries", len(entries))
	}

	// Remove: the original bytes come back, the local file is untouched.
	code, _, errOut = setupRun(t, root, "", false, "setup", "hook", "--remove", "--yes")
	if code != 0 || errOut != "" {
		t.Fatalf("remove: exit %d, stderr %q", code, errOut)
	}
	if back, _ := setupRead(root, ".claude/settings.json"); back != setupUserSeed {
		t.Errorf("remove did not restore the file:\n%s", back)
	}
	if local, _ := setupRead(root, ".claude/settings.local.json"); local != "not json LOCAL-SECRET" {
		t.Errorf("settings.local.json changed: %q", local)
	}
	if strings.Contains(out, "LOCAL-SECRET") || strings.Contains(errOut, "LOCAL-SECRET") {
		t.Error("settings.local.json leaked")
	}

	// Remove again: nothing to do.
	code, out, _ = setupRun(t, root, "", false, "setup", "hook", "--remove", "--yes")
	if code != 0 || out != "jitso setup hook: nothing to do (no jitso hook entries in .claude/settings.json)\n" {
		t.Errorf("remove again: exit %d, stdout %q", code, out)
	}
}

func TestSetupHookRemoveDeletesTheFileAndTheEmptyFolder(t *testing.T) {
	setupPatch(t, true, true)
	root := setupRepo(t)
	if code, _, errOut := setupRun(t, root, "", false, "setup", "hook", "--yes"); code != 0 {
		t.Fatalf("setup: %d %q", code, errOut)
	}
	code, out, errOut := setupRun(t, root, "", false, "setup", "hook", "--remove", "--dry-run")
	if code != 0 || errOut != "" || !strings.Contains(out, "--- a/.claude/settings.json\n+++ /dev/null\n") {
		t.Fatalf("dry run: exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
	if _, ok := setupRead(root, ".claude/settings.json"); !ok {
		t.Fatal("the dry run removed the file")
	}
	code, out, errOut = setupRun(t, root, "", false, "setup", "hook", "--remove", "--yes")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	for _, want := range []string{"backed up .claude/settings.json to ", "removed .claude/settings.json\n", "removed .claude\n", "jitso setup hook: done\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".claude")); err == nil {
		t.Error(".claude is still there")
	}
}

func TestSetupHookAbsolute(t *testing.T) {
	setupPatch(t, false, true)
	root := setupRepo(t)
	code, _, errOut := setupRun(t, root, "", false, "setup", "hook", "--absolute", "--yes")
	if code != 0 || errOut != "" { // an absolute command needs no PATH
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	got, _ := setupRead(root, ".claude/settings.json")
	re := regexp.MustCompile(`"command": "('[^"]*/jitso') hook guard"`)
	all := re.FindAllStringSubmatch(got, -1)
	if len(all) != 3 {
		t.Fatalf("want 3 absolute handlers:\n%s", got)
	}
	// A second --absolute run is nothing to do ...
	if code, out, _ := setupRun(t, root, "", false, "setup", "hook", "--absolute", "--yes"); code != 0 || !strings.Contains(out, "nothing to do") {
		t.Errorf("second absolute run: %d %q", code, out)
	}
	// ... and a plain run goes back to the bare word.
	setupPatch(t, true, true)
	if code, _, _ := setupRun(t, root, "", false, "setup", "hook", "--yes"); code != 0 {
		t.Fatalf("plain run: %d", code)
	}
	if got, _ = setupRead(root, ".claude/settings.json"); got != setupCanonical {
		t.Errorf("the bare word did not come back:\n%s", got)
	}
}

func TestSetupHookRefusesWhatItCannotEdit(t *testing.T) {
	setupPatch(t, true, true)
	tests := []struct {
		name string
		seed string
		want string
	}{
		{"not json", "{", ".claude/settings.json is not valid JSON ("},
		{"array", "[]", ".claude/settings.json must hold a JSON object; nothing written"},
		{"hooks is an array", `{"hooks": []}`, `"hooks" in .claude/settings.json must be an object; nothing written`},
		{"duplicate key", `{"a":1,"a":2}`, `.claude/settings.json has the key "a" twice at "."; nothing written`},
		{"foreign layout", "{\n  \"hooks\": {\n    \"Stop\": [{\"hooks\": []}]\n  }\n}\n", "is not laid out the way jitso writes JSON (one value per line, a consistent indent); nothing written. Add these entries by hand, or re-save the file with a 2-space indent and run again:\n{\n  \"hooks\": {"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := setupRepo(t)
			setupWrite(t, root, ".claude/settings.json", tt.seed)
			code, out, errOut := setupRun(t, root, "", false, "setup", "hook", "--yes")
			if code != 3 || out != "" || !strings.HasPrefix(errOut, "jitso: ") || !strings.Contains(errOut, tt.want) {
				t.Fatalf("exit %d\nstdout %q\nstderr %q", code, out, errOut)
			}
			if got, _ := setupRead(root, ".claude/settings.json"); got != tt.seed {
				t.Errorf("the file changed: %q", got)
			}
			if _, err := os.Stat(filepath.Join(root, ".jitso")); err == nil {
				t.Error("a refusal made a .jitso folder")
			}
		})
	}
}

func TestSetupHookOutsideGit(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	code, out, errOut := setupRun(t, dir, "", false, "setup", "hook", "--yes")
	if code != 3 || out != "" || !strings.Contains(errOut, "is not inside a git checkout") {
		t.Errorf("exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
}

func TestSetupHookLinkedWorktree(t *testing.T) {
	setupPatch(t, true, false)
	primary := setupRepo(t)
	setupGit(t, primary, "commit", "--allow-empty", "-q", "-m", "init")
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(parent, "wt")
	setupGit(t, primary, "worktree", "add", "-q", "-b", "wt", wt)

	if code, _, errOut := setupRun(t, wt, "", false, "setup", "hook", "--yes"); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if _, ok := setupRead(wt, ".claude/settings.json"); !ok {
		t.Error("the worktree has no settings file")
	}
	if _, err := os.Stat(filepath.Join(primary, ".claude")); err == nil {
		t.Error("the primary got a .claude folder")
	}
	code, out, errOut := setupRun(t, wt, "", false, "setup", "hook", "--remove", "--yes")
	if code != 0 || errOut != "" {
		t.Fatalf("remove: exit %d, stderr %q", code, errOut)
	}
	want := "backed up .claude/settings.json to " + primary + "/.jitso/backups/"
	if !strings.Contains(out, want) {
		t.Errorf("stdout lacks %q:\n%s", want, out)
	}
	if _, err := os.Stat(filepath.Join(wt, ".jitso")); err == nil {
		t.Error("the worktree got a .jitso folder")
	}
	entries, _ := os.ReadDir(filepath.Join(primary, ".jitso", "backups"))
	if len(entries) != 2 {
		t.Errorf("primary backups has %d entries", len(entries))
	}
}

func TestSetupHookUserScope(t *testing.T) {
	setupPatch(t, true, false)
	root := setupRepo(t)
	home := t.TempDir()

	code, out, errOut := setupRunHome(t, root, home, "", false, "setup", "hook", "--scope", "user", "--yes")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
	if got, _ := setupRead(home, ".claude/settings.json"); got != setupCanonical {
		t.Errorf("user file:\n%s", got)
	}
	if _, ok := setupRead(root, ".claude/settings.json"); ok {
		t.Error("user scope wrote the project file")
	}
	if !strings.Contains(out, "created ~/.claude/settings.json\n") || strings.Contains(out, "note:") {
		t.Errorf("stdout:\n%s", out)
	}

	code, out, _ = setupRunHome(t, root, home, "", false, "setup", "hook", "--scope", "user", "--yes")
	if code != 0 || !strings.Contains(out, "nothing to do (the guard is already registered in ~/.claude/settings.json)") {
		t.Errorf("second run: exit %d\n%s", code, out)
	}

	code, out, _ = setupRunHome(t, root, home, "", false, "setup", "hook", "--scope", "user", "--remove", "--yes")
	if code != 0 || !strings.Contains(out, "removed ~/.claude/settings.json") {
		t.Errorf("remove: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("~/.claude is left behind: %v", err)
	}
}

func TestSetupHookUserScopeKeepsTheUsersFileAndBacksItUp(t *testing.T) {
	setupPatch(t, true, false)
	root := setupRepo(t)
	home := t.TempDir()
	setupWrite(t, home, ".claude/settings.json", setupUserSeed)

	code, out, errOut := setupRunHome(t, root, home, "", false, "setup", "hook", "--scope", "user", "--yes")
	if code != 0 {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, out, errOut)
	}
	if strings.Contains(out, "ENV-CANARY-9") {
		t.Errorf("the preview printed a value of the user's file:\n%s", out)
	}
	if _, ok := setupRead(home, ".jitso/backups/.gitignore"); !ok {
		t.Error("no backup folder under ~/.jitso")
	}
	if code, _, _ = setupRunHome(t, root, home, "", false, "setup", "hook", "--scope", "user", "--remove", "--yes"); code != 0 {
		t.Fatalf("remove exit %d", code)
	}
	if got, _ := setupRead(home, ".claude/settings.json"); got != setupUserSeed {
		t.Errorf("round trip:\n%s", got)
	}
}

func TestSetupHookUserScopeNeedsHome(t *testing.T) {
	root := setupRepo(t)
	code, _, errOut := setupRunHome(t, root, "", "", false, "setup", "hook", "--scope", "user", "--yes")
	if code != 3 || errOut != "jitso: HOME is not set\n" {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}
