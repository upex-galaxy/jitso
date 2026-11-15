package engine

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"jitso-contract-runner/internal/expect"
	"jitso-contract-runner/internal/impl"
	"jitso-contract-runner/internal/jsonx"
	"jitso-contract-runner/internal/sandbox"
	"jitso-contract-runner/internal/spec"
)

// Step is one executable step kind (SPEC 6). Execute fills the observation and the report
// fields of res; an error is a runner error, never an expectation failure.
type Step interface {
	Execute(sb *sandbox.Sandbox, im impl.Implementation, res *StepResult) (*expect.Observation, error)
}

var stepKinds = map[string]func(*spec.Action) Step{
	expect.KindRun:         func(a *spec.Action) Step { return runStep{a} },
	expect.KindHook:        func(a *spec.Action) Step { return hookStep{a} },
	expect.KindExec:        func(a *spec.Action) Step { return execStep{a} },
	expect.KindWrite:       func(a *spec.Action) Step { return writeStep{a} },
	expect.KindGitWorktree: func(a *spec.Action) Step { return worktreeStep{a} },
	expect.KindGitInit:     func(a *spec.Action) Step { return gitInitStep{a} },
}

// runProcessStep is the part run, hook and exec share.
func runProcessStep(sb *sandbox.Sandbox, res *StepResult, kind string, argv []string, dir string, env []string, stdin *string) (*expect.Observation, error) {
	res.Argv, res.Cwd = argv, dir
	p, err := runProcess(argv, dir, env, stdin, StepTimeout)
	if err != nil {
		return nil, err
	}
	res.Exit, res.TimedOut, res.Stdout, res.Stderr, res.Elapsed = p.exit, p.timedOut, p.stdout, p.stderr, p.elapsed
	return &expect.Observation{Kind: kind, Exit: p.exit, TimedOut: p.timedOut, Stdout: p.stdout, Stderr: p.stderr, Elapsed: p.elapsed}, nil
}

// --- run: the CLI ---

type runStep struct{ a *spec.Action }

func (s runStep) Execute(sb *sandbox.Sandbox, im impl.Implementation, res *StepResult) (*expect.Observation, error) {
	dir := s.a.Cwd
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(sb.Project, dir)
	}
	if !sb.Inside(dir) {
		return nil, fmt.Errorf("cwd %s is outside the sandbox", dir)
	}
	argv, err := im.Command(sb, s.a.Argv)
	if err != nil {
		return nil, err
	}
	if err := sb.ClearClaudeDump(); err != nil {
		return nil, err
	}
	return runProcessStep(sb, res, expect.KindRun, argv, dir, sb.Env(s.a.Env), s.a.Stdin)
}

// --- hook: a hook script ---

type hookStep struct{ a *spec.Action }

func (s hookStep) Execute(sb *sandbox.Sandbox, im impl.Implementation, res *StepResult) (*expect.Observation, error) {
	argv, err := im.HookCommand(sb, s.a.Hook)
	if err != nil {
		return nil, err
	}
	env := map[string]string{"CLAUDE_PROJECT_DIR": sb.Project}
	for k, v := range s.a.Env {
		env[k] = v
	}
	obs, err := runProcessStep(sb, res, expect.KindHook, argv, sb.Project, sb.Env(env), &s.a.Input)
	if err != nil {
		return nil, err
	}
	obs.Decision, obs.Reason = ParseDecision(obs.Stdout)
	return obs, nil
}

// ParseDecision reads the hook output convention of SPEC 6.2.
func ParseDecision(stdout string) (decision, reason string) {
	text := strings.TrimSpace(stdout)
	if text == "" {
		return "none", ""
	}
	doc, err := jsonx.Decode([]byte(text))
	if err != nil {
		return "(stdout is not one JSON object)", ""
	}
	d, _ := jsonx.Lookup(doc, "hookSpecificOutput.permissionDecision")
	r, _ := jsonx.Lookup(doc, "hookSpecificOutput.permissionDecisionReason")
	ds, ok := d.(string)
	if !ok {
		return "(no hookSpecificOutput.permissionDecision)", ""
	}
	rs, _ := r.(string)
	return ds, rs
}

// --- exec: a generated run.sh against the fake claude ---

type execStep struct{ a *spec.Action }

func (s execStep) Execute(sb *sandbox.Sandbox, _ impl.Implementation, res *StepResult) (*expect.Observation, error) {
	script := filepath.Join(sb.Project, ".jitso", "launches", s.a.Launch, "run.sh")
	if err := sb.ClearClaudeDump(); err != nil {
		return nil, err
	}
	return runProcessStep(sb, res, expect.KindExec, []string{"/bin/sh", script}, sb.Project, sb.Env(s.a.Env), nil)
}

// --- write: change the sandbox between steps ---

type writeStep struct{ a *spec.Action }

func (s writeStep) Execute(sb *sandbox.Sandbox, _ impl.Implementation, res *StepResult) (*expect.Observation, error) {
	start := time.Now()
	if err := sandbox.Apply(sb.Project, s.a.ProjectFiles); err != nil {
		return nil, err
	}
	if err := sandbox.Apply(sb.Home, s.a.HomeFiles); err != nil {
		return nil, err
	}
	res.Elapsed = time.Since(start)
	return &expect.Observation{Kind: expect.KindWrite, Elapsed: res.Elapsed}, nil
}

// --- git_worktree: a linked worktree of the project ---

type worktreeStep struct{ a *spec.Action }

func (s worktreeStep) Execute(sb *sandbox.Sandbox, _ impl.Implementation, res *StepResult) (*expect.Observation, error) {
	start := time.Now()
	if err := sb.AddWorktree(s.a.Worktree); err != nil {
		return nil, err
	}
	res.Elapsed = time.Since(start)
	return &expect.Observation{Kind: expect.KindGitWorktree, Elapsed: res.Elapsed}, nil
}

// --- git_init: a nested git checkout inside the project or HOME ---

type gitInitStep struct{ a *spec.Action }

func (s gitInitStep) Execute(sb *sandbox.Sandbox, _ impl.Implementation, res *StepResult) (*expect.Observation, error) {
	start := time.Now()
	if err := sb.InitRepo(s.a.InitPath); err != nil {
		return nil, err
	}
	res.Elapsed = time.Since(start)
	return &expect.Observation{Kind: expect.KindGitInit, Elapsed: res.Elapsed}, nil
}
