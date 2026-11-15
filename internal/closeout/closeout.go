// Package closeout is `jitso close <launch_id>`: it saves the screen footer of
// the launch's terminal, writes the end event, closes that one terminal (never
// the worktree-wide close) and then reads the terminal list to confirm it is
// gone, because a success code describes the call, not the outcome.
//
// The package is named closeout so it does not shadow the close builtin.
package closeout

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Usage is the reason of every rejection of the arguments.
const Usage = "usage: jitso close <launch_id>"

// Orca is the part of the Orca client close calls.
type Orca interface {
	ReadScreen(handle string) (string, error)
	CloseTerminal(handle string) error
	ListHandles() ([]string, error)
}

// Deps is what Run needs from the outside.
type Deps struct {
	// StateDir is the .jitso/ folder of the primary checkout; the footer goes
	// to <StateDir>/launches/<id>/footer.txt.
	StateDir string
	Store    registry.Store
	Orca     Orca
	Now      func() time.Time
}

// ParseArgs is the launch id of `jitso close <launch_id>`: exactly one
// argument of the exact id form, else a rejection (exit 2).
func ParseArgs(args []string) (string, error) {
	if len(args) != 1 || !registry.IsLaunchID(args[0]) {
		return "", fail.Rejected(Usage)
	}
	return args[0], nil
}

// Run closes the launch and returns the line for stdout.
//
// An unknown launch, or one with no terminal handle, is a rejection (exit 2).
// Any Orca or file failure is a plain error (exit 3). A launch that already has
// an end event keeps it: no second event is written, but the terminal is still
// closed and listed. A terminal Orca reports as stale (the owner closed it) is
// treated as closed. The end event is written before the terminal is closed, so
// when the close then fails the launch stays ended and the exit is 3 (R1: the
// oracle's order, pinned by close/orca-close-failure-is-exit-3).
func Run(id string, d Deps) (string, error) {
	launches, err := registry.ReadLaunches(d.Store)
	if err != nil {
		return "", err
	}
	l, ok := registry.Find(launches, id)
	if !ok {
		return "", fail.Rejectedf("unknown launch %s", id)
	}
	handle, ok := l.Launch.String("orca.terminal")
	if !ok {
		return "", fail.Rejectedf("launch %s has no terminal handle", id)
	}

	if !l.Ended() {
		if err := end(l, handle, d); err != nil {
			return "", err
		}
	}
	if err := d.Orca.CloseTerminal(handle); err != nil && !isStale(err) {
		return "", err
	}
	handles, err := d.Orca.ListHandles()
	if err != nil {
		return "", err
	}
	if slices.Contains(handles, handle) {
		return "", fail.Envf("terminal %s is still open after close", handle)
	}
	return "jitso: closed " + id + " (terminal " + handle + ")", nil
}

// end saves the footer and appends the end event. A terminal that is already
// gone has no footer to save; the launch still ends, with footer_path null.
func end(l *registry.Launch, handle string, d Deps) error {
	folder := filepath.Join(d.StateDir, "launches", l.ID)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	var footerPath *string
	footer, err := d.Orca.ReadScreen(handle)
	switch {
	case err == nil:
		path := filepath.Join(folder, "footer.txt")
		if err := os.WriteFile(path, []byte(footer+"\n"), 0o644); err != nil {
			return err
		}
		footerPath = &path
	case !isStale(err):
		return err
	}
	ended := d.Now()
	return d.Store.Append(&registry.EndEvent{
		ID:              l.ID,
		TS:              jsonx.Time(ended),
		Source:          "jitso_close",
		OutcomeReported: "none",
		FooterPath:      footerPath,
		DurationS:       seconds(l.Launch, ended),
	})
}

func isStale(err error) bool {
	var oe *orca.Error
	return errors.As(err, &oe) && oe.Code == orca.CodeHandleStale
}

// seconds is the whole seconds from the launch's ts to now, rounded like
// JavaScript's Math.round and never negative (a launch stamped in the future
// gets 0). A ts that does not parse gives 0.
func seconds(launch registry.Event, now time.Time) int64 {
	ts, _ := launch.String("ts")
	started, ok := registry.ParseTime(ts)
	if !ok {
		return 0
	}
	s := float64(now.UnixMilli()-started.UnixMilli()) / 1000
	return max(0, int64(math.Floor(s+0.5)))
}
