package cli

import (
	"errors"
	"strconv"
	"strings"

	"github.com/upex-galaxy/jitso/internal/drafts"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// build discard deletes drafts (defect N-15). Drafts are local scratch, so the
// command needs the project and nothing else: it does not load the catalog and
// is not guarded.
//
//	build discard <id>   delete .jitso/drafts/<id>.yaml; `discarded <id>`
//	build discard --all  delete every *.yaml of the folder; `discarded <n> drafts`
//
// The arguments are checked before anything is deleted, and an id is never
// turned into a path unless it is kebab-case. Exit 0, 2 for a bad argument or
// an id with no draft, 3 for the environment.
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "discard",
		Order: 60,
		Usage: "discard <id>|--all             delete a draft, or every draft",
		Run:   runBuildDiscard,
	})
}

// discardTarget parses the arguments: the id to discard, or all.
func discardTarget(args []string) (id string, all bool, err error) {
	have := false
	for _, a := range args {
		if have {
			return "", false, fail.Rejectedf(`unknown argument "%s"`, a)
		}
		switch {
		case a == "--all":
			all = true
		case strings.HasPrefix(a, "-"):
			return "", false, fail.Rejectedf(`unknown argument "%s"`, a)
		default:
			id = a
		}
		have = true
	}
	switch {
	case !have:
		return "", false, fail.Rejected("build discard needs a draft id or --all")
	case !all && !drafts.ValidID(id):
		return "", false, fail.Rejected("build id must be kebab-case")
	}
	return id, all, nil
}

func runBuildDiscard(ctx *Context, args []string) error {
	id, all, err := discardTarget(args)
	if err != nil {
		return err
	}
	root, err := ctx.HomeProject()
	if err != nil {
		return err
	}
	if all {
		n, err := drafts.RemoveAll(root.Primary)
		if err != nil {
			return err
		}
		ctx.Println("discarded " + strconv.Itoa(n) + " drafts")
		return nil
	}
	if err := drafts.Remove(root.Primary, id); err != nil {
		if errors.Is(err, drafts.ErrNoDraft) {
			return fail.Rejectedf(`no draft "%s"`, id)
		}
		return err
	}
	ctx.Println("discarded " + id)
	return nil
}
