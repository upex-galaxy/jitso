package cli

import "github.com/upex-galaxy/jitso/internal/fleet"

// fleet lists the sessions of the project: the launches of the registry joined
// with the terminals Orca lists (internal/fleet). It only reads. The arguments
// are checked first (exit 2), then the project. When orca is not on PATH the
// rows still come from the registry alone, with state unknown, and the command
// exits 3 with the usual line after printing them.
func init() {
	Register(Command{
		Name:    "fleet",
		Order:   35,
		Summary: "list the live sessions of this project (read only): launch, role, harness, title, handle, parent, state",
		Run: func(ctx *Context, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				ctx.Println(fleet.Usage)
				return nil
			}
			opts, err := fleet.ParseArgs(args)
			if err != nil {
				return err
			}
			if _, err := ctx.Project(); err != nil {
				return err
			}
			store, err := ctx.Registry()
			if err != nil {
				return err
			}
			deps := fleet.Deps{Store: store}
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
