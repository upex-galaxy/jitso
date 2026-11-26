package drafts

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveContent(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
		err  error
	}{
		{"plain", "id: x\nstatus: draft\nrole: r\n", "id: x\nstatus: approved\nrole: r\n", nil},
		{"a comment on the line", "id: x\nstatus: draft  # not yet\n", "id: x\nstatus: approved  # not yet\n", nil},
		{"double quotes stay", "status: \"draft\"\nid: x\n", "status: \"approved\"\nid: x\n", nil},
		{"single quotes stay", "id: x\nstatus: 'draft'", "id: x\nstatus: 'approved'", nil},
		{"the word elsewhere stays", "id: x\ndescription: a draft status draft\nstatus: draft\n", "id: x\ndescription: a draft status draft\nstatus: approved\n", nil},
		{"CRLF stays", "id: x\r\nstatus: draft\r\n", "id: x\r\nstatus: approved\r\n", nil},
		{"already approved", "id: x\nstatus: approved\n", "", ErrStatusForm},
		{"a tag", "id: x\nstatus: !!str draft\n", "", ErrStatusForm},
		{"no status is added", "id: x\n", "status: approved\nid: x\n", nil},
		{"no status and an explicit key", "? id\n: x\n", "", ErrStatusForm},
		{"not yaml", "id: [\n", "", ErrStatusForm},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SaveContent([]byte(tt.src))
			if !errors.Is(err, tt.err) {
				t.Fatalf("error %v, want %v", err, tt.err)
			}
			if string(got) != tt.want {
				t.Errorf("content %q, want %q", got, tt.want)
			}
		})
	}
}

// Everything except the status scalar is the draft's, byte for byte.
func TestSaveContentChangesOnlyTheStatus(t *testing.T) {
	src := "# head\nid: x\n\nstatus: \"draft\"   # c\nlist: [a,  b]\nnote:   kept   \n"
	got, err := SaveContent([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	at := strings.Index(src, "draft\"")
	if string(got) != src[:at]+"approved"+src[at+len("draft"):] {
		t.Errorf("content %q", got)
	}
	if len(got)-len(src) != len("approved")-len("draft") {
		t.Errorf("length changed by %d", len(got)-len(src))
	}
}

func TestShow(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"lines", "a\nb\n", "+ a\n+ b\n"},
		{"no final newline", "a\nb", "+ a\n+ b\n"},
		{"a blank line", "a\n\nb\n", "+ a\n+ \n+ b\n"},
		{"two final newlines", "a\n\n", "+ a\n+ \n"},
		{"empty", "", "+ \n"},
		{"a tab stays", "a\tb\n", "+ a\tb\n"},
		{"accents and symbols stay", "café ✓\n", "+ café ✓\n"},
		{"an escape sequence is shown", "a\x1b[2Kb\n", "+ a\\x1b[2Kb\n"},
		{"a carriage return is shown", "a\r\nb\n", "+ a\\x0d\n+ b\n"},
		{"delete is shown", "a\x7fb\n", "+ a\\x7fb\n"},
		{"a C1 control is shown", "a\u009bb\n", "+ a\\x9bb\n"},
		{"a line separator is shown", "a b\n", "+ a\\x2028b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Show([]byte(tt.in)); got != tt.want {
				t.Errorf("Show(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSave(t *testing.T) {
	primary := t.TempDir()
	if err := Put(primary, "scout", []byte("id: scout\nstatus: draft\n")); err != nil {
		t.Fatal(err)
	}
	if err := Save(primary, "scout", []byte("id: scout\nstatus: approved\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(BuildFile(primary, "scout"))
	if err != nil || string(got) != "id: scout\nstatus: approved\n" {
		t.Fatalf("build file %q, %v", got, err)
	}
	if info, _ := os.Stat(BuildFile(primary, "scout")); info.Mode().Perm() != 0o644 && runtime.GOOS != "windows" {
		t.Errorf("mode %v", info.Mode().Perm())
	}
	if _, err := Get(primary, "scout"); !errors.Is(err, ErrNoDraft) {
		t.Errorf("the draft is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(primary, ".jitso", "drafts", ".gitignore")); err != nil {
		t.Errorf("the folder lost its .gitignore: %v", err)
	}
}

func TestSaveNeverOverwrites(t *testing.T) {
	primary := t.TempDir()
	if err := Put(primary, "scout", []byte("draft")); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(primary, "jitso", "builds")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scout.yaml"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(primary, "nowhere"), filepath.Join(dir, "dangling.yaml")); err != nil {
		t.Skipf("no symlinks: %v", err)
	}
	if err := Put(primary, "dangling", []byte("draft")); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"scout", "dangling"} {
		if err := Save(primary, id, []byte("new")); !errors.Is(err, ErrExists) {
			t.Errorf("%s: error %v, want ErrExists", id, err)
		}
		if _, err := Get(primary, id); err != nil {
			t.Errorf("%s: the draft was removed: %v", id, err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "scout.yaml")); string(got) != "mine" {
		t.Errorf("the build was overwritten: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(primary, "nowhere")); err == nil {
		t.Error("a write followed the symlink")
	}
}

func TestSaveRefusesABadID(t *testing.T) {
	primary := t.TempDir()
	for _, id := range []string{"", "../x", "A", "a/b", "x.yaml"} {
		if err := Save(primary, id, []byte("x")); err == nil {
			t.Errorf("id %q accepted", id)
		}
	}
	if _, err := os.Stat(filepath.Join(primary, "jitso")); err == nil {
		t.Error("a refused save created folders")
	}
}

// A save with no draft behind it still writes the build (the caller decides
// whether a draft exists), and says nothing about the missing draft.
func TestSaveWithoutADraft(t *testing.T) {
	primary := t.TempDir()
	if err := Save(primary, "scout", []byte("x")); err != nil {
		t.Fatal(err)
	}
}
