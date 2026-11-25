package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/graph"
	"github.com/upex-galaxy/jitso/internal/project"
)

// graph shows the sessions and who talks to whom (internal/graph): with
// --json one snapshot, else a page served on 127.0.0.1 until Ctrl-C. The
// arguments are checked first (exit 2). It needs no project: outside a git
// checkout, or in one with no registry, it shows what Orca knows. It writes
// nothing. Its Orca calls have a time limit. A missing, failing or late Orca
// leaves the rest of the answer: --json prints it and exits 3, the server shows
// it with a notice.
func init() {
	Register(Command{
		Name:    "graph",
		Order:   37,
		Summary: "serve the live constellation of sessions on 127.0.0.1 until Ctrl-C (--json: print one snapshot)",
		Run: func(ctx *Context, args []string) error {
			if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
				ctx.Println(graph.Usage)
				return nil
			}
			opts, err := graph.ParseArgs(args)
			if err != nil {
				return err
			}
			collector, err := graphCollector(ctx)
			if err != nil {
				return err
			}
			if opts.JSON {
				snapshot, err := collector.Snapshot()
				if err != nil {
					return err
				}
				out, err := snapshot.JSON()
				if err != nil {
					return err
				}
				ctx.Println(out)
				return snapshot.Failure
			}
			stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			return serveGraph(stop, ctx, collector, opts.Port)
		},
	})
}

// graphCollector reads the context's Orca and, inside a git checkout, the
// project's registry. Outside one (the Env error of Context.Project) the
// collector has no project.
func graphCollector(ctx *Context) (*graph.Collector, error) {
	c := &graph.Collector{
		Now: ctx.Sys.Now,
		Orca: func() (graph.Orca, error) {
			if err := ctx.OrcaInstalled(); err != nil {
				return nil, err
			}
			return ctx.OrcaWithin(graph.OrcaTimeout), nil
		},
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	root, err := ctx.Project()
	var failed *fail.Error
	if errors.As(err, &failed) && failed.Kind == fail.KindEnv {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	store, err := ctx.Registry()
	if err != nil {
		return nil, err
	}
	c.Store = store
	c.Registered = func() bool {
		_, err := os.Stat(store.Path())
		// Any other error is for the read to report.
		return !errors.Is(err, fs.ErrNotExist)
	}
	c.Checkouts = func() []string { return project.Checkouts(root) }
	return c, nil
}

// serveGraph serves the page until stop is done. The URL goes to stdout; an
// Orca that is not installed is one warning on stderr and the page is served
// all the same.
func serveGraph(stop context.Context, ctx *Context, collector *graph.Collector, port int) error {
	random := ctx.Sys.Rand
	if random == nil {
		random = rand.Reader
	}
	token, err := graph.NewToken(random)
	if err != nil {
		return err
	}
	ln, err := graph.Listen(port)
	if err != nil {
		return fail.Envf("cannot listen on %s port %d: %v", graph.Host, port, err)
	}
	defer ln.Close()
	listening := ln.Addr().(*net.TCPAddr).Port
	cache := &graph.Cache{TTL: graph.CacheTTL, Now: collector.Now, Load: collector.Snapshot}
	server := graph.Server{
		Port: listening, Token: token, Snapshot: cache.Get,
		Switch: func(handle string) error { return ctx.OrcaWithin(graph.OrcaTimeout).SwitchTerminal(handle) },
	}
	ctx.Println("jitso graph: http://" + graph.Host + ":" + strconv.Itoa(listening) + "/ (open it in a browser; Ctrl-C stops)")
	if err := ctx.OrcaInstalled(); err != nil {
		ctx.Warn(fail.EnvPrefix + err.Error())
	}
	return graph.Serve(stop, ln, server.Handler())
}
