package cli

import (
	"github.com/upex-galaxy/jitso/internal/analyze"
	"github.com/upex-galaxy/jitso/internal/reap"
)

// reap ends the launches whose terminal is gone, flags the workers no launch
// explains and analyzes the ended launches that were never analyzed
// (internal/reap). Its analysis is the `analyze` command run silently: the
// summary line of each is dropped, as the oracle does.
func init() {
	Register(Command{
		Name:    "reap",
		Order:   30,
		Summary: "mark finished launches against Orca and flag workers that have no launch event",
		Run: func(ctx *Context, args []string) error {
			runs, err := reap.ParseArgs(args)
			if err != nil {
				return err
			}
			root, err := ctx.Project()
			if err != nil {
				return err
			}
			store, err := ctx.Registry()
			if err != nil {
				return err
			}
			now := lifecycleNow(ctx)
			analyzeDeps := analyze.Deps{
				PricesPath: pricesPath(root.Primary),
				Store:      store,
				Catalog:    ctx.Catalog,
				Now:        now,
			}
			line, err := reap.Run(runs, reap.Deps{
				Store: store,
				Orca:  ctx.Orca(),
				Now:   now,
				Analyze: func(id string) error {
					_, err := analyze.Run([]string{id}, analyzeDeps)
					return err
				},
				Err: ctx.Warn,
			})
			if err != nil {
				return err
			}
			ctx.Println(line)
			return nil
		},
	})
}
