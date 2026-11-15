package adopt

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 11, 15, 7, 34, 55, 0, time.UTC)

// randBytes gives the random reader a fixed byte sequence.
func randBytes(b ...byte) io.Reader { return bytes.NewReader(b) }

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// relTo renders paths relative to root, the way a project-scope command does.
func relTo(root string) func(string) string {
	return func(p string) string {
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return p
		}
		return filepath.ToSlash(rel)
	}
}

// ReadManifest reads manifest.json of a backup folder.
func ReadManifest(dir string) (Manifest, error) {
	var m Manifest
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

// Restore puts a backup back under root: it copies each saved file over the
// current one, recreates a recorded link, and removes every path recorded as
// not existing before. It is the manual recovery written as code, used by the
// tests to prove a backup is complete.
func Restore(dir, root string) error {
	m, err := ReadManifest(dir)
	if err != nil {
		return err
	}
	for _, f := range m.Files {
		dst := filepath.Join(root, filepath.FromSlash(f.Path))
		switch {
		case !f.Existed:
			if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		case f.Symlink != "":
			if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(f.Symlink, dst); err != nil {
				return err
			}
		default:
			src := filepath.Join(dir, filepath.FromSlash(f.Path))
			info, err := os.Stat(src)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}
