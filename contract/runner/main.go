// Command jitso-contract-runner executes the JITSO contract cases (contract/SPEC.md) against
// one implementation. The same binary is also the fake `orca` and the fake `claude`: the
// runner links itself into each sandbox under those names and dispatches on os.Args[0].
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"jitso-contract-runner/internal/engine"
	"jitso-contract-runner/internal/fakes"
	"jitso-contract-runner/internal/impl"
	"jitso-contract-runner/internal/report"
	"jitso-contract-runner/internal/spec"
)

func main() {
	switch strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe") {
	case "orca":
		os.Exit(fakes.Orca(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
	case "claude":
		os.Exit(fakes.Claude(os.Args[1:], os.Environ(), os.Stderr))
	}
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type config struct {
	impl, tsSrc, bin string
	cases, fixtures  string
	filter           string
	skipTags         []string
	verbose, keep    bool
	jobs             int
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	var c config
	fs := flag.NewFlagSet("jitso-contract-runner", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.impl, "impl", "", "implementation under test: ts or bin")
	fs.StringVar(&c.tsSrc, "ts-src", "", "boilerplate checkout holding the TS oracle (--impl ts)")
	fs.StringVar(&c.bin, "bin", "", "path of the jitso executable (--impl bin)")
	fs.StringVar(&c.cases, "cases", "", "cases folder (default: contract/cases found above the cwd or the executable)")
	fs.StringVar(&c.fixtures, "fixtures", "", "fixtures folder (default: the sibling `fixtures` of the cases folder)")
	fs.StringVar(&c.filter, "filter", "", "run only cases whose expanded id matches this regex")
	fs.Func("skip-tag", "skip cases carrying this tag (repeatable)", func(s string) error {
		c.skipTags = append(c.skipTags, s)
		return nil
	})
	fs.BoolVar(&c.verbose, "v", false, "print argv, cwd, stdout, stderr and the sandbox of every step")
	fs.BoolVar(&c.keep, "keep", false, "keep each sandbox and print its path")
	fs.IntVar(&c.jobs, "jobs", 1, "cases run in parallel (timing expectations assume 1)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return c, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseFlags(args, stderr)
	if err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stderr, "jitso-contract-runner:", err)
		}
		return 2
	}
	usage := func(err error) int {
		fmt.Fprintln(stderr, "jitso-contract-runner:", err)
		return 2
	}
	im, err := newImplementation(cfg)
	if err != nil {
		return usage(err)
	}
	if err := locate(&cfg); err != nil {
		return usage(err)
	}
	var filter *regexp.Regexp
	if cfg.filter != "" {
		if filter, err = regexp.Compile(cfg.filter); err != nil {
			return usage(fmt.Errorf("--filter: %v", err))
		}
	}
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		return usage(fmt.Errorf("cannot find the runner binary: %v", err))
	}

	rep := &report.Text{W: stdout, Verbose: cfg.verbose}
	start := time.Now()
	xs, loadErrors, err := loadCases(cfg.cases, cfg.fixtures, filter, rep)
	if err != nil {
		return usage(err)
	}
	if len(xs) == 0 && len(loadErrors) == 0 {
		return usage(fmt.Errorf("no case selected under %s", cfg.cases))
	}
	opts := engine.Options{Impl: im, Self: self, Fixtures: cfg.fixtures, Keep: cfg.keep, SkipTags: cfg.skipTags}
	results := engine.RunAll(xs, opts, cfg.jobs, rep.Case)
	rep.Summary(im.Name(), results, loadErrors, time.Since(start))

	statuses := make([]engine.Status, len(results))
	for i, r := range results {
		statuses[i] = r.Status
	}
	return engine.ExitCode(statuses, len(loadErrors))
}

func newImplementation(cfg config) (impl.Implementation, error) {
	switch cfg.impl {
	case "ts":
		return impl.NewTS(cfg.tsSrc)
	case "bin":
		return impl.NewBin(cfg.bin)
	case "":
		return nil, errors.New("--impl is required: ts or bin")
	}
	return nil, fmt.Errorf("--impl: want ts or bin, got %q", cfg.impl)
}

// loadCases loads every case, reports load errors, and expands and filters the rest.
func loadCases(casesRoot, fixtures string, filter *regexp.Regexp, rep report.Reporter) ([]spec.Expansion, []string, error) {
	dirs, err := spec.Discover(casesRoot)
	if err != nil {
		return nil, nil, err
	}
	var xs []spec.Expansion
	var loadErrors []string
	for _, dir := range dirs {
		c, err := spec.Load(casesRoot, dir, fixtures)
		if err != nil {
			path := filepath.Join(dir, spec.CaseFile)
			rep.LoadError(path, err)
			loadErrors = append(loadErrors, path)
			continue
		}
		for _, x := range c.Expand() {
			if filter == nil || filter.MatchString(x.ID) {
				xs = append(xs, x)
			}
		}
	}
	return xs, loadErrors, nil
}

// locate fills the default cases and fixtures folders.
func locate(cfg *config) error {
	if cfg.cases == "" {
		contract, err := findContract()
		if err != nil {
			return err
		}
		cfg.cases = filepath.Join(contract, "cases")
	}
	abs, err := filepath.Abs(cfg.cases)
	if err != nil {
		return err
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return fmt.Errorf("--cases: %s is not a folder", abs)
	}
	cfg.cases = abs
	if cfg.fixtures == "" {
		cfg.fixtures = filepath.Join(filepath.Dir(abs), "fixtures")
	}
	if cfg.fixtures, err = filepath.Abs(cfg.fixtures); err != nil {
		return err
	}
	return nil
}

// findContract walks up from the cwd, then from the executable (useless under `go run`,
// whose binary lives in a build cache), to the folder holding SPEC.md and runner/go.mod.
func findContract() (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		for dir := start; ; dir = filepath.Dir(dir) {
			for _, candidate := range []string{dir, filepath.Join(dir, "contract")} {
				if isFile(filepath.Join(candidate, "SPEC.md")) && isFile(filepath.Join(candidate, "runner", "go.mod")) {
					return candidate, nil
				}
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return "", errors.New("cannot find contract/cases above the cwd or the executable; pass --cases")
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}
