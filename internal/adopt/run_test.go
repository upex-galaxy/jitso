package adopt

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
)

type runEnv struct {
	root string
	opts Options
	out  bytes.Buffer
}

func newRunEnv(t *testing.T) *runEnv {
	t.Helper()
	root := t.TempDir()
	return &runEnv{root: root, opts: Options{
		Command:    "setup hook",
		Scope:      "project",
		Version:    "v0.1.0",
		Root:       root,
		BackupsDir: filepath.Join(root, ".jitso", "backups"),
		Display:    relTo(root),
	}}
}

func (e *runEnv) ports(stdin string, terminal bool) Ports {
	return Ports{
		Stdin:    strings.NewReader(stdin),
		Stdout:   &e.out,
		Terminal: terminal,
		Now:      func() time.Time { return fixedNow },
		Rand:     randBytes(0xa3, 0xf9),
	}
}

func (e *runEnv) plan(t *testing.T) Plan {
	write(t, filepath.Join(e.root, "a.txt"), "one\ntwo\n", 0o644)
	return Plan{
		Changes: []Change{
			{Path: filepath.Join(e.root, "a.txt"), Display: "a.txt", Kind: KindFile, Old: []byte("one\ntwo\n"), New: []byte("one\n2\n")},
			{Path: filepath.Join(e.root, "b.txt"), Display: "b.txt", Kind: KindFile, New: []byte("b\n"), Brief: true},
		},
		Header: []string{"change two files"},
		Notes:  []string{"note: commit them"},
	}
}

const wantPreview = "change two files\n" +
	"--- a/a.txt\n+++ b/a.txt\n@@ -1,2 +1,2 @@\n one\n-two\n+2\n" +
	"new file b.txt (1 lines)\n"

func TestRunEmptyPlan(t *testing.T) {
	e := newRunEnv(t)
	for _, why := range []string{"the guard is already registered", ""} {
		e.out.Reset()
		p := Plan{Why: why, Changes: []Change{{Path: filepath.Join(e.root, "a"), Kind: KindFile, Old: []byte("x"), New: []byte("x")}}}
		if err := Run(e.opts, p, e.ports("y\n", true)); err != nil {
			t.Fatal(err)
		}
		want := "jitso setup hook: nothing to do (the guard is already registered)\n"
		if why == "" {
			want = "jitso setup hook: nothing to do\n"
		}
		if e.out.String() != want {
			t.Errorf("out = %q, want %q", e.out.String(), want)
		}
	}
}

func TestRunDryRun(t *testing.T) {
	for _, yes := range []bool{false, true} {
		e := newRunEnv(t)
		p := e.plan(t)
		e.opts.DryRun, e.opts.Yes = true, yes
		if err := Run(e.opts, p, e.ports("", false)); err != nil {
			t.Fatal(err)
		}
		if got, want := e.out.String(), wantPreview+"jitso setup hook: dry run, nothing written\n"; got != want {
			t.Errorf("yes=%v out\n%q\nwant\n%q", yes, got, want)
		}
		if read(t, filepath.Join(e.root, "a.txt")) != "one\ntwo\n" || exists(filepath.Join(e.root, "b.txt")) || exists(filepath.Join(e.root, ".jitso")) {
			t.Errorf("yes=%v: a dry run wrote something", yes)
		}
	}
}

func TestRunApplies(t *testing.T) {
	tests := []struct {
		name     string
		yes      bool
		stdin    string
		terminal bool
		prompt   bool
	}{
		{"yes without a terminal", true, "", false, false},
		{"yes on a terminal does not ask", true, "n\n", true, false},
		{"terminal y", false, "y\n", true, true},
		{"terminal YES", false, "YES\n", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newRunEnv(t)
			p := e.plan(t)
			e.opts.Yes = tt.yes
			if err := Run(e.opts, p, e.ports(tt.stdin, tt.terminal)); err != nil {
				t.Fatal(err)
			}
			want := wantPreview
			if tt.prompt {
				want += Prompt
			}
			want += "backed up a.txt to .jitso/backups/20261115T073455Z-a3f9/a.txt\n" +
				"wrote a.txt\ncreated b.txt\nnote: commit them\njitso setup hook: done\n"
			if e.out.String() != want {
				t.Errorf("out\n%q\nwant\n%q", e.out.String(), want)
			}
			if read(t, filepath.Join(e.root, "a.txt")) != "one\n2\n" || read(t, filepath.Join(e.root, "b.txt")) != "b\n" {
				t.Error("files not written")
			}
		})
	}
}

