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
// the catalog, so a new draft never breaks the project; the owner promotes it
// with `build save`. Nothing is written to jitso/builds/.
//
// In a fresh project (no jitso/jitso.yaml and no *.yaml in jitso/builds/) it
// first writes the jitso/jitso.yaml skeleton with empty allowlists. A project
// that has builds and no allowlists is broken, not new: that is the missing
// allowlists error, exit 3, like every catalog command. It never overwrites,
// and refuses the id of a saved build and the id of an existing
// draft, before anything is written.
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
	fresh := isFreshProject(root.Primary)
	var res *catalog.Result
	if fresh {
		// No allowlists to validate against yet: the folders are scanned only
		// for the ids they hold.
		res, err = scanIDs(root.Primary)
	} else {
		// The whole-catalog rule, and the missing allowlists error.
		if _, err = ctx.Catalog(); err == nil {
			res, err = ctx.Scan()
		}
	}
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

	if fresh {
		if err := writeSkeleton(root.Primary); err != nil {
			return err
		}
		ctx.Println("wrote " + catalog.ConfigPath + " (empty allowlists: fill them first)")
	}
	if err := drafts.Put(root.Primary, id, []byte(text)); err != nil {
		return err
	}
	ctx.Println("wrote " + catalog.DraftsDir + "/" + id + drafts.ExtensionYAML + " (tier " + tier + ", status draft): fill every empty value, check it with jitso build validate " + id + ", and ask the owner to run jitso build save " + id)
	return nil
}

// isFreshProject reports whether the project has neither jitso/jitso.yaml nor
// a build file in jitso/builds/: the one case where build new writes the
// allowlists skeleton.
func isFreshProject(primary string) bool {
	if _, err := os.Stat(filepath.Join(primary, filepath.FromSlash(catalog.ConfigPath))); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	entries, err := os.ReadDir(filepath.Join(primary, filepath.FromSlash(catalog.BuildsDir)))
	if err != nil {
		return errors.Is(err, fs.ErrNotExist)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), drafts.ExtensionYAML) {
			return false
		}
	}
	return true
}

// scanIDs scans the build folders of the project with no allowlists, which is enough
// to know which ids its saved builds hold.
func scanIDs(primary string) (*catalog.Result, error) {
	return catalog.ScanSources(catalog.Config{}, nil, catalog.Sources(primary))
}

// writeSkeleton writes jitso/jitso.yaml from the template, never over a file
// that is there.
func writeSkeleton(primary string) error {
	path := filepath.Join(primary, filepath.FromSlash(catalog.ConfigPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(templates.Config()); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	return f.Close()
}
