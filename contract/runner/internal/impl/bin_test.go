package impl

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNewBinResolvesThePath: the launcher is the real absolute path of --bin, also when
// the path given goes through a symlink (macOS /var is a link to /private/var), because
// the binary under test names itself by its real path.
func TestNewBinResolvesThePath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "real")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "jitso")
	if err := os.WriteFile(file, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dir, filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("cannot create a symlink: %v", err)
	}
	if err := os.Symlink(file, filepath.Join(root, "linkfile")); err != nil {
		t.Skipf("cannot create a symlink: %v", err)
	}
	t.Chdir(root)

	tests := []struct{ name, path string }{
		{"real path", file},
		{"through a linked folder", filepath.Join(root, "linkdir", "jitso")},
		{"a linked file", filepath.Join(root, "linkfile")},
		{"relative", filepath.Join("linkdir", "jitso")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewBin(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			if got := b.Launcher(nil); got != want {
				t.Errorf("launcher %q, want %q", got, want)
			}
		})
	}
}

func TestNewBinRejects(t *testing.T) {
	root := t.TempDir()
	plain := filepath.Join(root, "plain")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "missing")

	tests := []struct {
		name, path, want string
		posix            bool
	}{
		{"no path", "", "needs --bin", false},
		{"missing file", missing, "--bin: ", false},
		{"a folder", root, "is not an executable file", false},
		{"no executable bit", plain, "is not an executable file", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.posix && runtime.GOOS == "windows" {
				t.Skip("Windows has no executable bit")
			}
			_, err := NewBin(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %v, want %q", err, tt.want)
			}
			if tt.path == missing && !strings.Contains(err.Error(), missing) {
				t.Errorf("error %v does not name %s", err, missing)
			}
		})
	}
}
