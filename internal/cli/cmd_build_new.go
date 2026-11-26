package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/drafts"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/templates"
)

// build new starts a build from a template (defect N-14): it writes a DRAFT,
// .jitso/drafts/<id>.yaml, from the commented template of the tier (worker by
// default), with `id: <id>` filled and every allowlist-bound value empty. The
// draft is invalid until it is filled in, and an invalid draft is skipped by
// the catalog, so a new draft never breaks the project; `build save` keeps it.
// Nothing is written to jitso/builds/.
//
// In a project without jitso/jitso.yaml it also writes the skeleton, which
// restricts nothing. It never overwrites, and refuses the id of a saved build
// and the id of an existing draft, before anything is written.
//
//	build new <id> [--tier worker|conductor|subagent]
//
// Exit 0, 2 for a bad argument or a used id, 3 for the environment.
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "new",
		Order: 30,
		Usage: "new <id> [--tier <tier>]       start a draft from a template in .jitso/drafts/<id>.yaml",
		Run:   runBuildNew,
	})
}

// newTier is the tier of a new build when --tier is not given.
const newTier = "worker"

// parseNewArgs reads `<id> [--tier <tier>]`: the id is checked first, then the
// tier, so a bad word is rejected before the project is read.
func parseNewArgs(args []string) (id, tier string, err error) {
	tier = newTier
	haveID := false
	for i := 0; i < len(args); i++ {
		switch w := args[i]; {
		case w == "--tier":
			if i+1 >= len(args) {
				return "", "", fail.Rejected("--tier needs a value")
			}
			i++
			tier = args[i]
		case haveID || strings.HasPrefix(w, "-"):
			return "", "", fail.Rejectedf(`unknown argument "%s"`, w)
		default:
			id, haveID = w, true
		}
	}
	switch {
	case !haveID:
		return "", "", fail.Rejected("build new needs a build id")
	case !drafts.ValidID(id):
		return "", "", fail.Rejected("build id must be kebab-case")
	case !slices.Contains(templates.Tiers, tier):
		return "", "", fail.Rejected("--tier must be worker, conductor or subagent")
	}
	return id, tier, nil
}

func runBuildNew(ctx *Context, args []string) error {
	id, tier, err := parseNewArgs(args)
	if err != nil {
		return err
	}
	root, err := ctx.Project()
	if err != nil {
		return err
	}
	// The whole-catalog rule, as for every catalog command.
	if _, err := ctx.Catalog(); err != nil {
		return err
	}
	res, err := ctx.Scan()
	if err != nil {
		return err
	}
	if saved := res.Saved(id); saved != nil {
		return fail.Rejectedf(`build "%s" already exists (%s %s): a draft needs an id no build has`, id, saved.Layer, saved.Display)
	}
	if _, err := os.Lstat(drafts.File(root.Primary, id)); err == nil {
		return fail.Rejectedf(`draft "%s" already exists: edit %s/%s%s or discard it`, id, catalog.DraftsDir, id, drafts.ExtensionYAML)
	}
	text, err := templates.Build(tier, id)
	if err != nil {
		return err
	}

	wrote, err := writeSkeleton(root.Primary)
	if err != nil {
		return err
	}
	if wrote {
		ctx.Println("wrote " + catalog.ConfigPath + " (every allowlist is open: add a list there to restrict builds)")
	}
	if err := drafts.Put(root.Primary, id, []byte(text)); err != nil {
		return err
	}
	ctx.Println("wrote " + catalog.DraftsDir + "/" + id + drafts.ExtensionYAML + " (tier " + tier + ", status draft): fill every empty value and check it with jitso build validate " + id + "; jitso build save " + id + " keeps it")
	return nil
}

// writeSkeleton writes jitso/jitso.yaml from the template when nothing is at
// that path, and reports whether it did. It never writes over a file.
func writeSkeleton(primary string) (bool, error) {
	path := filepath.Join(primary, filepath.FromSlash(catalog.ConfigPath))
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false, err
	}
	if _, err := f.WriteString(templates.Config()); err != nil {
		f.Close()
		os.Remove(path)
		return false, err
	}
	return true, f.Close()
}
