package launch

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/upex-galaxy/jitso/internal/brief"
	"github.com/upex-galaxy/jitso/internal/card"
	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/harness"
	"github.com/upex-galaxy/jitso/internal/harness/claude"
	"github.com/upex-galaxy/jitso/internal/jsconv"
	"github.com/upex-galaxy/jitso/internal/orca"
	"github.com/upex-galaxy/jitso/internal/profile"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/registry"
	"github.com/upex-galaxy/jitso/internal/trust"
)

// Orca is the part of the Orca client a launch uses.
type Orca interface {
	CreateTerminal(worktree, title, command string) (orca.Terminal, error)
	WaitIdle(handle string, timeoutMS int) bool
	ReadScreen(handle string) (string, error)
	SendText(handle, text string) error
	CloseTerminal(handle string) error
	Call(args []string) (map[string]any, error)
}

// Adapters are the harnesses a build's harness field may name.
var Adapters = map[string]harness.Adapter{claude.Adapter{}.ID(): claude.Adapter{}}

// Deps is everything a launch reads, writes or calls.
type Deps struct {
	// Root is the project: the catalog, .mcp.json, the hard rules and
	// .jitso/ come from its primary checkout.
	Root     project.Root
	Catalog  *catalog.Catalog
	Registry registry.Store
	Orca     Orca
	// Home is $HOME: the dispatcher folder and the transcripts live under it.
	Home string
	// Cwd is the working directory a relative --brief is read from.
	Cwd   string
	Now   func() time.Time
	Sleep func(time.Duration)
	Rand  io.Reader
	// Out prints one line on stdout, Warn one line on stderr.
	Out  func(string)
	Warn func(string)
	// TrustExtra are extra trusted folders (tests only; no flag sets it).
	TrustExtra []string
}

// Run launches a, already parsed and checked (Parse, Args.Check). After the
// build, the brief and the parent are validated it prints the card; a dry run
// stops there. A real launch then executes (see execute).
func Run(a Args, d Deps) error {
	p, err := profile.Resolve(profile.Request{Build: a.Build, Fields: a.Fields}, d.Catalog)
	if err != nil {
		return err
	}
	harnessName := jsString(p.Build, "harness")
	adapter, ok := Adapters[harnessName]
	if !ok {
		return fail.Rejectedf(`no adapter for harness "%s"`, harnessName)
	}
	mcp := MCPAfterExclude(p.MCP, a.MCPExclude)

	briefPath, err := brief.ResolvePath(a.Brief, d.Cwd, AllowedBriefDirs(project.DispatchDir(d.Home), d.Root.StateDir()))
	if err != nil {
		return err
	}
	data, err := os.ReadFile(briefPath)
	if err != nil {
		return err
	}
	briefText := string(data)
	if failures := brief.Check(briefText, a.BriefLanguage, brief.Exists(d.Root.Primary)); len(failures) > 0 {
		return fail.Rejected(brief.Reason(failures))
	}

	lineage, err := lineageOf(a.Parent, d.Registry)
	if err != nil {
		return err
	}

	var random [2]byte
	if _, err := io.ReadFull(d.Rand, random[:]); err != nil {
		return err
	}
	id := ID(d.Now(), random)
	if !p.Build.Budget.Set {
		return fail.Envf("build %s has no budget mapping", p.Build.ID)
	}
	pkg, _ := os.ReadFile(filepath.Join(d.Root.Primary, "package.json"))
	cardText := card.Render(Card(id, a.Mode, p, mcp, card.LaunchCommand(pkg)))
	if a.DryRun {
		d.Out(cardText)
		d.Out(DryRunLine)
		return nil
	}
	return execute(prepared{args: a, profile: p, adapter: adapter, mcp: mcp, briefText: briefText, id: id, card: cardText, lineage: lineage}, d)
}

