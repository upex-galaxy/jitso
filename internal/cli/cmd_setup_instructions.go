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
// instruction files agents read (defect N-18): AGENTS.md of the checkout (and
// CLAUDE.md when Claude Code would read that instead), or with --scope user
// the user's own files. The block text, the markers and the byte-exact undo
// are internal/instructions; the preview, the backup, the confirmation and the
// write are internal/adopt.
//
// Exit codes: 0 done, nothing to do or dry run; 2 a bad argument, no --yes
// without a terminal, a declined prompt; 3 no git checkout (project scope),
// no HOME (user scope), a file with a broken marker, or a write failure.
func init() {
	registerSetupSub(setupSub{
		Name:      "instructions",
		Order:     20,
		UsageLine: "[--scope project|user] [--remove] [--dry-run] [--yes]",
		Help: "setup instructions keeps a JITSO block in the instruction files agents read:\n" +
			"  project  AGENTS.md of this checkout, and CLAUDE.md when it exists without an @AGENTS.md import\n" +
			"  user     ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md when ~/.codex exists,\n" +
			"           ~/.config/opencode/AGENTS.md when it exists\n" +
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

	env := instructions.Env{
		ReadFile: os.ReadFile,
		IsDir: func(p string) bool {
			info, err := os.Stat(p)
			return err == nil && info.IsDir()
		},
	}
	var (
		root, stateDir string
		display        func(string) string
		targets        []instructions.Target
		warnings       []string
		scope          = "project"
	)
	if flags.Scope == "user" {
		scope = "user"
		home := ctx.Home()
		if home == "" {
			return fail.Env("HOME is not set")
		}
		env.Home, env.CodexHome = home, ctx.Getenv("CODEX_HOME")
		root, stateDir = home, project.UserDir(home)
		display = func(p string) string {
			if rel, ok := setupRel(root, p); ok {
				return "~/" + rel
			}
			return p
		}
		targets, warnings, err = instructions.UserTargets(env, flags.Remove)
	} else {
		r, perr := ctx.Project()
		if perr != nil {
			return perr
		}
		env.Root = r.Checkout
		root, stateDir = r.Checkout, r.StateDir()
		display = func(p string) string {
			if rel, ok := setupRel(root, p); ok {
				return rel
			}
			return p
		}
		targets, warnings, err = instructions.ProjectTargets(env, flags.Remove)
	}
	if err != nil {
		return err
	}
	for _, w := range warnings {
		ctx.Warn(w)
	}

	plan, err := setupInstructionsPlan(targets, flags.Remove)
	if err != nil {
		return err
	}

	return adopt.Run(adopt.Options{
		Command:    "setup instructions",
		Scope:      scope,
		Version:    setupVersionWord(),
		Root:       root,
		BackupsDir: filepath.Join(stateDir, "backups"),
		DryRun:     flags.DryRun,
		Yes:        flags.Yes,
		Display:    display,
	}, plan, adopt.Ports{
		Stdin:    ctx.Sys.Stdin,
		Stdout:   ctx.Sys.Stdout,
		Terminal: ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal(),
		Now:      ctx.Sys.Now,
		Rand:     ctx.Sys.Rand,
	})
}

// setupInstructionsPlan is the change the command would make to each target:
// the block added or brought up to date (a file is created when it is absent),
// or taken out (a file jitso created is deleted when nothing else is in it).
func setupInstructionsPlan(targets []instructions.Target, remove bool) (adopt.Plan, error) {
	var plan adopt.Plan
	var shown []string
	for _, t := range targets {
		if remove && t.Old == nil {
			continue
		}
		shown = append(shown, t.Display)
		if remove {
			res, err := instructions.Remove(t.Old)
			if err != nil {
				return plan, setupInstructionsProblem(err, t.Display)
			}
			if !res.Found {
				continue
			}
			if res.Delete {
				plan.Add(adopt.Change{Path: t.Path, Display: t.Display, Old: t.Old, Kind: adopt.KindDelete})
				if t.PruneDir != "" {
					plan.Add(adopt.Change{Path: t.PruneDir, Display: t.PruneDisplay, Kind: adopt.KindDelete, Prune: true})
				}
				continue
			}
			plan.Add(adopt.Change{Path: t.Path, Display: t.Display, Old: t.Old, New: res.Out, Kind: adopt.KindFile})
			continue
		}
		out, err := instructions.Ensure(t.Old)
		if err != nil {
			return plan, setupInstructionsProblem(err, t.Display)
		}
		if t.Old != nil && bytes.Equal(out, t.Old) {
			continue
		}
		plan.Add(adopt.Change{Path: t.Path, Display: t.Display, Old: t.Old, New: out, Kind: adopt.KindFile})
	}
	if len(shown) == 0 && len(targets) > 0 {
		shown = append(shown, targets[0].Display)
	}
	list := strings.Join(shown, ", ")
	if remove {
		plan.Why = "no JITSO block in " + list
	} else {
		plan.Why = "the JITSO block is up to date in " + list
	}
	return plan, nil
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
