package cli

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/adopt"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/guardfiles"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/settingsjson"
	"github.com/upex-galaxy/jitso/internal/shell"
	"github.com/upex-galaxy/jitso/internal/version"
)

// setup hook registers `jitso hook guard` for the chosen harnesses, in the
// checkout of the working folder or, with --scope user, in the home folder
// for the owner's own sessions (defect N-17):
//
//	claude    .claude/settings.json       (user: ~/.claude/settings.json)
//	codex     .codex/hooks.json           (user: $CODEX_HOME/hooks.json, default ~/.codex)
//	opencode  .opencode/plugins/jitso-guard.js  (user: ~/.config/opencode/plugins/jitso-guard.js)
//	pi        .pi/extensions/jitso-guard.js     (user: ~/.pi/agent/extensions/jitso-guard.js)
//
// Claude Code and Codex read the same hooks object: JITSO's groups on
// PreToolUse for Bash and for the orca MCP tools, and on SessionEnd. The file
// is edited through an order-preserving JSON tree (internal/settingsjson),
// never touching a group the user wrote, and its preview prints no line of
// the user's (a token in the file is never printed). OpenCode and Pi get a
// generated JavaScript file (internal/guardfiles) that runs the same command;
// one with jitso's marker line is upgraded, one without it is refused unless
// --force, which backs it up first. All the chosen harnesses make one plan:
// one preview, one confirmation, one backup (internal/adopt). `--remove`
// takes out exactly what jitso added and the folders it leaves empty.
//
// The project files belong to the checkout the command runs in (a linked
// worktree has its own); backups and state go to the primary's .jitso/. The
// user files need no project and their backups go to ~/.jitso/. It is safe
// outside JITSO projects: the guard prints nothing without jitso/jitso.yaml.
// The file .claude/settings.local.json is never opened.
//
// Exit codes: 0 done, nothing to do or dry run; 2 a bad argument, no --yes
// without a terminal, a declined prompt, a guard file jitso did not generate
// without --force; 3 no git checkout, a settings file jitso will not edit
// (not JSON, wrong shape, a layout it cannot reproduce), a CODEX_HOME outside
// HOME at user scope, or a write failure.
func init() {
	registerSetupSub(setupSub{
		Name:      "hook",
		Order:     10,
		UsageLine: "[--scope project|user] [--harness <list>] [--absolute] [--remove] [--force] [--dry-run] [--yes]",
		Help: "setup hook registers `jitso hook guard` for Claude Code (.claude/settings.json),\n" +
			"Codex (.codex/hooks.json), OpenCode (.opencode/plugins/jitso-guard.js) and Pi\n" +
			"(.pi/extensions/jitso-guard.js), or in your home folder with --scope user:\n" +
			"on PreToolUse for Bash and the orca MCP tools, and on SessionEnd.\n" +
			"\n" +
			"  --scope     project (default): this checkout; user: your own sessions, in every folder\n" +
			"  --harness   claude, codex, opencode, pi as a comma list, or all (default: each one on PATH; claude when none is)\n" +
			"  --absolute  write this binary's absolute path instead of the word jitso\n" +
			"  --remove    remove the hook entries jitso added\n" +
			"  --force     replace or remove a guard file jitso did not generate (a backup is kept)\n" +
			"  --dry-run   show the changes, write nothing\n" +
			"  --yes       apply without asking (needed when stdin is not a terminal)",
		Run: runSetupHook,
	})
}

// The harnesses setup hook guards, in plan order.
const (
	harnessClaude   = "claude"
	harnessCodex    = "codex"
	harnessOpenCode = "opencode"
	harnessPi       = "pi"
)

// hookHarnesses are the names --harness takes, in plan order.
var hookHarnesses = []string{harnessClaude, harnessCodex, harnessOpenCode, harnessPi}

// setupSettingsRel is the Claude Code settings file, relative to the checkout
// (project scope) or to HOME (user scope).
const setupSettingsRel = ".claude/settings.json"

// setupCodexNote is printed after a write of the project's .codex/hooks.json.
const setupCodexNote = "note: Codex runs project hooks only in a trusted project, after you approve them in Codex (/hooks)"

// setupLookPath, setupFindHarness and setupGitTracked are the questions about
// the machine and the repository that tests answer on their own:
// setupLookPath looks the jitso word up for the PATH warning, and
// setupFindHarness a harness program for the default --harness list.
var (
	setupLookPath    = exec.LookPath
	setupFindHarness = exec.LookPath
	setupGitTracked  = func(checkout, rel string) bool {
		cmd := exec.Command("git", "-C", checkout, "ls-files", "--error-unmatch", rel)
		return cmd.Run() == nil
	}
)

