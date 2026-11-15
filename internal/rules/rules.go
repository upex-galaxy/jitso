// Package rules loads the hard rules a launch writes into core.md: the
// project's own jitso/hard-rules/ pair when the project has that folder, else
// the pair shipped inside the binary (docs/decisions.md, defect N-14).
package rules

import (
	_ "embed"
	"errors"
	"io/fs"
)

// Dir is the project's hard-rules folder. Its core.md and style.md are both
// required when the folder exists.
const Dir = "jitso/hard-rules"

// The two files of the pair, in the order they are joined.
const (
	CoreFile  = "core.md"
	StyleFile = "style.md"
)

//go:embed core.md
var embeddedCore string

//go:embed style.md
var embeddedStyle string

// Embedded is the core and the style shipped inside the binary, joined as Load
// joins any pair.
func Embedded() string {
	return Join(embeddedCore, embeddedStyle)
}

// Join is core.md of a launch: the two files joined by one newline, as read.
func Join(core, style string) string {
	return core + "\n" + style
}

// Load returns the hard rules for the project rooted at fsys. When
// jitso/hard-rules/ exists, its core.md and style.md are both required and
// are joined by one newline; a missing one is an error naming the file. When
// the folder does not exist, the embedded pair is returned.
func Load(fsys fs.FS) (string, error) {
	if _, err := fs.Stat(fsys, Dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Embedded(), nil
		}
		return "", err
	}
	core, err := fs.ReadFile(fsys, Dir+"/"+CoreFile)
	if err != nil {
		return "", err
	}
	style, err := fs.ReadFile(fsys, Dir+"/"+StyleFile)
	if err != nil {
		return "", err
	}
	return Join(string(core), string(style)), nil
}
