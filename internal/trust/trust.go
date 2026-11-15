// Package trust answers the trust-this-folder dialog of a freshly started
// session. The dialog appears even in bypass mode, and Enter alone picks
// "No, exit". The launcher confirms it only for a worktree of the primary
// checkout, the fixed dispatcher folder, or a folder the caller configured
// (tests); anywhere else it refuses and leaves the dialog alone.
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
	// DispatchDir is the fixed dispatcher folder.
	DispatchDir string
	// Extra are folders a caller configured. No CLI flag sets it.
	Extra []string
}

// Trusted reports whether dir is exactly a trusted folder, comparing real
// paths: a lookalike prefix or a subfolder is not trusted. An empty dir (Orca
// gave no folder) is never trusted.
func (p Policy) Trusted(dir string) bool {
	if dir == "" {
		return false
	}
	list := append(append(worktrees(p.Primary), p.DispatchDir), p.Extra...)
	for i := range list {
		list[i] = real(list[i])
	}
	return slices.Contains(list, real(dir))
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
