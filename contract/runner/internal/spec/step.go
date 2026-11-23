package spec

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"jitso-contract-runner/internal/expect"
	"jitso-contract-runner/internal/jsonx"
)

// Hook names (SPEC 6.2).
var hookNames = map[string]bool{"guard": true}

// Keys each step kind accepts besides its own (SPEC 6).
var stepKeys = map[string][]string{
	expect.KindRun:   {"run", "stdin", "env", "cwd", "capture", "expect"},
	expect.KindHook:  {"hook", "input", "env", "capture", "expect"},
	expect.KindExec:  {"exec", "launch", "env", "capture", "expect"},
	expect.KindWrite: {"write", "expect"},
	// git_worktree: SPEC 6.5
	expect.KindGitWorktree: {"git_worktree", "expect"},
	// git_init: SPEC 6.6
	expect.KindGitInit: {"git_init", "expect"},
}

// Action is the resolved, executable part of a step.
type Action struct {
	Kind string
	// run
	Argv  []string
	Stdin *string
	Cwd   string
	// hook
	Hook  string
	Input string
	// exec
	Launch string
	// write
	ProjectFiles FileMap
	HomeFiles    FileMap
	// git_worktree
	Worktree string
	// git_init: a path relative to the project dir, or absolute
	InitPath string

	Env      map[string]string
	Captures []Capture
}

// Capture takes a value from the step output, a file or the orca call log into a
// placeholder (SPEC 6.1).
type Capture struct {
	Name  string
	From  string // stdout, stderr, file, orca_log
	Path  string // with From == "file": relative to the project dir, or absolute
	Re    *regexp.Regexp
	Group int
}

// Capture sources.
var captureSources = []string{"stdout", "stderr", "file", "orca_log"}

// StepKind names the kind of a raw step, or fails when it is not exactly one.
func StepKind(step map[string]any) (string, error) {
	var kinds []string
	for _, k := range []string{expect.KindRun, expect.KindHook, expect.KindExec, expect.KindWrite, expect.KindGitWorktree, expect.KindGitInit} {
		if _, ok := step[k]; ok {
			kinds = append(kinds, k)
		}
	}
	if len(kinds) != 1 {
		return "", fmt.Errorf("a step has exactly one of run, hook, exec, write, git_worktree, git_init (got %v)", kinds)
	}
	return kinds[0], nil
}

