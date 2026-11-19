package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/launch"
)

// build validate checks every build file of the project against this project's
// allowlists and route ids, and against the rules a launch enforces
// (launch.Problems). It never stops at the first problem and never fails
// because a build file is invalid: an invalid file of the project itself is a
// finding here (exit 2), where every other catalog command exits 3.
//
//	build validate        one line per file, then `checked <n> builds: <a> ok, <b> invalid`
//	build validate <id>   the winner of that id, its problems and the files it shadows
//
// Exit 0 when every counted file is ok, 2 when one is invalid or the id is
// unknown, 3 for the environment (no jitso/jitso.yaml, bad user config,
// outside a git checkout, an unreadable folder).
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "validate",
		Order: 20,
		Usage: "validate [<id>]                check every build, or one, against this project's allowlists",
		Run:   runBuildValidate,
	})
}

func runBuildValidate(ctx *Context, args []string) error {
	id := ""
	for i, a := range args {
		if i > 0 || strings.HasPrefix(a, "-") {
			return fail.Rejected(`unknown argument "` + a + `"`)
		}
		id = a
	}
	root, err := ctx.Project()
	if err != nil {
		return err
	}
	res, err := catalog.ScanHome(os.DirFS(root.Primary), root.Primary, ctx.Home())
	if err != nil {
		return err
	}
	cat := res.Tolerant()
	lines, invalid, err := validateReport(res, id, func(f *catalog.File) []string {
		b := f.Build()
		if b == nil {
			return nil
		}
		return launch.Problems(b, cat)
	})
	if err != nil {
		return err
	}
	for _, l := range lines {
		ctx.Println(l)
	}
	if invalid > 0 {
		return fail.Status(fail.CodeRejected)
	}
	return nil
}

// validateReport is the output of `build validate`: the lines to print and
// the number of counted files that are invalid. With an id it is the winner
// of that id and the files it shadows, and only the winner is counted; an id
// no file has is a rejection. launchProblems returns the problems that only a
// launch sees for a file that is valid on its own (nil for the others); they
// follow the file's own problems.
func validateReport(res *catalog.Result, id string, launchProblems func(*catalog.File) []string) ([]string, int, error) {
	files := res.Files
	counted := len(files)
	var winner *catalog.File
	if id != "" {
		winner = res.Winner(id)
		if winner == nil {
			return nil, 0, fail.Rejectedf(`unknown build "%s"`, id)
		}
		files = nil
		for _, f := range res.Files {
			if f == winner || f.ShadowedBy == winner {
				files = append(files, f)
			}
		}
		counted = 1
	}
	var lines []string
	ok, invalid := 0, 0
	for _, f := range files {
		problems := append(append([]string(nil), f.Problems...), launchProblems(f)...)
		status, name := "ok", f.ID
		if len(problems) > 0 {
			status = "invalid"
		}
		if name == "" {
			name = "-"
		}
		line := fmt.Sprintf("%s %s %s %s", status, name, f.Layer, f.Display)
		if f.ShadowedBy != nil {
			line += fmt.Sprintf(" (shadowed by %s %s)", f.ShadowedBy.Layer, f.ShadowedBy.Display)
		}
		lines = append(lines, line)
		for _, p := range problems {
			lines = append(lines, "  "+p)
		}
		if id != "" && f != winner {
			continue
		}
		if len(problems) > 0 {
			invalid++
		} else {
			ok++
		}
	}
	lines = append(lines, fmt.Sprintf("checked %d builds: %d ok, %d invalid", counted, ok, invalid))
	return lines, invalid, nil
}
