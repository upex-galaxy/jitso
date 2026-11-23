package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// initRun runs `jitso init <args>` from dir with home as HOME; terminal sets
// whether stdin is a terminal, stdin is what it holds.
func initRun(t *testing.T, dir, home, stdin string, terminal bool, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	var seed byte
	code := Run(append([]string{"init"}, args...), System{
		Stdin:  strings.NewReader(stdin),
		Stdout: &out,
		Stderr: &errOut,
		LookupEnv: func(k string) (string, bool) {
			if k == "HOME" {
				return home, true
			}
			return "", false
		},
		Getwd:           func() (string, error) { return dir, nil },
		Executable:      func() (string, error) { return setupBinary(t), nil },
		Now:             func() time.Time { return time.Date(2026, 11, 23, 7, 0, 0, 0, time.UTC) },
		Rand:            readerFunc(func(p []byte) (int, error) { seed++; p[0], p[1] = seed, 0x1a; return 2, nil }),
		StdinIsTerminal: func() bool { return terminal },
	})
	return code, out.String(), errOut.String()
}

func TestInitAsksThenApplies(t *testing.T) {
	root := setupRepo(t)
	home := t.TempDir()
	// project only, guard for codex, then yes.
	code, out, errOut := initRun(t, root, home, "p\ncodex\ny\n", true)
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{
		"Install the jitso skill for your user account (every project) or in this project only? [U/p] ",
		"Register the guard for which harnesses? [Enter: ",
		"jitso init: skill in this project\n",
		"jitso init: guard for codex\n",
		"Apply these changes? [y/N] ",
		"jitso init: done\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout lacks %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "hooks.json")); err != nil {
		t.Errorf("codex hooks not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "settings.json")); err == nil {
		t.Errorf("claude settings written although only codex was chosen")
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); err == nil {
		t.Errorf("home touched with the project-only answer")
	}
}

func TestInitDeclinedWritesNothing(t *testing.T) {
	root := setupRepo(t)
	home := t.TempDir()
	code, out, _ := initRun(t, root, home, "\n\nn\n", true)
	if code != 2 {
		t.Fatalf("exit %d, want 2\n%s", code, out)
	}
	for _, p := range []string{filepath.Join(root, "AGENTS.md"), filepath.Join(home, ".agents")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s written after a no", p)
		}
	}
}

func TestInitBadAnswer(t *testing.T) {
	root := setupRepo(t)
	code, _, errOut := initRun(t, root, t.TempDir(), "maybe\n", true)
	if code != 2 || !strings.Contains(errOut, "answer u (user account) or p (this project)") {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
}

func TestInitReadLineLeavesTheRest(t *testing.T) {
	r := strings.NewReader("a\r\nb\nc")
	if got := initReadLine(r); got != "a" {
		t.Fatalf("first %q", got)
	}
	if got := initReadLine(r); got != "b" {
		t.Fatalf("second %q", got)
	}
	if got := initReadLine(r); got != "c" {
		t.Fatalf("third %q", got)
	}
	if got := initReadLine(nil); got != "" {
		t.Fatalf("nil %q", got)
	}
}
