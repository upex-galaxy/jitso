package cli

import (
	"time"

	"github.com/upex-galaxy/jitso/internal/closeout"
)

// lifecycleNow is the clock of close, reap and analyze: the context's, or the
// real one when a test left it unset.
func lifecycleNow(ctx *Context) func() time.Time {
	if ctx.Sys.Now != nil {
		return ctx.Sys.Now
	}
	return time.Now
}

// close saves the footer of a launch's terminal, ends the launch in the
// registry and closes that one terminal (internal/closeout). The arguments are
// checked before the project is resolved, so a bad id is exit 2 anywhere.
func init() {
	Register(Command{
		Name:    "close",
		Order:   20,
		Summary: "close a launch: save the screen footer, write the end event, close its terminal",
		Run: func(ctx *Context, args []string) error {
			id, err := closeout.ParseArgs(args)
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
			line, err := closeout.Run(id, closeout.Deps{
				StateDir: root.StateDir(),
				Store:    store,
				Orca:     ctx.Orca(),
				Now:      lifecycleNow(ctx),
			})
			if err != nil {
				return err
			}
			ctx.Println(line)
			return nil
		},
	})
}
