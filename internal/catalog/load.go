// Package catalog loads and validates a project's builds: the allowlists in
// jitso/jitso.yaml, the builds in jitso/builds/*.yaml, and the route ids that
// the front matter of .agents/instructions/agent-*.md declares.
//
// Every read goes through an io/fs.FS rooted at the primary checkout
// (os.DirFS in the CLI, fstest.MapFS in tests).
package catalog

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Paths inside the project.
const (
	ConfigPath      = "jitso/jitso.yaml"
	BuildsDir       = "jitso/builds"
	InstructionsDir = ".agents/instructions"
	buildsExtension = ".yaml"
)

// Catalog is a loaded, valid catalog.
type Catalog struct {
	Config Config
	// Builds are all builds, every tier and status, sorted by file name.
	Builds []Build
	// RouteIDs are the valid route-scope ids, in file name order.
	RouteIDs []string
}

// Find is the first build with the given id.
func (c *Catalog) Find(id string) (*Build, bool) {
	for i := range c.Builds {
		if c.Builds[i].ID == id {
			return &c.Builds[i], true
		}
	}
	return nil, false
}

// Launchable are the builds `jitso launch` may start and the dispatcher may
// offer: approved and not subagent tier, in file name order.
func (c *Catalog) Launchable() []Build {
	var out []Build
	for _, b := range c.Builds {
		if b.Launchable() {
			out = append(out, b)
		}
	}
	return out
}

// SubagentsOf resolves the subagents b lists, in its order. Load already
// checked that each name is a subagent-tier build.
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
// .agents/instructions/agent-*.md file, in file name order.
func RouteIDs(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, InstructionsDir)
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

// LoadConfig reads jitso/jitso.yaml.
func LoadConfig(fsys fs.FS) (Config, error) {
	data, err := fs.ReadFile(fsys, ConfigPath)
	if err != nil {
		return Config{}, err
	}
	raw, err := decodeYAML(data)
	if err != nil {
		return Config{}, &FileError{File: ConfigPath, Problems: []string{err.Error()}}
	}
	cfg, ok := configFrom(raw)
	if !ok {
		return Config{}, &FileError{File: ConfigPath, Problems: []string{"the top level is not a mapping"}}
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

// Load reads and validates the whole catalog. It fails on the first invalid
// build file in file name order (the oracle's rule), then on the first build
// whose subagents do not all name a subagent-tier build. The catalog is
// validated as a whole: one invalid file fails every command that loads it.
func Load(fsys fs.FS) (*Catalog, error) {
	cfg, err := LoadConfig(fsys)
	if err != nil {
		return nil, err
	}
	ids, err := RouteIDs(fsys)
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(fsys, BuildsDir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), buildsExtension) {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	cat := &Catalog{Config: cfg, RouteIDs: ids}
	for _, name := range names {
		data, err := fs.ReadFile(fsys, path.Join(BuildsDir, name))
		if err != nil {
			return nil, err
		}
		raw, err := decodeYAML(data)
		if err != nil {
			return nil, &FileError{File: path.Join(BuildsDir, name), Problems: []string{err.Error()}}
		}
		if problems := ValidateBuild(raw, cfg, ids); len(problems) > 0 {
			return nil, &FileError{File: path.Join(BuildsDir, name), Problems: problems}
		}
		cat.Builds = append(cat.Builds, buildFrom(name, raw.(*Map)))
	}
	for _, b := range cat.Builds {
		if problems := cat.subagentProblems(&b); len(problems) > 0 {
			return nil, &FileError{File: path.Join(BuildsDir, b.File), Problems: problems}
		}
	}
	return cat, nil
}

// subagentProblems checks that each name b lists is a subagent-tier build of
// this catalog (docs/decisions.md, tiers).
func (c *Catalog) subagentProblems(b *Build) []string {
	var problems []string
	for _, name := range b.Subagents {
		if s, ok := c.Find(name); !ok || s.Tier != TierSubagent {
			problems = append(problems, `subagent "`+name+`" is not a subagent-tier build`)
		}
	}
	return problems
}
