package cli

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/drafts"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/launch"
)

// build draft checks a build and keeps it as a draft: the bytes of the input,
// unchanged, in .jitso/drafts/<id>.yaml (defect N-15). It writes only that
// folder. The draft is checked like a build of the draft layer: the
// allowlists and the route scope of this project, the
// rules only a launch sees (the harness needs an adapter that can honor the
// build, a listed subagent must exist and be approved or a draft), and the id
// must be one no saved build has, so a draft never shadows a saved build.
//
//	build draft <file.yaml|.yml>   a file, at most 64 KiB
//	build draft -                  standard input, at most 64 KiB
//
// Exit 0 when kept, 2 for a bad argument, an unreadable or oversized input, an
// invalid draft or a saved id, 3 for the environment (outside git, an invalid
// project file, a write error).
func init() {
	RegisterSubcommand(Subcommand{
		Name:  "draft",
		Order: 40,
		Usage: "draft <file.yaml|->            check a build and keep it as a draft in .jitso/drafts/",
		Run:   runBuildDraft,
	})
}

// draftSource is the one argument of build draft: a path, or drafts.Stdin.
func draftSource(args []string) (string, error) {
	source, have := "", false
	for _, a := range args {
		if have || (a != drafts.Stdin && strings.HasPrefix(a, "-")) {
			return "", fail.Rejectedf(`unknown argument "%s"`, a)
		}
		source, have = a, true
	}
	if !have {
		return "", fail.Rejected(drafts.InputRule)
	}
	if _, err := drafts.CheckInput(source); err != nil {
		return "", fail.Rejected(err.Error())
	}
	return source, nil
}

func runBuildDraft(ctx *Context, args []string) error {
	source, err := draftSource(args)
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
	data, name, err := readDraftInput(ctx, source)
	if err != nil {
		return err
	}

	cand, f := res.Candidate(data)
	problems := append([]string(nil), f.Problems...)
	if b := f.Build(); b != nil {
		problems = append(problems, launch.Problems(b, cand.Tolerant())...)
	}
	if f.ID != "" && !drafts.ValidID(f.ID) {
		problems = append(problems, "id must be at most 64 characters")
	}
	if len(problems) > 0 {
		return fail.Rejectedf("draft %s is invalid: %s", name, strings.Join(problems, "; "))
	}
	if saved := res.Saved(f.ID); saved != nil {
		return fail.Rejectedf(`build "%s" already exists (%s %s): a draft needs an id no build has`, f.ID, saved.Layer, saved.Display)
	}
	if err := drafts.Put(root.Primary, f.ID, data); err != nil {
		return err
	}
	ctx.Println("drafted " + f.ID + ": " + catalog.DraftsDir + "/" + f.ID + drafts.ExtensionYAML + " (launch it with --build " + f.ID + "; keep it with jitso build save " + f.ID + ")")
	return nil
}

// readDraftInput reads the input named by source (a path relative to the
// working directory, or standard input) and names it for messages: the path as
// given, or `stdin`. A file that cannot be read, and an input over the limit,
// are rejections.
func readDraftInput(ctx *Context, source string) ([]byte, string, error) {
	var in io.Reader
	name := source
	if source == drafts.Stdin {
		name = "stdin"
		in = ctx.Sys.Stdin
		if in == nil {
			in = strings.NewReader("")
		}
	} else {
		path := source
		if !filepath.IsAbs(path) {
			getwd := ctx.Sys.Getwd
			if getwd == nil {
				getwd = os.Getwd
			}
			cwd, err := getwd()
			if err != nil {
				return nil, "", err
			}
			path = filepath.Join(cwd, path)
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, "", fail.Rejectedf("cannot read %s: %s", source, reason(err))
		}
		defer f.Close()
		in = f
	}
	data, err := drafts.Read(in)
	if errors.Is(err, drafts.ErrTooLarge) {
		return nil, "", fail.Rejected(drafts.InputRule)
	}
	if err != nil {
		return nil, "", fail.Rejectedf("cannot read %s: %s", name, reason(err))
	}
	return data, name, nil
}

// reason is the cause of a file error without the operation and the path.
func reason(err error) string {
	if errors.Is(err, fs.ErrNotExist) {
		return "no such file"
	}
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
