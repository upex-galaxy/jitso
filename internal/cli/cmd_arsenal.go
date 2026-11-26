package cli

import (
	"os"
	"strings"

	"github.com/upex-galaxy/jitso/internal/arsenal"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/launch"
	"github.com/upex-galaxy/jitso/internal/project"
)

// arsenal prints what a build may name in this project, one line per item:
// the models, efforts, tools, skills (the project's, JITSO's own and the
// user's user:<name>), MCP servers, approved subagent-tier builds, route ids
// and field patterns. An allowlist that restricts is printed as it is; an open
// one gets a line that says so, then what exists. --all adds what this machine
// holds that a restricting allowlist does not name. It reads only and prints
// names and key names, never a value. A project without jitso/jitso.yaml has
// every list open. --repo lists what a build launched into that repository may
// name.
func init() {
	Register(Command{
		Name:    "arsenal",
		Order:   65,
		Summary: "list what a build may name here (--all: also what no allowlist names yet)",
		Run:     runArsenal,
	})
}

const arsenalUsage = `Usage: jitso arsenal [--all] [--repo <path>]

Lists what a build may name in this project: models, efforts, tools, skills, MCP servers, subagents, route ids and fields.

  --all           also list the skills and MCP servers of this project and of this machine that no allowlist names yet
  --repo <path>   list what a build launched into that repository may name: its skills, MCP servers, tools and route ids`

func runArsenal(ctx *Context, args []string) error {
	all := false
	var repo *string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case i == 0 && (a == "--help" || a == "-h"):
			ctx.Println(arsenalUsage)
			return nil
		case a == "--all":
			all = true
		case a == "--repo":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return fail.Rejected("--repo needs a value")
			}
			i++
			repo = &args[i]
		default:
			return fail.Rejectedf(`unknown argument "%s"`, a)
		}
	}
	root, err := ctx.HomeProject()
	if err != nil {
		return err
	}
	var target *arsenal.Target
	if repo != nil {
		r, err := launch.ResolveRepo(*repo)
		if err != nil {
			return err
		}
		if !project.SameDir(r.Primary, root.Primary) {
			target = &arsenal.Target{FS: os.DirFS(r.Primary), Path: r.Primary}
		}
	}
	inv, err := arsenal.Collect(os.DirFS(root.Primary), root.Primary, ctx.Home(), target)
	if err != nil {
		return err
	}
	for _, line := range inv.Lines(all) {
		ctx.Println(line)
	}
	return nil
}