func runSetupHook(ctx *Context, args []string) error {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			ctx.Println(setupUsage())
			return nil
		}
	}
	flags, rest, err := ParseFlags(args, FlagScope|FlagHarness|FlagRemove|FlagForce|FlagAbsolute|FlagDryRun|FlagYes)
	if err != nil {
		return err
	}
	if err := RejectExtra(rest); err != nil {
		return err
	}

	hs, err := HookPlan(ctx, flags.Scope, flags.Harnesses, flags.Absolute, flags.Remove, flags.Force)
	if err != nil {
		return err
	}
	if hs.Refusal != "" {
		adopt.Preview(ctx.Sys.Stdout, hs.Plan)
		return fail.Rejected(hs.Refusal)
	}
	opts := adopt.Options{
		Command:    "setup hook",
		Scope:      hs.Scope,
		Version:    setupVersionWord(),
		Root:       hs.Root,
		BackupsDir: hs.BackupsDir,
		DryRun:     flags.DryRun,
		Yes:        flags.Yes,
		Display:    hs.Display,
	}
	ports := adopt.Ports{
		Stdin:    ctx.Sys.Stdin,
		Stdout:   ctx.Sys.Stdout,
		Terminal: ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal(),
		Now:      ctx.Sys.Now,
		Rand:     ctx.Sys.Rand,
	}
	if err := adopt.Run(opts, hs.Plan, ports); err != nil {
		return err
	}
	// Run returns nil for nothing to do, a dry run and an applied plan. Only
	// the last one has written the bare word that needs a PATH.
	if hs.BareCommand && !hs.Plan.Empty() && !flags.DryRun {
		if _, err := setupLookPath("jitso"); err != nil {
			ctx.Warn("jitso: warning: jitso is not on PATH, so the hook cannot start until it is (the guard then lets every call through)")
		}
	}
	return nil
}

// HookSetup is the change `jitso setup hook` would make, everything a caller
// needs to preview, confirm and apply it with adopt.Run (the setup hook
// command, and `jitso init` when it registers the guard).
type HookSetup struct {
	// Plan is the one plan for every chosen harness, in claude, codex,
	// opencode, pi order. Its Why says what an empty plan found and its
	// Notes are printed after a write (the commit note, the Codex trust note).
	Plan adopt.Plan
	// Scope is "project" or "user", for the backup manifest.
	Scope string
	// Root is the scope root every plan path lies under: the checkout for
	// project scope, HOME for user scope.
	Root string
	// BackupsDir is where adopt keeps the backups: the primary's
	// .jitso/backups, or ~/.jitso/backups.
	BackupsDir string
	// Display prints an absolute path as the plan does: relative to the
	// checkout, or ~/... for user scope.
	Display func(string) string
	// Refusal is set when a guard file jitso did not generate stops the run
	// (no --force): the caller shows Plan, which is what --force would do,
	// and fails with this text as a rejection (exit 2); nothing is written.
	Refusal string
	// BareCommand is true when the plan writes the bare word jitso, which
	// starts only when jitso is on PATH.
	BareCommand bool
}

// HookPlan builds the plan of `jitso setup hook`. scope is "project" (or "")
// or "user". harnesses are names of claude, codex, opencode and pi (or all),
// planned in that order whatever their order here; nil or empty takes the
// default: each harness whose program is on PATH, or claude
// when none is. absolute writes the quoted real path of this binary instead
// of the word jitso, remove plans the undo, and force lets a guard file jitso
// did not generate be backed up and replaced or removed.
//
// The error is a package fail error: exit 2 for an unknown harness name, exit
// 3 outside a git checkout (project
// scope), without HOME (user scope), for a settings file jitso will not edit,
// a CODEX_HOME outside HOME, or a file it cannot read.
func HookPlan(ctx *Context, scope string, harnesses []string, absolute, remove, force bool) (HookSetup, error) {
	if scope == "" {
		scope = "project"
	}
	hs := HookSetup{Scope: scope}
	if len(harnesses) == 0 {
		harnesses = defaultHarnesses()
	}
	harnesses, ok := parseHarnesses(strings.Join(harnesses, ","))
	if !ok {
		return hs, fail.Rejected("--harness must be a comma list of claude, codex, opencode and pi, or all")
	}
	user := scope == "user"

	var stateDir string
	if user {
		home := ctx.Home()
		if home == "" {
			return hs, fail.Env("HOME is not set")
		}
		hs.Root, stateDir = home, project.UserDir(home)
		hs.Display = func(p string) string {
			if rel, ok := setupRel(home, p); ok {
				return "~/" + rel
			}
			return p
		}
	} else {
		r, err := ctx.Project()
		if err != nil {
			return hs, err
		}
		root := r.Checkout
		hs.Root, stateDir = root, r.StateDir()
		hs.Display = func(p string) string {
			if rel, ok := setupRel(root, p); ok {
				return rel
			}
			return p
		}
	}
	hs.BackupsDir = filepath.Join(stateDir, "backups")

	command, program := settingsjson.Command, "jitso"
	if absolute && !remove {
		self, err := project.Self(ctx.Sys.Executable)
		if err != nil {
			return hs, fail.Envf("cannot find this binary's own path: %v", err)
		}
		quoted, err := shell.Quote(self)
		if err != nil {
			return hs, fail.Envf("cannot write this binary's path into a command: %v", err)
		}
		command, program = quoted+" hook guard", self
	}
	hs.BareCommand = !remove && command == settingsjson.Command

	b := hookBuilder{hs: &hs, user: user, remove: remove, force: force, command: command}
	for _, h := range harnesses {
		f, err := guardFile(ctx, h, hs.Root, user)
		if err != nil {
			return hs, err
		}
		b.displays = append(b.displays, hs.Display(f.path))
		switch h {
		case harnessClaude, harnessCodex:
			err = b.settings(f)
		case harnessOpenCode:
			err = b.generated(f, guardfiles.OpenCode(program, version.Current(), ""))
		case harnessPi:
			err = b.generated(f, guardfiles.Pi(program, version.Current(), ""))
		}
		if err != nil {
			return hs, err
		}
	}
	b.finish()
	return hs, nil
}

