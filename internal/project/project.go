// Package project finds the project a command works on: the git primary
// checkout of the working directory (docs/architecture.md section 6). It also
// names the fixed folders jitso uses (the state folder, the user folder) and
// the real path of the running executable.
package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// Root is where a command runs: the checkout holding the folder and the
// primary checkout that holds the catalog and the state.
type Root struct {
	// Primary is the real path of the primary checkout. The catalog, the hard
	// rules, .mcp.json and .jitso/ always live here.
	Primary string
	// Checkout is the real path of the checkout holding the folder (the
	// linked worktree when there is one).
	Checkout string
	// Linked is true when Checkout is a linked worktree of Primary.
	Linked bool
}

// StateDir is the .jitso/ folder of the primary checkout.
func (r Root) StateDir() string { return filepath.Join(r.Primary, ".jitso") }

// Resolve finds the project of cwd. Outside a git checkout it returns an Env
// error (exit 3) that names the folder: `<folder> is not inside a git
// checkout`.
func Resolve(cwd string) (Root, error) {
	dir := Real(cwd)
	root, ok := checkout(dir)
	if !ok {
		return Root{}, fail.Envf("%s is not inside a git checkout", dir)
	}
	return root, nil
}

// checkout runs one git rev-parse in dir. git's stderr is discarded, so a failure prints nothing of its own.
func checkout(dir string) (Root, bool) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return Root{}, false
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 || lines[0] == "" || lines[2] == "" {
		return Root{}, false
	}
	top, gitDir, common := lines[0], lines[1], lines[2]
	linked := Real(gitDir) != Real(common)
	r := Root{Checkout: Real(top), Linked: linked, Primary: Real(top)}
	if linked {
		r.Primary = Real(filepath.Dir(common))
	}
	return r, true
}

// Checkouts are the checkout folders of the project: the primary one and every
// worktree `git worktree list --porcelain` reports for it. When git fails, the
// primary alone.
func Checkouts(r Root) []string {
	dirs := []string{r.Primary}
	out, err := exec.Command("git", "-C", r.Primary, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return dirs
	}
	for _, line := range strings.Split(string(out), "\n") {
		if dir, ok := strings.CutPrefix(strings.TrimSuffix(line, "\r"), "worktree "); ok {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// Real is the path with every symlink resolved, or the path itself when it
// cannot be resolved (it does not exist).
func Real(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// UserDir is the user's own jitso folder, <home>/.jitso.
func UserDir(home string) string {
	return filepath.Join(home, ".jitso")
}

// Self is the real path of the running executable (symlinks resolved), the
// identity the hook compares a command's first word against.
func Self(executable func() (string, error)) (string, error) {
	if executable == nil {
		executable = os.Executable
	}
	path, err := executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(path)
}
