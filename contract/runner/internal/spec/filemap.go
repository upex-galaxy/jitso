package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"jitso-contract-runner/internal/jsonx"
)

// Entry kinds of a file map (SPEC 3).
const (
	EntryFile    = "file"
	EntrySymlink = "symlink"
	EntryDelete  = "delete"
)

// FileEntry is one resolved file map entry.
type FileEntry struct {
	Kind    string
	Content string
	Mode    os.FileMode // 0 = default (0644)
	Target  string      // symlink target
}

// FileMap maps a local relative path to its entry.
type FileMap map[string]FileEntry

// ParseFileMap reads an already resolved file map. A `from` file is read next to case.json
// (caseDir) and its content is resolved with vars like any other string of the case.
func ParseFileMap(v any, what, caseDir string, vars Vars) (FileMap, error) {
	if v == nil {
		return FileMap{}, nil
	}
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return nil, err
	}
	out := FileMap{}
	for _, path := range jsonx.SortedKeys(obj) {
		at := fmt.Sprintf("%s[%q]", what, path)
		if !filepath.IsLocal(path) {
			return nil, fmt.Errorf("%s: the path must be relative and stay inside its folder", at)
		}
		entry, err := parseEntry(obj[path], at, caseDir, vars)
		if err != nil {
			return nil, err
		}
		out[filepath.Clean(path)] = entry
	}
	return out, nil
}

func parseEntry(v any, what, caseDir string, vars Vars) (FileEntry, error) {
	switch e := v.(type) {
	case nil:
		return FileEntry{Kind: EntryDelete}, nil
	case string:
		return FileEntry{Kind: EntryFile, Content: e}, nil
	}
	obj, err := jsonx.Object(v, what)
	if err != nil {
		return FileEntry{}, fmt.Errorf("%s: want a string, null or an object", what)
	}
	if err := jsonx.Keys(obj, what, "from", "content", "mode", "symlink"); err != nil {
		return FileEntry{}, err
	}
	_, hasFrom := obj["from"]
	_, hasContent := obj["content"]
	_, hasLink := obj["symlink"]
	_, hasMode := obj["mode"]
	if btoi(hasFrom)+btoi(hasContent)+btoi(hasLink) != 1 {
		return FileEntry{}, fmt.Errorf("%s: give exactly one of from, content, symlink", what)
	}
	if hasLink {
		if hasMode {
			return FileEntry{}, fmt.Errorf("%s: a symlink takes no mode", what)
		}
		target, err := jsonx.String(obj["symlink"], what+".symlink")
		return FileEntry{Kind: EntrySymlink, Target: target}, err
	}
	entry := FileEntry{Kind: EntryFile}
	if hasMode {
		s, err := jsonx.String(obj["mode"], what+".mode")
		if err != nil {
			return FileEntry{}, err
		}
		mode, err := strconv.ParseUint(s, 8, 32)
		if err != nil || mode > 0o777 || mode == 0 {
			return FileEntry{}, fmt.Errorf("%s.mode: want an octal permission such as \"0755\", got %q", what, s)
		}
		entry.Mode = os.FileMode(mode)
	}
	if hasContent {
		entry.Content, err = jsonx.String(obj["content"], what+".content")
		return entry, err
	}
	from, err := jsonx.String(obj["from"], what+".from")
	if err != nil {
		return FileEntry{}, err
	}
	if filepath.IsAbs(from) {
		return FileEntry{}, fmt.Errorf("%s.from: the path is relative to the case folder", what)
	}
	data, err := os.ReadFile(filepath.Join(caseDir, from))
	if err != nil {
		return FileEntry{}, fmt.Errorf("%s.from: %v", what, err)
	}
	if entry.Content, err = Resolve(string(data), vars); err != nil {
		return FileEntry{}, fmt.Errorf("%s.from %s: %v", what, from, err)
	}
	return entry, nil
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
