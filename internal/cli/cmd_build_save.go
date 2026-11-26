package cli

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/drafts"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/launch"
)

// build save turns a draft into a build of the project (defect N-15): the
// draft bytes with the status scalar `draft` replaced by `approved` and
// nothing else changed, written to jitso/builds/<id>.yaml, and the draft
// removed. It is the one command that makes a build approved:
//
//   - it shows the content it writes, every line prefixed with `+ `;
//   - it asks nothing and needs neither a terminal nor a flag; `--yes` is
//     accepted and changes nothing;
//   - it never overwrites a file of jitso/builds/ and never takes an id a saved
//     build already has;
//   - the draft is checked again against the current allowlists first.
//
// Exit 0 when saved, 2 for a bad argument or a draft that cannot be saved, 3
// for the environment.
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "save",
		Order: 50,
		Usage: "save <id> [--yes]  turn a draft into an approved build",
		Run:   runBuildSave,
	})
}

// parseSaveArgs reads `<id> [--yes]` and returns the id. The id is checked
// last, so it is never turned into a path unless it is kebab-case.
func parseSaveArgs(args []string) (string, error) {
	_, rest, err := ParseFlags(args, FlagYes)
	if err != nil {
		return "", err
	}
	if len(rest) == 0 {
		return "", fail.Rejected("build save needs a draft id")
	}
	if err := RejectExtra(rest[1:]); err != nil {
		return "", err
	}
	if !drafts.ValidID(rest[0]) {
		return "", fail.Rejected("build id must be kebab-case")
	}
	return rest[0], nil
}

func runBuildSave(ctx *Context, args []string) error {
	id, err := parseSaveArgs(args)
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
	data, err := drafts.Get(root.Primary, id)
	if errors.Is(err, drafts.ErrNoDraft) {
		return fail.Rejectedf(`no draft "%s"`, id)
	}
	if err != nil {
		return err
	}
	if err := checkSavable(res, id, data, root.Primary); err != nil {
		return err
	}
	content, err := drafts.SaveContent(data)
	if err != nil {
		return fail.Rejectedf(`draft "%s" cannot be saved: write its status as the plain word draft, or in quotes`, id)
	}

	io.WriteString(ctx.Sys.Stdout, drafts.Show(content))
	if err := drafts.Save(root.Primary, id, content); err != nil {
		if errors.Is(err, drafts.ErrExists) {
			return neverOverwrite(id)
		}
		return err
	}
	ctx.Println("saved " + id + ": " + catalog.BuildsDir + "/" + id + drafts.ExtensionYAML + " (status approved); the draft is removed")
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
