package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// State is what a skill folder on disk is, compared with the shipped skill.
type State int

// The states of a target folder.
const (
	// Absent: there is no folder.
	Absent State = iota + 1
	// Current: the manifest matches the shipped skill and every file, and
	// there is no other file.
	Current
	// Outdated: every file matches its manifest hash, but the manifest is of
	// another version of the skill.
	Outdated
	// Modified: a listed file differs or is missing, or a file the manifest
	// does not list is there.
	Modified
	// Unmanaged: the folder has no valid manifest, so jitso did not install it.
	Unmanaged
	// NotFolder: the path is a file or a link, not a folder.
	NotFolder
)

func (s State) String() string {
	switch s {
	case Absent:
		return "absent"
	case Current:
		return "current"
	case Outdated:
		return "outdated"
	case Modified:
		return "modified"
	case Unmanaged:
		return "unmanaged"
	case NotFolder:
		return "not a folder"
	}
	return fmt.Sprintf("state(%d)", int(s))
}

// Found is what Inspect saw in a folder.
type Found struct {
	State State
	// Manifest is the installed manifest (Current, Outdated, Modified).
	Manifest Manifest
	// Why completes `is not a copy jitso installed (<why>)` for Unmanaged, and
	// names what the path is for NotFolder.
	Why string
	// Edited are the files that make the folder Modified: a listed file whose
	// hash differs, then a listed file that is missing, then a file the
	// manifest does not list, each group sorted.
	Edited []string
	// Files are the slash paths, relative to the folder, of every entry that
	// is not a folder (regular files and links), sorted. It includes the
	// manifest.
	Files []string
	// Dirs are the slash paths of the folders below the folder, sorted.
	Dirs []string
}

// Has reports whether the folder holds a non-folder entry at rel.
func (f Found) Has(rel string) bool {
	i := sort.SearchStrings(f.Files, rel)
	return i < len(f.Files) && f.Files[i] == rel
}

// Inspect reads the folder dir and decides its State against want, the
// manifest of the shipped skill. It never follows a link: a link in place of
// the folder is NotFolder, a link inside the folder is an entry. Only a folder
// with a valid manifest has its files read (an unmanaged folder may be big and
// is not ours).
func Inspect(dir string, want Manifest) (Found, error) {
	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Found{State: Absent}, nil
	case err != nil:
		return Found{}, err
	case info.Mode()&os.ModeSymlink != 0:
		target, _ := readLink(dir)
		return Found{State: NotFolder, Why: "a link to " + target}, nil
	case !info.IsDir():
		return Found{State: NotFolder, Why: "a file"}, nil
	}

	var found Found
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			found.Dirs = append(found.Dirs, rel)
		} else {
			found.Files = append(found.Files, rel)
		}
		return nil
	})
	if err != nil {
		return Found{}, err
	}
	sort.Strings(found.Files)
	sort.Strings(found.Dirs)

	if !found.Has(ManifestName) {
		found.State, found.Why = Unmanaged, "no "+ManifestName
		return found, nil
	}
	data, err := readRegular(filepath.Join(dir, ManifestName))
	var man Manifest
	if err == nil {
		man, err = ParseManifest(data)
	}
	if err != nil {
		found.State, found.Why = Unmanaged, ManifestName+" is not valid: "+err.Error()
		return found, nil
	}
	found.Manifest = man

	var changed, missing, extra []string
	for rel, hash := range man.Files {
		if !found.Has(rel) {
			missing = append(missing, rel)
			continue
		}
		got, err := readRegular(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || HashBytes(got) != hash {
			changed = append(changed, rel)
		}
	}
	for _, rel := range found.Files {
		if _, listed := man.Files[rel]; !listed && rel != ManifestName {
			extra = append(extra, rel)
		}
	}
	sort.Strings(changed)
	sort.Strings(missing)
	sort.Strings(extra)
	found.Edited = append(append(changed, missing...), extra...)
	switch {
	case len(found.Edited) > 0:
		found.State = Modified
	case man.Matches(want):
		found.State = Current
	default:
		found.State = Outdated
	}
	return found, nil
}

// readRegular reads a regular file, and fails for a link or anything else.
func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(path)
}
