package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsconv"
)

// File is one build file of one layer, with everything a scan learned about it.
type File struct {
	Layer Layer
	// Name is the file name, for example haiku-explorer.yaml.
	Name string
	// Display is the file as people see it: jitso/builds/<name> for the
	// project, .jitso/drafts/<name> for a draft.
	Display string
	// Path is the real absolute path of the file.
	Path string
	// SHA256 is the hex SHA-256 of the file's bytes, as read.
	SHA256 string
	// ID is the file's id: the value of `id` when the file decodes to a mapping
	// with a kebab-case string id, "" otherwise (such a file shadows nothing).
	ID string
	// Raw is the decoded file, nil when it does not decode to a mapping.
	Raw *Map
	// Problems are everything wrong with the file for the launching project:
	// the rules of ValidateBuild, then the subagent names that do not resolve.
	// Empty means the file is valid here.
	Problems []string
	// ShadowedBy is the file that has the same id in a nearer layer, or earlier
	// in the same layer, and so is the build of that id. Nil for the winner of
	// an id and for a file without an id.
	ShadowedBy *File

	// loadProblems is the number of leading Problems that come from decoding
	// and ValidateBuild; Load reports those of the project's files first.
	loadProblems int
}

// Valid is true when the file has no problem here.
func (f *File) Valid() bool { return len(f.Problems) == 0 }

// Build is the file as a build, with its layer, display path, real path and
// hash. It is nil for a file that is not valid here: only a valid file is a
// build, whether or not a nearer file shadows it.
func (f *File) Build() *Build {
	if !f.Valid() || f.Raw == nil {
		return nil
	}
	b := buildFrom(f.Name, f.Raw)
	b.Layer, b.Display, b.Path, b.SHA256 = f.Layer, f.Display, f.Path, f.SHA256
	// A draft with no status key is a draft.
	if _, ok := f.Raw.Get("status"); !ok && f.Layer == LayerDraft {
		b.Status = StatusDraft
	}
	return &b
}

// Result is a tolerant scan of every layer.
type Result struct {
	Config   Config
	RouteIDs []string
	// Sources are the folders scanned, nearest first.
	Sources []Source
	// Files are every build file of every source, in source order and, inside
	// a source, by file name.
	Files []*File
}

// Winner is the file that is the build of id: the first file with that id in
// source order, valid or not. Nil when no file has it.
func (r *Result) Winner(id string) *File {
	for _, f := range r.Files {
		if f.ID == id {
			return f
		}
	}
	return nil
}

// Saved is the file that is the build of id among the saved builds (the
// project's own jitso/builds), valid or not: the first file with that id in
// source order that is not a draft. Nil when no saved file has
// it. A draft may not take an id that Saved returns.
func (r *Result) Saved(id string) *File {
	if id == "" {
		return nil
	}
	for _, f := range r.Files {
		if f.ID == id && f.Layer != LayerDraft {
			return f
		}
	}
	return nil
}

// Drafts are the files of the draft layer, by file name.
func (r *Result) Drafts() []*File {
	var out []*File
	for _, f := range r.Files {
		if f.Layer == LayerDraft {
			out = append(out, f)
		}
	}
	return out
}

// Scan reads the allowlists and the route ids of the project (fsys is its
// primary checkout) and every build file of every layer, and validates each
// file against the project's allowlists and route ids: a build is valid for
// the project that launches it, not for the folder it lives in. It never
// fails because of an invalid build file; Load and `build validate` decide
// what an invalid file means for each layer.
//
// primary is the real path of the primary checkout, from which the layers are
// computed. An empty primary means the project layer only (tests).
func Scan(fsys fs.FS, primary string) (*Result, error) {
	return ScanHome(fsys, primary, "")
}

// ScanHome is Scan with the user layer of home: user:<name> items of a build
// resolve in its ~/.jitso/jitso.yaml. An empty home is no user layer. An
// invalid user file fails the scan only when a build file names a user
// item; every other scan ignores it.
func ScanHome(fsys fs.FS, primary, home string) (*Result, error) {
	cfg, err := LoadConfig(fsys)
	if err != nil {
		return nil, err
	}
	cfg.User = LoadUser(home)
	cfg.ProjectSkillDirs = projectSkillDirs(fsys)
	cfg.ProjectServers = projectServers(fsys)
	ids, err := RouteIDs(fsys)
	if err != nil {
		return nil, err
	}
	var sources []Source
	if primary == "" {
		sources = []Source{{Layer: LayerRepo, Dir: filepath.FromSlash(BuildsDir), Display: BuildsDir}}
	} else {
		sources = Sources(primary)
	}
	sources[0].fsys = subFS(fsys, BuildsDir)
	res, err := ScanSources(cfg, ids, sources)
	if err != nil {
		return nil, err
	}
	if cfg.User.Err != nil {
		for _, f := range res.Files {
			if f.Raw != nil && namesUser(f.Raw) {
				return nil, cfg.User.Err
			}
		}
	}
	return res, nil
}

