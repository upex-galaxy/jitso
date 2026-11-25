package cli

import "github.com/upex-galaxy/jitso/internal/link"

// link records that one session talked to another (internal/link). The
// arguments are checked first (exit 2), then the project. Orca is asked only
// when a reference needs it.
func init() {
	Register(Command{
		Name:    "link",
		Order:   36,
		Summary: "record that one session talked to another: <from> <to> (launch id, terminal handle or session title)",
		Run: func(ctx *Context, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				ctx.Println(link.Usage)
				return nil
			}
			from, to, err := link.ParseArgs(args)
			if err != nil {
				return err
			}
			store, err := ctx.Registry()
			if err != nil {
				return err
			}
			out, err := link.Run(from, to, link.Deps{
				Store: store,
				Now:   store.Now,
				Orca: func() (link.Orca, error) {
					if err := ctx.OrcaInstalled(); err != nil {
						return nil, err
					}
					return ctx.Orca(), nil
				},
			})
			if err != nil {
				return err
			}
			ctx.Println(out)
			return nil
		},
	})
}
