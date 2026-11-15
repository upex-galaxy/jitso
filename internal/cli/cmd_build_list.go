package cli

// build list prints one line per launchable build of the project's layered
// catalog (approved and not subagent tier), in layer order (the project, the
// trusted parents nearest first, the user folder) and by file name inside a
// layer. A build of a parent or user folder ends with ` (from <layer> <path>)`.
// A parent or user file that is invalid for this project is skipped with one
// warning line on stderr. A catalog that fails to load is exit 3 with the
// file named, the same handled error launch gives (TS-ODDITIES C3: the oracle
// crashes here).
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "list",
		Order: 10,
		Usage: "list                           the launchable builds of this project",
		Run: func(ctx *Context, args []string) error {
			cat, err := ctx.Catalog()
			if err != nil {
				return err
			}
			for _, s := range cat.Skipped {
				ctx.Warn(s.Warning())
			}
			ctx.Println(cat.List())
			return nil
		},
	})
}
