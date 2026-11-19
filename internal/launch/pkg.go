package launch

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/registry"
)

// PackageDir is the folder of a launch that holds what travels with it:
// .jitso/launches/<id>/pkg, passed to the harness as one extra folder whose
// .claude/skills the session loads under the names written there.
const PackageDir = "pkg"

// The size a launch carries per user skill: at most this many bytes and
// files, regular files only.
const (
	MaxSkillBytes = 5 << 20
	MaxSkillFiles = 500
)

// Package is what one launch carries beyond its own files. The zero value
// carries nothing: no pkg folder is written and the argv is unchanged.
type Package struct {
	Skills []packageSkill
}

type packageSkill struct {
	name   string
	scope  string
	sha256 string
	files  []packageFile
}

type packageFile struct {
	rel  string
	data []byte
	mode fs.FileMode
}

// Empty reports whether the launch carries nothing.
func (p Package) Empty() bool { return len(p.Skills) == 0 }

// Ref is the package key of the launch event, nil when nothing travels.
func (p Package) Ref() *registry.Package {
	if p.Empty() {
		return nil
	}
	ref := &registry.Package{Skills: []registry.PackageSkill{}, Flag: "add-dir"}
	for _, s := range p.Skills {
		ref.Skills = append(ref.Skills, registry.PackageSkill{Name: s.name, Scope: s.scope, SHA256: s.sha256})
	}
	return ref
}

// planPackage reads what the build carries: a frozen copy of each user skill
// it names, in skills_first order. A skill over the size a launch carries is
// a rejection.
func planPackage(b *catalog.Build, user catalog.User) (Package, error) {
	var pkg Package
	for _, item := range b.SkillsFirst {
		name, ok := catalog.UserName(item)
		if !ok {
			continue
		}
		s, err := readSkill(user.SkillDir(name))
		if err != nil {
			return Package{}, err
		}
		if s == nil {
			return Package{}, fail.Rejectedf(`user skill "%s" is larger than a launch carries`, name)
		}
		s.name, s.scope = name, catalog.ScopeUser
		pkg.Skills = append(pkg.Skills, *s)
	}
	return pkg, nil
}

// readSkill reads the regular files of the skill folder dir (a link to the
// folder is resolved, a link to a file is read through, a link to a folder
// inside it is left out), with the hash of the tree. It returns nil when the
// folder is over MaxSkillBytes or MaxSkillFiles.
func readSkill(dir string) (*packageSkill, error) {
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	s := &packageSkill{}
	total := 0
	tooLarge := false
	err = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		total += int(info.Size())
		if len(s.files)+1 > MaxSkillFiles || total > MaxSkillBytes {
			tooLarge = true
			return fs.SkipAll
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o644)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		s.files = append(s.files, packageFile{rel: filepath.ToSlash(rel), data: data, mode: mode})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if tooLarge {
		return nil, nil
	}
	s.sha256 = treeHash(s.files)
	return s, nil
}

// treeHash is the SHA-256 of one line per file, `<sha256 of the file>
// <relative path>`, sorted by path: what `sha256sum` prints for the tree.
func treeHash(files []packageFile) string {
	lines := make([]string, len(files))
	for i, f := range files {
		sum := sha256.Sum256(f.data)
		lines[i] = hex.EncodeToString(sum[:]) + "  " + f.rel + "\n"
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i][66:] < lines[j][66:] })
	sum := sha256.Sum256([]byte(strings.Join(lines, "")))
	return hex.EncodeToString(sum[:])
}

// write writes the package under dir/pkg and returns that folder.
func (p Package) write(dir string) (string, error) {
	root := filepath.Join(dir, PackageDir)
	for _, s := range p.Skills {
		base := filepath.Join(root, ".claude", "skills", s.name)
		for _, f := range s.files {
			path := filepath.Join(base, filepath.FromSlash(f.rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(path, f.data, f.mode); err != nil {
				return "", err
			}
			if err := os.Chmod(path, f.mode); err != nil {
				return "", err
			}
		}
	}
	return root, nil
}

// SessionSkillNames are the skills_first names as the launched session lists
// them: a user:<name> skill is carried in the package under its plain name.
func SessionSkillNames(skills []string) []string {
	out := make([]string, len(skills))
	for i, s := range skills {
		if name, ok := catalog.UserName(s); ok {
			s = name
		}
		out[i] = s
	}
	return out
}
