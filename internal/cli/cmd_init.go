package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/adopt"
	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/skill"
	"github.com/upex-galaxy/jitso/internal/templates"
	"github.com/upex-galaxy/jitso/internal/version"
)

const initUsage = `Usage: jitso init [--project-only | --user-only] [--harness <list>] [--absolute] [--remove] [--force] [--dry-run] [--yes]

Sets up JITSO in one run, with one preview and one question:
  user     the jitso skill in ~/.agents/skills/jitso (every harness reads it)
  project  (inside a git checkout) jitso/jitso.yaml when it is missing, .jitso/ in
           .gitignore, the JITSO block in AGENTS.md, and the guard for each
           harness on PATH

  --project-only  put the skill in the project instead; write nothing under the home folder
  --user-only     set up your user account only, even inside a project
  --harness       claude, codex, opencode, pi as a comma list, or all (default: those on PATH)
  --absolute      write this binary's absolute path in the guard instead of the word jitso
  --remove        undo the skill, the block and the guard (jitso/jitso.yaml and the
                  .gitignore line stay)
  --force         replace a skill or guard file that was edited by hand (a backup is kept)
  --dry-run       show the changes, write nothing
  --yes           apply without asking (needed when stdin is not a terminal)

On a terminal, without --yes, init asks where the skill goes and which harnesses
to guard, then shows every change and asks before writing.`

func init() {
	Register(Command{
		Name:    "init",
		Summary: "set up JITSO for your harnesses in one run: skill, instructions, guard",
		Order:   80,
		Run:     runInit,
	})
}

// initOptions are the answers init works from, from flags or the questions.
type initOptions struct {
	projectOnly, userOnly bool
	harnesses             []string
	absolute, remove      bool
	force                 bool
}

