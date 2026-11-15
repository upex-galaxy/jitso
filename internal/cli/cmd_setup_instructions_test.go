package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/instructions"
)

// setupInstrRun runs `jitso setup instructions <args>` from dir with a home
// folder and an optional CODEX_HOME; stdin is not a terminal.
func setupInstrRun(t *testing.T, dir, home, codex string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	var seed byte
	code := Run(append([]string{"setup", "instructions"}, args...), System{
		Stdin:  strings.NewReader(""),
		Stdout: &out,
		Stderr: &errOut,
		LookupEnv: func(k string) (string, bool) {
			switch k {
			case "HOME":
				return home, home != ""
			case "CODEX_HOME":
				return codex, codex != ""
			}
			return "", false
		},
		Getwd:           func() (string, error) { return dir, nil },
		Executable:      func() (string, error) { return setupBinary(t), nil },
		Now:             func() time.Time { return time.Date(2026, 11, 15, 7, 34, 55, 0, time.UTC) },
		Rand:            readerFunc(func(p []byte) (int, error) { seed++; p[0], p[1] = seed, 0xf9; return 2, nil }),
		StdinIsTerminal: func() bool { return false },
	})
	return code, out.String(), errOut.String()
}

func TestSetupInstructionsProjectRoundTrip(t *testing.T) {
	for name, seed := range map[string]string{
		"empty": "", "no newline": "# A", "newline": "# A\n", "crlf": "# A\r\n", "blank lines": "# A\n\n\n",
	} {
		t.Run(name, func(t *testing.T) {
			root := setupRepo(t)
			setupWrite(t, root, "AGENTS.md", seed)
			if code, out, errOut := setupInstrRun(t, root, "", "", "--yes"); code != 0 {
				t.Fatalf("add: exit %d\n%s\n%s", code, out, errOut)
			}
			got, _ := setupRead(root, "AGENTS.md")
			if !strings.Contains(got, "BEGIN JITSO block=1") {
				t.Fatalf("no block: %q", got)
			}
			if code, out, errOut := setupInstrRun(t, root, "", "", "--remove", "--yes"); code != 0 {
				t.Fatalf("remove: exit %d\n%s\n%s", code, out, errOut)
			}
			if got, _ := setupRead(root, "AGENTS.md"); got != seed {
				t.Errorf("after the round trip: %q, want %q", got, seed)
			}
		})
	}
}

