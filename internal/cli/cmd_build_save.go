package cli

import (
	"bufio"
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
// removed. It is the one way a build becomes approved, so it is the owner's:
//
//   - it shows the content it would write, every line prefixed with `+ `;
//   - on a terminal it asks `[y/N]`; anything but y or yes is `not saved`;
//   - without a terminal it refuses and prints the token of the content, the
//     first 12 hex characters of its SHA-256, which `--confirm <token>` must
//     repeat: the OK is bound to the bytes that were read;
//   - it never overwrites a file of jitso/builds/ and never takes an id a saved
//     build already has;
//   - the draft is checked again against the current allowlists first.
//
// The agent boundary is not this command but the launch guard, which denies
// every Bash segment that runs it (hook.VerbBuildSave): an agent can compute a
// token. Exit 0 when saved, 2 for a bad argument, a rejected draft, a missing
// OK or a wrong token, 3 for the environment.
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "save",
		Order: 50,
		Usage: "save <id> [--confirm <token>]  turn a draft into an approved build (the owner only)",
		Run:   runBuildSave,
	})
}

// saveArgs are the words after `build save`.
type saveArgs struct {
	id      string
	confirm string
	// confirmed is true when --confirm was given.
	confirmed bool
}

// parseSaveArgs reads `<id> [--confirm <token>]`. A later --confirm replaces an
// earlier one, as a later value of a launch flag does. The id is checked last,
// so it is never turned into a path unless it is kebab-case.
func parseSaveArgs(args []string) (saveArgs, error) {
	var a saveArgs
	haveID := false
	for i := 0; i < len(args); i++ {
		switch w := args[i]; {
		case w == "--confirm":
			if i+1 >= len(args) {
				return saveArgs{}, fail.Rejected("--confirm needs a value")
			}
			i++
			a.confirm, a.confirmed = args[i], true
		case haveID || strings.HasPrefix(w, "-"):
			return saveArgs{}, fail.Rejectedf(`unknown argument "%s"`, w)
		default:
			a.id, haveID = w, true
		}
	}
	switch {
	case !haveID:
		return saveArgs{}, fail.Rejected("build save needs a draft id")
	case !drafts.ValidID(a.id):
		return saveArgs{}, fail.Rejected("build id must be kebab-case")
	}
	return a, nil
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

	token := drafts.Token(content)
	io.WriteString(ctx.Sys.Stdout, drafts.Show(content))
	switch {
	case a.confirmed:
		if a.confirm != token {
			return fail.Rejectedf("--confirm %s does not match the draft (it changed, or the token is wrong); nothing saved", a.confirm)
		}
	case ctx.Sys.StdinIsTerminal != nil && ctx.Sys.StdinIsTerminal():
		if !askOwner(ctx, a.id) {
			return fail.Rejected("not saved")
		}
	default:
		return fail.Rejectedf(`saving a build needs the owner: run "jitso build save %s" in a terminal, or "jitso build save %s --confirm %s" after reading the file above`, a.id, a.id, token)
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

// askOwner asks the person at the terminal whether to save and reads one line:
// y or yes, in any case, is a yes; anything else, and the end of input, is a
// no.
func askOwner(ctx *Context, id string) bool {
	io.WriteString(ctx.Sys.Stdout, "Save it as "+catalog.BuildsDir+"/"+id+drafts.ExtensionYAML+" with status approved? [y/N] ")
	if ctx.Sys.Stdin == nil {
		return false
	}
	line, _ := bufio.NewReader(ctx.Sys.Stdin).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}
