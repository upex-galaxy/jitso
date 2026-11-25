// Command jitso launches agent sessions from approved builds, logs every
// launch and reads the log back. This file only wires the real ports and
// exits with the code the command layer returns.
package main

import (
	"crypto/rand"
	"os"
	"time"

	"github.com/upex-galaxy/jitso/internal/cli"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/tty"
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
		OrcaWithin: func(limit time.Duration) orca.Runner {
			return orca.ExecRunnerTimeout(orca.Command(os.LookupEnv), limit)
		},
		StdinIsTerminal: func() bool { return tty.IsTerminal(os.Stdin) },
	}))
}