func TestSetupInstructionsCreatesAndDeletes(t *testing.T) {
	root := setupRepo(t)
	code, out, _ := setupInstrRun(t, root, "", "", "--yes")
	if code != 0 || !strings.Contains(out, "created AGENTS.md\n") || !strings.HasSuffix(out, "jitso setup instructions: done\n") {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	code, out, _ = setupInstrRun(t, root, "", "", "--yes")
	if code != 0 || out != "jitso setup instructions: nothing to do (the JITSO block is up to date in AGENTS.md)\n" {
		t.Fatalf("second run: exit %d, stdout %q", code, out)
	}
	code, out, _ = setupInstrRun(t, root, "", "", "--remove", "--yes")
	if code != 0 || !strings.Contains(out, "removed AGENTS.md\n") {
		t.Fatalf("remove: exit %d, stdout %q", code, out)
	}
	if _, ok := setupRead(root, "AGENTS.md"); ok {
		t.Errorf("AGENTS.md should be gone")
	}
}

func TestSetupInstructionsConfirmation(t *testing.T) {
	root := setupRepo(t)
	code, out, errOut := setupInstrRun(t, root, "", "")
	if code != 2 || !strings.Contains(out, "+++ b/AGENTS.md") || !strings.Contains(errOut, "run it again with --yes") {
		t.Fatalf("no --yes: exit %d\n%s\n%s", code, out, errOut)
	}
	code, out, _ = setupInstrRun(t, root, "", "", "--dry-run", "--yes")
	if code != 0 || !strings.HasSuffix(out, "jitso setup instructions: dry run, nothing written\n") {
		t.Fatalf("dry run: exit %d\n%s", code, out)
	}
	if _, ok := setupRead(root, "AGENTS.md"); ok {
		t.Errorf("nothing may be written")
	}
}

func TestSetupInstructionsBrokenMarkerChangesNothing(t *testing.T) {
	root := setupRepo(t)
	const seed = "# A\n<!-- BEGIN JITSO block=1 added=1 -->\nx\n"
	setupWrite(t, root, "AGENTS.md", seed)
	for _, args := range [][]string{{"--yes"}, {"--remove", "--yes"}} {
		code, out, errOut := setupInstrRun(t, root, "", "", args...)
		want := "jitso: AGENTS.md has a broken JITSO block (a BEGIN without an END at line 2); fix or remove the marker lines by hand; nothing written\n"
		if code != 3 || out != "" || errOut != want {
			t.Errorf("%v: exit %d\nstdout %q\nstderr %q", args, code, out, errOut)
		}
	}
	if got, _ := setupRead(root, "AGENTS.md"); got != seed {
		t.Errorf("file changed: %q", got)
	}
	if _, ok := setupRead(root, ".jitso/backups/.gitignore"); ok {
		t.Errorf("a refusal must not create a backup folder")
	}
}

func TestSetupInstructionsCLAUDEMirror(t *testing.T) {
	root := setupRepo(t)
	setupWrite(t, root, "CLAUDE.md", "# Claude\n")
	if code, out, errOut := setupInstrRun(t, root, "", "", "--yes"); code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	a, _ := setupRead(root, "AGENTS.md")
	c, _ := setupRead(root, "CLAUDE.md")
	if !strings.HasPrefix(c, "# Claude\n\n") || !strings.HasSuffix(c, instructions.Render(instructions.Added1, "\n")+"\n") {
		t.Errorf("CLAUDE.md = %q", c)
	}
	if a != instructions.Render(instructions.AddedNew, "\n")+"\n" {
		t.Errorf("AGENTS.md = %q", a)
	}
	// Once CLAUDE.md imports AGENTS.md, --remove still cleans its old block.
	setupWrite(t, root, "CLAUDE.md", c+"@AGENTS.md\n")
	if code, out, errOut := setupInstrRun(t, root, "", "", "--remove", "--yes"); code != 0 {
		t.Fatalf("remove: exit %d\n%s\n%s", code, out, errOut)
	}
	if c, _ := setupRead(root, "CLAUDE.md"); c != "# Claude\n@AGENTS.md\n" {
		t.Errorf("CLAUDE.md after remove = %q", c)
	}
}

func TestSetupInstructionsUserScope(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	setupWrite(t, home, ".codex/config.toml", "x\n")
	setupWrite(t, home, ".config/opencode/AGENTS.md", "# oc\n")
	cwd := t.TempDir() // not a git checkout: user scope needs none
	code, out, errOut := setupInstrRun(t, cwd, home, "", "--scope", "user", "--yes")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	for _, line := range []string{"created ~/.claude/CLAUDE.md", "created ~/.codex/AGENTS.md", "wrote ~/.config/opencode/AGENTS.md"} {
		if !strings.Contains(out, line+"\n") {
			t.Errorf("stdout lacks %q:\n%s", line, out)
		}
	}
	if !regexp.MustCompile(`backed up ~/\.config/opencode/AGENTS\.md to ~/\.jitso/backups/\d{8}T\d{6}Z-[0-9a-f]{4}/\.config/opencode/AGENTS\.md\n`).MatchString(out) {
		t.Errorf("no backup line:\n%s", out)
	}
	code, out, errOut = setupInstrRun(t, cwd, home, "", "--scope", "user", "--remove", "--yes")
	if code != 0 {
		t.Fatalf("remove: exit %d\n%s\n%s", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("~/.claude should be pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "config.toml")); err != nil {
		t.Errorf("~/.codex must stay: %v", err)
	}
	if got, _ := setupRead(home, ".config/opencode/AGENTS.md"); got != "# oc\n" {
		t.Errorf("opencode file = %q", got)
	}
}

func TestSetupInstructionsUserScopeNeedsHome(t *testing.T) {
	code, _, errOut := setupInstrRun(t, t.TempDir(), "", "", "--scope", "user", "--yes")
	if code != 3 || errOut != "jitso: HOME is not set\n" {
		t.Errorf("exit %d, stderr %q", code, errOut)
	}
}

func TestSetupInstructionsCodexHomeOutsideHome(t *testing.T) {
	home, _ := filepath.EvalSymlinks(t.TempDir())
	other, _ := filepath.EvalSymlinks(t.TempDir())
	code, out, errOut := setupInstrRun(t, t.TempDir(), home, other, "--scope", "user", "--yes")
	if code != 0 || !strings.Contains(errOut, "CODEX_HOME ("+other+") is not inside your home folder") {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(other, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("a file was written outside the home folder")
	}
}

func TestSetupInstructionsUsageErrors(t *testing.T) {
	root := setupRepo(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--scope"}, "JITSO REJECTED: --scope needs a value\n"},
		{[]string{"--scope", "x"}, "JITSO REJECTED: --scope must be user or project\n"},
		{[]string{"--force"}, "JITSO REJECTED: unknown argument \"--force\"\n"},
		{[]string{"--absolute"}, "JITSO REJECTED: unknown argument \"--absolute\"\n"},
		{[]string{"extra"}, "JITSO REJECTED: unknown argument \"extra\"\n"},
	} {
		if code, out, errOut := setupInstrRun(t, root, "", "", tc.args...); code != 2 || out != "" || errOut != tc.want {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", tc.args, code, out, errOut)
		}
	}
	if code, out, _ := setupInstrRun(t, root, "", "", "--help"); code != 0 || !strings.Contains(out, "jitso setup instructions [--scope project|user] [--remove] [--dry-run] [--yes]") {
		t.Errorf("--help: exit %d\n%s", code, out)
	}
}
