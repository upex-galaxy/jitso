package sandbox

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"jitso-contract-runner/internal/jsonx"
	"jitso-contract-runner/internal/spec"
)

// CopyTree copies src to dst: folders, regular files with their mode, symlinks as symlinks.
// skip, when set, leaves out a file by its path relative to src.
func CopyTree(src, dst string, skip func(rel string) bool) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if skip != nil && !d.IsDir() && skip(rel) {
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
		return fmt.Errorf("%s: unsupported file type %v", path, info.Mode().Type())
	})
}

// Apply writes a file map under base (the project dir or HOME), in path order.
func Apply(base string, files spec.FileMap) error {
	for _, rel := range jsonx.SortedKeys(files) {
		entry := files[rel]
		path := filepath.Join(base, rel)
		if err := parentInside(base, path); err != nil {
			return err
		}
		if entry.Kind == spec.EntryDelete {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		if entry.Kind == spec.EntrySymlink {
			if err := os.Symlink(entry.Target, path); err != nil {
				return err
			}
			continue
		}
		mode := entry.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(path, []byte(entry.Content), mode); err != nil {
			return err
		}
		// WriteFile honors the umask; a declared mode is exact.
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
	}
	return nil
}

// parentInside refuses a write whose existing ancestor resolves outside base, so a symlink
// (node_modules, or one a case made) can never carry a write out of the sandbox.
func parentInside(base, path string) error {
	realBase, err := filepath.EvalSymlinks(base)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	for {
		if _, err := os.Lstat(dir); err == nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(realBase, real)
	if err != nil || (rel != "." && !filepath.IsLocal(rel)) {
		return fmt.Errorf("refusing to write %s: its folder resolves outside %s", path, base)
	}
	return nil
}