// lineageOf finds the parent launch: its root and its depth plus one. An
// unknown parent is a rejection, also on a dry run (TS-ODDITIES L5).
func lineageOf(parent *string, store registry.Store) (Lineage, error) {
	if parent == nil {
		return Lineage{}, nil
	}
	launches, err := registry.ReadLaunches(store)
	if err != nil {
		return Lineage{}, err
	}
	l, ok := registry.Find(launches, *parent)
	if !ok {
		return Lineage{}, fail.Rejected("unknown parent launch " + *parent)
	}
	root, _ := l.Launch.String("root")
	depth, _ := l.Launch.Number("depth")
	return Lineage{Parent: parent, Root: root, Depth: int(depth) + 1}, nil
}

// prepared is a validated launch, ready to execute.
type prepared struct {
	args      Args
	profile   profile.Profile
	adapter   harness.Adapter
	mcp       []string
	briefText string
	id        string
	card      string
	lineage   Lineage
}

// execute writes the launch folder and starts the session. It creates
// .jitso/launches/<id>/ with os.Mkdir, so the folder is new, and removes
// exactly that folder when anything fails before the registry line is
// appended (TS-ODDITIES L2): a file, the terminal, the idle wait, a refused
// trust dialog, worker-start, the registry lock. A terminal it opened is
// closed first. Once the line is written nothing is removed.
func execute(p prepared, d Deps) (err error) {
	launches := filepath.Join(d.Root.StateDir(), "launches")
	if err := os.MkdirAll(launches, 0o755); err != nil {
		return err
	}
	dir := filepath.Join(launches, p.id)
	if err := os.Mkdir(dir, 0o755); err != nil {
		return err
	}
	registered := false
	defer func() {
		if !registered {
			os.RemoveAll(dir)
		}
	}()

	files, err := writeFolder(p, d, dir)
	if err != nil {
		return err
	}
	term, err := d.Orca.CreateTerminal(deref(p.args.Worktree), files.title, files.command)
	if err != nil {
		return err
	}
	abandon := func() { _ = d.Orca.CloseTerminal(term.Handle) }

	if !d.Orca.WaitIdle(term.Handle, IdleTimeoutMS) {
		abandon()
		return fail.Envf("terminal %s did not reach %s in %d s; it was closed", term.Handle, p.adapter.Readiness(), IdleTimeoutMS/1000)
	}
	outcome, err := answerTrust(p.adapter, d, term)
	if err != nil {
		abandon()
		return err
	}
	if outcome == trust.Refused {
		abandon()
		folder := term.WorktreePath
		if folder == "" {
			folder = "(unknown)"
		}
		return fail.Rejectedf("untrusted folder %s: the launcher answers the trust dialog only for a worktree of this repo or the dispatcher directory; the terminal was closed", folder)
	}
	if p.args.Mode == ModeFleet {
		start := []string{"orchestration", "worker-start", "--run", deref(p.args.Run), "--task", deref(p.args.Task), "--terminal", term.Handle}
		if p.args.Worktree != nil {
			start = append(start, "--worktree", *p.args.Worktree)
		}
		if _, err := d.Orca.Call(start); err != nil {
			abandon()
			return err
		}
	}

	transcript := ""
	if term.WorktreePath != "" {
		transcript = p.adapter.TranscriptPath(term.WorktreePath, files.sessionID, d.Home)
	}
	event := Event(EventInput{
		ID: p.id, Args: p.args, Profile: p.profile, MCP: p.mcp, Lineage: p.lineage,
		Repo: filepath.Base(d.Root.Primary), Harness: p.adapter.ID(), Argv: files.argv, Prompted: p.args.Mode == ModeDirect,
		Rules: files.rules, BriefPath: files.brief, BriefText: p.briefText, SessionID: files.sessionID,
		Cwd: term.WorktreePath, Transcript: transcript, Trust: outcome, Terminal: term.Handle, Title: files.title, Now: d.Now(),
	})
	if err := d.Registry.Append(event); err != nil {
		abandon()
		return err
	}
	registered = true

	d.Out(p.card)
	if outcome == trust.Timeout {
		d.Warn("jitso: the trust dialog of terminal " + term.Handle + " did not clear in 10 s; the launch is registered")
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// jsString is the String() of a build key as the oracle prints it
// ("undefined" when the key is missing).
func jsString(b *catalog.Build, key string) string {
	return jsconv.String(b.Raw.Value(key))
}
