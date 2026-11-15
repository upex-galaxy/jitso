package expect

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"jitso-contract-runner/internal/jsonx"
)

// RegistryPath is the launch registry, relative to the project dir.
const RegistryPath = ".jitso/launches.jsonl"

// registry (SPEC 7.3): line count and per-line field checks on .jitso/launches.jsonl.
func init() {
	register("registry", allKinds, func(v any, what string) (Check, error) {
		obj, err := jsonx.Object(v, what)
		if err != nil {
			return nil, err
		}
		if err := jsonx.Keys(obj, what, "count", "events"); err != nil {
			return nil, err
		}
		rc := registryCheck{count: -1}
		if raw, ok := obj["count"]; ok {
			if rc.count, err = jsonx.Int(raw, what+".count"); err != nil {
				return nil, err
			}
		}
		if raw, ok := obj["events"]; ok {
			list, err := jsonx.List(raw, what+".events")
			if err != nil {
				return nil, err
			}
			for i, item := range list {
				ev, err := parseEventCheck(item, fmt.Sprintf("%s.events[%d]", what, i))
				if err != nil {
					return nil, err
				}
				rc.events = append(rc.events, ev)
			}
		}
		return rc, nil
	})
}

type eventCheck struct {
	at     int
	event  *string
	fields map[string]ValueMatcher
}

func parseEventCheck(v any, what string) (eventCheck, error) {
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return eventCheck{}, err
	}
	if err := jsonx.Keys(obj, what, "at", "event", "fields"); err != nil {
		return eventCheck{}, err
	}
	ev := eventCheck{fields: map[string]ValueMatcher{}}
	if ev.at, err = jsonx.Int(obj["at"], what+".at"); err != nil {
		return eventCheck{}, err
	}
	if ev.at < 0 {
		return eventCheck{}, fmt.Errorf("%s.at: want a zero-based line number", what)
	}
	if raw, ok := obj["event"]; ok {
		s, err := jsonx.String(raw, what+".event")
		if err != nil {
			return eventCheck{}, err
		}
		ev.event = &s
	}
	if raw, ok := obj["fields"]; ok {
		fields, err := jsonx.Object(raw, what+".fields")
		if err != nil {
			return eventCheck{}, err
		}
		for _, path := range jsonx.SortedKeys(fields) {
			m, err := ParseFieldValue(fields[path], what+".fields."+path)
			if err != nil {
				return eventCheck{}, err
			}
			ev.fields[path] = m
		}
	}
	return ev, nil
}

type registryCheck struct {
	count  int
	events []eventCheck
}

func (rc registryCheck) Eval(obs *Observation) []Failure {
	lines, err := ReadLines(filepath.Join(obs.Project, RegistryPath))
	if err != nil {
		return fail("registry", "a readable file", err.Error())
	}
	var out []Failure
	if rc.count >= 0 && len(lines) != rc.count {
		out = append(out, Failure{What: "registry count", Expected: strconv.Itoa(rc.count), Actual: strconv.Itoa(len(lines))})
	}
	for _, ev := range rc.events {
		label := fmt.Sprintf("registry line %d", ev.at)
		if ev.at >= len(lines) {
			out = append(out, Failure{What: label, Expected: "a line", Actual: fmt.Sprintf("the registry has %d lines", len(lines))})
			continue
		}
		line, err := jsonx.Decode([]byte(lines[ev.at]))
		if err != nil {
			out = append(out, Failure{What: label, Expected: "a JSON line", Actual: fmt.Sprintf("%v: %s", err, lines[ev.at])})
			continue
		}
		if ev.event != nil {
			got, _ := jsonx.Lookup(line, "event")
			if !jsonx.Equal(got, *ev.event) {
				out = append(out, Failure{What: label + " event", Expected: *ev.event, Actual: jsonx.Compact(got)})
			}
		}
		for _, path := range jsonx.SortedKeys(ev.fields) {
			v, present := jsonx.Lookup(line, path)
			if ok, expected, actual := ev.fields[path].Test(v, present); !ok {
				out = append(out, Failure{What: label + " " + path, Expected: expected, Actual: actual})
			}
		}
	}
	return out
}

// ReadLines returns the non-blank lines of a file, unparsed. A missing file is zero lines.
func ReadLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

// ReadJSONLines reads one JSON value per non-blank line. A missing file is zero lines.
func ReadJSONLines(path string) ([]any, error) {
	lines, err := ReadLines(path)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(lines))
	for i, line := range lines {
		if out[i], err = jsonx.Decode([]byte(line)); err != nil {
			return nil, fmt.Errorf("%s line %d: %v", filepath.Base(path), i+1, err)
		}
	}
	return out, nil
}
