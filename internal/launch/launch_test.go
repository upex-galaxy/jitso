package launch

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/registry"
)

const dialogScreen = "Do you trust this folder?\n 2. No, exit"

// fakeOrca records calls and fails the verbs listed in fail.
type fakeOrca struct {
	cwd     string
	screens []string
	reads   int
	idle    bool
	fail    map[string]bool
	calls   []string
}

var errFake = errors.New("fake failure")

func (f *fakeOrca) record(verb string) error {
	f.calls = append(f.calls, verb)
	if f.fail[verb] {
		return errFake
	}
	return nil
}

func (f *fakeOrca) CreateTerminal(worktree, title, command string) (orca.Terminal, error) {
	if err := f.record("terminal create"); err != nil {
		return orca.Terminal{}, err
	}
	return orca.Terminal{Handle: "term_fake", WorktreePath: f.cwd}, nil
}

func (f *fakeOrca) WaitIdle(string, int) bool {
	f.calls = append(f.calls, "terminal wait")
	return f.idle
}

func (f *fakeOrca) ReadScreen(string) (string, error) {
	if err := f.record("terminal read"); err != nil {
		return "", err
	}
	s := "❯ idle"
	if len(f.screens) > 0 {
		s = f.screens[min(f.reads, len(f.screens)-1)]
	}
	f.reads++
	return s, nil
}

func (f *fakeOrca) SendText(string, string) error { return f.record("terminal send") }
func (f *fakeOrca) CloseTerminal(string) error    { return f.record("terminal close") }
func (f *fakeOrca) Call(args []string) (map[string]any, error) {
	return nil, f.record(strings.Join(args[:2], " "))
}

// sandbox is a copy of the contract's default fixture with a brief in
// .jitso/briefs and one earlier launch folder.
type sandbox struct {
	root, home, briefPath string
	out, warn             []string
	store                 *registry.FileStore
	orca                  *fakeOrca
	// extra is the trusted folder the tests add; a test that needs the real
	// worktree list of a repository clears it.
	extra []string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	s := &sandbox{root: filepath.Join(tmp, "project"), home: filepath.Join(tmp, "home"), orca: &fakeOrca{idle: true}}
	if err := os.CopyFS(s.root, os.DirFS("../../contract/fixtures/default")); err != nil {
		t.Fatal(err)
	}
	brief, err := os.ReadFile("../../contract/cases/smoke/launch-dry-run/brief.md")
	if err != nil {
		t.Fatal(err)
	}
	s.briefPath = filepath.Join(s.root, ".jitso", "briefs", "b.md")
	for path, data := range map[string][]byte{s.briefPath: brief, filepath.Join(s.root, ".jitso", "launches", "L20261113T142233-a3f9", "brief.md"): []byte("seed")} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s.orca.cwd = s.root
	s.extra = []string{s.root}
	s.store = registry.NewFileStore(filepath.Join(s.root, ".jitso"))
	return s
}

func (s *sandbox) run(t *testing.T, argv ...string) error {
	t.Helper()
	a, err := Parse(argv)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Check(); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(os.DirFS(s.root))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 15, 1, 2, 3, 4_000_000, time.UTC)
	return Run(a, Deps{
		Root: project.Root{Primary: s.root, Checkout: s.root}, Catalog: cat, Registry: s.store, Orca: s.orca,
		Home: s.home, Cwd: s.root, TrustExtra: s.extra,
		Now:   func() time.Time { return now },
		Sleep: func(d time.Duration) { now = now.Add(d) },
		Rand:  bytes.NewReader(bytes.Repeat([]byte{0xab}, 64)),
		Out:   func(l string) { s.out = append(s.out, l) },
		Warn:  func(l string) { s.warn = append(s.warn, l) },
	})
}

