package expect

import (
	"fmt"
	"strconv"
	"strings"

	"jitso-contract-runner/internal/jsonx"
)

// orca_calls (SPEC 7.4): the call log the fake orca appended to.
func init() {
	register("orca_calls", allKinds, func(v any, what string) (Check, error) {
		obj, err := jsonx.Object(v, what)
		if err != nil {
			return nil, err
		}
		if err := jsonx.Keys(obj, what, "verbs", "calls", "none", "count", "empty"); err != nil {
			return nil, err
		}
		oc := orcaCheck{count: -1}
		if raw, ok := obj["verbs"]; ok {
			list, err := jsonx.Strings(raw, what+".verbs")
			if err != nil {
				return nil, err
			}
			oc.verbs = &list
		}
		if raw, ok := obj["none"]; ok {
			if oc.none, err = jsonx.Strings(raw, what+".none"); err != nil {
				return nil, err
			}
		}
		if raw, ok := obj["count"]; ok {
			if oc.count, err = jsonx.Int(raw, what+".count"); err != nil {
				return nil, err
			}
		}
		if raw, ok := obj["empty"]; ok {
			if oc.empty, err = jsonx.Bool(raw, what+".empty"); err != nil {
				return nil, err
			}
			if !oc.empty {
				return nil, fmt.Errorf("%s.empty: only true is meaningful", what)
			}
		}
		if raw, ok := obj["calls"]; ok {
			list, err := jsonx.List(raw, what+".calls")
			if err != nil {
				return nil, err
			}
			for i, item := range list {
				c, err := parseCallCheck(item, fmt.Sprintf("%s.calls[%d]", what, i))
				if err != nil {
					return nil, err
				}
				oc.calls = append(oc.calls, c)
			}
		}
		return oc, nil
	})
}

type callCheck struct {
	at   int
	argv []TextMatcher
}

func parseCallCheck(v any, what string) (callCheck, error) {
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return callCheck{}, err
	}
	if err := jsonx.Keys(obj, what, "at", "argv"); err != nil {
		return callCheck{}, err
	}
	c := callCheck{}
	if c.at, err = jsonx.Int(obj["at"], what+".at"); err != nil {
		return callCheck{}, err
	}
	c.argv, err = parseArgvPattern(obj["argv"], what+".argv")
	return c, err
}

// parseArgvPattern reads a list whose elements are each an argument pattern (parseArg).
func parseArgvPattern(v any, what string) ([]TextMatcher, error) {
	list, err := jsonx.List(v, what)
	if err != nil {
		return nil, err
	}
	out := make([]TextMatcher, len(list))
	for i, item := range list {
		if out[i], err = parseArg(item, fmt.Sprintf("%s[%d]", what, i)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// parseArg reads one argument pattern: a literal string is an exact match (never the
// `contains` shorthand of matcher lists), an object is any text matcher of SPEC 7.1.
func parseArg(v any, what string) (TextMatcher, error) {
	if s, ok := v.(string); ok {
		return textMatchers["equals"](s, what)
	}
	if _, err := jsonx.Object(v, what); err != nil {
		return nil, fmt.Errorf("%s: want a literal string or a text matcher object", what)
	}
	return ParseTextMatcher(v, what)
}

type orcaCheck struct {
	verbs *[]string
	calls []callCheck
	none  []string
	count int
	empty bool
}

// OrcaCalls reads the argv lists from the fake orca call log. A missing log is no calls.
func OrcaCalls(path string) ([][]string, error) {
	lines, err := ReadJSONLines(path)
	if err != nil {
		return nil, err
	}
	out := make([][]string, len(lines))
	for i, line := range lines {
		raw, _ := jsonx.Lookup(line, "argv")
		argv, err := jsonx.Strings(raw, fmt.Sprintf("orca call %d argv", i))
		if err != nil {
			return nil, err
		}
		out[i] = argv
	}
	return out, nil
}

// Verb is the first two argv words joined by a space.
func Verb(argv []string) string {
	if len(argv) > 2 {
		argv = argv[:2]
	}
	return strings.Join(argv, " ")
}

func (oc orcaCheck) Eval(obs *Observation) []Failure {
	calls, err := OrcaCalls(obs.OrcaLog)
	if err != nil {
		return fail("orca_calls", "a readable call log", err.Error())
	}
	verbs := make([]string, len(calls))
	for i, c := range calls {
		verbs[i] = Verb(c)
	}
	var out []Failure
	if oc.empty && len(calls) > 0 {
		out = append(out, Failure{What: "orca_calls empty", Expected: "no call", Actual: jsonx.Compact(verbs)})
	}
	if oc.count >= 0 && len(calls) != oc.count {
		out = append(out, Failure{What: "orca_calls count", Expected: strconv.Itoa(oc.count), Actual: fmt.Sprintf("%d %s", len(calls), jsonx.Compact(verbs))})
	}
	if oc.verbs != nil && !sameStrings(*oc.verbs, verbs) {
		out = append(out, Failure{What: "orca_calls verbs", Expected: jsonx.Compact(*oc.verbs), Actual: jsonx.Compact(verbs)})
	}
	for _, banned := range oc.none {
		for _, v := range verbs {
			if v == banned {
				out = append(out, Failure{What: "orca_calls none", Expected: "no " + strconv.Quote(banned) + " call", Actual: jsonx.Compact(verbs)})
				break
			}
		}
	}
	for _, c := range oc.calls {
		label := fmt.Sprintf("orca call %d argv", c.at)
		if c.at < 0 || c.at >= len(calls) {
			out = append(out, Failure{What: label, Expected: "a call", Actual: fmt.Sprintf("%d calls", len(calls))})
			continue
		}
		if miss := matchArgv(c.argv, calls[c.at]); miss != "" {
			out = append(out, Failure{What: label, Expected: miss, Actual: jsonx.Compact(calls[c.at])})
		}
	}
	return out
}

// matchArgv compares a whole argv to a pattern list; it returns "" on a match.
func matchArgv(pattern []TextMatcher, argv []string) string {
	if len(pattern) != len(argv) {
		return fmt.Sprintf("%d elements", len(pattern))
	}
	for i, m := range pattern {
		if miss := m.Match(argv[i]); miss != nil {
			return fmt.Sprintf("element %d: %s", i, miss.Expected)
		}
	}
	return ""
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
