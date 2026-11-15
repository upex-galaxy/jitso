// Package skill ships the jitso usage skill inside the binary and plans its
// installation: where the files go for a scope (the real folder under
// .agents/skills and the link or second copy under .claude/skills), what state
// an installed copy is in (absent, current, outdated, modified, unmanaged),
// and the adopt.Plan that installs, upgrades or removes it.
//
// The embedded tree is plain text under jitso/. It holds no dotfile and no
// symlink (go:embed would skip the first and cannot hold the second), and the
// manifest the installer writes next to SKILL.md is never embedded
// (docs/decisions.md, defect N-16).
package skill

import (
	"embed"
	"io/fs"
	"sort"
)

// Name is the skill's name and the name of its folder.
const Name = "jitso"

//go:embed jitso
var embedded embed.FS

// File is one embedded file: its slash path relative to the skill folder and
// its bytes.
type File struct {
	Path string
	Data []byte
}

// Files are the embedded files sorted by path.
func Files() []File {
	var out []File
	err := fs.WalkDir(embedded, Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := embedded.ReadFile(p)
		if err != nil {
			return err
		}
		out = append(out, File{Path: p[len(Name)+1:], Data: data})
		return nil
	})
	if err != nil {
		// The tree is compiled in: a read failure is a bug, not an environment.
		panic("skill: reading the embedded tree: " + err.Error())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}