func (s *sandbox) launchDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(s.root, ".jitso", "launches"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

const id = "L20261115T010203-abab"

func TestRunDirect(t *testing.T) {
	s := newSandbox(t)
	if err := s.run(t, "--build", "haiku-explorer", "--brief", s.briefPath, "--mode", "direct"); err != nil {
		t.Fatal(err)
	}
	if len(s.out) != 1 || !strings.HasPrefix(s.out[0], "JITSO "+id+" | operator/explorer | mode direct (asks you)") || len(s.warn) != 0 {
		t.Fatalf("out %q warn %q", s.out, s.warn)
	}
	if got := strings.Count(s.out[0], "\n") + 1; got != 6 {
		t.Errorf("card has %d lines", got)
	}
	dir := filepath.Join(s.root, ".jitso", "launches", id)
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if !strings.HasPrefix(read(FileBrief), s.out[0]+"\n\n## Goal") || read(FileCard) != s.out[0]+"\n" {
		t.Error("brief.md or card.txt")
	}
	if read(FileMCP) != "{\n  \"mcpServers\": {}\n}\n" {
		t.Errorf("mcp.json %q", read(FileMCP))
	}
	script := read(FileRunSh)
	if !strings.HasPrefix(script, "#!/bin/sh\nexport CLAUDE_CODE_DISABLE_CLAUDE_MDS='1'\nexec 'claude' ") ||
		!strings.HasSuffix(script, "'Read "+filepath.Join(dir, FileBrief)+" and follow it exactly.\nROUTE-SCOPE: none'\n") {
		t.Errorf("run.sh %q", script)
	}
	if info, _ := os.Stat(filepath.Join(dir, FileRunSh)); info.Mode().Perm() != 0o755 {
		t.Errorf("run.sh mode %v", info.Mode())
	}
	events, _ := s.store.Events()
	if len(events) != 1 {
		t.Fatalf("%d events", len(events))
	}
	e := events[0]
	for path, want := range map[string]any{"id": id, "root": id, "mode": "direct", "trust_dialog": "absent", "worktree": "primary",
		"cwd": s.root, "orca.terminal": "term_fake", "orca.title": "[jitso:" + id + "] haiku-explorer", "ts": "2026-11-15T01:02:03.004Z"} {
		if got, _ := e.Get(path); got != want {
			t.Errorf("%s = %v, want %v", path, got, want)
		}
	}
	if flags, _ := e.Strings("flags"); flags[len(flags)-1] != "--tools=Read,Glob,Grep,Bash" {
		t.Errorf("flags must drop the prompt: %q", flags)
	}
	if !reflect.DeepEqual(s.orca.calls, []string{"terminal create", "terminal wait", "terminal read"}) {
		t.Errorf("calls %q", s.orca.calls)
	}
}

func TestRunDryRun(t *testing.T) {
	s := newSandbox(t)
	if err := s.run(t, "--build", "sonnet-automator", "--brief", s.briefPath, "--mode", "fleet", "--run", "r", "--task", "t",
		"--field", "ticket_key=PRJ-1", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if len(s.out) != 2 || s.out[1] != DryRunLine || !strings.Contains(s.out[0], "\nsubagents plan,code,verifier (suggested; others allowed)\nROUTE-SCOPE: code-quickref,ticket-work\n") {
		t.Errorf("out %q", s.out)
	}
	if len(s.orca.calls) != 0 || !reflect.DeepEqual(s.launchDirs(t), []string{"L20261113T142233-a3f9"}) {
		t.Error("a dry run calls nothing and writes nothing")
	}
}

func TestRunFailuresRemoveOnlyTheirFolder(t *testing.T) {
	tests := []struct {
		name      string
		edit      func(*sandbox)
		argv      []string
		reason    string
		wantCalls []string
	}{
		{"missing .mcp.json", func(s *sandbox) { os.Remove(filepath.Join(s.root, ".mcp.json")) }, nil, "no such file", nil},
		{"null .mcp.json", func(s *sandbox) { os.WriteFile(filepath.Join(s.root, ".mcp.json"), []byte("null"), 0o644) }, nil, "null", nil},
		{"missing rules", func(s *sandbox) { os.Remove(filepath.Join(s.root, "jitso", "hard-rules", "style.md")) }, nil, "style.md", nil},
		{"terminal create", func(s *sandbox) { s.orca.fail = map[string]bool{"terminal create": true} }, nil, "fake failure", []string{"terminal create"}},
		{"idle timeout survives a failing close", func(s *sandbox) { s.orca.idle = false; s.orca.fail = map[string]bool{"terminal close": true} }, nil,
			"terminal term_fake did not reach tui-idle in 90 s; it was closed", []string{"terminal create", "terminal wait", "terminal close"}},
		{"read failure", func(s *sandbox) { s.orca.fail = map[string]bool{"terminal read": true} }, nil, "fake failure",
			[]string{"terminal create", "terminal wait", "terminal read", "terminal close"}},
		{"refused with a dialog", func(s *sandbox) { s.orca.cwd = s.home; s.orca.screens = []string{dialogScreen} }, nil, "JITSO REJECTED: untrusted folder ",
			[]string{"terminal create", "terminal close"}},
		{"refused in the dispatcher folder", func(s *sandbox) {
			s.orca.cwd = filepath.Join(s.home, ".jitso", "dispatch-cwd")
			s.orca.screens = []string{dialogScreen}
		}, nil,
			"JITSO REJECTED: untrusted folder ", []string{"terminal create", "terminal close"}},
		{"refused without a dialog", func(s *sandbox) { s.orca.cwd = s.home }, nil, "JITSO REJECTED: untrusted folder ",
			[]string{"terminal create", "terminal close"}},
		{"refused without a folder", func(s *sandbox) { s.orca.cwd = "" }, nil, "untrusted folder (unknown):",
			[]string{"terminal create", "terminal close"}},
		{"refused before the idle wait", func(s *sandbox) { s.orca.cwd = s.home; s.orca.idle = false }, nil, "JITSO REJECTED: untrusted folder ",
			[]string{"terminal create", "terminal close"}},
		{"refused with a failing close", func(s *sandbox) { s.orca.cwd = s.home; s.orca.fail = map[string]bool{"terminal close": true} }, nil, "JITSO REJECTED: untrusted folder ",
			[]string{"terminal create", "terminal close"}},
		{"fleet refused before worker-start", func(s *sandbox) { s.orca.cwd = s.home },
			[]string{"--build", "haiku-explorer", "--mode", "fleet", "--run", "r", "--task", "t"}, "JITSO REJECTED: untrusted folder ",
			[]string{"terminal create", "terminal close"}},
		{"worker-start", func(s *sandbox) {
			s.orca.fail = map[string]bool{"orchestration worker-start": true, "terminal close": true}
		},
			[]string{"--build", "haiku-explorer", "--mode", "fleet", "--run", "r", "--task", "t"}, "fake failure",
			[]string{"terminal create", "terminal wait", "terminal read", "orchestration worker-start", "terminal close"}},
		{"busy lock", func(s *sandbox) {
			s.store.LockTimeout = 0
			os.Mkdir(filepath.Join(s.root, ".jitso", registry.LockName), 0o755)
		}, nil, "lock", []string{"terminal create", "terminal wait", "terminal read", "terminal close"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSandbox(t)
			tt.edit(s)
			argv := tt.argv
			if argv == nil {
				argv = []string{"--build", "haiku-explorer", "--mode", "direct"}
			}
			err := s.run(t, append(argv, "--brief", s.briefPath)...)
			if err == nil || !strings.Contains(err.Error(), tt.reason) {
				t.Fatalf("err %v, want %q", err, tt.reason)
			}
			if !reflect.DeepEqual(s.launchDirs(t), []string{"L20261113T142233-a3f9"}) {
				t.Errorf("launch folders %q", s.launchDirs(t))
			}
			if !reflect.DeepEqual(s.orca.calls, tt.wantCalls) {
				t.Errorf("calls %q, want %q", s.orca.calls, tt.wantCalls)
			}
			if events, _ := s.store.Events(); len(events) != 0 || len(s.out) != 0 {
				t.Errorf("%d events, out %q", len(events), s.out)
			}
		})
	}
}

// gitRepo makes the sandbox project a real repository with one linked worktree
// and returns the real path of the linked worktree. The folder check then
// reads the real worktree list, not the tests' extra folder.
func (s *sandbox) gitRepo(t *testing.T) string {
	t.Helper()
	s.extra = nil
	linked := filepath.Join(filepath.Dir(s.root), "linked")
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init"},
		{"worktree", "add", "-q", "-b", "x", linked},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = s.root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return linked
}

// The folder Orca reports is checked on every launch against the real worktree
// list, and against the path a --worktree selector names (defect N-22).
func TestRunWorktreeSelector(t *testing.T) {
	tests := []struct {
		name      string
		selector  func(root, linked, home string) string
		cwd       func(root, linked, home string) string
		reason    string
		wantCalls []string
	}{
		{"no selector in the primary", nil, func(root, _, _ string) string { return root }, "", []string{"terminal create", "terminal wait", "terminal read"}},
		{"no selector in a linked worktree", nil, func(_, linked, _ string) string { return linked }, "", []string{"terminal create", "terminal wait", "terminal read"}},
		{"path selector honoured", func(_, linked, _ string) string { return "path:" + linked }, func(_, linked, _ string) string { return linked }, "", []string{"terminal create", "terminal wait", "terminal read"}},
		{"id selector honoured", func(_, linked, _ string) string { return "id:fake-uuid::" + linked }, func(_, linked, _ string) string { return linked }, "", []string{"terminal create", "terminal wait", "terminal read"}},
		{"path selector with another worktree", func(_, linked, _ string) string { return "path:" + linked }, func(root, _, _ string) string { return root },
			"--worktree asked for ", []string{"terminal create", "terminal close"}},
		{"name selector outside the repo", func(_, _, _ string) string { return "wt-1" }, func(_, _, home string) string { return home },
			"a launch runs only in a worktree of this repo", []string{"terminal create", "terminal close"}},
		{"path selector outside the repo is refused before Orca", func(_, _, home string) string { return "path:" + home }, nil,
			"which is not a worktree of this repo", nil},
		{"relative path selector is refused before Orca", func(_, _, _ string) string { return "path:rel/x" }, nil, "must name an absolute path", nil},
	}
	for _, tt := range tests {
		// The selector allowlist has no comma, so no test name may have one:
		// t.TempDir() puts the name in the project path.
		t.Run(tt.name, func(t *testing.T) {
			s := newSandbox(t)
			linked := s.gitRepo(t)
			linked, _ = filepath.EvalSymlinks(linked)
			if tt.cwd != nil {
				s.orca.cwd = tt.cwd(s.root, linked, s.home)
			}
			argv := []string{"--build", "haiku-explorer", "--mode", "direct", "--brief", s.briefPath}
			if tt.selector != nil {
				argv = append(argv, "--worktree", tt.selector(s.root, linked, s.home))
			}
			err := s.run(t, argv...)
			if tt.reason == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.HasPrefix(err.Error(), "JITSO REJECTED: ") || !strings.Contains(err.Error(), tt.reason) {
				t.Fatalf("err %v, want %q", err, tt.reason)
			}
			if !reflect.DeepEqual(s.orca.calls, tt.wantCalls) {
				t.Errorf("calls %q, want %q", s.orca.calls, tt.wantCalls)
			}
			wantDirs := 1
			if tt.reason == "" {
				wantDirs = 2
			}
			if got := s.launchDirs(t); len(got) != wantDirs {
				t.Errorf("launch folders %q", got)
			}
			events, _ := s.store.Events()
			if (tt.reason == "") != (len(events) == 1) {
				t.Errorf("%d events", len(events))
			}
		})
	}
}

func TestRunTrust(t *testing.T) {
	tests := []struct {
		name    string
		screens []string
		want    string
		warn    bool
	}{
		{"handled", []string{dialogScreen, "❯ idle"}, "handled", false},
		{"timeout keeps the launch and warns", []string{dialogScreen}, "timeout", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSandbox(t)
			s.orca.screens = tt.screens
			if err := s.run(t, "--build", "haiku-explorer", "--brief", s.briefPath, "--mode", "direct"); err != nil {
				t.Fatal(err)
			}
			events, _ := s.store.Events()
			if got, _ := events[0].String("trust_dialog"); got != tt.want {
				t.Errorf("trust_dialog %q", got)
			}
			if tt.warn != (len(s.warn) == 1 && strings.Contains(s.warn[0], "trust dialog of terminal term_fake did not clear")) {
				t.Errorf("warn %q", s.warn)
			}
			if len(s.out) != 1 {
				t.Errorf("out %q", s.out)
			}
		})
	}
}

func TestRunRejectsBeforeWriting(t *testing.T) {
	tests := []struct {
		name   string
		argv   []string
		reason string
	}{
		{"subagent tier", []string{"--build", "plan"}, `build "plan" has tier subagent`},
		{"missing field", []string{"--build", "sonnet-tester"}, "missing ticket_key"},
		{"unknown parent", []string{"--build", "haiku-explorer", "--parent", "L20260101T000000-ffff", "--dry-run"}, "unknown parent launch L20260101T000000-ffff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSandbox(t)
			err := s.run(t, append(tt.argv, "--brief", s.briefPath, "--mode", "direct")...)
			if err == nil || !strings.Contains(err.Error(), tt.reason) || !strings.HasPrefix(err.Error(), "JITSO REJECTED: ") {
				t.Fatalf("err %v", err)
			}
			if len(s.orca.calls) != 0 || len(s.out) != 0 {
				t.Error("nothing may run")
			}
		})
	}
}

