package cli

import (
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/hook"
)

// cmd_hook.go registers `jitso hook guard` (milestone 7): the Claude Code
// hook as a subcommand of the one binary. It reads the hook JSON on stdin and
// answers on stdout (SPEC 6.2) and always exits 0; the decisions live in
// internal/hook.
func init() {
	Register(Command{
		Name:    "hook",
		Order:   70,
		Summary: "run a Claude Code hook (guard): hook JSON on stdin, decision on stdout",
		Run:     runHook,
	})
}

func runHook(ctx *Context, args []string) error {
	if len(args) != 1 {
		return fail.Usage("jitso hook: expected guard")
	}
	deps := hook.Deps{
		Stdin:      ctx.Sys.Stdin,
		Stdout:     ctx.Sys.Stdout,
		Getenv:     ctx.Getenv,
		Getwd:      ctx.Sys.Getwd,
		Executable: ctx.Sys.Executable,
		Now:        ctx.Sys.Now,
		Sleep:      ctx.Sys.Sleep,
	}
	switch args[0] {
	case "guard":
		hook.RunGuard(deps)
	default:
		return fail.Usage("jitso hook: unknown hook \"" + args[0] + "\", expected guard")
	}
	return nil
}
