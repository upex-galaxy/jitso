package hook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// sessionEndLockWait is how long SessionEnd waits for the registry lock:
// the hook shares a 1.5 s budget with Claude Code's shutdown.
const sessionEndLockWait = 600 * time.Millisecond

// Deps are the ports of both hooks. The zero value of an optional field
// falls back to the real one.
type Deps struct {
	Stdin  io.Reader
	Stdout io.Writer
	// Getenv is the environment lookup, "" when unset.
	Getenv func(string) string
	// Getwd and Executable are os.Getwd and os.Executable.
	Getwd      func() (string, error)
	Executable func() (string, error)
	// Now and Sleep are the clock (registry timestamps and lock waits).
	Now   func() time.Time
	Sleep func(time.Duration)
}

func (d Deps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

func (d Deps) getwd() (string, error) {
	if d.Getwd != nil {
		return d.Getwd()
	}
	return os.Getwd()
}

// readInput is the hook payload as a JSON object. ok is false when stdin
// cannot be read, is not JSON, or is JSON that is not an object.
func (d Deps) readInput() (input map[string]any, ok bool) {
	if d.Stdin == nil {
		return nil, false
	}
	raw, err := io.ReadAll(d.Stdin)
	if err != nil {
		return nil, false
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil, false
	}
	input, ok = v.(map[string]any)
	return input, ok
}

// own is the real path of the running executable and the predicate "this word
// is that executable": an absolute path whose real path is the same. When the
// executable cannot be resolved the predicate is always false.
func (d Deps) own() (self string, isOwn func(string) bool) {
	self, err := project.Self(d.Executable)
	return self, func(word string) bool {
		return err == nil && filepath.IsAbs(word) && project.Real(word) == self
	}
}

// RunGuard is `jitso hook guard`: in a project that has jitso/jitso.yaml it
// denies the ways of launching an agent that skip the launch registry and any
// `jitso build save` (only the owner saves a draft as a build) and any
// `jitso setup` or `jitso install` that is not a dry run (only the owner changes
// the setup), and on
// SessionEnd it ends the launch of the closing session. It fails open: input
// it cannot read, or a project without a catalog, prints nothing.
func RunGuard(d Deps) {
	dir := d.Getenv("CLAUDE_PROJECT_DIR")
	if dir == "" {
		cwd, err := d.getwd()
		if err != nil {
			return
		}
		root, err := project.Resolve(cwd, "")
		if err != nil {
			return
		}
		dir = root.Primary
	}
	if _, err := os.Stat(filepath.Join(dir, "jitso", "jitso.yaml")); err != nil {
		return
	}
	input, ok := d.readInput()
	if !ok {
		return
	}
	if input["hook_event_name"] == "SessionEnd" {
		d.endSession(dir, input["session_id"])
		return
	}
	_, isOwn := d.own()
	launcher := Launcher{IsOwn: isOwn, Bounds: d.boundary(dir)}
	if denial := launcher.Guard(input); denial.Verb != "" {
		pkg, _ := os.ReadFile(filepath.Join(dir, "package.json"))
		io.WriteString(d.Stdout, string(Answer(Deny, denial.Reason(pkg))))
	}
}

// endSession writes the end event of the closing session's launch to the
// registry of the project's primary checkout. A registry problem is swallowed:
// it must not break the closing of a session.
func (d Deps) endSession(dir string, sessionID any) {
	id, ok := sessionID.(string)
	if !ok {
		return
	}
	state := filepath.Join(dir, ".jitso")
	if root, err := project.Resolve(dir, ""); err == nil {
		state = root.StateDir()
	}
	store := registry.NewFileStore(state)
	store.LockTimeout = sessionEndLockWait
	if d.Now != nil {
		store.Now = d.Now
	}
	if d.Sleep != nil {
		store.Sleep = d.Sleep
	}
	_ = EndLaunch(store, store.Path(), id, d.now())
}

// RunDispatchGuard is `jitso hook dispatch-guard`: it answers allow for the
// one launcher line and the brief writes the dispatcher may do, and deny for
// everything else, including input it cannot read (it fails closed).
func RunDispatchGuard(d Deps) {
	answer := func(decision, reason string) {
		io.WriteString(d.Stdout, string(Answer(decision, DispatchReason(reason))))
	}
	input, ok := d.readInput()
	if !ok {
		answer(Deny, "the hook input was not a JSON object")
		return
	}
	home := d.Getenv("HOME")
	if home == "" {
		answer(Deny, "HOME is not set, so the dispatcher folder is unknown")
		return
	}
	self, isOwn := d.own()
	g := DispatchGuard{
		DispatchDir: project.DispatchDir(home),
		Launcher:    self,
		IsOwn:       isOwn,
		Real:        project.Real,
		IsSymlink: func(path string) bool {
			info, err := os.Lstat(path)
			return err == nil && info.Mode()&os.ModeSymlink != 0
		},
		Exists: func(path string) bool {
			_, err := os.Lstat(path)
			return err == nil
		},
	}
	g.Primary, g.PrimaryProblem = d.primary(home)
	if problem := g.Problem(input); problem != "" {
		answer(Deny, problem)
		return
	}
	answer(Allow, AllowedReason)
}

// primary is the real path of the primary checkout JITSO_PROJECT names, which
// the dispatcher sets for its claude. When it cannot be known, problem says why
// and the guard denies a line that holds --worktree.
func (d Deps) primary(home string) (primary, problem string) {
	env := d.Getenv(project.EnvVar)
	if env == "" {
		return "", project.EnvVar + " is not set, so the primary worktree is unknown"
	}
	root, err := project.Resolve(project.DispatchDir(home), env)
	if err != nil {
		return "", project.EnvVar + " does not name a git checkout"
	}
	return root.Primary, ""
}

// boundary is the write boundary of a guard that runs for the checkout dir.
// The primary checkout is looked up only when a removal operand needs it.
func (d Deps) boundary(dir string) *Boundary {
	var primary string
	var done bool
	return &Boundary{
		Dir:  dir,
		Home: d.Getenv("HOME"),
		FS:   OSFS(),
		Primary: func() string {
			if !done {
				done = true
				if root, err := project.Resolve(dir, ""); err == nil {
					primary = root.Primary
				}
			}
			return primary
		},
	}
}
