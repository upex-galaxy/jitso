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
	// Windows has no /bin/sh whatever Stat says: launch does not run there.
	want(t, checkShell(Ports{GOOS: "windows", Stat: present}), Warn, "shell", "launch needs /bin/sh and does not run on windows")
	want(t, checkShell(Ports{GOOS: "windows", Stat: missing}), Warn, "shell", "does not run on windows")
}

func TestCheckOrca(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	// Looked up, never run: the stand-in runner fails the test when called.
	boom := func(string, string, ...string) Result { t.Error("orca must not run"); return Result{} }

	want(t, checkOrca(Ports{LookPath: lookup(map[string]string{"orca": "/usr/bin/orca"}), Getenv: env(nil), Run: boom}), OK, "orca", "/usr/bin/orca")
	want(t, checkOrca(Ports{LookPath: lookup(nil), Getenv: env(nil), Run: boom}), Fail, "orca", "orca is not on PATH; install Orca from https://github.com/stablyai/orca and open it once")
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
	only := func(names ...string) func(string) (string, error) {
		return func(n string) (string, error) {
			for _, x := range names {
				if x == n {
					return "/bin/" + n, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	want(t, checkClaude(Ports{LookPath: only("opencode", "pi")}), Warn, "claude", "claude is not on PATH; builds with harness: claude cannot launch here (found: opencode, pi)")
}

func TestCheckProject(t *testing.T) {
	root := project.Root{Primary: "/p", Checkout: "/p"}
	line, got, ok := checkProject(Ports{Resolve: func() (project.Root, error) { return root, nil }})
	want(t, line, OK, "project", "/p")
	if !ok || got != root || line.Detail != "/p" {
		t.Errorf("got %+v %v %q", got, ok, line.Detail)
	}

	linked := project.Root{Primary: "/p", Checkout: "/w/feature", Linked: true}
	line, _, _ = checkProject(Ports{Resolve: func() (project.Root, error) { return linked, nil }})
	if line.Detail != "/p, checkout /w/feature (linked worktree)" {
		t.Errorf("detail = %q", line.Detail)
	}

	// The error text is the *fail.Error message, never "jitso: ..." twice.
	line, _, ok = checkProject(Ports{Resolve: func() (project.Root, error) {
		return project.Root{}, fail.Envf("%s is not inside a git checkout", "/tmp/x")
	}})
	want(t, line, Warn, "project", "/tmp/x is not inside a git checkout; the project checks are skipped")
	if ok || strings.Contains(line.Detail, "jitso:") {
		t.Errorf("ok = %v, detail %q", ok, line.Detail)
	}
}

func TestRunOutsideGitLeavesProjectChecksOut(t *testing.T) {
	home := t.TempDir()
	run := &script{answers: map[string]Result{"git --version": {Stdout: "git version 2\n"}, "/bin/claude --version": {}}}
	self, _ := os.Executable() // a file that exists on every OS
	lines := Run(Ports{
		GOOS: "linux", Version: "dev", Home: home,
		Executable: func() (string, error) { return self, nil },
		LookPath:   lookup(map[string]string{"git": "/usr/bin/git", "claude": "/bin/claude", "orca": "/bin/orca", "jitso": self}),
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
	const wantChecks = "binary path git shell orca claude project skill-user hooks-user instructions-user"
	if got != wantChecks {
		t.Errorf("checks = %s\nwant     %s", got, wantChecks)
	}
	if _, _, failed := Counts(lines); failed != 0 {
		t.Errorf("fails = %d, want none:\n%v", failed, lines)
	}
	for _, c := range run.calls {
		if !strings.HasPrefix(c, "git --version") && !strings.HasPrefix(c, "/bin/claude --version") {
			t.Errorf("unexpected program run: %s", c)
		}
	}
}
