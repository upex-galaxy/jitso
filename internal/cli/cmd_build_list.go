package cli

import (
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// build list prints one line per launchable build of the project's layered
// catalog (approved and not subagent tier), in layer order (the project, the
// trusted parents nearest first, the user folder) and by file name inside a
// layer. A build of a parent or user folder ends with ` (from <layer> <path>)`.
// A parent, user or draft file that is invalid for this project is skipped with
// one warning line on stderr. A catalog that fails to load is exit 3 with the
// file named, the same handled error launch gives (TS-ODDITIES C3: the oracle
// crashes here). A draft is never launchable, so it is never listed here.
//
// With --drafts it prints the drafts instead (defect N-15): one line per draft
// file, by file name, so a person sees what is in .jitso/drafts/ and why a
// draft would not launch. Any other argument is rejected.
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "list",
		Order: 10,
		Usage: "list [--drafts]                the launchable builds of this project, or its drafts",
		Run:   runBuildList,
	})
}

func runBuildList(ctx *Context, args []string) error {
	showDrafts := false
	for _, a := range args {
		if a != "--drafts" {
			return fail.Rejectedf(`unknown argument "%s"`, a)
		}
		showDrafts = true
	}
	cat, err := ctx.Catalog()
	if err != nil {
		return err
	}
	if showDrafts {
		res, err := ctx.Scan()
		if err != nil {
			return err
		}
		var lines []string
		for _, f := range res.Drafts() {
			lines = append(lines, catalog.DraftLine(f))
		}
		ctx.Println(strings.Join(lines, "\n"))
		return nil
	}
	for _, s := range cat.Skipped {
		ctx.Warn(s.Warning())
	}
	ctx.Println(cat.List())
	return nil
}
