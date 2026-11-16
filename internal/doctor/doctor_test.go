package doctor

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
)

// script is a Runner that answers by "<name> <args...>" and records the calls.
type script struct {
	answers map[string]Result
	calls   []string
}

func (s *script) run(dir, name string, args ...string) Result {
	key := name + " " + strings.Join(args, " ")
	s.calls = append(s.calls, key)
	if r, ok := s.answers[key]; ok {
		return r
	}
	return Result{Err: errors.New("unexpected call: " + key)}
}

// lookup is a LookPath over a fixed table.
func lookup(found map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if p, ok := found[name]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
}

func want(t *testing.T, got Line, status Status, check, detail string) {
	t.Helper()
	if got.Status != status || got.Check != check || !strings.Contains(got.Detail, detail) {
		t.Errorf("got %q, want %s %s: ...%s...", got.String(), status, check, detail)
	}
}

func TestLineFormat(t *testing.T) {
	cases := []struct {
		line Line
		want string
	}{
		{Line{OK, "binary", "jitso dev at /x"}, "ok   binary: jitso dev at /x"},
		{Line{Warn, "path", "jitso is not on PATH"}, "warn path: jitso is not on PATH"},
		{Line{Fail, "git", "git is not on PATH"}, "fail git: git is not on PATH"},
		{Line{OK, "catalog", "bad\nyaml\r\nerror"}, "ok   catalog: bad yaml error"},
		{Line{OK, "x", " \n "}, "ok   x: no detail"},
	}
	for _, c := range cases {
		if got := c.line.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

func TestSummary(t *testing.T) {
	lines := []Line{{OK, "a", "x"}, {OK, "b", "x"}, {Warn, "c", "x"}, {Fail, "d", "x"}}
	ok, warn, failed := Counts(lines)
	if ok != 2 || warn != 1 || failed != 1 {
		t.Errorf("Counts = %d %d %d", ok, warn, failed)
	}
	if got := Summary(lines); got != "doctor: 2 ok, 1 warn, 1 fail" {
		t.Errorf("Summary = %q", got)
	}
}

func TestCheckBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "jitso")
	if err := os.WriteFile(bin, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	line, self := checkBinary(Ports{Version: "v1.2.3", Executable: func() (string, error) { return link, nil }})
	real, _ := filepath.EvalSymlinks(bin)
	want(t, line, OK, "binary", "jitso v1.2.3 at "+real)
	if self != real {
		t.Errorf("self = %q, want the real path %q", self, real)
	}

	line, self = checkBinary(Ports{Executable: func() (string, error) { return "", errors.New("no way") }})
	want(t, line, Fail, "binary", "cannot resolve the path of this binary: no way")
	if self != "" {
		t.Errorf("self = %q", self)
	}
}

func TestCheckPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "real", "jitso")
	other := filepath.Join(dir, "other", "jitso")
	for _, p := range []string{bin, other} {
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, nil, 0o755)
	}
	link := filepath.Join(dir, "bin-link")
	os.Symlink(bin, link)
	self := project.Real(bin)

	want(t, checkPath(Ports{LookPath: lookup(nil)}, self), Warn, "path", "jitso is not on PATH")
	want(t, checkPath(Ports{LookPath: lookup(map[string]string{"jitso": link})}, self), OK, "path", "jitso on PATH is this binary")
	got := checkPath(Ports{LookPath: lookup(map[string]string{"jitso": other})}, self)
	want(t, got, Warn, "path", "jitso on PATH is "+other+", not this binary ("+self+")")
	want(t, checkPath(Ports{LookPath: lookup(map[string]string{"jitso": other})}, ""), OK, "path", "jitso on PATH is "+other)
}

