package cli

import (
	"crypto/rand"
	"os"
	"time"

	"github.com/upex-galaxy/jitso/internal/launch"
)

// cmd_launch.go is `jitso launch`. The arguments are parsed and checked first
// (pure, no project needed), then the project and its catalog are resolved
// (exit 3 outside a git checkout), and only then is the brief read. `launch
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

Launches a worker from an approved build and a brief file: prints its card, opens its terminal and writes its registry line.

  --build <id>                 the build to launch (required)
  --brief <file>               the brief, a file under .jitso/briefs/ of the project (required)
  --mode direct|fleet          direct: the session reports to you; fleet: a worker of an Orca run (required)
  --run <run>                  the Orca run (fleet mode, required there; JITSO creates the Task from the brief)
  --parent <launch_id>         the launch that starts this one
  --worktree <selector>        the Orca worktree the terminal opens in
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
	root, err := ctx.Project()
	if err != nil {
		return err
	}
	cat, err := ctx.Catalog()
	if err != nil {
		return err
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
		Root:       root,
		Catalog:    cat,
		Registry:   store,
		Orca:       ctx.Orca(),
		Home:       ctx.Home(),
		Cwd:        cwd,
		Now:        ctx.Sys.Now,
		Sleep:      ctx.Sys.Sleep,
		Rand:       ctx.Sys.Rand,
		Out:        ctx.Println,
		Warn:       ctx.Warn,
		Executable: ctx.Sys.Executable,
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
