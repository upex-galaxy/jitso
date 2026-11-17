package cli

import (
	"io"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// Run is the whole CLI: it picks the command for argv, runs it, prints the
// error it returns (if any) on stderr and returns the exit code. It never
// calls os.Exit; cmd/jitso does.
//
//	no argument, --help or -h       usage on stdout, 0
//	--version                       the version command (same output as `version`)
//	a known command                 that command
//	any other word (also "", or a word with spaces)
//	                                "jitso: unknown command" and the usage on stderr, 2
func Run(argv []string, sys System) int {
	ctx := &Context{Sys: sys}
	return report(sys.Stderr, route(ctx, argv))
}

func route(ctx *Context, argv []string) error {
	if len(argv) == 0 || argv[0] == "--help" || argv[0] == "-h" {
		ctx.Println(Usage())
		return nil
	}
	first := argv[0]
	if first == "--version" {
		first = "version"
	}
	cmd, ok := Lookup(first)
	if !ok {
		return fail.Usage(`jitso: unknown command "` + first + "\"\n\n" + Usage())
	}
	return cmd.Run(ctx, argv[1:])
}

func report(stderr io.Writer, err error) int {
	if text := fail.Text(err); text != "" {
		io.WriteString(stderr, text+"\n")
	}
	return fail.Code(err)
}