func TestCheckGit(t *testing.T) {
	ok := &script{answers: map[string]Result{"git --version": {Stdout: "git version 2.50.1\n"}}}
	want(t, checkGit(Ports{LookPath: lookup(map[string]string{"git": "/usr/bin/git"}), Run: ok.run}), OK, "git", "git version 2.50.1")
	want(t, checkGit(Ports{LookPath: lookup(nil), Run: ok.run}), Fail, "git", "git is not on PATH")
	bad := &script{answers: map[string]Result{"git --version": {Exit: 1}}}
	want(t, checkGit(Ports{LookPath: lookup(map[string]string{"git": "/usr/bin/git"}), Run: bad.run}), Fail, "git", "git --version failed: exit 1")
}

func TestCheckShell(t *testing.T) {
	present := func(string) (fs.FileInfo, error) { return nil, nil }
	missing := func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }
	want(t, checkShell(Ports{GOOS: "linux", Stat: present}), OK, "shell", "/bin/sh is present")
	want(t, checkShell(Ports{GOOS: "linux", Stat: missing}), Fail, "shell", "/bin/sh is missing")
	// Windows has no /bin/sh whatever Stat says: launch and the dispatcher do not run there.
	want(t, checkShell(Ports{GOOS: "windows", Stat: present}), Warn, "shell", "launch and the dispatcher need /bin/sh and do not run on windows")
	want(t, checkShell(Ports{GOOS: "windows", Stat: missing}), Warn, "shell", "do not run on windows")
}

func TestCheckDispatcher(t *testing.T) {
	want(t, checkDispatcher(Ports{GOOS: "linux"}, "/opt/jitso/bin/jitso"), OK, "dispatcher", "can allow this launcher path")
	want(t, checkDispatcher(Ports{GOOS: "linux"}, "/opt/my tools/jitso"), OK, "dispatcher", "can allow")
	want(t, checkDispatcher(Ports{GOOS: "linux"}, "/opt/it's/jitso"), Fail, "dispatcher", "cannot allow the path of this binary (/opt/it's/jitso)")
	want(t, checkDispatcher(Ports{GOOS: "linux"}, "/opt/two  spaces/jitso"), Fail, "dispatcher", "cannot allow")
	want(t, checkDispatcher(Ports{GOOS: "linux"}, ""), Fail, "dispatcher", "unknown")
}

// On windows the dispatcher does not run, so a launcher path the guard's rule
// refuses (a backslash) is a warning that exits 0, not a failure (defect N-32).
// The rule itself is the guard's, whatever the OS.
func TestCheckDispatcherOnWindows(t *testing.T) {
	const bad = `C:\Users\x\jitso.exe`
	cases := []struct {
		goos   string
		self   string
		status Status
		detail string
	}{
		{"windows", bad, Warn, "cannot allow the path of this binary (" + bad + ")"},
		{"windows", bad, Warn, "launch and the dispatcher do not run on windows, so this is not a failure"},
		{"windows", "C:/Users/x/jitso.exe", OK, "can allow this launcher path"},
		{"windows", "", Fail, "unknown"},
		{"linux", bad, Fail, "cannot allow the path of this binary (" + bad + ")"},
		{"darwin", bad, Fail, "cannot allow the path of this binary (" + bad + ")"},
		{"linux", "/opt/jitso/bin/jitso", OK, "can allow this launcher path"},
	}
	for _, c := range cases {
		got := checkDispatcher(Ports{GOOS: c.goos}, c.self)
		want(t, got, c.status, "dispatcher", c.detail)
		if c.goos != "windows" && strings.Contains(got.Detail, "windows") {
			t.Errorf("%s says windows: %q", c.goos, got.Detail)
		}
	}
	// Exit code: the command exits 3 only when Counts reports a failure.
	if _, warn, failed := Counts([]Line{checkDispatcher(Ports{GOOS: "windows"}, bad)}); failed != 0 || warn != 1 {
		t.Errorf("windows: warn %d fail %d, want 1 and 0", warn, failed)
	}
	if _, _, failed := Counts([]Line{checkDispatcher(Ports{GOOS: "linux"}, bad)}); failed != 1 {
		t.Errorf("linux: fail %d, want 1", failed)
	}
}

