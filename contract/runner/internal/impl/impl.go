// Package impl adapts an implementation under test to the runner (SPEC 9). An adapter says
// how to prepare a sandbox for it and which argv starts the CLI or a hook; nothing else in
// the runner knows which implementation it is talking to.
package impl

import "jitso-contract-runner/internal/sandbox"

// Implementation is one `jitso` under test.
type Implementation interface {
	// Name is what the report prints.
	Name() string
	// StatusKey is the key of case.json `status` this implementation is judged by.
	StatusKey() string
	// Prepare installs whatever the implementation needs into a fresh sandbox.
	Prepare(sb *sandbox.Sandbox) error
	// Command is the argv that runs the CLI with the given arguments.
	Command(sb *sandbox.Sandbox, args []string) ([]string, error)
	// HookCommand is the argv that runs a hook (`guard` or `dispatch-guard`).
	HookCommand(sb *sandbox.Sandbox, hook string) ([]string, error)
	// Launcher is the value of {{JITSO}}: the command line, as one string of words, that a
	// hook or a shell would use to start this implementation in the sandbox.
	Launcher(sb *sandbox.Sandbox) string
	// AroundStep is called before a `run` or `hook` step. It may prepare the sandbox for the
	// step and returns after, which the engine calls right after the step's process ends,
	// before captures and expectations, even when the step failed to run. An adapter keeps
	// its state in the closure, never in its own value: sandboxes run in parallel. A failing
	// after is a runner error. `bin` has nothing to do; `ts` maps the registry schema (SPEC 9).
	AroundStep(sb *sandbox.Sandbox) (after func() error, err error)
}
