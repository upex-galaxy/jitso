package expect

import (
	"fmt"
	"strconv"

	"jitso-contract-runner/internal/jsonx"
)

// exit, elapsed_ms_lt, stdout, stderr, decision, reason: the keys that read the process itself.
func init() {
	register("exit", []string{KindRun, KindHook, KindExec}, func(v any, what string) (Check, error) {
		want, err := jsonx.Int(v, what)
		if err != nil {
			return nil, err
		}
		return checkFunc(func(obs *Observation) []Failure {
			if obs.TimedOut {
				return fail("exit", strconv.Itoa(want), "the process timed out and was killed")
			}
			if obs.Exit != want {
				return fail("exit", strconv.Itoa(want), strconv.Itoa(obs.Exit))
			}
			return nil
		}), nil
	})

	register("exit_not", []string{KindRun, KindHook, KindExec}, func(v any, what string) (Check, error) {
		banned, err := jsonx.Int(v, what)
		if err != nil {
			return nil, err
		}
		return checkFunc(func(obs *Observation) []Failure {
			if obs.TimedOut {
				return fail("exit_not", "not "+strconv.Itoa(banned), "the process timed out and was killed")
			}
			if obs.Exit == banned {
				return fail("exit_not", "not "+strconv.Itoa(banned), strconv.Itoa(obs.Exit))
			}
			return nil
		}), nil
	})

	register("exit_in", []string{KindRun, KindHook, KindExec}, func(v any, what string) (Check, error) {
		list, err := jsonx.List(v, what)
		if err != nil {
			return nil, err
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("%s: give at least one exit code", what)
		}
		codes := make([]int, len(list))
		for i, item := range list {
			if codes[i], err = jsonx.Int(item, fmt.Sprintf("%s[%d]", what, i)); err != nil {
				return nil, err
			}
		}
		return checkFunc(func(obs *Observation) []Failure {
			want := fmt.Sprintf("one of %v", codes)
			if obs.TimedOut {
				return fail("exit_in", want, "the process timed out and was killed")
			}
			for _, c := range codes {
				if obs.Exit == c {
					return nil
				}
			}
			return fail("exit_in", want, strconv.Itoa(obs.Exit))
		}), nil
	})

	register("elapsed_ms_lt", allKinds, func(v any, what string) (Check, error) {
		limit, err := jsonx.Int(v, what)
		if err != nil {
			return nil, err
		}
		if limit <= 0 {
			return nil, fmt.Errorf("%s: want a positive number of milliseconds", what)
		}
		return checkFunc(func(obs *Observation) []Failure {
			if ms := obs.Elapsed.Milliseconds(); ms >= int64(limit) {
				return fail("elapsed_ms_lt", fmt.Sprintf("< %d ms", limit), fmt.Sprintf("%d ms", ms))
			}
			return nil
		}), nil
	})

	register("stdout", []string{KindRun, KindHook, KindExec}, streamBuilder("stdout", func(o *Observation) string { return o.Stdout }))
	register("stderr", []string{KindRun, KindHook, KindExec}, streamBuilder("stderr", func(o *Observation) string { return o.Stderr }))
	register("reason", []string{KindHook}, streamBuilder("reason", func(o *Observation) string { return o.Reason }))

	register("decision", []string{KindHook}, func(v any, what string) (Check, error) {
		want, err := jsonx.String(v, what)
		if err != nil {
			return nil, err
		}
		switch want {
		case "allow", "deny", "ask", "none":
		default:
			return nil, fmt.Errorf("%s: want allow, deny, ask or none, got %q", what, want)
		}
		return checkFunc(func(obs *Observation) []Failure {
			if obs.Decision != want {
				return fail("decision", want, obs.Decision)
			}
			return nil
		}), nil
	})
}

func streamBuilder(name string, text func(*Observation) string) func(v any, what string) (Check, error) {
	return func(v any, what string) (Check, error) {
		matchers, err := ParseTextMatchers(v, what)
		if err != nil {
			return nil, err
		}
		return checkFunc(func(obs *Observation) []Failure {
			return evalText(name, matchers, text(obs))
		}), nil
	}
}

func evalText(name string, matchers []TextMatcher, text string) []Failure {
	var out []Failure
	for i, m := range matchers {
		if miss := m.Match(text); miss != nil {
			actual := text
			if miss.Actual != "" {
				actual = miss.Actual
			}
			out = append(out, Failure{What: fmt.Sprintf("%s[%d] %s", name, i, m.Name()), Expected: miss.Expected, Actual: actual})
		}
	}
	return out
}
