// Package trust decides where a launch may run and answers the
// trust-this-folder dialog of a freshly started session. A launch runs only in
// a worktree of its target repository (the primary checkout and the worktrees
// git lists for it), or a folder the caller configured
// (tests): Precheck and Place enforce it on every launch (defect N-22),
// whether or not a dialog shows. The dialog appears even in bypass mode, and Enter alone picks "No, exit"; Handle
// confirms it only for a trusted folder and leaves it alone anywhere else.
package trust

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/harness"
)

// Outcome is how the dialog was handled; it is the trust_dialog field of the
// launch event.
type Outcome string

// The outcomes.
const (
	// Absent: the first screen is not the dialog; no key was sent.
	Absent Outcome = "absent"
	// Handled: the dialog was confirmed and went away.
	Handled Outcome = "handled"
	// Timeout: the dialog was confirmed but did not go away in Wait.
	Timeout Outcome = "timeout"
	// Refused: the dialog is up in a folder that is not trusted; no key was sent.
	Refused Outcome = "refused"
)

// Wait is how long Handle polls for the dialog to go away, and Poll the pause
// between two reads.
const (
	Wait = 10 * time.Second
	Poll = 500 * time.Millisecond
)

// Policy is the set of trusted folders.
type Policy struct {
	// Primary is the primary checkout; every worktree `git worktree list`
	// reports for it is trusted.
	Primary string
	// Extra are folders a caller configured. No CLI flag sets it.
	Extra []string
	// Repo names the repository in a rejection: the path of the target of a
	// launch into another repository, "" for "this repo".
	Repo string
}

func (p Policy) repo() string {
	if p.Repo != "" {
		return p.Repo
	}
	return "this repo"
}

// Trusted reports whether dir is exactly a trusted folder, comparing real
// paths: a lookalike prefix or a subfolder is not trusted. An empty dir (Orca
// gave no folder) is never trusted.
func (p Policy) Trusted(dir string) bool {
	if dir == "" {
		return false
	}
	return slices.Contains(p.allowed(), real(dir))
}

// allowed are the real paths of every folder a launch may run in.
func (p Policy) allowed() []string {
	list := append(worktrees(p.Primary), p.Extra...)
	for i := range list {
		list[i] = real(list[i])
	}
	return list
}

// SelectorPath is the folder a --worktree selector names: P of path:<P> and
// of id:<id>::<P> (Orca's own worktreeId), cleaned. ok is false for any other
// selector or none, which names no path.
func SelectorPath(selector string) (path string, ok bool) {
	if rest, found := strings.CutPrefix(selector, "path:"); found {
		return filepath.Clean(rest), true
	}
	if rest, found := strings.CutPrefix(selector, "id:"); found {
		if _, p, found := strings.Cut(rest, "::"); found {
			return filepath.Clean(p), true
		}
	}
	return "", false
}

// Precheck is "" when the selector may be used, else the rejection text
// without the "JITSO REJECTED: " prefix. A selector that names a path must
// name an absolute path of a worktree of this repo; any other selector has no
// pre-check.
func (p Policy) Precheck(selector string) string {
	path, ok := SelectorPath(selector)
	if !ok {
		return ""
	}
	if !filepath.IsAbs(path) {
		return "--worktree " + selector + " must name an absolute path"
	}
	if !slices.Contains(p.allowed(), real(path)) {
		return "--worktree " + selector + " names " + path + ", which is not a worktree of " + p.repo()
	}
	return ""
}

// Place is "" when the folder Orca reported (reported, "" when none) may run
// the launch for selector, else the rejection text without the prefix. The
// folder must be a trusted one and, when the selector names a path, must be
// that path.
func (p Policy) Place(reported, selector string) string {
	list := p.allowed()
	if reported != "" && slices.Contains(list, real(reported)) {
		path, ok := SelectorPath(selector)
		if !ok || real(path) == real(reported) {
			return ""
		}
	}
	return p.Untrusted(reported, selector)
}

// Untrusted is the rejection text for a folder that may not run the launch.
// It names the path the selector asked for when the selector names one.
func (p Policy) Untrusted(reported, selector string) string {
	folder := reported
	if folder == "" {
		folder = "(unknown)"
	}
	if path, ok := SelectorPath(selector); ok {
		return "untrusted folder " + folder + ": --worktree asked for " + path + "; the terminal was closed"
	}
	return "untrusted folder " + folder + ": a launch runs only in a worktree of " + p.repo() + "; the terminal was closed"
}

// worktrees are the folders `git worktree list --porcelain` reports for the
// primary checkout, none when git fails.
func worktrees(primary string) []string {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = primary
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, line := range strings.Split(string(out), "\n") {
		if dir, ok := strings.CutPrefix(line, "worktree "); ok {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func real(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// Screen is the terminal the dialog shows on.
type Screen interface {
	// Read is the rendered screen.
	Read() (string, error)
	// Send sends raw key text.
	Send(text string) error
}

// Clock is the time source of the poll loop.
type Clock struct {
	Now   func() time.Time
	Sleep func(time.Duration)
}

// Handle reads the screen once. No dialog: Absent, no key. A dialog in an
// untrusted folder: Refused, no key. Otherwise it sends the confirm keys and
// polls for up to Wait for the dialog to go away: Handled, or Timeout. A
// failed read or send is returned as an error.
func Handle(dialog harness.TrustDialog, screen Screen, clock Clock, trusted bool) (Outcome, error) {
	first, err := screen.Read()
	if err != nil {
		return "", err
	}
	if !dialog.Detect(first) {
		return Absent, nil
	}
	if !trusted {
		return Refused, nil
	}
	for _, key := range dialog.ConfirmKeys {
		if err := screen.Send(key); err != nil {
			return "", err
		}
	}
	start := clock.Now()
	for clock.Now().Sub(start) < Wait {
		clock.Sleep(Poll)
		text, err := screen.Read()
		if err != nil {
			return "", err
		}
		if !dialog.Detect(text) {
			return Handled, nil
		}
	}
	return Timeout, nil
}
