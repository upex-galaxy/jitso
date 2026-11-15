package cli

import (
	"crypto/rand"
	"os"
	"time"

	"github.com/upex-galaxy/jitso/internal/launch"
)

// cmd_launch.go is `jitso launch`. The arguments are parsed and checked first
// (pure, no project needed), then the project and its catalog are resolved
// (exit 3 outside a git checkout), and only then is the brief read.
func init() {
	Register(Command{
		Name:    "launch",
		Order:   10,
		Summary: "launch a worker from a build and a brief file, write its registry event, print its card",
		Run:     runLaunch,
	})
}

func runLaunch(ctx *Context, argv []string) error {
	args, err := launch.Parse(argv)
	if err != nil {
		return err
	}
	if err := args.Check(); err != nil {
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
	store, err := ctx.Registry()
	if err != nil {
		return err
	}
	getwd := ctx.Sys.Getwd
	if getwd == nil {
		getwd = os.Getwd
	}
	cwd, err := getwd()
	if err != nil {
		return err
	}
	d := launch.Deps{
		Root:     root,
		Catalog:  cat,
		Registry: store,
		Orca:     ctx.Orca(),
		Home:     ctx.Home(),
		Cwd:      cwd,
		Now:      ctx.Sys.Now,
		Sleep:    ctx.Sys.Sleep,
		Rand:     ctx.Sys.Rand,
		Out:      ctx.Println,
		Warn:     ctx.Warn,
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Sleep == nil {
		d.Sleep = time.Sleep
	}
	if d.Rand == nil {
		d.Rand = rand.Reader
	}
	return launch.Run(args, d)
}
