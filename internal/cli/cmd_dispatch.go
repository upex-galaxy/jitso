package cli

import (
	"os"

	dispatcher "github.com/upex-galaxy/jitso/internal/dispatch"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
)

// cmd_dispatch.go is the dispatcher (milestone 6): a first word with
// whitespace opens one interactive claude for the request (internal/dispatch, imported as dispatcher because run.go has a dispatch function).
// `jitso dispatch` stays an unknown command, as in the oracle.
func init() {
	SetRequestHandler(runRequest)
}

// runRequest checks the request before it touches git, then resolves the
// project (so the dispatched claude can be handed it through JITSO_PROJECT)
// and opens the dispatcher. The environment claude inherits is the process
// environment: System carries lookups only, not the whole list.
func runRequest(ctx *Context, request string) error {
	text, err := dispatcher.CheckRequest(request)
	if err != nil {
		return err
	}
	root, err := ctx.Project()
	if err != nil {
		return err
	}
	cat, err := ctx.Catalog()
	if err != nil {
		return err
	}
	self, err := project.Self(ctx.Sys.Executable)
	if err != nil {
		return fail.Envf("cannot find the path of jitso: %v", err)
	}
	if ctx.Sys.Spawn == nil {
		return fail.Env("the dispatcher cannot run: no way to start claude")
	}
	return dispatcher.Open(dispatcher.Options{
		Request:  text,
		Root:     root.Primary,
		Home:     ctx.Home(),
		Summary:  cat.Summary(),
		Launcher: self,
		Environ:  os.Environ(),
		Spawn:    ctx.Sys.Spawn,
	})
}
