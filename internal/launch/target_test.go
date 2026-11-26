package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
)

// gitIn runs git in dir with no owner config, no signing and no hooks.
func gitIn(t *testing.T, dir string, args ...string) {
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

// repositories are a home, a target with a linked worktree and a link to the
// target, under one temp folder.
func repositories(t *testing.T) (home project.Root, target, linked, alias string) {
	t.Helper()
	base := project.Real(t.TempDir())
	homeDir := filepath.Join(base, "home repo")
	target, linked, alias = filepath.Join(base, "target"), filepath.Join(base, "target-wt"), filepath.Join(base, "alias")
	for _, dir := range []string{homeDir, target} {
		if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		gitIn(t, dir, "init", "-q")
		gitIn(t, dir, "commit", "-q", "--allow-empty", "-m", "init")
	}
	gitIn(t, target, "worktree", "add", "-q", "-b", "wt", linked)
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
	return project.Root{Primary: homeDir, Checkout: homeDir}, target, linked, alias
}

func TestResolveTarget(t *testing.T) {
	home, target, linked, alias := repositories(t)
	sub := filepath.Join(target, "sub")
	tests := []struct {
		name           string
		repo, worktree string
		primary        string
		checkout       string
		cross          bool
		rejected       string
	}{
		{name: "no flag", primary: home.Primary, checkout: home.Primary},
		{name: "a name selector", worktree: "name:x", primary: home.Primary, checkout: home.Primary},
		{name: "an id selector names no target", worktree: "id:u::" + target, primary: home.Primary, checkout: home.Primary},
		{name: "a folder of no repository", worktree: "path:/nowhere", primary: home.Primary, checkout: home.Primary},
		{name: "repo", repo: target, primary: target, checkout: target, cross: true},
		{name: "a subfolder of the repo", repo: sub, primary: target, checkout: target, cross: true},
		{name: "a linked worktree names its primary", repo: linked, primary: target, checkout: target, cross: true},
		{name: "through a symlink", repo: alias, primary: target, checkout: target, cross: true},
		{name: "the home repository", repo: home.Primary, primary: home.Primary, checkout: home.Primary},
		{name: "a worktree of another repository", worktree: "path:" + linked, primary: target, checkout: linked, cross: true},
		{name: "repo and its worktree", repo: target, worktree: "path:" + linked, primary: target, checkout: linked, cross: true},
		{name: "repo and an id selector of it", repo: target, worktree: "id:u::" + linked, primary: target, checkout: linked, cross: true},
		{name: "a subfolder is no worktree", worktree: "path:" + sub, rejected: "which is not a worktree of " + target},
		{name: "repo and a worktree elsewhere", repo: target, worktree: "path:" + home.Primary, rejected: "which is not a worktree of " + target},
		{name: "the home repo and a worktree elsewhere", repo: home.Primary, worktree: "path:" + target, rejected: "which is not a worktree of this repo"},
		{name: "relative", repo: "target", rejected: "invalid value for --repo"},
		{name: "too long", repo: "/" + strings.Repeat("a", 1024), rejected: "invalid value for --repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := Args{}
			if tt.repo != "" {
				a.Repo = &tt.repo
			}
			if tt.worktree != "" {
				a.Worktree = &tt.worktree
			}
			got, err := ResolveTarget(a, home)
			if tt.rejected != "" {
				if err == nil || fail.Code(err) != fail.CodeRejected || !strings.Contains(fail.Text(err), tt.rejected) {
					t.Fatalf("got %+v, %v; want a rejection with %q", got, err, tt.rejected)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Primary != tt.primary || got.Checkout != tt.checkout || got.Cross != tt.cross {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestOpenSelector(t *testing.T) {
	selector := "name:x"
	repo := "/t"
	target := Target{Root: project.Root{Primary: "/t"}, Cross: true}
	tests := []struct {
		name string
		args Args
		away bool
		want string
	}{
		{"nothing said", Args{}, false, ""},
		{"the command line wins", Args{Worktree: &selector, Repo: &repo}, true, "name:x"},
		{"repo", Args{Repo: &repo}, false, "path:/t"},
		{"the caller is elsewhere", Args{}, true, "path:/t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openSelector(tt.args, project.Root{Away: tt.away}, target); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRegistered(t *testing.T) {
	home, target, linked, alias := repositories(t)
	tests := []struct {
		name  string
		repos []string
		want  bool
	}{
		{"none", nil, false},
		{"another repository", []string{home.Primary}, false},
		{"a subfolder", []string{filepath.Join(target, "sub")}, false},
		{"the primary", []string{home.Primary, target}, true},
		{"a linked worktree counts", []string{linked}, true},
		{"through a symlink", []string{alias}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := registered(&fakeOrca{repos: tt.repos}, target)
			if err != nil || got != tt.want {
				t.Errorf("registered = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestUnregisteredQuotesTheCommand(t *testing.T) {
	for path, want := range map[string]string{
		"/a/b-c":   "orca repo add --path /a/b-c",
		"/a b/c":   "orca repo add --path '/a b/c'",
		"/a/$(x)c": "orca repo add --path '/a/$(x)c'",
	} {
		err := unregistered(path)
		if fail.Code(err) != fail.CodeEnv || !strings.HasSuffix(fail.Text(err), want) {
			t.Errorf("unregistered(%q) = %v", path, err)
		}
	}
}
