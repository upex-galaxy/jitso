package skill

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// ManifestName is the manifest the installer writes next to SKILL.md.
const ManifestName = ".jitso-skill.json"

// ManifestVersion is the `v` of the manifest format.
const ManifestVersion = 1

// Manifest is .jitso-skill.json: the jitso version that wrote the copy, a hash
// of the whole content, and one SHA-256 per file. Hand edits, extra or missing
// files and upgrades are detected from it.
type Manifest struct {
	V             int               `json:"v"`
	Skill         string            `json:"skill"`
	JitsoVersion  string            `json:"jitso_version"`
	ContentSHA256 string            `json:"content_sha256"`
	Files         map[string]string `json:"files"`
}

// HashBytes is the lower-case hex SHA-256 of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ContentHash is the SHA-256 of the concatenation, in path order, of
// `<path>\x00<file hex>\n` over the files map.
func ContentHash(files map[string]string) string {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var b bytes.Buffer
	for _, p := range paths {
		b.WriteString(p)
		b.WriteByte(0)
		b.WriteString(files[p])
		b.WriteByte('\n')
	}
	return HashBytes(b.Bytes())
}

// NewManifest is the manifest of a set of files written by the given jitso version.
func NewManifest(version string, files []File) Manifest {
	m := Manifest{V: ManifestVersion, Skill: Name, JitsoVersion: version, Files: map[string]string{}}
	for _, f := range files {
		m.Files[f.Path] = HashBytes(f.Data)
	}
	m.ContentSHA256 = ContentHash(m.Files)
	return m
}

// Marshal is the manifest as written to disk: two-space JSON with sorted keys
// and a trailing newline.
func (m Manifest) Marshal() ([]byte, error) {
	out, err := jsonx.MarshalIndent(m, "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Matches reports whether the manifest describes exactly these files: the
// same paths with the same hashes, and the stored content hash is theirs.
func (m Manifest) Matches(other Manifest) bool {
	if m.ContentSHA256 != other.ContentSHA256 || len(m.Files) != len(other.Files) {
		return false
	}
	for p, h := range other.Files {
		if m.Files[p] != h {
			return false
		}
	}
	return true
}

// ParseManifest reads a manifest and checks that it is one this jitso can
// trust to name files: format 1, this skill, and every path a clean relative
// slash path that is neither the manifest itself nor outside the folder.
func ParseManifest(data []byte) (Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("not valid JSON: %v", err)
	}
	switch {
	case m.V != ManifestVersion:
		return Manifest{}, fmt.Errorf("unknown format version %d", m.V)
	case m.Skill != Name:
		return Manifest{}, errors.New("it is the manifest of another skill")
	case m.Files == nil:
		return Manifest{}, errors.New("it lists no files")
	}
	for p, h := range m.Files {
		if !validRel(p) || p == ManifestName {
			return Manifest{}, fmt.Errorf("it lists the path %q", p)
		}
		if len(h) != 64 || strings.Trim(h, "0123456789abcdef") != "" {
			return Manifest{}, fmt.Errorf("it holds a bad hash for %q", p)
		}
	}
	return m, nil
}

// validRel reports whether p is a clean, relative, slash path that stays
// inside its folder.
func validRel(p string) bool {
	if p == "" || strings.ContainsAny(p, "\\\x00") || path.IsAbs(p) {
		return false
	}
	c := path.Clean(p)
	return c == p && c != "." && c != ".." && !strings.HasPrefix(c, "../")
}
