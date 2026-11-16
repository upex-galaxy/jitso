package cli

import (
	"path/filepath"

	"github.com/upex-galaxy/jitso/internal/analyze"
)

// pricesPath is the project's price table, in its primary checkout.
func pricesPath(primary string) string {
	return filepath.Join(primary, "jitso", "prices.json")
}

// analyze reads the transcript of a launch and appends its analyzed event, or
// prints the numbers of a transcript file and writes nothing (internal/analyze).
// The arguments are checked first, then the project, then the price table.
func init() {
	Register(Command{
		Name:    "analyze",
		Order:   50,
		Summary: "read a launch transcript and write its analyzed event",
		Run: func(ctx *Context, args []string) error {
			req, err := analyze.ParseArgs(args)
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
			out, err := analyze.Run(req, analyze.Deps{
				PricesPath: pricesPath(root.Primary),
				Store:      store,
				Catalog:    ctx.Catalog,
				Now:        lifecycleNow(ctx),
			})
			if err != nil {
				return err
			}
			ctx.Println(out)
			return nil
		},
	})
}
