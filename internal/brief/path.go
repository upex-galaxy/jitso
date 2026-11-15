package brief

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// The brief path rejections (exit 2).
const (
	reasonEmpty      = "brief path is empty or contains a NUL byte"
	reasonNotRegular = "brief path is not a regular file"
	reasonMissing    = "brief file does not exist"
	reasonOutside    = "brief file is outside the allowed folders"
)

// ResolvePath is the brief path guard. The brief must be a regular file whose
// real path (every symlink and .. resolved, in order, as realpath does) lies
// inside one of the allowed folders, themselves resolved; a folder that does
// not exist is skipped. A relative file is read from cwd. It returns the real
// path, or a Rejected error.
func ResolvePath(file, cwd string, allowedDirs []string) (string, error) {
	if file == "" || strings.ContainsRune(file, 0) {
		return "", fail.Rejected(reasonEmpty)
	}
	path := file
	if !filepath.IsAbs(path) {
		// No Clean: "link/../x" must climb from where link points, not from
		// the folder that holds link.
		path = cwd + string(filepath.Separator) + path
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fail.Rejected(reasonMissing)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", fail.Rejected(reasonMissing)
	}
	if !info.Mode().IsRegular() {
		return "", fail.Rejected(reasonNotRegular)
	}
	for _, dir := range allowedDirs {
		realDir, err := filepath.EvalSymlinks(dir)
		if err != nil {
			continue
		}
		if inside(realDir, real) {
			return real, nil
		}
	}
	return "", fail.Rejected(reasonOutside)
}

// inside reports whether child is strictly below parent: a sibling that only
// shares a prefix (briefs-evil next to briefs) is not inside.
func inside(parent, child string) bool {
	if !strings.HasSuffix(parent, string(filepath.Separator)) {
		parent += string(filepath.Separator)
	}
	return strings.HasPrefix(child, parent)
}

// Exists is the existence probe Check uses: an absolute path as is, a
// relative one against the project root. A dangling symlink does not exist.
func Exists(root string) func(path string) bool {
	return func(path string) bool {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		_, err := os.Stat(path)
		return err == nil
	}
}