// defaultHarnesses is every harness whose program is on PATH, or claude when
// none is.
func defaultHarnesses() []string {
	var found []string
	for _, h := range hookHarnesses {
		if _, err := setupFindHarness(h); err == nil {
			found = append(found, h)
		}
	}
	if len(found) == 0 {
		return []string{harnessClaude}
	}
	return found
}

// hookFile is the guard file of one harness.
type hookFile struct {
	harness string
	// path is the absolute path of the file.
	path string
	// rel is the path relative to the checkout (project scope), for the
	// commit note; "" at user scope.
	rel string
	// top is the outermost folder --remove deletes when the removal leaves it
	// empty (.claude, .codex, .opencode, .pi, or their home counterparts);
	// the folders between it and the file go too when empty.
	top string
}

// guardFile is where harness h keeps its guard at the given scope.
func guardFile(ctx *Context, h, root string, user bool) (hookFile, error) {
	var rel, top string
	switch {
	case h == harnessClaude:
		rel, top = setupSettingsRel, ".claude"
	case h == harnessCodex && !user:
		rel, top = ".codex/hooks.json", ".codex"
	case h == harnessCodex:
		dir := filepath.Join(root, ".codex")
		if v := ctx.Getenv("CODEX_HOME"); v != "" {
			abs, err := filepath.Abs(v)
			if err != nil {
				return hookFile{}, fail.Envf("cannot read CODEX_HOME (%s): %v; nothing written", v, err)
			}
			if _, ok := setupRel(root, abs); !ok {
				return hookFile{}, fail.Envf("CODEX_HOME (%s) is not inside HOME, and setup hook --scope user writes only under HOME; nothing written", v)
			}
			dir = abs
		}
		return hookFile{harness: h, path: filepath.Join(dir, "hooks.json"), top: dir}, nil
	case h == harnessOpenCode && !user:
		rel, top = ".opencode/plugins/jitso-guard.js", ".opencode"
	case h == harnessOpenCode:
		rel, top = ".config/opencode/plugins/jitso-guard.js", ".config/opencode"
	case h == harnessPi && !user:
		rel, top = ".pi/extensions/jitso-guard.js", ".pi"
	case h == harnessPi:
		rel, top = ".pi/agent/extensions/jitso-guard.js", ".pi"
	default:
		return hookFile{}, fail.Envf("internal error: no guard file for the harness %q", h)
	}
	f := hookFile{harness: h, path: filepath.Join(root, filepath.FromSlash(rel)), top: filepath.Join(root, filepath.FromSlash(top))}
	if !user {
		f.rel = rel
	}
	return f, nil
}

// hookBuilder collects the changes of every harness into one plan.
type hookBuilder struct {
	hs      *HookSetup
	user    bool
	remove  bool
	force   bool
	command string
	// displays are the guard files as printed, for the plan's Why.
	displays []string
	// written are the project files that change (not removed), for the
	// commit note; codex is set when .codex/hooks.json changes.
	written []string
	codex   bool
}

// read is the file's bytes, nil when it does not exist.
func (b *hookBuilder) read(f hookFile) ([]byte, error) {
	data, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fail.Envf("could not read %s: %v", b.hs.Display(f.path), setupUnwrap(err))
	}
	return data, nil
}

