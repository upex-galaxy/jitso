package cli

// tree is not implemented yet: it answers on stdout and exits 0, with or
// without a root argument, as the oracle does (TS-ODDITIES C1, keep for now).
func init() {
	Register(Command{
		Name:    "tree",
		Order:   40,
		Summary: "show the launch tree with its rollups",
		Run: func(ctx *Context, _ []string) error {
			ctx.Println("jitso tree: not implemented yet")
			return nil
		},
	})
}
