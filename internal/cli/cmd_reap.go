package cli

import "github.com/upex-galaxy/jitso/internal/reap"

// reap ends the launches whose terminal is gone and lists the workers no
// launch explains (internal/reap); the list is output only.
func init() {
	Register(Command{
		Name:    "reap",
		Order:   30,
		Summary: "mark finished launches against Orca and list workers that have no launch event",
		Run: func(ctx *Context, args []string) error {
			runs, err := reap.ParseArgs(args)
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
			out, err := reap.Run(runs, reap.Deps{
				Store: store,
				Orca:  ctx.Orca(),
				Now:   lifecycleNow(ctx),
				Err:   ctx.Warn,
			})
			if err != nil {
				return err
			}
			ctx.Println(out)
			return nil
		},
	})
}
