// Package registry is the launch registry: .jitso/launches.jsonl in the
// primary checkout, one JSON event per line, append-only, schema version 2.
//
// A launch's state is the fold of its events by id: the first launch line of
// an id wins, a later end or analyzed event wins over an earlier one, nothing
// is rewritten. Every write is one line under the launches.lock folder (mkdir
// is atomic), so concurrent writers never interleave.
//
// Writers build one of the typed records (LaunchEvent, EndEvent,
// AnalyzedEvent, RuleBreakEvent). Readers get Event values: the decoded JSON
// object of a valid line, read through tolerant accessors, because lines may
// be written by other versions or by hand and carry fields of any type.
package registry

import (
	"regexp"
	"strconv"
	"strings"
)

// The event kinds.
const (
	KindLaunch    = "launch"
	KindEnd       = "end"
	KindAnalyzed  = "analyzed"
	KindRuleBreak = "rule_break"
)

// Version is the schema version of every line a writer appends and the only one
// a reader accepts: a line of another version (the TypeScript era wrote v: 1,
// with the build id under the key `preset`) is an invalid line and is skipped.
const Version = 2

// Allowed values (schema v2). Modes are only direct and fleet: the Spanish
// aliases are invalid everywhere, registry lines included (TS-ODDITIES L8).
var (
	Modes          = []string{"direct", "fleet"}
	EndSources     = []string{"worker_done", "session_end_hook", "jitso_close", "reap"}
	Outcomes       = []string{"succeeded", "failed", "blocked", "none"}
	Verdicts       = []string{"success", "fail", "unknown"}
	RuleBreakKinds = []string{"unlogged_worker", "secret_path_read", "budget_over"}
)

var launchIDPattern = regexp.MustCompile(`^L[0-9]{8}T[0-9]{6}-[0-9a-f]{4}$`)

// IsLaunchID reports whether s has the shape of a launch id,
// L<yyyymmdd>T<hhmmss>-<4 hex>.
func IsLaunchID(s string) bool { return launchIDPattern.MatchString(s) }

// Event is one valid registry line as decoded JSON (numbers are float64,
// objects map[string]any, lists []any).
type Event map[string]any

// Kind is the event field: launch, end, analyzed or rule_break.
func (e Event) Kind() string { s, _ := e["event"].(string); return s }

// ID is the id field, "" when it is null or not a string.
func (e Event) ID() string { s, _ := e["id"].(string); return s }

// Get is the value at a dot path (a.b.0.c); a numeric segment indexes a list.
func (e Event) Get(path string) (any, bool) {
	var cur any = map[string]any(e)
	for _, seg := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(node) {
				return nil, false
			}
			cur = node[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// String is the value at path when it is a JSON string.
func (e Event) String(path string) (string, bool) {
	v, ok := e.Get(path)
	s, isString := v.(string)
	return s, ok && isString
}

// Number is the value at path when it is a JSON number.
func (e Event) Number(path string) (float64, bool) {
	v, ok := e.Get(path)
	f, isNumber := v.(float64)
	return f, ok && isNumber
}

// Strings is the value at path when it is a list of strings only.
func (e Event) Strings(path string) ([]string, bool) {
	v, ok := e.Get(path)
	list, isList := v.([]any)
	if !ok || !isList {
		return nil, false
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
