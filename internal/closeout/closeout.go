// Package closeout is `jitso close <launch_id>`: it reads the screen footer of
// the launch's terminal, closes that one terminal (never the worktree-wide
// close), reads the terminal list to confirm it is gone, and only then saves
// the footer and writes the end event, because a success code describes the
// call, not the outcome.
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
// treated as closed.
//
// For a launch that is still running the order is: read the footer, close the
// terminal, list the terminals, and only then write footer.txt and the end
// event (TS-ODDITIES R1, defect N-28). A failure at any step writes nothing, so
// the launch stays running: a later close retries cleanly, and reap ends it
// once the terminal is gone.
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

	var footer footerText
	if !l.Ended() {
		if footer, err = readFooter(handle, d); err != nil {
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
	if !l.Ended() {
		if err := record(l, footer, d); err != nil {
			return "", err
		}
	}
	return "jitso: closed " + id + " (terminal " + handle + ")", nil
}

// footerText is the screen of the terminal read before it was closed; Found is
// false for a terminal Orca reported as stale, which has no footer to save.
type footerText struct {
	Text  string
	Found bool
}

// readFooter reads the terminal's screen. A terminal that is already gone has
// no footer; any other error is returned.
func readFooter(handle string, d Deps) (footerText, error) {
	text, err := d.Orca.ReadScreen(handle)
	switch {
	case err == nil:
		return footerText{Text: text, Found: true}, nil
	case isStale(err):
		return footerText{}, nil
	}
	return footerText{}, err
}

// record saves the footer and appends the end event, once the terminal is
// confirmed gone. A launch with no footer still ends, with footer_path null.
func record(l *registry.Launch, footer footerText, d Deps) error {
	folder := filepath.Join(d.StateDir, "launches", l.ID)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	var footerPath *string
	if footer.Found {
		path := filepath.Join(folder, "footer.txt")
		if err := os.WriteFile(path, []byte(footer.Text+"\n"), 0o644); err != nil {
			return err
		}
		footerPath = &path
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