// settings plans the hooks object of Claude Code's settings file or Codex's
// hooks.json: rewritten, created or removed (and the folders it leaves empty).
func (b *hookBuilder) settings(f hookFile) error {
	display := b.hs.Display(f.path)
	old, err := b.read(f)
	if err != nil {
		return err
	}
	var out []byte
	if b.remove {
		out, err = settingsjson.Unregister(old)
	} else {
		out, err = settingsjson.Register(old, b.command)
	}
	if err != nil {
		return setupProblem(err, display, b.command)
	}

	change := adopt.Change{Path: f.path, Display: display, Old: old, Kind: adopt.KindFile, Sensitive: true, Written: settingsjson.WrittenLine(b.command)}
	switch {
	case b.remove && old == nil, old != nil && out != nil && bytes.Equal(out, old):
		// No change: a missing file, or one that already has the answer.
	case b.remove && out == nil:
		change.Kind = adopt.KindDelete
		b.hs.Plan.Add(change)
		b.prune(f)
	default:
		change.New = out
		b.hs.Plan.Add(change)
		b.wrote(f)
	}
	return nil
}

// generated plans a generated guard file: created, upgraded when it carries
// jitso's marker and other content, refused (or with force replaced) when it
// does not carry the marker; with remove, deleted on the same rule.
func (b *hookBuilder) generated(f hookFile, content []byte) error {
	display := b.hs.Display(f.path)
	old, err := b.read(f)
	if err != nil {
		return err
	}
	if old == nil {
		if !b.remove {
			b.hs.Plan.Add(adopt.Change{Path: f.path, Display: display, Kind: adopt.KindFile, New: content, Brief: true})
			b.wrote(f)
		}
		return nil
	}
	_, ours := guardfiles.MarkerVersion(old)
	if !ours && !b.force && b.hs.Refusal == "" {
		verb := "replace"
		if b.remove {
			verb = "remove"
		}
		b.hs.Refusal = display + " was not written by jitso setup hook (no generated marker); nothing written; run again with --force to " + verb + " it (a backup is kept)"
	}
	change := adopt.Change{Path: f.path, Display: display, Old: old, Kind: adopt.KindFile}
	if b.remove {
		change.Kind, change.Brief = adopt.KindDelete, true
		b.hs.Plan.Add(change)
		b.prune(f)
		return nil
	}
	if bytes.Equal(old, content) {
		return nil
	}
	change.New = content
	b.hs.Plan.Add(change)
	b.wrote(f)
	return nil
}

// prune plans the removal of the folders from the file's folder up to its
// top, each only when it is empty by then.
func (b *hookBuilder) prune(f hookFile) {
	for dir := filepath.Dir(f.path); ; dir = filepath.Dir(dir) {
		b.hs.Plan.Add(adopt.Change{Path: dir, Display: b.hs.Display(dir), Kind: adopt.KindDelete, Prune: true})
		if dir == f.top || len(dir) <= len(f.top) {
			return
		}
	}
}

// wrote records a file the plan writes, for the notes.
func (b *hookBuilder) wrote(f hookFile) {
	if f.harness == harnessCodex && !b.user {
		b.codex = true
	}
	if f.rel != "" && !setupGitTracked(b.hs.Root, f.rel) {
		b.written = append(b.written, f.rel)
	}
}

// finish sets the plan's Why and its notes.
func (b *hookBuilder) finish() {
	plan := &b.hs.Plan
	if b.remove {
		plan.Why = "no jitso hook entries in " + joinAnd(b.displays)
		return
	}
	plan.Why = "the guard is already registered in " + joinAnd(b.displays)
	if len(b.written) > 0 {
		plan.Notes = append(plan.Notes, "note: commit "+joinAnd(b.written)+" so linked worktrees and other clones get the guard")
	}
	if b.codex {
		plan.Notes = append(plan.Notes, setupCodexNote)
	}
}

// joinAnd is "a", "a and b", "a, b and c".
func joinAnd(items []string) string {
	if len(items) < 2 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// setupProblem turns a settingsjson error into the environment error (exit 3)
// that names the file. A layout refusal also prints the fragment to add by hand.
func setupProblem(err error, display, command string) error {
	var p *settingsjson.Problem
	if !errors.As(err, &p) {
		return fail.Envf("%v", err)
	}
	msg := p.Message(display)
	if p.Kind == settingsjson.KindLayout {
		msg += "\n" + strings.TrimRight(string(settingsjson.Fragment(command)), "\n")
	}
	return fail.Envf("%s", msg)
}

// setupVersionWord is the version of this binary for a backup manifest: the
// second word of the line `jitso version` prints.
func setupVersionWord() string {
	if f := strings.Fields(version.Line()); len(f) > 1 {
		return f[1]
	}
	return "dev"
}

// setupRel is path relative to root, in slash form, when it lies inside it.
func setupRel(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// setupUnwrap drops the operation and path an *fs.PathError repeats.
func setupUnwrap(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
