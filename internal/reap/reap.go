// Package reap is `jitso reap`: the audit that catches what the hooks cannot
// see. Prevention lives in the guard hook; this is detection, in two phases.
//
//  1. A launch with no end event whose terminal is gone from `terminal list`
//     gets an end event with source reap.
//  2. Every worker of the runs the registry knows (plus any --run) that no
//     launch explains is printed as an `unlogged worker:` line. A worker is
//     explained by its terminal handle (the key terminal of the default
//     worker-list view, else agentTerminalHandle, else
//     resource.terminalHandle) matching a launch's, or by the
//     [jitso:<id>] mark of a known launch in any string of its entry. Orca has
//     no verb that lists every run, so a run the registry never heard of is
//     audited only when named with --run. The phase is output only: it writes
//     no event and remembers nothing, so every reap lists the same worker
//     again. A worker seen in several runs of one reap is listed once.
//
// The registry is read again at the start of every phase, so the second sees
// what the first wrote.
package reap

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsonx"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// Usage is the reason of every rejection of the arguments.
const Usage = "usage: jitso reap [--run <run_id>]..."

// runPattern is the allowlist of a run id: plain characters, never starting
// with "-", at most 64 long, and never holding "..". It is the pattern the
// launcher uses for --run (shell.CheckLaunchField), copied here because the
// lifecycle track does not import the launch track's package.
var runPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)

func validRun(s string) bool { return runPattern.MatchString(s) && !strings.Contains(s, "..") }

// Orca is the part of the Orca client reap calls.
type Orca interface {
	ListHandles() ([]string, error)
	Call(args []string) (map[string]any, error)
}

// Deps is what Run needs from the outside.
type Deps struct {
	Store registry.Store
	Orca  Orca
	Now   func() time.Time
	// Err receives the lines for stderr (a run that could not be listed); they
	// do not fail the reap.
	Err func(line string)
}

// ParseArgs is the extra runs of `jitso reap [--run <run_id>]...`: only pairs
// of --run and a valid id. Anything else rejects the whole call (exit 2).
func ParseArgs(args []string) ([]string, error) {
	var runs []string
	for i := 0; i < len(args); i += 2 {
		if args[i] != "--run" || i+1 >= len(args) || !validRun(args[i+1]) {
			return nil, fail.Rejected(Usage)
		}
		runs = append(runs, args[i+1])
	}
	return runs, nil
}

// Run reaps and returns the text for stdout: one `unlogged worker:` line per
// worker no launch explains, then the summary line. extraRuns are the --run
// values, audited in the order given before the runs the registry knows.
func Run(extraRuns []string, d Deps) (string, error) {
	live, err := d.Orca.ListHandles()
	if err != nil {
		return "", err
	}
	ended, err := endGone(d, live)
	if err != nil {
		return "", err
	}
	unlogged, err := auditWorkers(d, extraRuns)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(unlogged)+1)
	for _, evidence := range unlogged {
		lines = append(lines, "unlogged worker: "+evidence)
	}
	lines = append(lines, fmt.Sprintf("jitso reap: %d launch(es) marked ended, %d unlogged worker(s)", ended, len(unlogged)))
	return strings.Join(lines, "\n"), nil
}

// endGone is phase 1. A launch whose terminal handle is not in the list is
// ended, and so is one with no string handle at all (absent, null, a number):
// it can never be matched against the list, so leaving it running would be
// forever (defect N-28). Such a line is never written by
// `jitso launch`; it comes from hand-made or foreign lines.
func endGone(d Deps, live []string) (int, error) {
	launches, err := registry.ReadLaunches(d.Store)
	if err != nil {
		return 0, err
	}
	ended := 0
	for _, l := range launches {
		if l.Ended() {
			continue
		}
		terminal, ok := l.Launch.String("orca.terminal")
		if ok && slices.Contains(live, terminal) {
			continue
		}
		now := d.Now()
		err := d.Store.Append(&registry.EndEvent{
			ID: l.ID, TS: jsonx.Time(now), Source: "reap", OutcomeReported: "none",
			FooterPath: nil, DurationS: seconds(l.Launch, now),
		})
		if err != nil {
			return 0, err
		}
		ended++
	}
	return ended, nil
}

