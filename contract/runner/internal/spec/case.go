package spec

import (
	"fmt"
	"strconv"
	"strings"

	"jitso-contract-runner/internal/expect"
	"jitso-contract-runner/internal/fakes"
	"jitso-contract-runner/internal/jsonx"
)

// Status values per implementation (SPEC 2).
const (
	StatusPass  = "pass"
	StatusXfail = "xfail"
)

// Case is one loaded and validated case.json. The sections that may hold placeholders are
// kept as decoded JSON trees and resolved at run time, section by section.
type Case struct {
	ID     string
	Title  string
	Source []string
	Tags   []string
	Status map[string]string
	Defect string
	Intent string
	// Dir is the folder holding case.json; `from` entries are read from here.
	Dir string
	// Base is the fixture folder copied as the project.
	Base    string
	ForEach *ForEach

	projectFiles any
	homeFiles    any
	orca         any
	claude       any
	steps        []map[string]any
	// worktrees are the names of the case's git_worktree steps, in order.
	worktrees []string
}

// ForEach expands a case once per row (SPEC 2). The single-var form is one var per row.
type ForEach struct {
	Vars []string
	Rows [][]string
	// IDs, when set, name the expansions instead of their index.
	IDs []string
}

// Worktrees lists the names of the case's git_worktree steps.
func (c *Case) Worktrees() []string { return c.worktrees }

// WorktreeVar is the placeholder holding the real path of a linked worktree.
func WorktreeVar(name string) string { return WorktreePrefix + name }

// StatusFor is the declared status for an implementation's status key (default pass).
func (c *Case) StatusFor(key string) string {
	if s, ok := c.Status[key]; ok {
		return s
	}
	return StatusPass
}

// HasTag reports whether the case carries tag.
func (c *Case) HasTag(tag string) bool {
	for _, t := range c.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Expansion is one concrete run of a case: the case itself, or one for_each value.
type Expansion struct {
	Case  *Case
	Index int
	ID    string
	Vars  Vars
}

// Expand returns the runs of the case, in order.
func (c *Case) Expand() []Expansion {
	if c.ForEach == nil {
		return []Expansion{{Case: c, ID: c.ID, Vars: Vars{}}}
	}
	out := make([]Expansion, len(c.ForEach.Rows))
	for i, row := range c.ForEach.Rows {
		vars := Vars{VarIndex: strconv.Itoa(i)}
		for j, name := range c.ForEach.Vars {
			vars[name] = row[j]
		}
		name := strconv.Itoa(i)
		if c.ForEach.IDs != nil {
			name = c.ForEach.IDs[i]
		}
		out[i] = Expansion{Case: c, Index: i, ID: fmt.Sprintf("%s[%s]", c.ID, name), Vars: vars}
	}
	return out
}

// Bind returns the variables of a run: the built-ins plus the for_each values, in which
// the built-ins (and only they) are replaced. Everything else in a value is verbatim.
func (x Expansion) Bind(builtins Vars) Vars {
	values := Vars{}
	for name, v := range x.Vars {
		if name != VarIndex {
			for _, b := range jsonx.SortedKeys(builtins) {
				v = strings.ReplaceAll(v, "{{"+b+"}}", builtins[b])
			}
		}
		values[name] = v
	}
	return builtins.With(values)
}

// Setup is the resolved sandbox part of an expansion.
type Setup struct {
	ProjectFiles FileMap
	HomeFiles    FileMap
	// Scenario holds the fake orca scenario and the fake claude knobs.
	Scenario fakes.Scenario
}

// Setup resolves project.files, home.files and the orca scenario. vars holds the built-ins
// and the for_each values; captures do not exist yet.
func (x Expansion) Setup(vars Vars) (Setup, error) {
	var s Setup
	var err error
	resolve := func(v any, what string) (any, error) {
		r, err := ResolveTree(v, vars)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		return r, nil
	}
	tree, err := resolve(x.Case.projectFiles, "project.files")
	if err != nil {
		return s, err
	}
	if s.ProjectFiles, err = ParseFileMap(tree, "project.files", x.Case.Dir, vars); err != nil {
		return s, err
	}
	if tree, err = resolve(x.Case.homeFiles, "home.files"); err != nil {
		return s, err
	}
	if s.HomeFiles, err = ParseFileMap(tree, "home.files", x.Case.Dir, vars); err != nil {
		return s, err
	}
	if tree, err = resolve(x.Case.orca, "orca"); err != nil {
		return s, err
	}
	if s.Scenario, err = fakes.ParseScenario(tree, vars[VarProject]); err != nil {
		return s, err
	}
	s.Scenario.Claude, err = fakes.ParseClaudeScenario(x.Case.claude)
	return s, err
}

// StepCount is the number of steps.
func (x Expansion) StepCount() int { return len(x.Case.steps) }

// Action resolves and parses the action part of step i (everything but `expect`).
func (x Expansion) Action(i int, vars Vars) (*Action, error) {
	return parseAction(x.Case.steps[i], vars, x.Case.Dir)
}

// Expect resolves and builds the checks of step i. Call it after the step's captures ran.
func (x Expansion) Expect(i int, kind string, vars Vars) (expect.Set, error) {
	tree, err := ResolveTree(x.Case.steps[i]["expect"], vars)
	if err != nil {
		return nil, fmt.Errorf("expect: %w", err)
	}
	return expect.Build(tree, kind)
}

// captureNames lists every capture name of every step.
func (c *Case) captureNames() (map[string]bool, error) {
	names := map[string]bool{}
	for i, step := range c.steps {
		raw, ok := step["capture"]
		if !ok {
			continue
		}
		obj, err := jsonx.Object(raw, fmt.Sprintf("steps[%d].capture", i))
		if err != nil {
			return nil, err
		}
		for name := range obj {
			if !namePattern.MatchString(name) {
				return nil, fmt.Errorf("steps[%d].capture: %q is not a placeholder name", i, name)
			}
			if isReserved(name) || (c.ForEach != nil && contains(c.ForEach.Vars, name)) {
				return nil, fmt.Errorf("steps[%d].capture: %q shadows a built-in or for_each placeholder", i, name)
			}
			names[name] = true
		}
	}
	return names, nil
}
