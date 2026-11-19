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
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/settingsjson"
	"github.com/upex-galaxy/jitso/internal/shell"
	"github.com/upex-galaxy/jitso/internal/version"
)

// setup hook registers `jitso hook guard` in the settings file of the
// checkout of the working folder, or with --scope user in ~/.claude/settings.json
// for the owner's own session (defect N-17): on PreToolUse for Bash and for
// the orca MCP tools, and on SessionEnd. It edits the file through an
// order-preserving JSON tree (internal/settingsjson), never touches a group
// the user wrote, shows the diff first (no unchanged line, so a token in the
// file is never printed), saves a backup of an existing file and asks before
// it writes (internal/adopt). `--remove` takes out exactly what jitso added.
//
// The project file belongs to the checkout the command runs in (a linked
// worktree has its own); backups and state go to the primary's .jitso/. The
// user file needs no project and its backups go to ~/.jitso/. It is safe
// outside JITSO projects: the guard prints nothing without jitso/jitso.yaml.
// The file .claude/settings.local.json is never opened.
//
// Exit codes: 0 done, nothing to do or dry run; 2 a bad argument, no --yes
// without a terminal, a declined prompt; 3 no git checkout, a settings file
// jitso will not edit (not JSON, wrong shape, a layout it cannot reproduce) or
// a write failure.
func init() {
	registerSetupSub(setupSub{
		Name:      "hook",
		Order:     10,
		UsageLine: "[--scope project|user] [--absolute] [--remove] [--dry-run] [--yes]",
		Help: "setup hook registers `jitso hook guard` in this checkout's .claude/settings.json\n" +
			"(or in ~/.claude/settings.json with --scope user):\n" +
			"on PreToolUse for Bash and the orca MCP tools, and on SessionEnd.\n" +
			"\n" +
			"  --scope     project (default): this checkout; user: your own sessions, in every folder\n" +
			"  --absolute  write this binary's absolute path instead of the word jitso\n" +
			"  --remove    remove the hook entries jitso added\n" +
			"  --dry-run   show the changes, write nothing\n" +
			"  --yes       apply without asking (needed when stdin is not a terminal)",
		Run: runSetupHook,
	})
}

// setupSettingsRel is the settings file, relative to the checkout (project
// scope) or to HOME (user scope).
const setupSettingsRel = ".claude/settings.json"

// setupUserDisplay is the user settings file as printed.
const setupUserDisplay = "~/" + setupSettingsRel

// setupLookPath and setupGitTracked are the two questions about the machine
// and the repository that tests answer on their own.
var (
	setupLookPath   = exec.LookPath
	setupGitTracked = func(checkout, rel string) bool {
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
	flags, rest, err := ParseFlags(args, FlagScope|FlagRemove|FlagAbsolute|FlagDryRun|FlagYes)
	if err != nil {
		return err
	}
	if err := RejectExtra(rest); err != nil {
		return err
	}

	var root, stateDir, display string
	var shown func(string) string
	if flags.Scope == "user" {
		home := ctx.Home()
		if home == "" {
			return fail.Env("HOME is not set")
		}
		root, stateDir, display = home, project.UserDir(home), setupUserDisplay
		shown = func(p string) string {
			if rel, ok := setupRel(root, p); ok {
				return "~/" + rel
			}
			return p
		}
	} else {
		r, err := ctx.Project()
		if err != nil {
			return err
		}
		root, stateDir, display = r.Checkout, r.StateDir(), setupSettingsRel
		shown = func(p string) string {
			if rel, ok := setupRel(root, p); ok {
				return rel
			}
			return p
		}
	}

	command := settingsjson.Command
	if flags.Absolute && !flags.Remove {
		self, err := project.Self(ctx.Sys.Executable)
		if err != nil {
			return fail.Envf("cannot find this binary's own path: %v", err)
		}
		quoted, err := shell.Quote(self)
		if err != nil {
			return fail.Envf("cannot write this binary's path into a command: %v", err)
		}
		command = quoted + " hook guard"
	}

	file := filepath.Join(root, filepath.FromSlash(setupSettingsRel))
	old, err := os.ReadFile(file)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		old = nil
	case err != nil:
		return fail.Envf("could not read %s: %v", display, setupUnwrap(err))
	}

	plan, err := setupHookPlan(root, file, display, flags.Scope == "user", old, command, flags.Remove)
	if err != nil {
		return err
	}

	scope := flags.Scope
	if scope == "" {
		scope = "project"
	}
	opts := adopt.Options{
		Command:    "setup hook",
		Scope:      scope,
		Version:    setupVersionWord(),
		Root:       root,
		BackupsDir: filepath.Join(stateDir, "backups"),
		DryRun:     flags.DryRun,
		Yes:        flags.Yes,
		Display:    shown,
	}
	ports := adopt.Ports{
		Stdin:    ctx.Sys.Stdin,
		Stdout:   ctx.Sys.Stdout,
		Terminal: ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal(),
		Now:      ctx.Sys.Now,
		Rand:     ctx.Sys.Rand,
	}
	if err := adopt.Run(opts, plan, ports); err != nil {
		return err
	}
	// Run returns nil for nothing to do, a dry run and an applied plan. Only
	// the last one has written the bare word that needs a PATH.
	if !plan.Empty() && !flags.DryRun && !flags.Remove && command == settingsjson.Command {
		if _, err := setupLookPath("jitso"); err != nil {
			ctx.Warn("jitso: warning: jitso is not on PATH, so the hook cannot start until it is (the guard then lets every call through)")
		}
	}
	return nil
}

// setupHookPlan is the change the command would make: the settings file
// rewritten, created or removed (and the .claude folder it leaves empty).
// display is the file as printed; user is true for the file in HOME.
func setupHookPlan(root, file, display string, user bool, old []byte, command string, remove bool) (adopt.Plan, error) {
	var plan adopt.Plan
	var out []byte
	var err error
	if remove {
		out, err = settingsjson.Unregister(old)
	} else {
		out, err = settingsjson.Register(old, command)
	}
	if err != nil {
		return plan, setupProblem(err, display, command)
	}

	change := adopt.Change{Path: file, Display: display, Old: old, Kind: adopt.KindFile, Sensitive: true, Written: settingsjson.WrittenLine(command)}
	switch {
	case remove && old == nil, old != nil && out != nil && bytes.Equal(out, old):
		// No change: a missing file, or one that already has the answer.
	case remove && out == nil:
		change.Kind = adopt.KindDelete
		plan.Add(change)
		plan.Add(adopt.Change{
			Path:    filepath.Dir(file),
			Display: strings.TrimSuffix(display, "/"+filepath.Base(setupSettingsRel)),
			Kind:    adopt.KindDelete,
			Prune:   true,
		})
	default:
		change.New = out
		plan.Add(change)
	}

	if remove {
		plan.Why = "no jitso hook entries in " + display
		return plan, nil
	}
	plan.Why = "the guard is already registered in " + display
	if !user && !plan.Empty() && !setupGitTracked(root, setupSettingsRel) {
		plan.Notes = append(plan.Notes, "note: commit "+setupSettingsRel+" so linked worktrees and other clones get the guard")
	}
	return plan, nil
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