// ScanSources is Scan over given folders: the pure part, with the allowlists
// and the route ids already read.
func ScanSources(cfg Config, routeIDs []string, sources []Source) (*Result, error) {
	res := &Result{Config: cfg, RouteIDs: routeIDs, Sources: sources}
	for _, src := range sources {
		files, err := scanSource(src, cfg, routeIDs)
		if err != nil {
			return nil, err
		}
		res.Files = append(res.Files, files...)
	}
	res.resolve()
	return res, nil
}

// resolve decides which file is the build of each id (the first file of each id
// in source order shadows the others) and resolves the subagent names of every
// file that is valid so far. It runs once over files whose Problems hold only
// their load problems.
func (r *Result) resolve() {
	winners := map[string]*File{}
	for _, f := range r.Files {
		if f.ID == "" {
			continue
		}
		if w, ok := winners[f.ID]; ok {
			f.ShadowedBy = w
		} else {
			winners[f.ID] = f
		}
	}
	// Subagent names resolve with the same lookup as every build: the winner
	// of the name, which must itself be valid here and subagent tier.
	for _, f := range r.Files {
		if f.Raw == nil || len(f.Problems) > 0 {
			continue
		}
		for _, name := range jsconv.Strings(f.Raw.Value("subagents")) {
			if w, ok := winners[name]; !ok || w.loadProblems > 0 || tierOf(w.Raw) != TierSubagent {
				f.Problems = append(f.Problems, `subagent "`+name+`" is not a subagent-tier build`)
			}
		}
	}
}

// Candidate is the scan as it would be once a draft holding data is written to
// the draft layer, and that draft's file: the draft of the same id (the same
// file name) is replaced, every other file is as scanned, and the new file is
// validated like every draft (the draft status rule, the allowlists, the
// subagent names resolved through the same lookup). The receiver is not
// changed. A draft that has no valid id has no file name: its Name is "" and
// it shadows nothing.
//
// It is the pure half of `jitso build draft`: the caller adds the rules only a
// launch sees and refuses an id a saved build already has.
func (r *Result) Candidate(data []byte) (*Result, *File) {
	f := &File{Layer: LayerDraft}
	fillFile(f, data, r.Config, r.RouteIDs)
	display, dir := DraftsDir, ""
	for _, src := range r.Sources {
		if src.Layer == LayerDraft {
			display, dir = src.Display, src.Dir
		}
	}
	if f.ID != "" {
		f.Name = f.ID + buildsExtension
		f.Display = display + "/" + f.Name
		f.Path = realPath(filepath.Join(dir, f.Name))
	}

	out := &Result{Config: r.Config, RouteIDs: r.RouteIDs, Sources: r.Sources}
	at := -1
	for _, old := range r.Files {
		if old.Layer == LayerDraft && f.Name != "" && old.Name == f.Name {
			continue
		}
		c := *old
		c.Problems = append([]string(nil), old.Problems[:old.loadProblems]...)
		c.ShadowedBy = nil
		out.Files = append(out.Files, &c)
		if c.Layer == LayerDraft && (f.Name == "" || c.Name < f.Name) {
			at = len(out.Files)
		}
	}
	// The new file goes by name among the drafts, after every saved layer.
	if at < 0 {
		at = len(out.Files)
		for i, c := range out.Files {
			if c.Layer == LayerDraft {
				at = i
				break
			}
		}
	}
	out.Files = slices.Insert(out.Files, at, f)
	out.resolve()
	return out, f
}

// fillFile reads data into f: its hash, then decoding and the rules of
// ValidateBuildIn for the file's layer.
func fillFile(f *File, data []byte, cfg Config, routeIDs []string) {
	sum := sha256.Sum256(data)
	f.SHA256 = hex.EncodeToString(sum[:])
	raw, err := decodeYAML(data)
	if err != nil {
		f.Problems = []string{err.Error()}
	} else {
		f.Problems = ValidateBuildIn(raw, cfg, routeIDs, f.Layer)
		if m, ok := raw.(*Map); ok {
			f.Raw = m
			if id, ok := m.Value("id").(string); ok && kebab.MatchString(id) {
				f.ID = id
			}
		}
	}
	f.loadProblems = len(f.Problems)
}

// scanSource reads, decodes and validates the *.yaml files of one folder, by
// file name. A missing folder is an empty layer. Any other read error of the
// folder fails the scan; an unreadable or undecodable file is a problem of
// that file.
func scanSource(src Source, cfg Config, routeIDs []string) ([]*File, error) {
	fsys := src.open()
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), buildsExtension) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	files := make([]*File, 0, len(names))
	for _, name := range names {
		f := &File{Layer: src.Layer, Name: name, Display: src.displayFile(name), Path: realPath(filepath.Join(src.Dir, name))}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			f.Problems = []string{"cannot read the file: " + err.Error()}
			f.loadProblems = len(f.Problems)
			files = append(files, f)
			continue
		}
		fillFile(f, data, cfg, routeIDs)
		files = append(files, f)
	}
	return files, nil
}

// subFS is the folder dir of fsys. A folder that does not exist is an empty
// layer: reading it reports fs.ErrNotExist.
func subFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, path.Clean(dir))
	if err != nil {
		return emptyFS{}
	}
	return sub
}

// emptyFS is a file system with nothing in it.
type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
