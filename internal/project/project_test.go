package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// git runs git in dir with no owner config, no signing and no hooks, the way
// the contract runner does (SPEC section 6.5).
func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

type fixture struct {
	primary, sub, worktree, outside string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := Real(t.TempDir())
	primary := filepath.Join(base, "repo")
	sub := filepath.Join(primary, "src", "deep")
	outside := filepath.Join(base, "plain")
	for _, d := range []string{sub, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, primary, "init", "-q")
	git(t, primary, "commit", "-q", "--allow-empty", "-m", "init")
	worktree := filepath.Join(base, "wt")
	git(t, primary, "worktree", "add", "-q", "-b", "contract-wt", worktree)
	return fixture{primary: primary, sub: sub, worktree: worktree, outside: outside}
}

func TestResolve(t *testing.T) {
	f := newFixture(t)
	link := filepath.Join(filepath.Dir(f.primary), "link")
	if err := os.Symlink(f.primary, link); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, cwd, override string
		primary, checkout   string
		linked              bool
	}{
		{"primary root", f.primary, "", f.primary, f.primary, false},
		{"subfolder", f.sub, "", f.primary, f.primary, false},
		{"linked worktree", f.worktree, "", f.primary, f.worktree, true},
		{"through a symlink", link, "", f.primary, f.primary, false},
		{"override wins over cwd", f.outside, f.primary, f.primary, f.primary, false},
		{"override to a worktree", f.outside, f.worktree, f.primary, f.worktree, true},
		{"relative override", f.primary, "src", f.primary, f.primary, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Resolve(tt.cwd, tt.override)
			if err != nil {
				t.Fatal(err)
			}
			if r.Primary != tt.primary || r.Checkout != tt.checkout || r.Linked != tt.linked {
				t.Errorf("got %+v, want primary %s checkout %s linked %v", r, tt.primary, tt.checkout, tt.linked)
			}
			if r.StateDir() != filepath.Join(tt.primary, ".jitso") {
				t.Errorf("StateDir = %s", r.StateDir())
			}
		})
	}
}

func TestResolveOutsideGit(t *testing.T) {
	f := newFixture(t)
	missing := filepath.Join(f.outside, "missing")
	tests := []struct {
		name, cwd, override, named string
	}{
		{"cwd outside", f.outside, "", f.outside},
		{"override outside, cwd inside", f.primary, f.outside, f.outside},
		{"override missing", f.primary, missing, missing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Resolve(tt.cwd, tt.override)
			if fail.Code(err) != fail.CodeEnv {
				t.Fatalf("code %d, err %v", fail.Code(err), err)
			}
			want := "jitso: " + tt.named + " is not inside a git checkout"
			if got := fail.Text(err); got != want {
				t.Errorf("text %q, want %q", got, want)
			}
			if strings.Contains(fail.Text(err), "\n") {
				t.Error("the message must be one line")
			}
		})
	}
}

func TestDispatchDir(t *testing.T) {
	if got := DispatchDir("/h"); got != "/h/.jitso/dispatch-cwd" {
		t.Errorf("got %s", got)
	}
}

func TestSelf(t *testing.T) {
	dir := Real(t.TempDir())
	target := filepath.Join(dir, "jitso-real")
	if err := os.WriteFile(target, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "jitso")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	got, err := Self(func() (string, error) { return link, nil })
	if err != nil || got != target {
		t.Errorf("Self = %q, %v; want %q", got, err, target)
	}
}
