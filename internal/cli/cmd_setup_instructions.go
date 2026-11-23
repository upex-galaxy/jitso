package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/adopt"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/instructions"
	"github.com/upex-galaxy/jitso/internal/project"
)

// setup instructions keeps a delimited, reversible JITSO block in the
// AGENTS.md files agents read (defect N-18): AGENTS.md of the checkout, or
// with --scope user the AGENTS.md of each harness user folder that exists. It
// never writes CLAUDE.md: a block an older jitso put in a CLAUDE.md that
// imports AGENTS.md is taken out by the same run, so the block moves to
// AGENTS.md; a CLAUDE.md that does not import it keeps its block and gets a
// warning, and ~/.claude/CLAUDE.md is cleaned only by --remove. The block
// text, the markers and the byte-exact undo are internal/instructions;
// the preview, the backup, the confirmation and the write are internal/adopt.
//
// Exit codes: 0 done, nothing to do or dry run; 2 a bad argument, no --yes
// without a terminal, a declined prompt; 3 no git checkout (project scope),
// no HOME (user scope), a file with a broken marker, or a write failure.
func init() {
	registerSetupSub(setupSub{
		Name:      "instructions",
		Order:     20,
		UsageLine: "[--scope project|user] [--remove] [--dry-run] [--yes]",
		Help: "setup instructions keeps a JITSO block in the AGENTS.md files agents read; it never writes CLAUDE.md:\n" +
			"  project  AGENTS.md of this checkout\n" +
			"  user     $CODEX_HOME/AGENTS.md (default ~/.codex/AGENTS.md),\n" +
			"           ~/.config/opencode/AGENTS.md and ~/.pi/agent/AGENTS.md, each when its folder exists;\n" +
			"           ~/.config/opencode/AGENTS.md is not created while ~/.claude/CLAUDE.md exists\n" +
			"A CLAUDE.md that imports AGENTS.md (@AGENTS.md, or @../AGENTS.md in .claude/CLAUDE.md)\n" +
			"loses a block an older jitso put there, in the same run; one that does not import it\n" +
			"keeps its block, with a warning. ~/.claude/CLAUDE.md is changed only by --remove.\n" +
			"Files that are links to one real file are changed once.\n" +
			"\n" +
			"  --scope   project (default) or user\n" +
			"  --remove  remove the block; the rest of each file stays byte for byte\n" +
			"  --dry-run show the changes, write nothing\n" +
			"  --yes     apply without asking (needed when stdin is not a terminal)",
		Run: runSetupInstructions,
	})
}

func runSetupInstructions(ctx *Context, args []string) error {
	// --help and -h are answered before the flags are parsed: the parser
	// would call them unknown arguments.
	for _, a := range args {
		if a == "--help" || a == "-h" {
			ctx.Println(setupUsage())
			return nil
		}
	}
	flags, rest, err := ParseFlags(args, FlagScope|FlagRemove|FlagDryRun|FlagYes)
	if err != nil {
		return err
	}
	if err := RejectExtra(rest); err != nil {
		return err
	}
	scope := "project"
	if flags.Scope == "user" {
		scope = "user"
	}
	s, err := InstructionsPlan(ctx, scope, flags.Remove)
	if err != nil {
		return err
	}
	for _, w := range s.Warnings {
		ctx.Warn(w)
	}
	return adopt.Run(adopt.Options{
		Command:    "setup instructions",
		Scope:      s.Scope,
		Version:    setupVersionWord(),
		Root:       s.Root,
		BackupsDir: s.BackupsDir,
		DryRun:     flags.DryRun,
		Yes:        flags.Yes,
		Display:    s.Display,
	}, s.Plan, adopt.Ports{
		Stdin:    ctx.Sys.Stdin,
		Stdout:   ctx.Sys.Stdout,
		Terminal: ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal(),
		Now:      ctx.Sys.Now,
		Rand:     ctx.Sys.Rand,
	})
}

// InstructionsSetup is one scope's `setup instructions` change, built but not
// run: what adopt.Run needs besides the command name, the version and the
// confirmation flags, so a caller can preview several plans and apply them
// after one confirmation.
type InstructionsSetup struct {
	// Scope is "project" or "user".
	Scope string
	// Root is the scope root every plan path lies under: the checkout, or HOME.
	Root string
	// BackupsDir is where the backup of the files the plan changes goes:
	// .jitso/backups of the primary checkout, or ~/.jitso/backups.
	BackupsDir string
	// Display prints a backup location: relative to the checkout, or ~/...
	Display func(string) string
	// Plan is the change; its Notes name the paths that are the same file as
	// a target, and its Why explains an empty plan.
	Plan adopt.Plan
	// Warnings are for stderr, before the preview: the files jitso skips.
	Warnings []string
}

