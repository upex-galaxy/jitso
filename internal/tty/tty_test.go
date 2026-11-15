package tty

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTerminalFalse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	regular, err := os.Create(filepath.Join(t.TempDir(), "file"))
	if err != nil {
		t.Fatal(err)
	}
	defer regular.Close()

	closed, err := os.Create(filepath.Join(t.TempDir(), "closed"))
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()

	tests := []struct {
		name string
		file func(t *testing.T) *os.File
	}{
		{"nil", func(*testing.T) *os.File { return nil }},
		{"pipe read end", func(*testing.T) *os.File { return r }},
		{"pipe write end", func(*testing.T) *os.File { return w }},
		{"regular file", func(*testing.T) *os.File { return regular }},
		{"closed file", func(*testing.T) *os.File { return closed }},
		{"/dev/null", func(t *testing.T) *os.File {
			f, err := os.Open(os.DevNull)
			if err != nil {
				t.Skipf("no %s here: %v", os.DevNull, err)
			}
			t.Cleanup(func() { f.Close() })
			return f
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsTerminal(tt.file(t)) {
				t.Error("IsTerminal = true, want false")
			}
		})
	}
}

// TestIsTerminalControllingTerminal checks the true side when the test run has
// a controlling terminal (a developer's shell, or `script -q /dev/null go test`).
// Without one it skips: a test run cannot make a terminal.
func TestIsTerminalControllingTerminal(t *testing.T) {
	f, err := os.Open("/dev/tty")
	if err != nil {
		t.Skipf("no controlling terminal: %v", err)
	}
	defer f.Close()
	if !IsTerminal(f) {
		t.Error("IsTerminal(/dev/tty) = false, want true")
	}
}
