// Package impl adapts the `jitso` binary under test to the runner (SPEC 9). The adapter says
// how to prepare a sandbox for it and which argv starts the CLI or a hook; nothing else in
// the runner knows where the binary lives.
package impl

import "jitso-contract-runner/internal/sandbox"

// Implementation is the `jitso` under test.
type Implementation interface {
	// Name is what the report prints.
	Name() string
	// Prepare installs whatever the implementation needs into a fresh sandbox.
	Prepare(sb *sandbox.Sandbox) error
	// Command is the argv that runs the CLI with the given arguments.
	Command(sb *sandbox.Sandbox, args []string) ([]string, error)
	// HookCommand is the argv that runs a hook (`guard`).
	HookCommand(sb *sandbox.Sandbox, hook string) ([]string, error)
	// Launcher is the value of {{JITSO}}: the command line, as one string of words, that a
	// hook or a shell would use to start this implementation in the sandbox.
	Launcher(sb *sandbox.Sandbox) string
}
