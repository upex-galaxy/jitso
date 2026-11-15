package cli

import (
	"io"
	"os"
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
	// Spawn runs a program in the foreground with the terminal inherited and
	// returns its exit status; an error means it could not start.
	Spawn func(argv []string, dir string, env []string) (int, error)
}

// Context is what a command runs with: the System plus memoized lookups of
// the project, its catalog and its registry. Nothing is resolved until a
// command asks, so help, an unknown command and the not-implemented answers
// never run git or touch a file.
type Context struct {
	Sys System

	root       *project.Root
	rootErr    error
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

// Project resolves the project once: the git primary checkout of
// JITSO_PROJECT when it is set and not empty, else of the cwd. Outside a
// checkout it is an Env error (exit 3) naming the folder.
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
			r, err := project.Resolve(cwd, c.Getenv(project.EnvVar))
			c.root, c.rootErr = &r, err
		}
	}
	if c.rootErr != nil {
		return project.Root{}, c.rootErr
	}
	return *c.root, nil
}

// Catalog loads the project's catalog once, from its primary checkout.
func (c *Context) Catalog() (*catalog.Catalog, error) {
	if c.catalog == nil && c.catalogErr == nil {
		root, err := c.Project()
		if err != nil {
			return nil, err
		}
		c.catalog, c.catalogErr = catalog.Load(os.DirFS(root.Primary))
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
		lookup := c.Sys.LookupEnv
		if lookup == nil {
			lookup = os.LookupEnv
		}
		run = orca.ExecRunner(orca.Command(lookup))
	}
	return orca.NewClient(run)
}
