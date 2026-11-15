// Package expect turns the `expect` object of a step into checks and evaluates them against
// what the step left behind (SPEC section 7). Each expectation key and each text matcher is
// a small builder registered by name, so a new one is one file with an init function.
package expect

import (
	"fmt"
	"time"

	"jitso-contract-runner/internal/jsonx"
)

// Step kinds, as SPEC section 6 names them.
const (
	KindRun   = "run"
	KindHook  = "hook"
	KindExec  = "exec"
	KindWrite = "write"
	// KindGitWorktree adds a linked git worktree to the sandbox (SPEC 6.5).
	KindGitWorktree = "git_worktree"
)

// Observation is everything a check may look at after one step.
type Observation struct {
	Kind       string
	Exit       int
	TimedOut   bool
	Stdout     string
	Stderr     string
	Elapsed    time.Duration
	Project    string
	OrcaLog    string
	ClaudeDump string
	Decision   string
	Reason     string
}

// Failure is one expectation that did not hold.
type Failure struct {
	What     string
	Expected string
	Actual   string
}

// Check is one built expectation.
type Check interface {
	Eval(obs *Observation) []Failure
}

// Set is the checks of one step, in a stable order.
type Set []Check

// Eval runs every check and collects every failure.
func (s Set) Eval(obs *Observation) []Failure {
	var out []Failure
	for _, c := range s {
		out = append(out, c.Eval(obs)...)
	}
	return out
}

type builder struct {
	on    map[string]bool
	build func(v any, what string) (Check, error)
}

var builders = map[string]builder{}

// any step kind
var allKinds = []string{KindRun, KindHook, KindExec, KindWrite, KindGitWorktree}

// register adds an expectation key valid on the given step kinds.
func register(name string, on []string, build func(v any, what string) (Check, error)) {
	set := map[string]bool{}
	for _, k := range on {
		set[k] = true
	}
	builders[name] = builder{on: set, build: build}
}

// Build turns an `expect` object into checks for a step of the given kind.
func Build(v any, kind string) (Set, error) {
	if v == nil {
		return nil, nil
	}
	obj, err := jsonx.Object(v, "expect")
	if err != nil {
		return nil, err
	}
	var set Set
	for _, key := range jsonx.SortedKeys(obj) {
		b, ok := builders[key]
		if !ok {
			return nil, fmt.Errorf("expect: unknown key %q (allowed: %v)", key, jsonx.SortedKeys(builders))
		}
		if !b.on[kind] {
			return nil, fmt.Errorf("expect.%s is not available on a %q step", key, kind)
		}
		c, err := b.build(obj[key], "expect."+key)
		if err != nil {
			return nil, err
		}
		set = append(set, c)
	}
	return set, nil
}

// checkFunc adapts a function to Check.
type checkFunc func(obs *Observation) []Failure

func (f checkFunc) Eval(obs *Observation) []Failure { return f(obs) }

func fail(what, expected, actual string) []Failure {
	return []Failure{{What: what, Expected: expected, Actual: actual}}
}
