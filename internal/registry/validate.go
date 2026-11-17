package registry

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// safeID is the oracle's id pattern: 1 to 64 letters, digits, dot, dash or
// underscore, starting with a letter or digit. The classes are spelled out
// (no (?i)) so that Unicode case folding cannot widen them.
var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// Validate checks one decoded line (the oracle's validateEvent). It returns
// one short message per problem, each naming the field; empty means valid.
func Validate(v any) []string {
	e, ok := v.(map[string]any)
	if !ok {
		return []string{"event must be a JSON object"}
	}
	var problems []string
	add := func(msg string) { problems = append(problems, msg) }
	if n, ok := e["v"].(float64); !ok || n != Version {
		add("v must be " + strconv.Itoa(Version))
	}
	if ts, ok := e["ts"].(string); !ok || ts == "" || !isTimestamp(ts) {
		add("ts must be an ISO timestamp")
	}
	switch e["event"] {
	case KindLaunch:
		if !isID(e["id"]) {
			add("id must be 1 to 64 safe characters (letters, digits, dot, dash, underscore)")
		}
		if !isID(e["root"]) {
			add("root must be a safe id")
		}
		if parent, present := e["parent"]; !present || (parent != nil && !isID(parent)) {
			add("parent must be null or a safe id")
		}
		if d, ok := e["depth"].(float64); !ok || d < 0 || d != math.Trunc(d) || math.IsInf(d, 0) {
			add("depth must be an integer >= 0")
		}
		if !isOneOf(Modes, e["mode"]) {
			add("mode must be one of " + strings.Join(Modes, "|"))
		}
		if s, ok := e["build"].(string); !ok || s == "" {
			add("build must be a non-empty string")
		}
	case KindEnd:
		if !isID(e["id"]) {
			add("id must be a safe id")
		}
		if !isOneOf(EndSources, e["source"]) {
			add("source must be one of " + strings.Join(EndSources, "|"))
		}
		if !isOneOf(Outcomes, e["outcome_reported"]) {
			add("outcome_reported must be one of " + strings.Join(Outcomes, "|"))
		}
	default:
		add("event must be one of launch|end")
	}
	return problems
}

func isID(v any) bool {
	s, ok := v.(string)
	return ok && safeID.MatchString(s)
}

func isOneOf(list []string, v any) bool {
	s, ok := v.(string)
	return ok && slices.Contains(list, s)
}

// timestampLayouts are the ISO 8601 forms accepted where the oracle accepts
// anything Date.parse understands. Date.parse also takes non-ISO text such as
// "Nov 15 2026"; the port does not.
var timestampLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04Z07:00",
	"2006-01-02T15:04",
	"2006-01-02",
	"2006-01",
	"2006",
}

func isTimestamp(s string) bool {
	_, ok := ParseTime(s)
	return ok
}

// ParseTime parses a registry timestamp (the ts field). A form without a zone
// is read as UTC.
func ParseTime(s string) (time.Time, bool) {
	for _, layout := range timestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
