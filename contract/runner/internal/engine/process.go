package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// StepTimeout bounds every process a step starts (SPEC 6.1).
const StepTimeout = 60 * time.Second

type procResult struct {
	exit     int
	timedOut bool
	stdout   string
	stderr   string
	elapsed  time.Duration
}

// runProcess starts argv (argv[0] absolute) with exactly env, in dir, with stdin (nil = empty).
// A non-zero exit is a result, not an error; an error means the process could not run at all.
func runProcess(argv []string, dir string, env []string, stdin *string, timeout time.Duration) (procResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	if stdin != nil {
		cmd.Stdin = strings.NewReader(*stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.WaitDelay = 2 * time.Second
	configureKill(cmd)

	start := time.Now()
	err := cmd.Run()
	res := procResult{elapsed: time.Since(start), stdout: stdout.String(), stderr: stderr.String()}
	killGroup(cmd)

	if ctx.Err() == context.DeadlineExceeded {
		res.timedOut, res.exit = true, -1
		return res, nil
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		res.exit = 0
	case errors.As(err, &exitErr):
		res.exit = exitErr.ExitCode()
	case cmd.ProcessState != nil:
		// The process ran and exited; the error is about its pipes (for example ErrWaitDelay).
		res.exit = cmd.ProcessState.ExitCode()
	default:
		return res, fmt.Errorf("could not start %s: %w", argv[0], err)
	}
	return res, nil
}
