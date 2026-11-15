// Package dispatch opens the interactive dispatcher: `jitso "<request>"`
// starts one fresh claude in a fixed folder outside every checkout, with a
// system prompt rendered from the project's jitso/dispatcher/prompt.md, two
// tools and no permission bypass. The dispatch guard hook (package hook) is
// registered on it through --settings and narrows Bash to one `jitso launch`
// line and Write to the briefs folder.
//
// The plan (the claude argv) is a pure value; Open is the I/O around it.
package dispatch

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
)

// PromptFile is the dispatcher prompt template, relative to the project.
const PromptFile = "jitso/dispatcher/prompt.md"

// Options are the inputs and the ports of Open.
type Options struct {
	// Request is a checked request (CheckRequest).
	Request string
	// Root is the primary checkout of the project.
	Root string
	// Home is $HOME: the dispatcher folder lives in it.
	Home string
	// Summary is the catalog summary (catalog.Summary).
	Summary string
	// Launcher is the real path of this executable.
	Launcher string
	// Environ is the environment claude inherits; JITSO_PROJECT is added.
	Environ []string
	// Spawn runs argv in dir in the foreground with the terminal inherited and
	// returns its exit status; an error means the program could not start.
	Spawn func(argv []string, dir string, env []string) (int, error)
}

// Open runs the dispatcher for one request. It returns nil when claude exits
// 0, fail.Status(code) for any other exit status of claude (nothing is
// printed), and an environment error (exit 3) when the prompt cannot be read or
// claude cannot start.
func Open(o Options) error {
	if o.Home == "" {
		return fail.Env("HOME is not set, so the dispatcher folder is unknown")
	}
	template, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(PromptFile)))
	if err != nil {
		return fail.Envf("cannot read %s: %v", PromptFile, err)
	}
	dir := project.DispatchDir(o.Home)
	plan, err := BuildPlan(PlanInput{Request: o.Request, Template: string(template), Summary: o.Summary, Launcher: o.Launcher, DispatchDir: dir})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(BriefsDir(dir), 0o755); err != nil {
		return fail.Envf("cannot create %s: %v", BriefsDir(dir), err)
	}
	status, err := o.Spawn(plan.Argv, plan.Dir, HandoffEnv(o.Environ, o.Root))
	if err != nil {
		return fail.Env("could not start claude (is it on PATH?)")
	}
	switch {
	case status == 0:
		return nil
	case status < 0:
		return fail.Status(1)
	}
	return fail.Status(status)
}

// HandoffEnv is environ with JITSO_PROJECT set to the primary checkout, so the
// dispatched claude's one `jitso launch` finds the project from outside git. An
// inherited JITSO_PROJECT is replaced, never duplicated.
func HandoffEnv(environ []string, primary string) []string {
	prefix := project.EnvVar + "="
	env := make([]string, 0, len(environ)+1)
	for _, entry := range environ {
		if !strings.HasPrefix(entry, prefix) {
			env = append(env, entry)
		}
	}
	return append(env, prefix+primary)
}
