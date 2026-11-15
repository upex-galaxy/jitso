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
		if !strings.Contains(string(out), "worktree "+sb.Worktree(name)+"\n") {
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
