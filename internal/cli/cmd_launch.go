package cli

import (
	"crypto/rand"
	"os"
	"runtime"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/launch"
)

// launchGOOS is runtime.GOOS; a test sets it to look at another system.
var launchGOOS = runtime.GOOS

// launchWindows is the one line a real launch prints on Windows. docs/decisions.md
// records why: a launch writes run.sh and starts it through /bin/sh in an Orca
// terminal, and nothing proves another way works there.
const launchWindows = "jitso launch does not run on Windows: it starts each worker through /bin/sh in an Orca terminal. Run it on macOS, Linux or WSL; --dry-run, build, doctor, init and the other commands work here."

// cmd_launch.go is `jitso launch`. The arguments are parsed and checked first
// (pure, no project needed), then a real launch checks that orca is on PATH
// (exit 3), then the project and its catalog are resolved (exit 3 outside a
// git checkout), and only then is the brief read. `launch
// --help` or `-h` as the first word prints the usage and needs none of that.
func init() {
	Register(Command{
		Name:    "launch",
		Order:   10,
		Summary: "launch a worker from a build and a brief file, write its registry event, print its card",
		Run:     runLaunch,
	})
}

// launchUsage is what `jitso launch --help` prints (defect N-26).
const launchUsage = `Usage: jitso launch --build <id> --brief <file> --mode direct|fleet [options]

Launches a worker from a build (saved, or a draft) and a brief file: prints its card, opens its terminal and writes its registry line.

  --build <id>                 the build to launch (required)
  --brief <file>               the brief, a file under .jitso/briefs/ of the project (required)
  --mode direct|fleet          direct: the session reports to you; fleet: a worker of an Orca run (required)
  --run <run>                  the Orca run (fleet mode, required there; JITSO creates the Task from the brief)
  --parent <launch_id>         the launch that starts this one
  --name <name>                the session name peers address (default: the build id); a name in use gets a suffix
  --worktree <selector>        the Orca worktree the terminal opens in; path:<folder> of another repository launches into it
  --repo <path>                launch into another repository: an absolute path in its checkout, registered in Orca
  --mcp-exclude <name>         leave one MCP server of the build out (repeatable)
  --field <name>=<value>       a value the build requires (repeatable)
  --brief-language en|es|any   the language the brief check expects (default en)
  --dry-run                    check everything and print the card; open nothing, write nothing`

func runLaunch(ctx *Context, argv []string) error {
	// Only the first word asks for help; anywhere else --help is an unknown
	// argument. Nothing is read, so it works outside a git checkout.
	if len(argv) > 0 && (argv[0] == "--help" || argv[0] == "-h") {
		ctx.Println(launchUsage)
		return nil
	}
	args, err := launch.Parse(argv)
	if err != nil {
		return err
	}
	if err := args.Check(); err != nil {
		return err
	}
	// Only a real launch needs Orca; a dry run into another repository asks it
	// when it is there.
	if !args.DryRun {
		if launchGOOS == "windows" {
			return fail.Env(launchWindows)
		}
		if err := ctx.OrcaInstalled(); err != nil {
			return err
		}
	}
	root, err := ctx.HomeProject()
	if err != nil {
		return err
	}
	cat, err := ctx.Catalog()
	if err != nil {
		return err
	}
	target, err := launch.ResolveTarget(args, root)
	if err != nil {
		return err
	}
	if target.Cross {
		// The builds are checked against what the target holds; an invalid one
		// fails only its own launch.
		res, err := ctx.TargetScan(target.Primary)
		if err != nil {
			return err
		}
		cat = res.Tolerant()
	}
	store, err := ctx.Registry()
	if err != nil {
		return err
	}
	getwd := ctx.Sys.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	cwd, err := getwd()
	if err != nil {
		return err
	}
	d := launch.Deps{
		Root:        root,
		Target:      target,
		Catalog:     cat,
		Registry:    store,
		Orca:        ctx.Orca(),
		OrcaMissing: ctx.OrcaInstalled,
		Home:        ctx.Home(),
		Cwd:         cwd,
		Now:         ctx.Sys.Now,
		Sleep:       ctx.Sys.Sleep,
		Rand:        ctx.Sys.Rand,
		Out:         ctx.Println,
		Warn:        ctx.Warn,
		Executable:  ctx.Sys.Executable,
		LookupEnv:   ctx.Sys.LookupEnv,
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Sleep == nil {
		d.Sleep = time.Sleep
	}
	if d.Rand == nil {
		d.Rand = rand.Reader
	}
	return launch.Run(args, d)
}
