// Package orca is a thin client over the orca CLI. Every call goes through a
// Runner, so tests pass a scripted one and never touch a real Orca. The real
// runner appends --json, closes stdin and runs the program named by
// ORCA_CLI_COMMAND (default orca).
//
// The terminal verbs are typed in client.go and the orchestration reads of
// jitso graph in messages.go; the other orchestration verbs go through Call.
package orca

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// CommandEnv names the variable that overrides the orca program.
const CommandEnv = "ORCA_CLI_COMMAND"

// DefaultCommand is the program run when CommandEnv is unset or empty.
const DefaultCommand = "orca"

// StatusTimeout is the Status of a call that ExecRunnerTimeout gave up on;
// Stderr then says after how long.
const StatusTimeout = -2

// CodeTimeout is the Error code of such a call.
const CodeTimeout = "jitso_timeout"

// Result is what one orca process gave back.
type Result struct {
	// Status is the exit status, -1 when there is none (the program did not
	// start or was killed by a signal), or StatusTimeout.
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
		return run(exec.Command(program, withJSON(args)...))
	}
}

// pipeWait is how long ExecRunnerTimeout waits for the output of a program that
// ended or was killed while a child of it still holds the pipes.
const pipeWait = time.Second

// ExecRunnerTimeout is ExecRunner with a limit: a program that has not ended
// after it is killed and the result is StatusTimeout. It is for a caller that
// must stay responsive; the commands that act on Orca use ExecRunner and wait.
func ExecRunnerTimeout(program string, limit time.Duration) Runner {
	return func(args []string) Result {
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		defer cancel()
		cmd := exec.CommandContext(ctx, program, withJSON(args)...)
		cmd.WaitDelay = pipeWait
		res := run(cmd)
		if ctx.Err() != nil && res.Status != 0 {
			return Result{Status: StatusTimeout, Stderr: fmt.Sprintf("did not answer within %g s", limit.Seconds())}
		}
		return res
	}
}

func withJSON(args []string) []string {
	return append(append([]string(nil), args...), "--json")
}

func run(cmd *exec.Cmd) Result {
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

// InstallURL is where Orca is installed from.
const InstallURL = "https://github.com/stablyai/orca"

// Installed checks that the orca program can start, without running it: the
// program of Command(lookupEnv) is looked up with lookPath (exec.LookPath).
// When it is not found the error is one line saying what is missing and what
// to do: the install link for the default program, the variable and its value
// when CommandEnv names the program.
func Installed(lookupEnv func(string) (string, bool), lookPath func(string) (string, error)) error {
	program := Command(lookupEnv)
	if _, err := lookPath(program); err == nil {
		return nil
	}
	if program != DefaultCommand {
		return &Error{Message: CommandEnv + " is set to " + program + ", which is not an executable on PATH. Point it at the orca program or unset it, then run jitso doctor."}
	}
	return &Error{Message: "Orca is not installed (orca is not on PATH). jitso opens and tracks each worker in an Orca terminal: install Orca from " + InstallURL + ", open it once, then run jitso doctor."}
}