// auditWorkers is phase 2. It returns the evidence text of each worker no
// launch explains, in the order found. A worker is keyed by its terminal handle
// when it has one (so it is listed once across runs) and by its evidence text
// when it has none.
func auditWorkers(d Deps, extraRuns []string) ([]string, error) {
	launches, err := registry.ReadLaunches(d.Store)
	if err != nil {
		return nil, err
	}
	handles := map[string]bool{}
	var marks []string
	var runs []string // insertion-ordered set: --run values first, then the registry's
	addRun := func(r string) {
		if !slices.Contains(runs, r) {
			runs = append(runs, r)
		}
	}
	for _, r := range extraRuns {
		addRun(r)
	}
	for _, l := range launches {
		if t, ok := l.Launch.String("orca.terminal"); ok {
			handles[t] = true
		}
		if r, ok := l.Launch.String("orca.run"); ok && validRun(r) {
			addRun(r)
		}
		marks = append(marks, "[jitso:"+l.ID+"]")
	}

	var unlogged []string
	seen := map[string]bool{}
	for _, run := range runs {
		res, err := d.Orca.Call([]string{"orchestration", "worker-list", "--run", run})
		if err != nil {
			d.Err("jitso: could not list the workers of run " + run + ": " + err.Error())
			continue
		}
		for _, w := range workersOf(res) {
			handle, hasHandle := handleOf(w)
			if (hasHandle && handles[handle]) || marked(w, marks) {
				continue
			}
			shown := "(no terminal handle)"
			key := ""
			if hasHandle {
				shown = handle
				key = "handle:" + handle
			}
			evidence := "worker " + shown + " in run " + run + " has no launch event"
			if !hasHandle {
				key = "evidence:" + evidence
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			unlogged = append(unlogged, evidence)
		}
	}
	return unlogged, nil
}

// workersOf is the list of worker objects in a worker-list result: the
// property named workers when it is a list, else the first list-valued
// property in key order. (The client's result is an unordered map, so the
// order is made explicit.) Entries that are not objects are dropped.
func workersOf(result map[string]any) []map[string]any {
	list, ok := result["workers"].([]any)
	if !ok {
		keys := make([]string, 0, len(result))
		for k := range result {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if l, isList := result[k].([]any); isList {
				list, ok = l, true
				break
			}
		}
	}
	var workers []map[string]any
	for _, item := range list {
		if w, isObject := item.(map[string]any); isObject {
			workers = append(workers, w)
		}
	}
	return workers
}

// handleOf is the terminal handle of a worker: terminal (the default view of
// worker-list), else agentTerminalHandle, else resource.terminalHandle (the
// --full record). A key whose value is not a string is skipped.
func handleOf(w map[string]any) (string, bool) {
	if h, ok := w["terminal"].(string); ok {
		return h, true
	}
	if h, ok := w["agentTerminalHandle"].(string); ok {
		return h, true
	}
	if res, ok := w["resource"].(map[string]any); ok {
		if h, ok := res["terminalHandle"].(string); ok {
			return h, true
		}
	}
	return "", false
}

// marked is true when any string anywhere inside the worker entry (keys
// excluded) holds the mark of a known launch.
func marked(w map[string]any, marks []string) bool {
	found := false
	walkStrings(w, func(s string) {
		if found {
			return
		}
		for _, m := range marks {
			if strings.Contains(s, m) {
				found = true
				return
			}
		}
	})
	return found
}

func walkStrings(v any, fn func(string)) {
	switch x := v.(type) {
	case string:
		fn(x)
	case []any:
		for _, item := range x {
			walkStrings(item, fn)
		}
	case map[string]any:
		for _, item := range x {
			walkStrings(item, fn)
		}
	}
}

// seconds is the whole seconds from the launch's ts to now, rounded like
// JavaScript's Math.round and never negative; 0 when the ts does not parse.
func seconds(launch registry.Event, now time.Time) int64 {
	ts, _ := launch.String("ts")
	started, ok := registry.ParseTime(ts)
	if !ok {
		return 0
	}
	s := float64(now.UnixMilli()-started.UnixMilli()) / 1000
	return max(0, int64(math.Floor(s+0.5)))
}