func TestEventFlags(t *testing.T) {
	cat, err := catalog.Load(os.DirFS("../../contract/fixtures/default"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := cat.Find("haiku-explorer")
	in := EventInput{ID: id, Args: Args{Mode: ModeFleet, BriefLanguage: "en"}, Argv: []string{"claude", "--a", "--b"}}
	in.Profile.Build = b
	if e := Event(in); !reflect.DeepEqual(e.Flags, []string{"--a", "--b"}) || e.Cwd != nil || e.TranscriptPath != nil || e.Worktree != "primary" {
		t.Errorf("fleet event %+v", e)
	}
	in.Prompted = true
	if e := Event(in); !reflect.DeepEqual(e.Flags, []string{"--a"}) {
		t.Errorf("direct flags %q", e.Flags)
	}
}

// The launch event names the file the session was launched from.
func TestEventNamesTheBuildFile(t *testing.T) {
	b := &catalog.Build{ID: "x", Version: 1, Layer: catalog.LayerUser, Path: "/h/.jitso/builds/x.yaml", SHA256: "abc"}
	in := EventInput{ID: id, Args: Args{Mode: ModeFleet, BriefLanguage: "en"}, Argv: []string{"claude"}}
	in.Profile.Build = b
	if e := Event(in); e.BuildLayer != "user" || e.BuildFile != "/h/.jitso/builds/x.yaml" || e.BuildSHA256 != "abc" {
		t.Errorf("event %+v", e)
	}
	b.Layer = catalog.LayerDraft
	if e := Event(in); e.BuildLayer != "draft" {
		t.Errorf("a draft records its layer, got %q", e.BuildLayer)
	}
	b.Layer = ""
	if e := Event(in); e.BuildLayer != "repo" {
		t.Errorf("a build made by hand reads as repo, got %q", e.BuildLayer)
	}
}
