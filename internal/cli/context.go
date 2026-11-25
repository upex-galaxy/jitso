package cli

import (
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// System is every port a command may touch. cmd/jitso wires the real ones;
// tests pass stand-ins.
type System struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// LookupEnv is os.LookupEnv.
	LookupEnv func(string) (string, bool)
	// Getwd is os.Getwd.
	Getwd func() (string, error)
	// Executable is os.Executable.
	Executable func() (string, error)
	// Now and Sleep are the clock.
	Now   func() time.Time
	Sleep func(time.Duration)
	// Rand is the randomness source (crypto/rand.Reader).
	Rand io.Reader
	// Orca runs the orca program.
	Orca orca.Runner
	// OrcaWithin is a runner that gives up on a call after a limit
	// (orca.ExecRunnerTimeout); when nil, Orca is used.
	OrcaWithin func(limit time.Duration) orca.Runner
	// StdinIsTerminal reports whether standard input is a terminal
	// (tty.IsTerminal(os.Stdin)); nil means it is not. A command that asks
	// a person a question asks only when it is true.
	StdinIsTerminal func() bool
}

// Context is what a command runs with: the System plus memoized lookups of
// the project, its catalog and its registry. Nothing is resolved until a
// command asks, so help, an unknown command and the not-implemented answers
// never run git or touch a file.
type Context struct {
	Sys System

	root       *project.Root
	rootErr    error
	scan       *catalog.Result
	scanErr    error
	catalog    *catalog.Catalog
	catalogErr error
}

// Getenv is the value of a variable, "" when unset.
func (c *Context) Getenv(name string) string {
	if c.Sys.LookupEnv == nil {
		return ""
	}
	v, _ := c.Sys.LookupEnv(name)
	return v
}

// Home is $HOME.
func (c *Context) Home() string { return c.Getenv("HOME") }

// Println writes one line to stdout.
func (c *Context) Println(line string) {
	io.WriteString(c.Sys.Stdout, line+"\n")
}

// Warn writes one line to stderr, for a warning that does not fail the
// command (for example the trust timeout, L3).
func (c *Context) Warn(line string) {
	io.WriteString(c.Sys.Stderr, line+"\n")
}

// Project resolves the project once: the git primary checkout of the cwd.
// Outside a checkout it is an Env error (exit 3) naming the folder.
func (c *Context) Project() (project.Root, error) {
	if c.root == nil && c.rootErr == nil {
		getwd := c.Sys.Getwd
		if getwd == nil {
			getwd = os.Getwd
		}
		cwd, err := getwd()
		if err != nil {
			c.rootErr = err
		} else {
			r, err := project.Resolve(cwd)
			c.root, c.rootErr = &r, err
		}
	}
	if c.rootErr != nil {
		return project.Root{}, c.rootErr
	}
	return *c.root, nil
}

// Scan reads every build file of the project once, with no whole-catalog rule:
// an invalid file is a problem of that file (see catalog.Scan). Commands that
// show files, drafts included, use it after Catalog has applied the rule.
func (c *Context) Scan() (*catalog.Result, error) {
	if c.scan == nil && c.scanErr == nil {
		root, err := c.Project()
		if err != nil {
			return nil, err
		}
		c.scan, c.scanErr = catalog.ScanHome(os.DirFS(root.Primary), root.Primary, c.Home())
	}
	return c.scan, c.scanErr
}

// Catalog loads the project's catalog once: its own builds from the primary
// checkout, then its drafts (the folders are computed from the primary's real
// path). The project's own files keep the whole-catalog rule: one invalid file
// is the error.
func (c *Context) Catalog() (*catalog.Catalog, error) {
	if c.catalog == nil && c.catalogErr == nil {
		res, err := c.Scan()
		if err != nil {
			return nil, err
		}
		c.catalog, c.catalogErr = res.Catalog()
	}
	return c.catalog, c.catalogErr
}

// Registry is the registry store of the project's primary checkout, on the
// context's clock.
func (c *Context) Registry() (*registry.FileStore, error) {
	root, err := c.Project()
	if err != nil {
		return nil, err
	}
	s := registry.NewFileStore(root.StateDir())
	if c.Sys.Now != nil {
		s.Now = c.Sys.Now
	}
	if c.Sys.Sleep != nil {
		s.Sleep = c.Sys.Sleep
	}
	return s, nil
}

// Orca is a client over the context's orca runner.
func (c *Context) Orca() *orca.Client {
	run := c.Sys.Orca
	if run == nil {
		run = orca.ExecRunner(orca.Command(c.lookupEnv()))
	}
	return orca.NewClient(run)
}

// OrcaWithin is a client whose calls are given up after limit, for the command
// that must stay responsive.
func (c *Context) OrcaWithin(limit time.Duration) *orca.Client {
	var run orca.Runner
	switch {
	case c.Sys.OrcaWithin != nil:
		run = c.Sys.OrcaWithin(limit)
	case c.Sys.Orca != nil:
		run = c.Sys.Orca
	default:
		run = orca.ExecRunnerTimeout(orca.Command(c.lookupEnv()), limit)
	}
	return orca.NewClient(run)
}

// OrcaInstalled checks that the orca program is on PATH (orca.Installed),
// without running it. A command that calls Orca checks it before it writes
// anything or makes its first call.
func (c *Context) OrcaInstalled() error {
	return orca.Installed(c.lookupEnv(), exec.LookPath)
}

// lookupEnv is the context's LookupEnv, os.LookupEnv when unset.
func (c *Context) lookupEnv() func(string) (string, bool) {
	if c.Sys.LookupEnv == nil {
		return os.LookupEnv
	}
	return c.Sys.LookupEnv
}
