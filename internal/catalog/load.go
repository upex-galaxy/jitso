// Package catalog loads and validates a project's builds: the allowlists in
// jitso/jitso.yaml, the builds in jitso/builds/*.yaml, and the route ids that
// the front matter of .agents/instructions/agent-*.md declares.
//
// Every read goes through an io/fs.FS rooted at the primary checkout
// (os.DirFS in the CLI, fstest.MapFS in tests).
package catalog

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

// Paths inside the project.
const (
	ConfigPath = "jitso/jitso.yaml"
	BuildsDir  = "jitso/builds"
	// DraftsDir is the folder of the draft layer, from the primary checkout.
	DraftsDir       = ".jitso/drafts"
	InstructionsDir = ".agents/instructions"
	buildsExtension = ".yaml"
)

// Catalog is a loaded catalog: the builds of the project and its drafts that
// are valid for the project that loaded it.
type Catalog struct {
	Config Config
	// Builds are the build of every id that is valid here: for each id the file
	// of the nearest layer. They come in layer order (the project, then its
	// drafts) and by file name inside a layer.
	// Every tier and status is here.
	Builds []Build
	// RouteIDs are the valid route-scope ids, in file name order.
	RouteIDs []string
	// Skipped are the files that are invalid for this project and so are no
	// build here: the draft files for Catalog, any file for Tolerant. A file is
	// listed when it is the build of its id (the nearest file with that id) or
	// has no id; a farther file that a nearer one shadows is not used and not
	// listed.
	Skipped []Skipped
}

// Skipped is a file that is invalid for the launching project and is left out.
type Skipped struct {
	// ID is the file's id, "" when it has none.
	ID      string
	Layer   Layer
	Display string
	// Problems are what is wrong with it here.
	Problems []string
}

// Warning is the line `build list` prints on stderr for the skipped file:
// `jitso: warning: skipped build <layer> <display path>: <problems joined by "; ">`.
func (s Skipped) Warning() string {
	return "jitso: warning: skipped build " + string(s.Layer) + " " + s.Display + ": " + strings.Join(s.Problems, "; ")
}

// Find is the build with the given id: the file of the nearest layer that has
// it. An id whose nearest file is invalid here has no build (see Invalid).
func (c *Catalog) Find(id string) (*Build, bool) {
	for i := range c.Builds {
		if c.Builds[i].ID == id {
			return &c.Builds[i], true
		}
	}
	return nil, false
}

// Invalid is the skipped file that is the build of id: the id is unusable in
// this project, and farther files with the same id are not used.
func (c *Catalog) Invalid(id string) (*Skipped, bool) {
	if id == "" {
		return nil, false
	}
	for i := range c.Skipped {
		if c.Skipped[i].ID == id {
			return &c.Skipped[i], true
		}
	}
	return nil, false
}

// Launchable are the builds `jitso launch` may start: approved and not subagent tier, in catalog order.
func (c *Catalog) Launchable() []Build {
	var out []Build
	for _, b := range c.Builds {
		if b.Launchable() {
			out = append(out, b)
		}
	}
	return out
}

// SubagentsOf resolves the subagents b lists, in its order, with the same
// lookup as every build. Load already checked that each name is a
// subagent-tier build.
func (c *Catalog) SubagentsOf(b *Build) []*Build {
	out := make([]*Build, 0, len(b.Subagents))
	for _, name := range b.Subagents {
		if s, ok := c.Find(name); ok {
			out = append(out, s)
		}
	}
	return out
}

var routeFile = regexp.MustCompile(`^agent-.+\.md$`)
var routeIDLine = regexp.MustCompile(`(?m)^id:\s*(\S+)`)

