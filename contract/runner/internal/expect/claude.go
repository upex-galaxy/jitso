package expect

import (
	"encoding/json"
	"fmt"
	"os"

	"jitso-contract-runner/internal/jsonx"
)

// ClaudeDump is what the fake claude writes (SPEC 5.2).
type ClaudeDump struct {
	Argv []string          `json:"argv"`
	Cwd  string            `json:"cwd"`
	Env  map[string]string `json:"env"`
}

// claude (SPEC 7.5): checks on the fake claude dump left by this step.
func init() {
	register("claude", []string{KindRun, KindExec}, func(v any, what string) (Check, error) {
		return parseClaudeCheck(v, what)
	})
}

type claudeCheck struct {
	ran          *bool
	argv         []TextMatcher
	hasArgv      bool
	contains     []string
	notContains  []string
	argvCount    map[string]int
	lastArg      []TextMatcher
	cwd          *string
	env          map[string]ValueMatcher
	argAfter     map[string]TextMatcher
	jsonArgAfter map[string][]JSONCheck
}

func parseClaudeCheck(v any, what string) (Check, error) {
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return nil, err
	}
	if err := jsonx.Keys(obj, what, "ran", "argv", "argv_contains", "argv_not_contains", "argv_count", "last_arg", "cwd", "env", "arg_after", "json_arg_after"); err != nil {
		return nil, err
	}
	cc := claudeCheck{env: map[string]ValueMatcher{}, argAfter: map[string]TextMatcher{}, argvCount: map[string]int{}, jsonArgAfter: map[string][]JSONCheck{}}
	if raw, ok := obj["ran"]; ok {
		b, err := jsonx.Bool(raw, what+".ran")
		if err != nil {
			return nil, err
		}
		cc.ran = &b
	}
	if raw, ok := obj["argv"]; ok {
		if cc.argv, err = parseArgvPattern(raw, what+".argv"); err != nil {
			return nil, err
		}
		cc.hasArgv = true
	}
	if raw, ok := obj["argv_count"]; ok {
		counts, err := jsonx.Object(raw, what+".argv_count")
		if err != nil {
			return nil, err
		}
		for _, flag := range jsonx.SortedKeys(counts) {
			n, err := jsonx.Int(counts[flag], what+".argv_count."+flag)
			if err != nil {
				return nil, err
			}
			if n < 0 {
				return nil, fmt.Errorf("%s.argv_count.%s: want 0 or more", what, flag)
			}
			cc.argvCount[flag] = n
		}
	}
	if raw, ok := obj["argv_contains"]; ok {
		if cc.contains, err = jsonx.Strings(raw, what+".argv_contains"); err != nil {
			return nil, err
		}
	}
	if raw, ok := obj["argv_not_contains"]; ok {
		if cc.notContains, err = jsonx.Strings(raw, what+".argv_not_contains"); err != nil {
			return nil, err
		}
	}
	if raw, ok := obj["last_arg"]; ok {
		if cc.lastArg, err = ParseTextMatchers(raw, what+".last_arg"); err != nil {
			return nil, err
		}
	}
	if raw, ok := obj["cwd"]; ok {
		s, err := jsonx.String(raw, what+".cwd")
		if err != nil {
			return nil, err
		}
		cc.cwd = &s
	}
	if raw, ok := obj["env"]; ok {
		env, err := jsonx.Object(raw, what+".env")
		if err != nil {
			return nil, err
		}
		for _, name := range jsonx.SortedKeys(env) {
			m, err := ParseFieldValue(env[name], what+".env."+name)
			if err != nil {
				return nil, err
			}
			if m.mode == "equals" {
				if _, ok := m.want.(string); !ok {
					return nil, fmt.Errorf("%s.env.%s: want a string, {\"regex\": ...} or {\"absent\": true}", what, name)
				}
			}
			cc.env[name] = m
		}
	}
	if raw, ok := obj["arg_after"]; ok {
		m, err := jsonx.Object(raw, what+".arg_after")
		if err != nil {
			return nil, err
		}
		for _, flag := range jsonx.SortedKeys(m) {
			if cc.argAfter[flag], err = parseArg(m[flag], what+".arg_after."+flag); err != nil {
				return nil, err
			}
		}
	}
	if raw, ok := obj["json_arg_after"]; ok {
		m, err := jsonx.Object(raw, what+".json_arg_after")
		if err != nil {
			return nil, err
		}
		for _, flag := range jsonx.SortedKeys(m) {
			if cc.jsonArgAfter[flag], err = ParseJSONChecks(m[flag], what+".json_arg_after."+flag); err != nil {
				return nil, err
			}
		}
	}
	if cc.ran != nil && !*cc.ran && len(obj) > 1 {
		return nil, fmt.Errorf("%s: ran:false takes no other check", what)
	}
	return cc, nil
}

