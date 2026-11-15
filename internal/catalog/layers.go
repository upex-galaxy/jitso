package catalog

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/project"
)

// Layer names where a build file lives. Builds come from layers, nearest
// first (docs/decisions.md, build layers): the project's own jitso/builds
// folder, each trusted parent folder from the nearest to the farthest, then
// the user's folder, then the draft layer last: a draft never shadows a saved
// build. The lookup treats the order of the Source list as the order of
// precedence.
type Layer string

// The layer words, as shown in build list, the card and the registry.
const (
	LayerRepo   Layer = "repo"
	LayerParent Layer = "parent"
	LayerUser   Layer = "user"
	// LayerDraft is <primary>/.jitso/drafts, the just-in-time builds written
	// by `jitso build draft`. It is the only layer where status is draft.
	LayerDraft Layer = "draft"
)

// userBuildsDir is the user's builds folder inside project.UserDir.
const userBuildsDir = "builds"

// Source is one folder of builds.
type Source struct {
	Layer Layer
	// Dir is the absolute folder holding the *.yaml files. It is built from
	// real paths (the primary's and the parents' symlinks are resolved).
	Dir string
	// Display is the folder as people see it: jitso/builds for the project,
	// ~/<path>/jitso/builds for a parent under HOME (the absolute path
	// otherwise), ~/.jitso/builds for the user folder.
	Display string
	// fsys is the folder's files, rooted at Dir. Nil means os.DirFS(Dir).
	fsys fs.FS
}

// open is the folder as a file system.
func (s Source) open() fs.FS {
	if s.fsys != nil {
		return s.fsys
	}
	return os.DirFS(s.Dir)
}

// displayFile is the display path of one file of the folder.
func (s Source) displayFile(name string) string {
	return s.Display + "/" + name
}

// Sources are the folders of builds of the project whose primary checkout has
// the real path primary, nearest first: the project, each trusted parent that
// strictly contains it (the nearest parent first), the user folder, then the
// drafts folder of the project.
// Every folder is computed from the primary's real path, so a linked worktree
// gets the layers of its primary checkout. An empty home means no user layer;
// the draft layer always exists (a missing folder is an empty layer).
func Sources(primary, home string, cfg UserConfig) []Source {
	out := []Source{{Layer: LayerRepo, Dir: filepath.Join(primary, filepath.FromSlash(BuildsDir)), Display: BuildsDir}}
	for _, p := range cfg.ParentsOf(primary) {
		out = append(out, Source{
			Layer:   LayerParent,
			Dir:     filepath.Join(p, filepath.FromSlash(BuildsDir)),
			Display: path.Join(displayFolder(p, home), BuildsDir),
		})
	}
	if home != "" {
		out = append(out, Source{
			Layer:   LayerUser,
			Dir:     filepath.Join(project.UserDir(home), userBuildsDir),
			Display: "~/.jitso/" + userBuildsDir,
		})
	}
	return append(out, Source{Layer: LayerDraft, Dir: filepath.Join(primary, filepath.FromSlash(DraftsDir)), Display: DraftsDir})
}

// displayFolder is ~/<rel> for a folder strictly inside HOME and the absolute
// path (with forward slashes) for any other folder.
func displayFolder(dir, home string) string {
	if home != "" {
		for _, h := range []string{realPath(home), home} {
			if rel, ok := within(h, dir); ok {
				return "~/" + filepath.ToSlash(rel)
			}
		}
	}
	return filepath.ToSlash(dir)
}

// within reports whether child is strictly inside parent and, when it is, the
// path of child relative to parent. Both must be clean absolute paths. A
// folder is not inside itself, and a sibling that only shares a name prefix
// is not inside it either.
func within(parent, child string) (string, bool) {
	rel, err := filepath.Rel(parent, child)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// realPath is the path with its symlinks resolved, or the path itself when it
// cannot be resolved (it does not exist).
func realPath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}