// RouteIDs reads the route-scope ids: the first `id:` line of each
// .agents/instructions/agent-*.md file, in file name order. A project with no
// such folder has no route ids: route_scope is optional.
func RouteIDs(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, InstructionsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if !routeFile.MatchString(e.Name()) {
			continue
		}
		data, err := fs.ReadFile(fsys, path.Join(InstructionsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		if m := routeIDLine.FindSubmatch(data); m != nil {
			ids = append(ids, string(m[1]))
		}
	}
	return ids, nil
}

// LoadConfig reads jitso/jitso.yaml. A project without the file has every
// allowlist open.
func LoadConfig(fsys fs.FS) (Config, error) {
	data, err := fs.ReadFile(fsys, ConfigPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{Version: 1}, nil
		}
		return Config{}, err
	}
	raw, err := decodeYAML(data)
	if err != nil {
		return Config{}, &FileError{File: ConfigPath, Problems: []string{err.Error()}}
	}
	cfg, problems, ok := configFrom(raw)
	if !ok {
		return Config{}, &FileError{File: ConfigPath, Problems: []string{"the top level is not a mapping"}}
	}
	if len(problems) > 0 {
		return Config{}, &FileError{File: ConfigPath, Problems: problems}
	}
	return cfg, nil
}

// FileError is an invalid catalog file. Its text is
// `build <display path> is invalid: <problems joined by "; ">`, for example
// `build jitso/builds/broken.yaml is invalid: ...`.
type FileError struct {
	// File is the path of the file from the project root (for a build,
	// jitso/builds/<name>), or ConfigPath.
	File     string
	Problems []string
}

func (e *FileError) Error() string {
	if e.File == ConfigPath {
		return e.File + " is invalid: " + strings.Join(e.Problems, "; ")
	}
	return "build " + e.File + " is invalid: " + strings.Join(e.Problems, "; ")
}

// Load reads the catalog of the project's own builds only (fsys is its
// primary checkout), with no draft layer.
func Load(fsys fs.FS) (*Catalog, error) {
	return LoadLayers(fsys, "")
}

// LoadLayers reads the catalog of a project. fsys is its primary checkout and
// primary the real path of that checkout; the layers are computed from it (see
// Sources). An empty primary loads the project layer only.
//
// The project's own files keep the whole-catalog rule: it fails on the first
// invalid build file in file name order (a rule kept from the first implementation), then on the first
// build whose subagents do not all name a subagent-tier build, so one invalid
// file fails every command that loads it. An invalid draft file is skipped
// instead (Catalog.Skipped): a draft in progress must not break every command.
func LoadLayers(fsys fs.FS, primary string) (*Catalog, error) {
	res, err := Scan(fsys, primary)
	if err != nil {
		return nil, err
	}
	return res.Catalog()
}

// Catalog turns the scan into the catalog of the project, or the error of its
// first invalid file.
func (r *Result) Catalog() (*Catalog, error) {
	for _, f := range r.Files {
		if f.Layer == LayerRepo && f.loadProblems > 0 {
			return nil, &FileError{File: f.Display, Problems: f.Problems[:f.loadProblems]}
		}
	}
	for _, f := range r.Files {
		if f.Layer == LayerRepo && len(f.Problems) > f.loadProblems {
			return nil, &FileError{File: f.Display, Problems: f.Problems[f.loadProblems:]}
		}
	}
	return r.Tolerant(), nil
}

// Tolerant is the catalog of the scan with no whole-catalog rule: the winner
// of every id that is valid here is a build, and any other winner is skipped,
// whatever its layer. `build validate` uses it to resolve the subagents a file
// lists while it reports the invalid files itself; Catalog is Tolerant after
// the project's own files passed.
func (r *Result) Tolerant() *Catalog {
	cat := &Catalog{Config: r.Config, RouteIDs: r.RouteIDs}
	for _, f := range r.Files {
		if f.ShadowedBy != nil {
			continue
		}
		if !f.Valid() {
			cat.Skipped = append(cat.Skipped, Skipped{ID: f.ID, Layer: f.Layer, Display: f.Display, Problems: f.Problems})
			continue
		}
		cat.Builds = append(cat.Builds, *f.Build())
	}
	return cat
}
