package cli

// presets list prints one line per launchable build of the project's
// catalog (approved and not subagent tier), in file name order. Any other
// `presets ...` answers not implemented and exits 0, as the oracle does. A
// catalog that fails to load is exit 3 with the file named, the same handled
// error launch gives (TS-ODDITIES C3: the oracle crashes here).
func init() {
	Register(Command{
		Name:    "presets",
		Order:   60,
		Summary: "list the approved presets",
		Run: func(ctx *Context, args []string) error {
			if len(args) == 0 || args[0] != "list" {
				ctx.Println("jitso presets: not implemented yet")
				return nil
			}
			cat, err := ctx.Catalog()
			if err != nil {
				return err
			}
			ctx.Println(cat.Summary())
			return nil
		},
	})
}