// A whole windows run with a backslash executable path fails only where
// another check fails (here the project, outside git), never on the dispatcher.
func TestRunOnWindowsDispatcherIsAWarning(t *testing.T) {
	home := t.TempDir()
	// A real file whose name has a backslash stands in for jitso.exe: the
	// binary check resolves the path, and a backslash is legal in a posix name.
	exe := filepath.Join(home, `x\jitso.exe`)
	if err := os.WriteFile(exe, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	run := &script{answers: map[string]Result{"git --version": {Stdout: "git version 2\n"}, "/bin/claude --version": {}}}
	lines := Run(Ports{
		GOOS: "windows", Version: "dev", Home: home,
		Executable: func() (string, error) { return exe, nil },
		LookPath:   lookup(map[string]string{"git": "/usr/bin/git", "claude": "/bin/claude", "orca": "/bin/orca"}),
		Run:        run.run,
		Resolve: func() (project.Root, error) {
			return project.Root{}, fail.Envf("%s is not inside a git checkout", home)
		},
	})
	for _, l := range lines {
		switch {
		case l.Check == "dispatcher" && l.Status != Warn:
			t.Errorf("dispatcher is %s, want warn: %s", l.Status, l)
		case l.Status == Fail && l.Check != "project":
			t.Errorf("unexpected failure: %s", l)
		}
	}
}

func TestCheckOrca(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	// Looked up, never run: the stand-in runner fails the test when called.
	boom := func(string, string, ...string) Result { t.Error("orca must not run"); return Result{} }

	want(t, checkOrca(Ports{LookPath: lookup(map[string]string{"orca": "/usr/bin/orca"}), Getenv: env(nil), Run: boom}), OK, "orca", "/usr/bin/orca")
	want(t, checkOrca(Ports{LookPath: lookup(nil), Getenv: env(nil), Run: boom}), Fail, "orca", "orca is not on PATH")
	set := env(map[string]string{"ORCA_CLI_COMMAND": "/x/fake-orca"})
	got := checkOrca(Ports{LookPath: lookup(map[string]string{"/x/fake-orca": "/x/fake-orca"}), Getenv: set, Run: boom})
	want(t, got, OK, "orca", "/x/fake-orca (from ORCA_CLI_COMMAND)")
	want(t, checkOrca(Ports{LookPath: lookup(nil), Getenv: set, Run: boom}), Fail, "orca", "ORCA_CLI_COMMAND is set to /x/fake-orca")
}

func TestCheckClaude(t *testing.T) {
	found := lookup(map[string]string{"claude": "/bin/claude"})
	run := func(r Result) Ports {
		s := &script{answers: map[string]Result{"/bin/claude --version": r}}
		return Ports{LookPath: found, Run: s.run}
	}
	want(t, checkClaude(run(Result{Stdout: "\n2.1.325 (Claude Code)\n"})), OK, "claude", "/bin/claude, 2.1.325 (Claude Code)")
	// The contract's fake exits 0 with no output.
	want(t, checkClaude(run(Result{})), OK, "claude", "/bin/claude, version unknown")
	want(t, checkClaude(run(Result{Exit: 1, Stdout: "x"})), OK, "claude", "version unknown")
	want(t, checkClaude(run(Result{Err: errors.New("it did not answer within 5s")})), OK, "claude", "version unknown")
	want(t, checkClaude(Ports{LookPath: lookup(nil)}), Fail, "claude", "claude is not on PATH")
}

func TestCheckUserConfig(t *testing.T) {
	home := t.TempDir()
	write := func(text string) {
		t.Helper()
		os.MkdirAll(filepath.Join(home, ".jitso"), 0o755)
		if err := os.WriteFile(filepath.Join(home, ".jitso", "config.yaml"), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := Ports{Home: home}.withDefaults()

	want(t, checkUserConfig(p), OK, "user-config", "no ~/.jitso/config.yaml")

	os.MkdirAll(filepath.Join(home, "clients"), 0o755)
	write("version: 1\ntrusted_parents:\n  - ~/clients\n")
	want(t, checkUserConfig(p), OK, "user-config", "~/.jitso/config.yaml: 1 trusted parent")

	write("trusted_parents:\n  - ~/clients\n  - \"~/gone\"   # moved\n  - /nowhere/else\n")
	want(t, checkUserConfig(p), Warn, "user-config", "1 trusted parent; not found, so skipped: ~/gone, /nowhere/else")
	// A block list may start at the key's own indent.
	write("trusted_parents:\n- ~/gone\n")
	want(t, checkUserConfig(p), Warn, "user-config", "0 trusted parents; not found, so skipped: ~/gone")

	write("trusted_parents: [\n")
	want(t, checkUserConfig(p), Fail, "user-config", "~/.jitso/config.yaml is invalid")
	write("trusted_parents:\n  - /\n")
	want(t, checkUserConfig(p), Fail, "user-config", "too broad")
	want(t, checkUserConfig(Ports{}.withDefaults()), Warn, "user-config", "HOME is not set")
}

func TestCheckProject(t *testing.T) {
	root := project.Root{Primary: "/p", Checkout: "/p"}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	line, got, ok := checkProject(Ports{Getenv: env(nil), Resolve: func() (project.Root, error) { return root, nil }})
	want(t, line, OK, "project", "/p")
	if !ok || got != root || line.Detail != "/p" {
		t.Errorf("got %+v %v %q", got, ok, line.Detail)
	}

	linked := project.Root{Primary: "/p", Checkout: "/w/feature", Linked: true}
	line, _, _ = checkProject(Ports{Getenv: env(map[string]string{"JITSO_PROJECT": "/p"}), Resolve: func() (project.Root, error) { return linked, nil }})
	if line.Detail != "/p, checkout /w/feature (linked worktree), from JITSO_PROJECT" {
		t.Errorf("detail = %q", line.Detail)
	}

	// The error text is the *fail.Error message, never "jitso: ..." twice.
	line, _, ok = checkProject(Ports{Getenv: env(nil), Resolve: func() (project.Root, error) {
		return project.Root{}, fail.Envf("%s is not inside a git checkout", "/tmp/x")
	}})
	want(t, line, Fail, "project", "/tmp/x is not inside a git checkout")
	if ok || strings.Contains(line.Detail, "jitso:") {
		t.Errorf("ok = %v, detail %q", ok, line.Detail)
	}
}

func TestRunOutsideGitLeavesProjectChecksOut(t *testing.T) {
	home := t.TempDir()
	run := &script{answers: map[string]Result{"git --version": {Stdout: "git version 2\n"}, "/bin/claude --version": {}}}
	lines := Run(Ports{
		GOOS: "linux", Version: "dev", Home: home,
		Executable: func() (string, error) { return "/bin/sh", nil },
		LookPath:   lookup(map[string]string{"git": "/usr/bin/git", "claude": "/bin/claude", "orca": "/bin/orca", "jitso": "/bin/sh"}),
		Run:        run.run,
		Resolve: func() (project.Root, error) {
			return project.Root{}, fail.Envf("%s is not inside a git checkout", home)
		},
	})
	var checks []string
	for _, l := range lines {
		checks = append(checks, l.Check)
	}
	got := strings.Join(checks, " ")
	const wantChecks = "binary path git shell dispatcher orca claude user-config project skill-user instructions-user"
	if got != wantChecks {
		t.Errorf("checks = %s\nwant     %s", got, wantChecks)
	}
	if _, _, failed := Counts(lines); failed != 1 {
		t.Errorf("fails = %d, want only the project", failed)
	}
	for _, c := range run.calls {
		if !strings.HasPrefix(c, "git --version") && !strings.HasPrefix(c, "/bin/claude --version") {
			t.Errorf("unexpected program run: %s", c)
		}
	}
}