// runInit composes the adoption commands into one run (install skill, setup
// instructions, setup hook, and the jitso/jitso.yaml skeleton), previewed and
// confirmed once through adopt.RunParts. Exit 0 when done, nothing to do or a
// dry run; 2 for a usage error, a refusal or a declined question; 3 for the
// environment.
func runInit(ctx *Context, args []string) error {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			ctx.Println(initUsage)
			return nil
		}
	}
	var o initOptions
	var rest []string
	for _, a := range args {
		switch a {
		case "--project-only":
			o.projectOnly = true
		case "--user-only":
			o.userOnly = true
		default:
			rest = append(rest, a)
		}
	}
	flags, rest, err := ParseFlags(rest, FlagHarness|FlagAbsolute|FlagRemove|FlagForce|FlagDryRun|FlagYes)
	if err != nil {
		return err
	}
	if err := RejectExtra(rest); err != nil {
		return err
	}
	if o.projectOnly && o.userOnly {
		return fail.Rejected("--project-only and --user-only cannot be used together")
	}
	if o.userOnly && flags.Harnesses != nil {
		return fail.Rejected("--harness picks the project's guard, so it cannot be used with --user-only")
	}
	o.harnesses, o.absolute, o.remove, o.force = flags.Harnesses, flags.Absolute, flags.Remove, flags.Force

	root, projErr := ctx.Project()
	inProject := projErr == nil
	if !inProject && o.projectOnly {
		return projErr
	}
	terminal := ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal()
	asked := o.projectOnly || o.userOnly || o.harnesses != nil
	var stdin io.Reader = ctx.Sys.Stdin
	if terminal && !flags.Yes && !flags.DryRun && !o.remove && !asked && inProject {
		if err := initAsk(ctx, &o); err != nil {
			return err
		}
	}

	var parts []adopt.Part
	var warnings, notes []string
	bare := false

	if !o.projectOnly {
		part, err := initSkillPart(ctx, "user", project.Root{}, o)
		if err != nil {
			return err
		}
		parts = append(parts, part)
	}
	if inProject && !o.userOnly {
		if !o.remove {
			if part, ok := initConfigPart(root); ok {
				parts = append(parts, part)
			} else if root.Linked && !configExists(root.Primary) {
				notes = append(notes, "note: run jitso init in the primary checkout to create "+catalog.ConfigPath)
			}
		}
		if !o.remove {
			if part, ok := initIgnorePart(root); ok {
				parts = append(parts, part)
			}
		}
		if o.projectOnly {
			part, err := initSkillPart(ctx, "project", root, o)
			if err != nil {
				return err
			}
			parts = append(parts, part)
		}
		is, err := InstructionsPlan(ctx, "project", o.remove)
		if err != nil {
			return err
		}
		warnings = append(warnings, is.Warnings...)
		parts = append(parts, adopt.Part{Label: "instructions (AGENTS.md)", Plan: is.Plan, Options: adopt.Options{
			Scope: is.Scope, Version: version.Current(), Root: is.Root, BackupsDir: is.BackupsDir, Display: is.Display}})

		hs, err := HookPlan(ctx, "project", o.harnesses, o.absolute, o.remove, o.force)
		if err != nil {
			return err
		}
		if hs.Refusal != "" {
			adopt.Preview(ctx.Sys.Stdout, hs.Plan)
			return fail.Rejected(hs.Refusal)
		}
		bare = hs.BareCommand && !hs.Plan.Empty()
		parts = append(parts, adopt.Part{Label: "guard for " + strings.Join(initHarnessNames(o.harnesses), ", "), Plan: hs.Plan, Options: adopt.Options{
			Scope: hs.Scope, Version: version.Current(), Root: hs.Root, BackupsDir: hs.BackupsDir, Display: hs.Display}})
	}
	if !inProject && !o.remove {
		parts[len(parts)-1].Plan.Notes = append(parts[len(parts)-1].Plan.Notes,
			"note: this folder is not inside a git checkout, so only your user account was set up; run jitso init inside a project to set it up too")
	}

	for _, w := range warnings {
		ctx.Warn(w)
	}
	why := "JITSO is already set up here"
	if o.remove {
		why = "no JITSO setup to remove"
	}
	written := false
	for _, p := range parts {
		if !p.Plan.Empty() {
			written = true
		}
	}
	if err := adopt.RunParts("init", parts, flags.DryRun, flags.Yes, why, adopt.Ports{
		Stdin:    stdin,
		Stdout:   ctx.Sys.Stdout,
		Terminal: terminal,
		Now:      ctx.Sys.Now,
		Rand:     ctx.Sys.Rand,
	}); err != nil {
		return err
	}
	if !written || flags.DryRun {
		return nil
	}
	for _, n := range notes {
		ctx.Println(n)
	}
	if o.remove {
		return nil
	}
	if bare {
		if _, err := setupLookPath("jitso"); err != nil {
			ctx.Warn("jitso: warning: jitso is not on PATH, so the hook cannot start until it is (the guard then lets every call through)")
		}
	}
	if err := ctx.OrcaInstalled(); err != nil {
		ctx.Println("note: " + err.Error())
	}
	if inProject && !o.userOnly {
		ctx.Println("next: jitso build new <id> starts your first build; jitso doctor checks the setup")
	} else {
		ctx.Println("next: jitso doctor checks the setup")
	}
	return nil
}

// initAsk asks the two questions of an interactive init: where the skill goes
// and which harnesses get the guard. An empty answer takes the default.
func initAsk(ctx *Context, o *initOptions) error {
	found := defaultHarnesses()
	ctx.Println("jitso init sets up JITSO for your agent harnesses.")
	fmt.Fprint(ctx.Sys.Stdout, "Install the jitso skill for your user account (every project) or in this project only? [U/p] ")
	switch strings.ToLower(strings.TrimSpace(initReadLine(ctx.Sys.Stdin))) {
	case "", "u", "user":
	case "p", "project":
		o.projectOnly = true
	default:
		return fail.Rejected("answer u (user account) or p (this project)")
	}
	fmt.Fprintf(ctx.Sys.Stdout, "Register the guard for which harnesses? [Enter: %s] ", strings.Join(found, ", "))
	answer := strings.TrimSpace(initReadLine(ctx.Sys.Stdin))
	if answer == "" {
		o.harnesses = found
		return nil
	}
	list, ok := parseHarnesses(strings.ReplaceAll(answer, " ", ""))
	if !ok {
		return fail.Rejected("--harness must be a comma list of claude, codex, opencode and pi, or all")
	}
	o.harnesses = list
	return nil
}

// initReadLine reads one line a byte at a time, so the bytes after it stay in
// stdin for the next question and the final confirmation.
func initReadLine(r io.Reader) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			break
		}
	}
	return strings.TrimSuffix(b.String(), "\r")
}

// initHarnessNames is the harness list a label names: the given one, or the
// default.
func initHarnessNames(list []string) []string {
	if len(list) == 0 {
		return defaultHarnesses()
	}
	if names, ok := parseHarnesses(strings.Join(list, ",")); ok {
		return names
	}
	return list
}

