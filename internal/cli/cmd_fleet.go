package cli

import (
	"github.com/upex-galaxy/jitso/internal/fleet"
	"github.com/upex-galaxy/jitso/internal/project"
)

// fleet lists the sessions Orca lists, this project first: the launches of the
// registry joined with those terminals (internal/fleet). It only reads. The
// arguments are checked first (exit 2), then the project. When orca is not on
// PATH the rows still come from the registry alone, with state unknown, and the
// command exits 3 with the usual line after printing them.
func init() {
	Register(Command{
		Name:    "fleet",
		Order:   35,
		Summary: "list the sessions Orca lists, this project first (read only): launch, role, harness, state, handle, parent, repo, title",
		Run: func(ctx *Context, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				ctx.Println(fleet.Usage)
				return nil
			}
			opts, err := fleet.ParseArgs(args)
			if err != nil {
				return err
			}
			root, err := ctx.HomeProject()
			if err != nil {
				return err
			}
			store, err := ctx.Registry()
			if err != nil {
				return err
			}
			deps := fleet.Deps{
				Store:     store,
				Checkouts: func() []string { return project.Checkouts(root) },
				RepoName:  fleet.RepoName,
			}
			missing := ctx.OrcaInstalled()
			if missing == nil {
				deps.Orca = ctx.Orca()
			}
			out, err := fleet.Run(opts, deps)
			if err != nil {
				return err
			}
			ctx.Println(out)
			return missing
		},
	})
}
