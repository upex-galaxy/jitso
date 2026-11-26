package cli

import (
	"os"

	"github.com/upex-galaxy/jitso/internal/arsenal"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// arsenal prints what a build may name in this project, one line per item:
// the models, efforts, tools, skills (the project's, JITSO's own and the
// user's user:<name>), MCP servers, approved subagent-tier builds, route ids
// and field patterns. An allowlist that restricts is printed as it is; an open
// one gets a line that says so, then what exists. --all adds what this machine
// holds that a restricting allowlist does not name. It reads only and prints
// names and key names, never a value. A project without jitso/jitso.yaml has
// every list open.
func init() {
	Register(Command{
		Name:    "arsenal",
		Order:   65,
		Summary: "list what a build may name here (--all: also what no allowlist names yet)",
		Run:     runArsenal,
	})
}

const arsenalUsage = `Usage: jitso arsenal [--all]

Lists what a build may name in this project: models, efforts, tools, skills, MCP servers, subagents, route ids and fields.

  --all   also list the skills and MCP servers of this project and of this machine that no allowlist names yet`

func runArsenal(ctx *Context, args []string) error {
	all := false
	for i, a := range args {
		switch {
		case i == 0 && (a == "--help" || a == "-h"):
			ctx.Println(arsenalUsage)
			return nil
		case a == "--all":
			all = true
		default:
			return fail.Rejectedf(`unknown argument "%s"`, a)
		}
	}
	root, err := ctx.Project()
	if err != nil {
		return err
	}
	inv, err := arsenal.Collect(os.DirFS(root.Primary), root.Primary, ctx.Home())
	if err != nil {
		return err
	}
	for _, line := range inv.Lines(all) {
		ctx.Println(line)
	}
	return nil
}