// initSkillPart is the install skill plan at scope.
func initSkillPart(ctx *Context, scope string, root project.Root, o initOptions) (adopt.Part, error) {
	var base, stateDir, label string
	var display func(string) string
	if scope == "user" {
		home := ctx.Home()
		if home == "" {
			return adopt.Part{}, fail.Env("HOME is not set")
		}
		base, stateDir, label = home, project.UserDir(home), "skill for your user account"
		display = func(p string) string {
			if rel, ok := under(base, p); ok {
				return "~/" + rel
			}
			return p
		}
	} else {
		base, stateDir, label = root.Checkout, root.StateDir(), "skill in this project"
		display = func(p string) string {
			if rel, ok := under(base, p); ok {
				return rel
			}
			return p
		}
	}
	res, err := skill.Build(skill.Options{Scope: scope, Root: base, Display: display, Version: version.Current(), Remove: o.remove, Force: o.force})
	if err != nil {
		return adopt.Part{}, err
	}
	if res.Refusal != "" {
		adopt.Preview(ctx.Sys.Stdout, res.Plan)
		return adopt.Part{}, fail.Rejected(res.Refusal)
	}
	return adopt.Part{Label: label, Plan: res.Plan, Options: adopt.Options{
		Scope: scope, Version: version.Current(), Root: base, BackupsDir: filepath.Join(stateDir, "backups"), Display: display}}, nil
}

// initConfigPart plans the jitso/jitso.yaml skeleton when the checkout is the
// primary one and has none: the guard does nothing in a folder without it.
func initConfigPart(root project.Root) (adopt.Part, bool) {
	if root.Linked || configExists(root.Primary) {
		return adopt.Part{}, false
	}
	path := filepath.Join(root.Primary, filepath.FromSlash(catalog.ConfigPath))
	var plan adopt.Plan
	plan.Add(adopt.Change{Path: path, Display: catalog.ConfigPath, New: []byte(templates.Config()), Kind: adopt.KindFile, Brief: true})
	plan.Notes = append(plan.Notes, "note: "+catalog.ConfigPath+" lists no allowlist: every one is open until you add a list")
	return adopt.Part{Label: "allowlists (" + catalog.ConfigPath + ")", Plan: plan, Options: adopt.Options{
		Scope: "project", Version: version.Current(), Root: root.Checkout, BackupsDir: filepath.Join(root.StateDir(), "backups"),
		Display: func(p string) string {
			if rel, ok := under(root.Checkout, p); ok {
				return rel
			}
			return p
		}}}, true
}

// initIgnorePart plans the line .jitso/ at the end of the primary checkout's
// .gitignore when git does not ignore the launch log there: .jitso/ holds
// local state (launches, drafts, backups) that is never committed. Only in the
// primary checkout, and only when git answers that the path is not ignored.
func initIgnorePart(root project.Root) (adopt.Part, bool) {
	if root.Linked {
		return adopt.Part{}, false
	}
	cmd := exec.Command("git", "-C", root.Primary, "check-ignore", "-q", ".jitso/launches.jsonl")
	if err := cmd.Run(); err == nil {
		return adopt.Part{}, false
	} else if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
		return adopt.Part{}, false
	}
	path := filepath.Join(root.Primary, ".gitignore")
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return adopt.Part{}, false
	}
	next := append([]byte{}, old...)
	if len(next) > 0 && next[len(next)-1] != '\n' {
		next = append(next, '\n')
	}
	next = append(next, ".jitso/\n"...)
	var plan adopt.Plan
	c := adopt.Change{Path: path, Display: ".gitignore", New: next, Kind: adopt.KindFile}
	if err == nil {
		c.Old = old
	}
	plan.Add(c)
	return adopt.Part{Label: "local state (.gitignore)", Plan: plan, Options: adopt.Options{
		Scope: "project", Version: version.Current(), Root: root.Checkout, BackupsDir: filepath.Join(root.StateDir(), "backups"),
		Display: func(p string) string {
			if rel, ok := under(root.Checkout, p); ok {
				return rel
			}
			return p
		}}}, true
}

// configExists reports whether primary holds jitso/jitso.yaml (anything at
// that path counts: init never writes over it).
func configExists(primary string) bool {
	_, err := os.Lstat(filepath.Join(primary, filepath.FromSlash(catalog.ConfigPath)))
	return err == nil || !errors.Is(err, os.ErrNotExist)
}