// InstructionsPlan reads the instruction files of scope ("project" or
// "user") and builds the change `jitso setup instructions` would make: add or
// update the block in the AGENTS.md targets and take an old block out of the
// Claude Code memory files, or with remove take the block out of every file
// that has one. It writes nothing. Project scope needs a git checkout and
// user scope HOME (exit 3 otherwise); a broken marker is exit 3 too.
func InstructionsPlan(ctx *Context, scope string, remove bool) (InstructionsSetup, error) {
	env := instructions.Env{
		ReadFile: os.ReadFile,
		IsDir: func(p string) bool {
			info, err := os.Stat(p)
			return err == nil && info.IsDir()
		},
		EvalSymlinks: filepath.EvalSymlinks,
	}
	var (
		s       = InstructionsSetup{Scope: scope}
		targets instructions.Targets
		err     error
	)
	if scope == "user" {
		home := ctx.Home()
		if home == "" {
			return s, fail.Env("HOME is not set")
		}
		env.Home, env.CodexHome = home, ctx.Getenv("CODEX_HOME")
		s.Root, s.BackupsDir = home, filepath.Join(project.UserDir(home), "backups")
		s.Display = func(p string) string {
			if rel, ok := setupRel(home, p); ok {
				return "~/" + rel
			}
			return p
		}
		targets, err = instructions.UserTargets(env, remove)
	} else {
		r, perr := ctx.Project()
		if perr != nil {
			return s, perr
		}
		env.Root = r.Checkout
		s.Root, s.BackupsDir = r.Checkout, filepath.Join(r.StateDir(), "backups")
		s.Display = func(p string) string {
			if rel, ok := setupRel(r.Checkout, p); ok {
				return rel
			}
			return p
		}
		targets, err = instructions.ProjectTargets(env, remove)
	}
	if err != nil {
		return s, err
	}
	s.Warnings = targets.Warnings
	s.Plan, err = setupInstructionsPlan(targets, remove)
	return s, err
}

// setupInstructionsPlan is the change the command would make to each target:
// the block added or brought up to date in AGENTS.md (a file is created when
// it is absent), or taken out (of a Legacy file also when adding; a file
// jitso created is deleted when nothing else is in it).
func setupInstructionsPlan(targets instructions.Targets, remove bool) (adopt.Plan, error) {
	var plan adopt.Plan
	if !remove && targets.None != "" {
		plan.Why = targets.None
		return plan, nil
	}
	var shown []string
	for _, t := range targets.List {
		if remove || t.Legacy {
			if remove && t.Old != nil {
				shown = append(shown, t.Display)
			}
			if err := setupInstructionsTakeOut(&plan, t); err != nil {
				return plan, err
			}
			continue
		}
		shown = append(shown, t.Display)
		out, err := instructions.Ensure(t.Old)
		if err != nil {
			return plan, setupInstructionsProblem(err, t.Display)
		}
		if t.Old != nil && bytes.Equal(out, t.Old) {
			continue
		}
		plan.Add(adopt.Change{Path: t.Path, Display: t.Display, Old: t.Old, New: out, Kind: adopt.KindFile})
	}
	if len(shown) == 0 && len(targets.List) > 0 {
		shown = append(shown, targets.List[0].Display)
	}
	list := strings.Join(shown, ", ")
	if remove {
		plan.Why = "no JITSO block in " + list
	} else {
		plan.Why = "the JITSO block is up to date in " + list
	}
	plan.Notes = append(plan.Notes, targets.Notes...)
	return plan, nil
}

// setupInstructionsTakeOut adds the change that takes the block out of t,
// when it has one: the file rewritten without it, or deleted (and its
// PruneDir after it) when jitso created it and nothing else is left.
func setupInstructionsTakeOut(plan *adopt.Plan, t instructions.Target) error {
	if t.Old == nil {
		return nil
	}
	res, err := instructions.Remove(t.Old)
	if err != nil {
		return setupInstructionsProblem(err, t.Display)
	}
	switch {
	case !res.Found:
	case res.Delete:
		plan.Add(adopt.Change{Path: t.Path, Display: t.Display, Old: t.Old, Kind: adopt.KindDelete})
		if t.PruneDir != "" {
			plan.Add(adopt.Change{Path: t.PruneDir, Display: t.PruneDisplay, Kind: adopt.KindDelete, Prune: true})
		}
	default:
		plan.Add(adopt.Change{Path: t.Path, Display: t.Display, Old: t.Old, New: res.Out, Kind: adopt.KindFile})
	}
	return nil
}

// setupInstructionsProblem turns a marker or fence problem into the
// environment error (exit 3) that names the file.
func setupInstructionsProblem(err error, display string) error {
	var p *instructions.Problem
	if errors.As(err, &p) {
		return fail.Env(p.Message(display))
	}
	return fail.Envf("%v", err)
}
