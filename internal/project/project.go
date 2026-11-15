// Package project finds the project a command works on: the git primary
// checkout of the working directory, or of JITSO_PROJECT when it is set (the
// dispatcher handoff, docs/architecture.md section 6). It also names the fixed
// folders jitso uses (the state folder, the dispatcher folder) and the real
// path of the running executable.
package project

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// EnvVar is the variable that names the project when the cwd cannot, set by
// the dispatcher for the claude it starts.
const EnvVar = "JITSO_PROJECT"

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

// Resolve finds the project. When override is not empty (the value of
// JITSO_PROJECT) it names the folder, relative to cwd when it is not absolute,
// and cwd is ignored. Outside a git checkout it returns an Env error (exit 3)
// that names the folder: `<folder> is not inside a git checkout`.
func Resolve(cwd, override string) (Root, error) {
	dir := cwd
	if override != "" {
		dir = override
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
	}
	dir = Real(dir)
	root, ok := checkout(dir)
	if !ok {
		return Root{}, fail.Envf("%s is not inside a git checkout", dir)
	}
	return root, nil
}

// checkout runs one git rev-parse in dir. git's stderr is discarded, as the
// oracle does, so a failure prints nothing of its own.
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

// Real is the path with every symlink resolved, or the path itself when it
// cannot be resolved (it does not exist).
func Real(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return path
}

// DispatchDir is the fixed folder the dispatcher runs from:
// <home>/.jitso/dispatch-cwd, outside every checkout.
func DispatchDir(home string) string {
	return filepath.Join(home, ".jitso", "dispatch-cwd")
}

// Self is the real path of the running executable (symlinks resolved), the
// identity both hooks compare a command's first word against.
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
