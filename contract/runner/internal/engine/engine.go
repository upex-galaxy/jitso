// Package engine runs one case expansion: it builds the sandbox, executes the steps in
// order, takes captures, evaluates each step's expectations and classifies the outcome.
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"jitso-contract-runner/internal/expect"
	"jitso-contract-runner/internal/impl"
	"jitso-contract-runner/internal/sandbox"
	"jitso-contract-runner/internal/spec"
)

// Options is what every case run shares.
type Options struct {
	Impl     impl.Implementation
	Self     string // the runner binary, linked into each sandbox as the fakes
	Fixtures string
	Keep     bool
	SkipTags []string
}

// StepResult is what one step did, for the report.
type StepResult struct {
	Index    int
	Kind     string
	Argv     []string
	Cwd      string
	Exit     int
	TimedOut bool
	Stdout   string
	Stderr   string
	Elapsed  time.Duration
	Failures []expect.Failure
}

// Result is the outcome of one case expansion.
type Result struct {
	ID          string
	Status      Status
	SkipReason  string
	Steps       []StepResult
	TotalSteps  int
	RunnerError string
	Sandbox     string // the sandbox root; removed after the run unless Kept
	Kept        bool
	Elapsed     time.Duration
}

// RunAll runs the expansions on `jobs` workers and calls report in input order.
func RunAll(xs []spec.Expansion, opts Options, jobs int, report func(Result)) []Result {
	if jobs < 1 {
		jobs = 1
	}
	results := make([]Result, len(xs))
	done := make([]chan struct{}, len(xs))
	for i := range done {
		done[i] = make(chan struct{})
	}
	work := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < jobs; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				results[i] = RunCase(xs[i], opts)
				close(done[i])
			}
		}()
	}
	go func() {
		for i := range xs {
			work <- i
		}
		close(work)
	}()
	for i := range xs {
		<-done[i]
		report(results[i])
	}
	wg.Wait()
	return results
}

// RunCase runs one expansion in a fresh sandbox.
func RunCase(x spec.Expansion, opts Options) Result {
	start := time.Now()
	res := Result{ID: x.ID, TotalSteps: x.StepCount()}
	for _, tag := range opts.SkipTags {
		if x.Case.HasTag(tag) {
			res.Status, res.SkipReason = Skip, "tag "+tag
			return res
		}
	}
	held, err := run(x, opts, &res)
	if err != nil {
		res.RunnerError = err.Error()
	}
	res.Status = Classify(held, err != nil)
	res.Elapsed = time.Since(start)
	return res
}

func run(x spec.Expansion, opts Options, res *Result) (bool, error) {
	sb, err := sandbox.New(opts.Self)
	if err != nil {
		return false, fmt.Errorf("sandbox: %w", err)
	}
	res.Sandbox, res.Kept = sb.Root, opts.Keep
	if !opts.Keep {
		defer sb.Remove()
	}
	builtins := spec.Vars{spec.VarProject: sb.Project, spec.VarHome: sb.Home, spec.VarBin: sb.Bin, spec.VarJitso: opts.Impl.Launcher(sb)}
	for _, name := range x.Case.Worktrees() {
		builtins[spec.WorktreeVar(name)] = sb.Worktree(name)
	}
	vars := x.Bind(builtins)
	if err := build(x, sb, vars, opts); err != nil {
		return false, fmt.Errorf("sandbox: %w", err)
	}
	for i := 0; i < x.StepCount(); i++ {
		sr, next, err := runStepAt(x, i, sb, vars, opts.Impl)
		res.Steps = append(res.Steps, sr)
		if err != nil {
			return false, fmt.Errorf("step %d: %w", i+1, err)
		}
		vars = next
		if len(sr.Failures) > 0 {
			return false, nil
		}
	}
	return true, nil
}

// build lays out the project and HOME and writes the fake orca scenario.
func build(x spec.Expansion, sb *sandbox.Sandbox, vars spec.Vars, opts Options) error {
	setup, err := x.Setup(vars)
	if err != nil {
		return err
	}
	if err := sandbox.CopyTree(filepath.Join(opts.Fixtures, x.Case.Base), sb.Project, nil); err != nil {
		return err
	}
	if err := sb.GitInit(); err != nil {
		return err
	}
	if err := sandbox.Apply(sb.Project, setup.ProjectFiles); err != nil {
		return err
	}
	if err := sandbox.Apply(sb.Home, setup.HomeFiles); err != nil {
		return err
	}
	if err := sb.WriteScenario(setup.Scenario); err != nil {
		return err
	}
	if setup.Scenario.Claude.Missing {
		if err := sb.RemoveFake("claude"); err != nil {
			return err
		}
	}
	return opts.Impl.Prepare(sb)
}

// runStepAt executes step i and returns the variables extended with its captures.
func runStepAt(x spec.Expansion, i int, sb *sandbox.Sandbox, vars spec.Vars, im impl.Implementation) (StepResult, spec.Vars, error) {
	sr := StepResult{Index: i}
	a, err := x.Action(i, vars)
	if err != nil {
		return sr, vars, err
	}
	sr.Kind = a.Kind
	obs, err := stepKinds[a.Kind](a).Execute(sb, im, &sr)
	if err != nil {
		return sr, vars, err
	}
	obs.Project, obs.OrcaLog, obs.ClaudeDump = sb.Project, sb.OrcaLog, sb.ClaudeDump

	captured := spec.Vars{}
	for _, c := range a.Captures {
		text := captureText(c, obs, sb)
		if m := c.Re.FindStringSubmatch(text); m != nil {
			captured[c.Name] = m[c.Group]
			continue
		}
		// Keep evaluating the step's other expectations; the placeholder carries the failure.
		captured[c.Name] = fmt.Sprintf("<capture %s failed>", c.Name)
		sr.Failures = append(sr.Failures, expect.Failure{What: "capture " + c.Name, Expected: fmt.Sprintf("%s matching /%s/", c.From, c.Re), Actual: text})
	}
	next := vars.With(captured)
	checks, err := x.Expect(i, a.Kind, next)
	if err != nil {
		return sr, next, err
	}
	sr.Failures = append(sr.Failures, checks.Eval(obs)...)
	return sr, next, nil
}

// captureText is the text a capture reads: the step's stdout or stderr, a file (relative
// to the project dir), or the raw fake orca call log. A missing file is empty text.
func captureText(c spec.Capture, obs *expect.Observation, sb *sandbox.Sandbox) string {
	path := ""
	switch c.From {
	case "stdout":
		return obs.Stdout
	case "stderr":
		return obs.Stderr
	case "orca_log":
		path = sb.OrcaLog
	case "file":
		path = c.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(sb.Project, path)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
