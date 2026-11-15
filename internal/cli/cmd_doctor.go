package cli

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/upex-galaxy/jitso/internal/doctor"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/version"
)

const doctorUsage = `Usage: jitso doctor

Checks this machine and the project of the cwd for JITSO, and changes nothing.
One line per check: ok, warn or fail. Exit 0 when nothing fails, 3 when a check fails.`

// doctor is the read-only health check (defect N-20). It prints one line per
// check, `<status> <check>: <detail>`, then `doctor: <n> ok, <n> warn, <n>
// fail`. It writes nothing and runs only git and `claude --version`; orca is
// looked up on PATH, never run.
//
//	doctor            exit 0 when no check fails (warnings allowed), 3 when one does
//	doctor --help     the usage on stdout, exit 0
//	doctor <x>        `jitso doctor: takes no arguments`, exit 2
func init() {
	Register(Command{
		Name:    "doctor",
		Order:   84,
		Summary: "check this machine and project for JITSO, changing nothing",
		Run:     runDoctor,
	})
}

func runDoctor(ctx *Context, args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		ctx.Println(doctorUsage)
		return nil
	}
	if len(args) > 0 {
		return fail.Usage("jitso doctor: takes no arguments")
	}

	ports := doctor.Ports{
		GOOS:       runtime.GOOS,
		Version:    version.Current(),
		Home:       ctx.Home(),
		Getenv:     ctx.Getenv,
		Executable: ctx.Sys.Executable,
		LookPath:   exec.LookPath,
		Run:        doctor.ExecRunner,
		Now:        ctx.Sys.Now,
		Resolve:    ctx.Project,
	}
	if ctx.Sys.Executable == nil {
		ports.Executable = os.Executable
	}
	lines := doctor.Run(ports)
	for _, l := range lines {
		ctx.Println(l.String())
	}
	ctx.Println(doctor.Summary(lines))
	if _, _, failed := doctor.Counts(lines); failed > 0 {
		return fail.Status(fail.CodeEnv)
	}
	return nil
}
