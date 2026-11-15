// Package orca is a thin client over the orca CLI. Every call goes through a
// Runner, so tests pass a scripted one and never touch a real Orca. The real
// runner appends --json, closes stdin and runs the program named by
// ORCA_CLI_COMMAND (default orca).
//
// Only terminal verbs are typed here; orchestration verbs go through Call.
package orca

import (
	"bytes"
	"errors"
	"os/exec"
)

// CommandEnv names the variable that overrides the orca program.
const CommandEnv = "ORCA_CLI_COMMAND"

// DefaultCommand is the program run when CommandEnv is unset or empty.
const DefaultCommand = "orca"

// Result is what one orca process gave back.
type Result struct {
	// Status is the exit status, or -1 when there is none (the program did not
	// start or was killed by a signal).
	Status int
	Stdout string
	Stderr string
}

// Runner runs orca with args (without --json) and returns its result.
type Runner func(args []string) Result

// Command is the orca program for a lookup of the environment: the value of
// ORCA_CLI_COMMAND, or orca when it is unset or empty.
func Command(lookupEnv func(string) (string, bool)) string {
	if v, ok := lookupEnv(CommandEnv); ok && v != "" {
		return v
	}
	return DefaultCommand
}

// ExecRunner runs program as a process: args plus --json, stdin closed (the
// null device), stdout and stderr captured. When the program cannot start,
// Status is -1 and Stderr holds the start error.
func ExecRunner(program string) Runner {
	return func(args []string) Result {
		cmd := exec.Command(program, append(append([]string(nil), args...), "--json")...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		res := Result{Status: -1, Stdout: stdout.String(), Stderr: stderr.String()}
		var exitErr *exec.ExitError
		switch {
		case err == nil:
			res.Status = 0
		case errors.As(err, &exitErr):
			res.Status = exitErr.ExitCode()
			// A process ended by a signal has no exit code (-1): keep the
			// reason ("signal: killed") instead of an empty message.
			if res.Status == -1 && res.Stderr == "" {
				res.Stderr = err.Error()
			}
		default:
			if res.Stderr == "" {
				res.Stderr = err.Error()
			}
		}
		return res
	}
}