func TestRunDeclines(t *testing.T) {
	tests := []struct {
		name     string
		stdin    string
		terminal bool
		text     string
		prompt   bool
	}{
		{"terminal N", "N\n", true, "JITSO REJECTED: not applied; nothing written", true},
		{"terminal EOF", "", true, "JITSO REJECTED: not applied; nothing written", true},
		{"terminal anything else", "sure\n", true, "JITSO REJECTED: not applied; nothing written", true},
		{"no terminal, no yes", "y\n", false, "JITSO REJECTED: this changes files: read the preview above, then run it again with --yes, or in a terminal to answer the prompt; nothing written", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newRunEnv(t)
			p := e.plan(t)
			err := Run(e.opts, p, e.ports(tt.stdin, tt.terminal))
			if fail.Code(err) != 2 || fail.Text(err) != tt.text {
				t.Errorf("code %d text %q", fail.Code(err), fail.Text(err))
			}
			want := wantPreview
			if tt.prompt {
				want += Prompt
			}
			if e.out.String() != want {
				t.Errorf("out\n%q\nwant\n%q", e.out.String(), want)
			}
			if read(t, filepath.Join(e.root, "a.txt")) != "one\ntwo\n" || exists(filepath.Join(e.root, "b.txt")) || exists(filepath.Join(e.root, ".jitso")) {
				t.Error("a declined run wrote something")
			}
		})
	}
}

func TestRunNilStdinIsEndOfInput(t *testing.T) {
	e := newRunEnv(t)
	p := e.plan(t)
	ports := e.ports("", true)
	ports.Stdin = nil
	if err := Run(e.opts, p, ports); fail.Code(err) != 2 {
		t.Errorf("err = %v", err)
	}
}

func TestRunRejectsABadPlanBeforeAnyOutput(t *testing.T) {
	e := newRunEnv(t)
	p := Plan{Changes: []Change{{Path: "relative", Kind: KindFile}}}
	err := Run(e.opts, p, e.ports("", false))
	var fe *fail.Error
	if !errors.As(err, &fe) || fail.Code(err) != 3 || e.out.Len() != 0 {
		t.Errorf("err %v out %q", err, e.out.String())
	}
}

func TestRunDefaultsClockAndRandom(t *testing.T) {
	e := newRunEnv(t)
	p := e.plan(t)
	e.opts.Yes = true
	ports := e.ports("", false)
	ports.Now, ports.Rand = nil, nil
	if err := Run(e.opts, p, ports); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.out.String(), "backed up a.txt to .jitso/backups/") {
		t.Errorf("out = %q", e.out.String())
	}
}

func TestPreviewKinds(t *testing.T) {
	long := strings.Repeat("keep\n", 5)
	settings := "{\n  \"env\": {\"T\": \"s3cret\"},\n  \"hooks\": {}\n}\n"
	settingsNew := "{\n  \"env\": {\"T\": \"s3cret\"},\n  \"hooks\": {\"a\": 1}\n}\n"
	tests := []struct {
		name string
		c    Change
		want string
	}{
		{"new file, brief", Change{Display: "n", Kind: KindFile, New: []byte("a\nb\n"), Brief: true}, "new file n (2 lines)\n"},
		{"new file, diff", Change{Display: "n", Kind: KindFile, New: []byte("a\n")}, "--- /dev/null\n+++ b/n\n@@ -0,0 +1 @@\n+a\n"},
		{"new empty file, brief", Change{Display: "n", Kind: KindFile, New: nil, Brief: true}, "new file n (0 lines)\n"},
		{"changed file stays a diff even when brief", Change{Display: "c", Kind: KindFile, Old: []byte("a\n"), New: []byte("b\n"), Brief: true}, "--- a/c\n+++ b/c\n@@ -1 +1 @@\n-a\n+b\n"},
		{"three lines of context", Change{Display: "c", Kind: KindFile, Old: []byte(long + "x\n" + long), New: []byte(long + "y\n" + long)}, "--- a/c\n+++ b/c\n@@ -3,7 +3,7 @@\n keep\n keep\n keep\n-x\n+y\n keep\n keep\n keep\n"},
		{"sensitive prints no unchanged line", Change{Display: ".claude/settings.json", Kind: KindFile, Old: []byte(settings), New: []byte(settingsNew), Sensitive: true, Written: func(l string) bool { return strings.HasPrefix(l, `"hooks"`) }}, "--- a/.claude/settings.json\n+++ b/.claude/settings.json\n@@ -3 +3 @@\n-  \"hooks\": {}\n+  \"hooks\": {\"a\": 1}\n"},
		{"delete, brief", Change{Display: "d", Kind: KindDelete, Old: []byte("a\n"), Brief: true}, "delete d\n"},
		{"delete, unknown content", Change{Display: "d", Kind: KindDelete}, "delete d\n"},
		{"delete, diff", Change{Display: "d", Kind: KindDelete, Old: []byte("a\n")}, "--- a/d\n+++ /dev/null\n@@ -1 +0,0 @@\n-a\n"},
		{"link", Change{Display: "l", Kind: KindSymlink, Target: "../t"}, "link l -> ../t\n"},
		{"unlink", Change{Display: "l", Kind: KindSymlink, OldTarget: "../t"}, "unlink l\n"},
		{"folder", Change{Display: "f", Kind: KindMkdir}, "new folder f\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			Preview(&b, Plan{Changes: []Change{tt.c}})
			if b.String() != tt.want {
				t.Errorf("got\n%q\nwant\n%q", b.String(), tt.want)
			}
		})
	}
	t.Run("an empty plan prints nothing, header included", func(t *testing.T) {
		var b bytes.Buffer
		Preview(&b, Plan{Header: []string{"h"}})
		if b.Len() != 0 {
			t.Errorf("got %q", b.String())
		}
	})
}
