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
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/registry"
	"github.com/upex-galaxy/jitso/internal/usage"
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
	// Warn receives a warning that does not fail the close (an unreadable
	// transcript); nil drops it.
	Warn func(string)
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
// event (defect N-28). A failure at any step writes nothing, so
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
	line := "jitso: closed " + id + " (terminal " + handle + ")"
	if !l.Ended() {
		totals, err := readTokens(l, d)
		if err != nil {
			return "", err
		}
		if err := record(l, footer, totals, d); err != nil {
			return "", err
		}
		if totals != nil {
			line += "\n" + tokensLine(totals)
		}
	}
	return line, nil
}

func tokensLine(t *usage.Totals) string {
	return fmt.Sprintf("tokens: input %d, cache write %d, cache read %d, output %d (%d messages, %d subagent transcripts)",
		t.Input, t.CacheWrite, t.CacheRead, t.Output, t.Messages, t.SubagentFiles)
}

// readTokens sums the launch's transcript and every subagent transcript next
// to it. It returns nil when the launch has no transcript (silently, a launch
// run before a session id existed never has one) or when it cannot be read
// (with one warning): tokens never fail a close.
func readTokens(l *registry.Launch, d Deps) (*usage.Totals, error) {
	path, ok := l.Launch.String("transcript_path")
	if !ok || path == "" {
		return nil, nil
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	total, err := sumFile(path)
	if err != nil {
		if d.Warn != nil {
			d.Warn("jitso: warning: could not read the transcript " + path + ": " + err.Error())
		}
		return nil, nil
	}
	subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(path, ".jsonl"), "subagents", "*.jsonl"))
	for _, sub := range subs {
		part, err := sumFile(sub)
		if err != nil {
			continue
		}
		total.Add(part)
		total.SubagentFiles++
	}
	return &total, nil
}

func sumFile(path string) (usage.Totals, error) {
	f, err := os.Open(path)
	if err != nil {
		return usage.Totals{}, err
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && info.IsDir() {
		return usage.Totals{}, errors.New("is a directory")
	}
	return usage.Sum(f)
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
func record(l *registry.Launch, footer footerText, totals *usage.Totals, d Deps) error {
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
		Tokens:          &registry.Tokens{Totals: totals},
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