func parseAction(step map[string]any, vars Vars, caseDir string) (*Action, error) {
	kind, err := StepKind(step)
	if err != nil {
		return nil, err
	}
	if err := jsonx.Keys(step, kind+" step", stepKeys[kind]...); err != nil {
		return nil, err
	}
	raw := make(map[string]any, len(step))
	for k, v := range step {
		if k != "expect" {
			raw[k] = v
		}
	}
	tree, err := ResolveTree(raw, vars)
	if err != nil {
		return nil, err
	}
	s := tree.(map[string]any)
	a := &Action{Kind: kind}
	if v, ok := s["env"]; ok {
		if a.Env, err = jsonx.StringMap(v, "env"); err != nil {
			return nil, err
		}
	}
	if v, ok := s["capture"]; ok {
		if a.Captures, err = parseCaptures(v); err != nil {
			return nil, err
		}
	}
	switch kind {
	case expect.KindRun:
		err = parseRun(s, a)
	case expect.KindHook:
		err = parseHook(s, a)
	case expect.KindExec:
		err = parseExec(s, a)
	case expect.KindWrite:
		err = parseWrite(s, a, caseDir, vars)
	case expect.KindGitWorktree:
		a.Worktree, err = worktreeName(s["git_worktree"], "git_worktree")
	case expect.KindGitInit:
		a.InitPath, err = parseGitInit(s["git_init"])
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// parseGitInit reads a git_init step: {"path": "<relative to the project dir, or absolute>"}.
// A relative path must stay inside the project dir by its text; an absolute one (a placeholder
// path) is checked against the real sandbox when the step runs.
func parseGitInit(v any) (string, error) {
	obj, err := jsonx.Object(v, "git_init")
	if err != nil {
		return "", err
	}
	if err := jsonx.Keys(obj, "git_init", "path"); err != nil {
		return "", err
	}
	path, err := jsonx.String(obj["path"], "git_init.path")
	if err != nil {
		return "", fmt.Errorf("a git_init step needs path: %w", err)
	}
	if path == "" {
		return "", fmt.Errorf("git_init.path: must not be empty")
	}
	rooted := strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) // a stand-in path at load time has no drive letter
	if !filepath.IsAbs(path) && !rooted && !filepath.IsLocal(path) {
		return "", fmt.Errorf("git_init.path: %q leaves the project dir", path)
	}
	return path, nil
}

func parseRun(s map[string]any, a *Action) error {
	var err error
	if a.Argv, err = jsonx.Strings(s["run"], "run"); err != nil {
		return err
	}
	if v, ok := s["stdin"]; ok {
		text, err := jsonx.String(v, "stdin")
		if err != nil {
			return err
		}
		a.Stdin = &text
	}
	a.Cwd = "."
	if v, ok := s["cwd"]; ok {
		if a.Cwd, err = jsonx.String(v, "cwd"); err != nil {
			return err
		}
	}
	return nil
}

func parseHook(s map[string]any, a *Action) error {
	var err error
	if a.Hook, err = jsonx.String(s["hook"], "hook"); err != nil {
		return err
	}
	if !hookNames[a.Hook] {
		return fmt.Errorf("hook: want guard, got %q", a.Hook)
	}
	input, ok := s["input"]
	if !ok {
		return fmt.Errorf("a hook step needs input")
	}
	if text, isText := input.(string); isText {
		a.Input = text
		return nil
	}
	data, err := json.Marshal(input)
	a.Input = string(data)
	return err
}

func parseExec(s map[string]any, a *Action) error {
	which, err := jsonx.String(s["exec"], "exec")
	if err != nil {
		return err
	}
	if which != "run_sh" {
		return fmt.Errorf("exec: the only executable is \"run_sh\", got %q", which)
	}
	if a.Launch, err = jsonx.String(s["launch"], "launch"); err != nil {
		return fmt.Errorf("an exec step needs launch: %w", err)
	}
	if !filepath.IsLocal(a.Launch) || filepath.Base(a.Launch) != a.Launch {
		return fmt.Errorf("launch: %q is not a launch id", a.Launch)
	}
	return nil
}

func parseWrite(s map[string]any, a *Action, caseDir string, vars Vars) error {
	obj, err := jsonx.Object(s["write"], "write")
	if err != nil {
		return err
	}
	if err := jsonx.Keys(obj, "write", "project", "home"); err != nil {
		return err
	}
	if a.ProjectFiles, err = ParseFileMap(obj["project"], "write.project", caseDir, vars); err != nil {
		return err
	}
	a.HomeFiles, err = ParseFileMap(obj["home"], "write.home", caseDir, vars)
	return err
}

func parseCaptures(v any) ([]Capture, error) {
	obj, err := jsonx.Object(v, "capture")
	if err != nil {
		return nil, err
	}
	var out []Capture
	for _, name := range jsonx.SortedKeys(obj) {
		at := "capture." + name
		spec, err := jsonx.Object(obj[name], at)
		if err != nil {
			return nil, err
		}
		if err := jsonx.Keys(spec, at, "from", "path", "regex", "group"); err != nil {
			return nil, err
		}
		c := Capture{Name: name}
		if c.From, err = jsonx.String(spec["from"], at+".from"); err != nil {
			return nil, err
		}
		if !contains(captureSources, c.From) {
			return nil, fmt.Errorf("%s.from: want one of %v, got %q", at, captureSources, c.From)
		}
		raw, hasPath := spec["path"]
		if hasPath != (c.From == "file") {
			return nil, fmt.Errorf("%s: path goes with from \"file\", and only there", at)
		}
		if hasPath {
			if c.Path, err = jsonx.String(raw, at+".path"); err != nil {
				return nil, err
			}
		}
		pattern, err := jsonx.String(spec["regex"], at+".regex")
		if err != nil {
			return nil, err
		}
		if c.Re, err = expect.CompileRegex(pattern, at+".regex"); err != nil {
			return nil, err
		}
		if c.Re.NumSubexp() > 0 {
			c.Group = 1
		}
		if g, ok := spec["group"]; ok {
			if c.Group, err = jsonx.Int(g, at+".group"); err != nil {
				return nil, err
			}
		}
		if c.Group < 0 || c.Group > c.Re.NumSubexp() {
			return nil, fmt.Errorf("%s.group: %d, but the regex has %d groups", at, c.Group, c.Re.NumSubexp())
		}
		out = append(out, c)
	}
	return out, nil
}
