// Package sandbox builds the per-case temp root of SPEC 5: home/, project/, bin/ with the
// fakes, tools/ with the programs an adapter needs, the scenario and the fakes' log files,
// and the from-scratch environment every process of the case gets.
package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"jitso-contract-runner/internal/fakes"
)

// SystemPath is the minimal system path after {{BIN}} (SPEC 5): /usr/bin and /bin on
// POSIX. On Windows it is the folder of git (the CLI under test runs git) and System32.
func SystemPath() string {
	if runtime.GOOS != "windows" {
		return "/usr/bin" + string(os.PathListSeparator) + "/bin"
	}
	dirs := []string{}
	if git, err := exec.LookPath("git"); err == nil {
		dirs = append(dirs, filepath.Dir(git))
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		dirs = append(dirs, filepath.Join(root, "System32"))
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

// exeName is the file name of a program: Windows needs the .exe.
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Sandbox is one case's temp root. Every path is a real path (symlinks resolved).
type Sandbox struct {
	Root       string
	Project    string
	Home       string
	Bin        string
	Tools      string
	Scenario   string
	OrcaLog    string
	ClaudeDump string

	extraEnv map[string]string
}

// New creates the temp root and links the runner binary (self) into bin/ as orca and claude.
func New(self string) (*Sandbox, error) {
	tmp, err := os.MkdirTemp("", "jitso-contract-")
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	s := &Sandbox{
		Root:       root,
		Project:    filepath.Join(root, "project"),
		Home:       filepath.Join(root, "home"),
		Bin:        filepath.Join(root, "bin"),
		Tools:      filepath.Join(root, "tools"),
		Scenario:   filepath.Join(root, "orca-scenario.json"),
		OrcaLog:    filepath.Join(root, "orca-calls.jsonl"),
		ClaudeDump: filepath.Join(root, "claude-dump.json"),
		extraEnv:   map[string]string{},
	}
	for _, dir := range []string{s.Home, s.Bin, s.Tools} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			s.Remove()
			return nil, err
		}
	}
	for _, name := range []string{"orca", "claude"} {
		if err := linkOrCopy(self, filepath.Join(s.Bin, exeName(name))); err != nil {
			s.Remove()
			return nil, fmt.Errorf("install fake %s: %w", name, err)
		}
	}
	return s, nil
}

// Remove deletes the whole temp root.
func (s *Sandbox) Remove() error { return os.RemoveAll(s.Root) }

// Inside reports whether path, with its symlinks resolved when it exists, lies inside the
// sandbox root.
func (s *Sandbox) Inside(path string) bool {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	rel, err := filepath.Rel(s.Root, path)
	return err == nil && filepath.IsLocal(rel)
}

// LinkTool makes one program reachable on PATH through tools/, and nothing else of its folder.
func (s *Sandbox) LinkTool(name, target string) error {
	return os.Symlink(target, filepath.Join(s.Tools, name))
}

// SetEnv adds a variable an adapter needs to start its implementation.
func (s *Sandbox) SetEnv(name, value string) { s.extraEnv[name] = value }

// Env is the environment of every process of the case: built from scratch, never inherited.
func (s *Sandbox) Env(step map[string]string) []string {
	env := map[string]string{
		"HOME":              s.Home,
		"PATH":              strings.Join([]string{s.Bin, SystemPath(), s.Tools}, string(os.PathListSeparator)),
		"ORCA_CLI_COMMAND":  filepath.Join(s.Bin, exeName("orca")),
		fakes.EnvScenario:   s.Scenario,
		fakes.EnvOrcaLog:    s.OrcaLog,
		fakes.EnvClaudeDump: s.ClaudeDump,
	}
	if runtime.GOOS == "windows" {
		// A Windows process needs its profile folder and a few system variables to start.
		env["USERPROFILE"] = s.Home
		for _, k := range []string{"SystemRoot", "SystemDrive", "windir", "TEMP", "TMP", "ComSpec", "PATHEXT"} {
			if v := os.Getenv(k); v != "" {
				env[k] = v
			}
		}
	}
	for k, v := range s.extraEnv {
		env[k] = v
	}
	for k, v := range step {
		env[k] = v
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = k + "=" + env[k]
	}
	return out
}

