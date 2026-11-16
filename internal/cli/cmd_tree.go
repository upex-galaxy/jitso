package cli

import (
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/registry"
	launchtree "github.com/upex-galaxy/jitso/internal/tree"
)

// treeUsage is the reason of every rejection of the arguments of `jitso tree`.
const treeUsage = "usage: jitso tree [<launch_id>]"

// tree prints the launch tree with the rollups of each branch (internal/tree),
// read only: no write and no Orca call. The arguments are checked before the
// project is resolved, so a bad argument is exit 2 anywhere (TS-ODDITIES C1,
// defect N-30).
func init() {
	Register(Command{
		Name:    "tree",
		Order:   40,
		Summary: "show the launch tree with its rollups",
		Run:     runTree,
	})
}

func runTree(ctx *Context, args []string) error {
	root := ""
	switch {
	case len(args) == 0:
	case len(args) == 1 && registry.IsLaunchID(args[0]):
		root = args[0]
	default:
		return fail.Rejected(treeUsage)
	}
	store, err := ctx.Registry()
	if err != nil {
		return err
	}
	launches, err := registry.ReadLaunches(store)
	if err != nil {
		return err
	}
	if root != "" {
		if _, ok := registry.Find(launches, root); !ok {
			return fail.Rejectedf("unknown launch %s", root)
		}
	}
	lines := launchtree.Lines(launchtree.Build(launches, root))
	if len(lines) == 0 {
		ctx.Println("jitso tree: no launches")
		return nil
	}
	for _, l := range lines {
		ctx.Println(l)
	}
	return nil
}
