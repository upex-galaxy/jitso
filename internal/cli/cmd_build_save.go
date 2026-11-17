package cli

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/upex-galaxy/jitso/internal/adopt"
	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/drafts"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/launch"
)

// build save turns a draft into a build of the project (defect N-15): the
// draft bytes with the status scalar `draft` replaced by `approved` and
// nothing else changed, written to jitso/builds/<id>.yaml, and the draft
// removed. It is the one way a build becomes approved, so it is the owner's:
//
//   - it shows the content it would write, every line prefixed with `+ `;
//   - on a terminal it asks `[y/N]`; anything but y or yes is `not saved`;
//   - `--yes` saves without asking, on a terminal or not, as it does for the
//     adoption commands (the rule is adopt.Confirm);
//   - without a terminal and without `--yes` it refuses and saves nothing;
//   - it never overwrites a file of jitso/builds/ and never takes an id a saved
//     build already has;
//   - the draft is checked again against the current allowlists first.
//
// The agent boundary is not this command but the launch guard, which denies
// every Bash segment that runs it (hook.VerbBuildSave). Exit 0 when saved, 2
// for a bad argument, a rejected draft or a missing OK, 3 for the environment.
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "save",
		Order: 50,
		Usage: "save <id> [--yes]  turn a draft into an approved build (the owner only)",
		Run:   runBuildSave,
	})
}

// saveArgs are the words after `build save`.
type saveArgs struct {
	id string
	// yes is true when --yes was given.
	yes bool
}

// parseSaveArgs reads `<id> [--yes]`. The id is checked last, so it is never
// turned into a path unless it is kebab-case.
func parseSaveArgs(args []string) (saveArgs, error) {
	flags, rest, err := ParseFlags(args, FlagYes)
	if err != nil {
		return saveArgs{}, err
	}
	if len(rest) == 0 {
		return saveArgs{}, fail.Rejected("build save needs a draft id")
	}
	if err := RejectExtra(rest[1:]); err != nil {
		return saveArgs{}, err
	}
	if !drafts.ValidID(rest[0]) {
		return saveArgs{}, fail.Rejected("build id must be kebab-case")
	}
	return saveArgs{id: rest[0], yes: flags.Yes}, nil
}

func runBuildSave(ctx *Context, args []string) error {
	a, err := parseSaveArgs(args)
	if err != nil {
		return err
	}
	root, err := ctx.Project()
	if err != nil {
		return err
	}
	// The whole-catalog rule first, as for every catalog command.
	if _, err := ctx.Catalog(); err != nil {
		return err
	}
	res, err := ctx.Scan()
	if err != nil {
		return err
	}
	data, err := drafts.Get(root.Primary, a.id)
	if errors.Is(err, drafts.ErrNoDraft) {
		return fail.Rejectedf(`no draft "%s"`, a.id)
	}
	if err != nil {
		return err
	}
	if err := checkSavable(res, a.id, data, root.Primary); err != nil {
		return err
	}
	content, err := drafts.SaveContent(data)
	if err != nil {
		return fail.Rejectedf(`draft "%s" cannot be saved: write its status as the plain word draft, or in quotes`, a.id)
	}

	io.WriteString(ctx.Sys.Stdout, drafts.Show(content))
	terminal := ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal()
	switch (adopt.Confirm{Yes: a.yes, Terminal: terminal}).Before() {
	case adopt.DecideRefuse:
		return fail.Rejected("saving a build needs the owner: read the file above, then run it again with --yes, or in a terminal to answer the prompt; nothing saved")
	case adopt.DecideAsk:
		if !askOwner(ctx, a.id) {
			return fail.Rejected("not saved")
		}
	}

	if err := drafts.Save(root.Primary, a.id, content); err != nil {
		if errors.Is(err, drafts.ErrExists) {
			return neverOverwrite(a.id)
		}
		return err
	}
	ctx.Println("saved " + a.id + ": " + catalog.BuildsDir + "/" + a.id + drafts.ExtensionYAML + " (status approved); the draft is removed")
	return nil
}

// neverOverwrite is the rejection for a target that exists.
func neverOverwrite(id string) error {
	return fail.Rejectedf("%s/%s%s already exists: save never overwrites a build", catalog.BuildsDir, id, drafts.ExtensionYAML)
}

// checkSavable re-checks the draft data of id as it would be written: it is a
// valid draft of this project now (the allowlists may have changed), its id is
// its file name, nothing is at jitso/builds/<id>.yaml, and no saved build has
// the id.
func checkSavable(res *catalog.Result, id string, data []byte, primary string) error {
	cand, f := res.Candidate(data)
	problems := append([]string(nil), f.Problems...)
	if b := f.Build(); b != nil {
		problems = append(problems, launch.Problems(b, cand.Tolerant())...)
	}
	if f.ID != "" && f.ID != id {
		problems = append(problems, `the id "`+f.ID+`" must be the file name "`+id+`"`)
	}
	if len(problems) > 0 {
		return fail.Rejectedf(`draft "%s" is invalid: %s`, id, strings.Join(problems, "; "))
	}
	// Lstat, so a dangling symlink in the way counts as a file too.
	if _, err := os.Lstat(drafts.BuildFile(primary, id)); err == nil {
		return neverOverwrite(id)
	}
	if saved := res.Saved(id); saved != nil {
		return fail.Rejectedf(`build "%s" already exists (%s %s): a draft needs an id no build has`, id, saved.Layer, saved.Display)
	}
	return nil
}

// askOwner asks the person at the terminal whether to save and reads one line,
// which adopt.Reply judges: y or yes, in any case, is a yes; anything else, and
// the end of input, is a no.
func askOwner(ctx *Context, id string) bool {
	io.WriteString(ctx.Sys.Stdout, "Save it as "+catalog.BuildsDir+"/"+id+drafts.ExtensionYAML+" with status approved? [y/N] ")
	if ctx.Sys.Stdin == nil {
		return false
	}
	line, err := bufio.NewReader(ctx.Sys.Stdin).ReadString('\n')
	return adopt.Reply(line, err != nil) == adopt.DecideApply
}