// WriteScenario writes the normalized fake orca scenario.
func (s *Sandbox) WriteScenario(sc fakes.Scenario) error {
	data, err := json.MarshalIndent(sc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.Scenario, data, 0o644)
}

// GitInit runs `git init -q` in the project with the sandbox environment.
func (s *Sandbox) GitInit() error {
	return s.git("init", "-q")
}

// gitIdentity is the runner-controlled author of the sandbox's one commit; no user or
// system config is read, so no signing key or hook of the owner can take part.
var gitIdentity = map[string]string{
	"GIT_AUTHOR_NAME":     "jitso-contract",
	"GIT_AUTHOR_EMAIL":    "contract@example.invalid",
	"GIT_COMMITTER_NAME":  "jitso-contract",
	"GIT_COMMITTER_EMAIL": "contract@example.invalid",
	"GIT_CONFIG_NOSYSTEM": "1",
	"GIT_CONFIG_GLOBAL":   os.DevNull,
}

// git runs one git command in the project, with the sandbox environment plus gitIdentity,
// commit signing off and hooks disabled.
func (s *Sandbox) git(args ...string) error { return s.gitIn(s.Project, args...) }

// gitIn is git run in dir.
func (s *Sandbox) gitIn(dir string, args ...string) error {
	path := "/usr/bin/git"
	if _, err := os.Stat(path); err != nil || runtime.GOOS == "windows" {
		if path, err = exec.LookPath("git"); err != nil {
			return err
		}
	}
	cmd := exec.Command(path, append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.DevNull}, args...)...)
	cmd.Dir = dir
	cmd.Env = s.Env(gitIdentity)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Worktree is the real path a linked worktree of that name gets (it may not exist yet).
func (s *Sandbox) Worktree(name string) string {
	return filepath.Join(s.Root, "worktrees", name)
}

// AddWorktree makes one empty commit when HEAD has none yet, then `git worktree add`s a new
// branch `contract-<name>` at Worktree(name).
func (s *Sandbox) AddWorktree(name string) error {
	if s.git("rev-parse", "--verify", "-q", "HEAD") != nil {
		if err := s.git("commit", "--allow-empty", "--no-verify", "-q", "-m", "contract: initial commit"); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(s.Worktree(name)), 0o755); err != nil {
		return err
	}
	return s.git("worktree", "add", "-q", "-b", "contract-"+name, s.Worktree(name))
}

// InitRepo creates path (a folder relative to the project dir, or an absolute path) and its
// parents, then runs `git init -q` there with the same controlled git settings as every other
// git command of the sandbox. The path, with its existing symlinks resolved, must lie inside
// the project dir or HOME, and must not hold a `.git` yet: both are runner errors.
func (s *Sandbox) InitRepo(path string) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.Project, path)
	}
	path = filepath.Clean(path)
	if err := s.insideProjectOrHome(path); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
		return fmt.Errorf("git_init: %s already holds a .git", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	return s.gitIn(path, "init", "-q")
}

// insideProjectOrHome refuses a path whose deepest existing ancestor resolves, through any
// symlink, outside the project dir and HOME. The part below that ancestor does not exist yet,
// so it holds no symlink and the cleaned path cannot climb out through it.
func (s *Sandbox) insideProjectOrHome(path string) error {
	existing := path
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		existing = parent
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return err
	}
	for _, base := range []string{s.Project, s.Home} {
		realBase, err := filepath.EvalSymlinks(base)
		if err != nil {
			continue
		}
		if rel, err := filepath.Rel(realBase, real); err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return nil
		}
	}
	return fmt.Errorf("git_init: %s resolves outside the project dir and HOME", path)
}

// RemoveFake takes a fake off PATH (the case field `claude: {"missing": true}`).
func (s *Sandbox) RemoveFake(name string) error {
	return os.Remove(filepath.Join(s.Bin, exeName(name)))
}

// ClearClaudeDump removes the fake claude dump, so a check sees only the current step.
func (s *Sandbox) ClearClaudeDump() error {
	err := os.Remove(s.ClaudeDump)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// linkOrCopy installs the runner binary under a fake's name. A symlink comes
// first: on macOS a freshly hard-linked or copied executable was occasionally
// killed with SIGKILL on its first execution, while a symlink resolves to the
// binary that is already running and was already checked. Then a hard link,
// then a copy.
func linkOrCopy(src, dst string) error {
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}
