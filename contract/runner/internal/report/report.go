// Package report prints what the runner did (SPEC 8): one line per case expansion, the
// details of a FAIL, the full step record under -v, and a summary.
package report

import (
	"fmt"
	"io"
	"strings"
	"time"

	"jitso-contract-runner/internal/engine"
)

// Reporter receives the run as it happens.
type Reporter interface {
	LoadError(path string, err error)
	Case(r engine.Result)
	Summary(impl string, results []engine.Result, loadErrors []string, elapsed time.Duration)
}

// Text is the human-readable reporter.
type Text struct {
	W       io.Writer
	Verbose bool
}

const maxShown = 2000

// LoadError prints a case that could not be loaded.
func (t *Text) LoadError(path string, err error) {
	fmt.Fprintf(t.W, "LOAD  %s\n      %v\n", path, err)
}

// Case prints one expansion.
func (t *Text) Case(r engine.Result) {
	fmt.Fprintf(t.W, "%-5s %s%s\n", r.Status, r.ID, t.suffix(r))
	if r.RunnerError != "" {
		fmt.Fprintf(t.W, "      runner error: %s\n", r.RunnerError)
	}
	if r.Status == engine.Fail || t.Verbose {
		t.steps(r)
	}
	switch {
	case r.Sandbox == "":
	case r.Kept && (r.Status == engine.Fail || r.Status == engine.Xpass || t.Verbose):
		fmt.Fprintf(t.W, "      sandbox: %s\n", r.Sandbox)
	case t.Verbose:
		fmt.Fprintf(t.W, "      sandbox: %s (removed; rerun with --keep)\n", r.Sandbox)
	}
}

func (t *Text) suffix(r engine.Result) string {
	switch r.Status {
	case engine.Skip:
		return "  (" + r.SkipReason + ")"
	case engine.Xpass:
		return fmt.Sprintf("  (declared xfail for %s, but every expectation held: promote it to pass) %s", r.Defect, ms(r.Elapsed))
	case engine.Xfail:
		return fmt.Sprintf("  (%s) %s", r.Defect, ms(r.Elapsed))
	}
	return "  " + ms(r.Elapsed)
}

func (t *Text) steps(r engine.Result) {
	for _, s := range r.Steps {
		if t.Verbose {
			t.record(s)
		}
		for _, f := range s.Failures {
			fmt.Fprintf(t.W, "      step %d (%s): %s\n", s.Index+1, s.Kind, f.What)
			fmt.Fprintf(t.W, "        expected: %s\n", indent(f.Expected))
			fmt.Fprintf(t.W, "        actual:   %s\n", indent(clip(f.Actual)))
		}
	}
	if ran := len(r.Steps); ran < r.TotalSteps && r.Status != engine.Skip {
		fmt.Fprintf(t.W, "      steps %d..%d not run\n", ran+1, r.TotalSteps)
	}
}

func (t *Text) record(s engine.StepResult) {
	fmt.Fprintf(t.W, "      step %d (%s)", s.Index+1, s.Kind)
	if s.Argv == nil {
		fmt.Fprintln(t.W)
		return
	}
	fmt.Fprintf(t.W, " exit %d in %s\n", s.Exit, ms(s.Elapsed))
	fmt.Fprintf(t.W, "        argv:   %q\n", s.Argv)
	fmt.Fprintf(t.W, "        cwd:    %s\n", s.Cwd)
	fmt.Fprintf(t.W, "        stdout: %s\n", indent(clip(s.Stdout)))
	fmt.Fprintf(t.W, "        stderr: %s\n", indent(clip(s.Stderr)))
}

// Summary prints the counts and repeats the load errors, so they are not lost above.
func (t *Text) Summary(impl string, results []engine.Result, loadErrors []string, elapsed time.Duration) {
	counts := map[engine.Status]int{}
	for _, r := range results {
		counts[r.Status]++
	}
	fmt.Fprintf(t.W, "\n%s: %d cases, %d PASS, %d FAIL, %d XFAIL, %d XPASS, %d SKIP, %d LOAD ERROR in %s\n",
		impl, len(results), counts[engine.Pass], counts[engine.Fail], counts[engine.Xfail], counts[engine.Xpass], counts[engine.Skip], len(loadErrors), elapsed.Round(10*time.Millisecond))
	for _, path := range loadErrors {
		fmt.Fprintf(t.W, "  load error: %s\n", path)
	}
}

func ms(d time.Duration) string { return fmt.Sprintf("%d ms", d.Milliseconds()) }

func clip(s string) string {
	if s == "" {
		return "(empty)"
	}
	if len(s) > maxShown {
		return fmt.Sprintf("%s... (%d more bytes)", s[:maxShown], len(s)-maxShown)
	}
	return s
}

func indent(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n                  ")
}
