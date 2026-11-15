// Command jitso launches agent sessions from approved builds, logs every
// launch and reads the log back. This file only wires the real ports and
// exits with the code the command layer returns.
package main

import (
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/upex-galaxy/jitso/internal/cli"
	"github.com/upex-galaxy/jitso/internal/orca"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.System{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		LookupEnv:  os.LookupEnv,
		Getwd:      os.Getwd,
		Executable: os.Executable,
		Now:        time.Now,
		Sleep:      time.Sleep,
		Rand:       rand.Reader,
		Orca:       orca.ExecRunner(orca.Command(os.LookupEnv)),
		Spawn:      spawn,
	}))
}

// spawn runs argv in dir with the terminal inherited and returns its exit
// status. An error means the program could not start. A nil env inherits the
// process environment.
func spawn(argv []string, dir string, env []string) (int, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return 0, err
	}
	return 0, nil
}
