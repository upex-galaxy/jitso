package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddWorktree(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sb, err := New(self)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Remove()
	if err := os.Mkdir(sb.Project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sb.GitInit(); err != nil {
		t.Fatal(err)
	}
	if err := sb.AddWorktree("linked"); err != nil {
		t.Fatal(err)
	}
	if err := sb.AddWorktree("second"); err != nil {
		t.Fatalf("a second worktree reuses the first commit: %v", err)
	}
	out, err := exec.Command("git", "-C", sb.Project, "worktree", "list", "--porcelain").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"linked", "second"} {
		if !strings.Contains(string(out), "worktree "+filepath.ToSlash(sb.Worktree(name))+"\n") {
			t.Errorf("git worktree list lacks %s:\n%s", sb.Worktree(name), out)
		}
	}
	if real, _ := filepath.EvalSymlinks(sb.Worktree("linked")); real != sb.Worktree("linked") {
		t.Errorf("the worktree path must be real: %s vs %s", real, sb.Worktree("linked"))
	}
	if err := sb.RemoveFake("claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(sb.Bin, "claude")); !os.IsNotExist(err) {
		t.Error("RemoveFake leaves the fake in place")
	}
}

func newSandbox(t *testing.T) *Sandbox {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	sb, err := New(self)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sb.Remove() })
	if err := os.Mkdir(sb.Project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := sb.GitInit(); err != nil {
		t.Fatal(err)
	}
	return sb
}

func TestInitRepo(t *testing.T) {
	outside := t.TempDir()
	cases := []struct {
		name    string
		setup   func(sb *Sandbox) error
		path    func(sb *Sandbox) string
		wantErr string // empty: success, and path/.git must exist
	}{
		{name: "relative path", path: func(*Sandbox) string { return "nested" }},
		{name: "nested path creates parents", path: func(*Sandbox) string { return "a/b/c" }},
		{name: "absolute path in HOME", path: func(sb *Sandbox) string { return filepath.Join(sb.Home, "repo") }},
		{
			name:  "existing empty folder",
			setup: func(sb *Sandbox) error { return os.Mkdir(filepath.Join(sb.Project, "empty"), 0o755) },
			path:  func(*Sandbox) string { return "empty" },
		},
		{name: "already a repo (the project itself)", path: func(*Sandbox) string { return "." }, wantErr: "already holds a .git"},
		{
			name: "already a repo (a nested one)",
			setup: func(sb *Sandbox) error {
				if err := os.Mkdir(filepath.Join(sb.Project, "twice"), 0o755); err != nil {
					return err
				}
				return sb.InitRepo("twice")
			},
			path:    func(*Sandbox) string { return "twice" },
			wantErr: "already holds a .git",
		},
		{name: "absolute outside the sandbox", path: func(*Sandbox) string { return filepath.Join(outside, "repo") }, wantErr: "resolves outside"},
		{name: "sibling of project and HOME", path: func(sb *Sandbox) string { return filepath.Join(sb.Root, "elsewhere") }, wantErr: "resolves outside"},
		{name: "dot dot climbs out", path: func(*Sandbox) string { return "../outside" }, wantErr: "resolves outside"},
		{
			name:    "symlinked parent resolving outside",
			setup:   func(sb *Sandbox) error { return os.Symlink(outside, filepath.Join(sb.Project, "link")) },
			path:    func(*Sandbox) string { return "link/repo" },
			wantErr: "resolves outside",
		},
		{
			name:    "the symlink itself resolving outside",
			setup:   func(sb *Sandbox) error { return os.Symlink(outside, filepath.Join(sb.Project, "link")) },
			path:    func(*Sandbox) string { return "link" },
			wantErr: "resolves outside",
		},
		{
			name:  "symlinked parent resolving inside",
			setup: func(sb *Sandbox) error { return os.Symlink(sb.Home, filepath.Join(sb.Project, "home-link")) },
			path:  func(*Sandbox) string { return "home-link/repo" },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sb := newSandbox(t)
			if c.setup != nil {
				if err := c.setup(sb); err != nil {
					t.Fatal(err)
				}
			}
			path := c.path(sb)
			err := sb.InitRepo(path)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want an error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(sb.Project, path)
			}
			out, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output()
			if err != nil {
				t.Fatal(err)
			}
			want, _ := filepath.EvalSymlinks(path)
			want = filepath.ToSlash(want) // git prints forward slashes on Windows
			if got := strings.TrimSpace(string(out)); got != want {
				t.Errorf("the repo top level is %s, want %s (a nested checkout of its own)", got, want)
			}
		})
	}
}
