package cli

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/adopt"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/project"
	"github.com/upex-galaxy/jitso/internal/skill"
	"github.com/upex-galaxy/jitso/internal/version"
)

const installSkillUsage = `Usage: jitso install skill --scope user|project [--remove] [--force] [--dry-run] [--yes]

Installs the jitso usage skill where agent harnesses look for skills:
  project  .agents/skills/jitso/ and the link .claude/skills/jitso
  user     ~/.agents/skills/jitso/ and the link ~/.claude/skills/jitso

  --scope    user or project (required)
  --remove   remove the copy jitso installed
  --force    replace or remove a copy that was edited by hand or not installed by jitso (a backup is kept)
  --dry-run  show the changes, write nothing
  --yes      apply without asking (needed when stdin is not a terminal)`

// runInstallSkill installs, upgrades or removes the jitso usage skill (defect
// N-16). The real files go to <root>/.agents/skills/jitso, where Codex and
// OpenCode read them, with a relative link <root>/.claude/skills/jitso for
// Claude Code; the root is the checkout of the working folder for project
// scope and HOME for user scope. A copy that was edited by hand, or that
// jitso did not install, is never replaced or removed without --force, which
// backs it up first. The state is read from the manifest .jitso-skill.json.
//
//	install skill --scope user|project [--remove] [--force] [--dry-run] [--yes]
//
// Exit 0 when done, nothing to do or a dry run; 2 for a bad argument, a
// refusal or a declined question; 3 for the environment.
func runInstallSkill(ctx *Context, args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		ctx.Println(installSkillUsage)
		return nil
	}
	flags, rest, err := ParseFlags(args, FlagScope|FlagRemove|FlagForce|FlagDryRun|FlagYes)
	if err != nil {
		return err
	}
	if err := RejectExtra(rest); err != nil {
		return err
	}
	if flags.Scope == "" {
		return fail.Rejected("install skill needs --scope user or --scope project")
	}

	var root, stateDir string
	var display func(string) string
	if flags.Scope == "user" {
		home := ctx.Home()
		if home == "" {
			return fail.Env("HOME is not set")
		}
		root, stateDir = home, project.UserDir(home)
		display = func(p string) string {
			if rel, ok := under(root, p); ok {
				return "~/" + rel
			}
			return p
		}
	} else {
		r, err := ctx.Project()
		if err != nil {
			return err
		}
		root, stateDir = r.Checkout, r.StateDir()
		display = func(p string) string {
			if rel, ok := under(root, p); ok {
				return rel
			}
			return p
		}
	}

	res, err := skill.Build(skill.Options{
		Scope:   flags.Scope,
		Root:    root,
		Display: display,
		Version: version.Current(),
		Remove:  flags.Remove,
		Force:   flags.Force,
	})
	if err != nil {
		return err
	}
	if res.Refusal != "" {
		adopt.Preview(ctx.Sys.Stdout, res.Plan)
		return fail.Rejected(res.Refusal)
	}
	if flags.Scope == "project" && !flags.Remove && !res.Plan.Empty() && untracked(root, skill.RealRel, skill.LinkRel) {
		res.Plan.Notes = append(res.Plan.Notes, "note: commit "+skill.RealRel+" and "+skill.LinkRel+" so other clones get the skill")
	}
	if flags.Scope == "user" && !flags.Remove && !res.Plan.Empty() {
		res.Plan.Notes = append(res.Plan.Notes, "note: a launched session does not read the user skill; a launch carries the jitso skill for a conductor build or a build that names jitso, and a build gets other user skills only as user:<name> allowlisted in ~/.jitso/jitso.yaml")
	}

	return adopt.Run(adopt.Options{
		Command:    "install skill",
		Scope:      flags.Scope,
		Version:    version.Current(),
		Root:       root,
		BackupsDir: filepath.Join(stateDir, "backups"),
		DryRun:     flags.DryRun,
		Yes:        flags.Yes,
		Display:    display,
	}, res.Plan, adopt.Ports{
		Stdin:    ctx.Sys.Stdin,
		Stdout:   ctx.Sys.Stdout,
		Terminal: ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal(),
		Now:      ctx.Sys.Now,
		Rand:     ctx.Sys.Rand,
	})
}

// under is path relative to root in slash form, when it lies inside root.
func under(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// untracked reports whether git does not track at least one of the paths in
// the checkout. A checkout git cannot read counts as tracked: the note is a
// courtesy and is never worth a false claim.
func untracked(checkout string, rels ...string) bool {
	for _, rel := range rels {
		out, err := exec.Command("git", "-C", checkout, "ls-files", "--", rel).Output()
		if err == nil && len(strings.TrimSpace(string(out))) == 0 {
			return true
		}
	}
	return false
}