func (cc claudeCheck) Eval(obs *Observation) []Failure {
	data, err := os.ReadFile(obs.ClaudeDump)
	if os.IsNotExist(err) {
		if cc.ran != nil && !*cc.ran {
			return nil
		}
		return fail("claude ran", "the fake claude ran", "it did not run (no dump)")
	}
	if err != nil {
		return fail("claude dump", "readable", err.Error())
	}
	if cc.ran != nil && !*cc.ran {
		return fail("claude ran", "the fake claude did not run", "it ran")
	}
	var dump ClaudeDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return fail("claude dump", "valid JSON", err.Error())
	}
	return cc.evalDump(dump)
}

func (cc claudeCheck) evalDump(d ClaudeDump) []Failure {
	var out []Failure
	argvText := jsonx.Compact(d.Argv)
	if cc.hasArgv {
		if miss := matchArgv(cc.argv, d.Argv); miss != "" {
			out = append(out, Failure{What: "claude argv", Expected: miss, Actual: argvText})
		}
	}
	for _, flag := range jsonx.SortedKeys(cc.argvCount) {
		n := 0
		for _, a := range d.Argv {
			if a == flag {
				n++
			}
		}
		if n != cc.argvCount[flag] {
			out = append(out, Failure{What: "claude argv_count " + flag, Expected: fmt.Sprintf("%d occurrences", cc.argvCount[flag]), Actual: fmt.Sprintf("%d in %s", n, argvText)})
		}
	}
	for _, s := range cc.contains {
		if indexOf(d.Argv, s) < 0 {
			out = append(out, Failure{What: "claude argv_contains", Expected: fmt.Sprintf("an element equal to %q", s), Actual: argvText})
		}
	}
	for _, s := range cc.notContains {
		if indexOf(d.Argv, s) >= 0 {
			out = append(out, Failure{What: "claude argv_not_contains", Expected: fmt.Sprintf("no element equal to %q", s), Actual: argvText})
		}
	}
	if len(cc.lastArg) > 0 {
		if len(d.Argv) == 0 {
			out = append(out, Failure{What: "claude last_arg", Expected: "an argument", Actual: "argv is empty"})
		} else {
			out = append(out, evalText("claude last_arg", cc.lastArg, d.Argv[len(d.Argv)-1])...)
		}
	}
	if cc.cwd != nil && d.Cwd != *cc.cwd {
		out = append(out, Failure{What: "claude cwd", Expected: *cc.cwd, Actual: d.Cwd})
	}
	for _, name := range jsonx.SortedKeys(cc.env) {
		val, present := d.Env[name]
		var v any = val
		if ok, expected, actual := cc.env[name].Test(v, present); !ok {
			out = append(out, Failure{What: "claude env " + name, Expected: expected, Actual: actual})
		}
	}
	for _, flag := range jsonx.SortedKeys(cc.argAfter) {
		m := cc.argAfter[flag]
		got, ok := argAfter(d.Argv, flag)
		if !ok {
			out = append(out, Failure{What: "claude arg_after " + flag, Expected: describe(m), Actual: "flag not followed by an argument: " + argvText})
		} else if miss := m.Match(got); miss != nil {
			out = append(out, Failure{What: "claude arg_after " + flag, Expected: miss.Expected, Actual: got})
		}
	}
	for _, flag := range jsonx.SortedKeys(cc.jsonArgAfter) {
		got, ok := argAfter(d.Argv, flag)
		if !ok {
			out = append(out, Failure{What: "claude json_arg_after " + flag, Expected: "a JSON argument after the flag", Actual: argvText})
			continue
		}
		doc, err := jsonx.Decode([]byte(got))
		if err != nil {
			out = append(out, Failure{What: "claude json_arg_after " + flag, Expected: "valid JSON", Actual: got})
			continue
		}
		out = append(out, evalJSON("claude json_arg_after "+flag, cc.jsonArgAfter[flag], doc)...)
	}
	return out
}

func indexOf(list []string, s string) int {
	for i, item := range list {
		if item == s {
			return i
		}
	}
	return -1
}

// argAfter returns the element after the first occurrence of flag.
func argAfter(argv []string, flag string) (string, bool) {
	i := indexOf(argv, flag)
	if i < 0 || i+1 >= len(argv) {
		return "", false
	}
	return argv[i+1], true
}

// describe is the expectation text of a matcher, as its failure would print it.
func describe(m TextMatcher) string {
	if miss := m.Match("\x00<no argument>\x00"); miss != nil {
		return miss.Expected
	}
	return m.Name()
}
