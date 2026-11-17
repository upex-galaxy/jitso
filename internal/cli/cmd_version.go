package cli

import (
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/version"
)

// version prints the one-line identity of this binary. `jitso --version` is
// the same command (see route).
func init() {
	Register(Command{
		Name:    "version",
		Order:   90,
		Summary: "print the version of this binary",
		Run: func(ctx *Context, args []string) error {
			if len(args) > 0 {
				return fail.Usage("jitso version: takes no arguments")
			}
			ctx.Println(version.Line())
			return nil
		},
	})
}
