package catalog

import (
	"io/fs"
	"os"
	"path/filepath"
)

// Layer names where a build file lives. Builds come from two layers, nearest
// first (docs/decisions.md, build layers and the v0.1.0 cut): the project's
// own jitso/builds folder, then the draft layer last: a draft never shadows a
// saved build. The lookup treats the order of the Source list as the order of
// precedence.
type Layer string

// The layer words, as shown in build list, the card and the registry.
const (
	LayerRepo Layer = "repo"
	// LayerDraft is <primary>/.jitso/drafts, the just-in-time builds written
	// by `jitso build draft`. It is the only layer where status is draft.
	LayerDraft Layer = "draft"
)

// Source is one folder of builds.
type Source struct {
	Layer Layer
	// Dir is the absolute folder holding the *.yaml files. It is built from
	// real paths (the primary's symlinks are resolved).
	Dir string
	// Display is the folder as people see it: jitso/builds for the project,
	// .jitso/drafts for the drafts.
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
// the real path primary, nearest first: the project, then its drafts folder.
// Every folder is computed from the primary's real path, so a linked worktree
// gets the layers of its primary checkout. The draft layer always exists (a
// missing folder is an empty layer).
func Sources(primary string) []Source {
	return []Source{
		{Layer: LayerRepo, Dir: filepath.Join(primary, filepath.FromSlash(BuildsDir)), Display: BuildsDir},
		{Layer: LayerDraft, Dir: filepath.Join(primary, filepath.FromSlash(DraftsDir)), Display: DraftsDir},
	}
}

// realPath is the path with its symlinks resolved, or the path itself when it
// cannot be resolved (it does not exist).
func realPath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}
