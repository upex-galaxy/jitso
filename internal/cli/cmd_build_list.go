package cli

// build list prints one line per launchable build of the project's catalog
// (approved and not subagent tier), in file name order. A catalog that fails
// to load is exit 3 with the file named, the same handled error launch gives
// (TS-ODDITIES C3: the oracle crashes here).
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
			ctx.Println(cat.Summary())
			return nil
		},
	})
}
