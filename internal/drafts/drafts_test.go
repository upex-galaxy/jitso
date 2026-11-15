package drafts

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"scout", true},
		{"a", true},
		{"my-build-2", true},
		{strings.Repeat("a", 64), true},
		{strings.Repeat("a", 65), false},
		{"", false},
		{"Scout", false},
		{"2fast", false},
		{"-lead", false},
		{"under_score", false},
		{"dot.yaml", false},
		{"../x", false},
		{"a/b", false},
		{"a\\b", false},
		{"a b", false},
		{"a\n", false},
		{"é", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if got := ValidID(tt.id); got != tt.want {
				t.Errorf("ValidID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestCheckInput(t *testing.T) {
	tests := []struct {
		arg   string
		stdin bool
		ok    bool
	}{
		{"-", true, true},
		{"build.yaml", false, true},
		{"build.yml", false, true},
		{"dir/build.yaml", false, true},
		{"/abs/build.yml", false, true},
		{".yaml", false, true},
		{"build.txt", false, false},
		{"build", false, false},
		{"build.yaml.bak", false, false},
		{"build.YAML", false, false},
		{"--", false, false},
		{"", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			stdin, err := CheckInput(tt.arg)
			if stdin != tt.stdin || (err == nil) != tt.ok {
				t.Errorf("CheckInput(%q) = %v, %v", tt.arg, stdin, err)
			}
			if err != nil && (!errors.Is(err, ErrBadInput) || err.Error() != InputRule) {
				t.Errorf("error %v", err)
			}
		})
	}
}

// failing returns n bytes, then an error.
type failing struct{ n int }

func (f *failing) Read(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errors.New("disk on fire")
	}
	n := min(len(p), f.n)
	f.n -= n
	return n, nil
}

// endless never ends and counts what was asked of it.
type endless struct{ read int }

func (e *endless) Read(p []byte) (int, error) {
	e.read += len(p)
	return len(p), nil
}

func TestRead(t *testing.T) {
	tests := []struct {
		name string
		in   io.Reader
		len  int
		err  error
	}{
		{"empty", strings.NewReader(""), 0, nil},
		{"small", strings.NewReader("id: x\n"), 6, nil},
		{"exactly the limit", bytes.NewReader(make([]byte, MaxBytes)), MaxBytes, nil},
		{"one byte over", bytes.NewReader(make([]byte, MaxBytes+1)), 0, ErrTooLarge},
		{"far over", bytes.NewReader(make([]byte, 3*MaxBytes)), 0, ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Read(tt.in)
			if !errors.Is(err, tt.err) || len(data) != tt.len {
				t.Errorf("Read = %d bytes, %v; want %d, %v", len(data), err, tt.len, tt.err)
			}
		})
	}
	t.Run("a read error is passed on", func(t *testing.T) {
		if _, err := Read(&failing{n: 10}); err == nil || errors.Is(err, ErrTooLarge) {
			t.Errorf("error %v", err)
		}
	})
	t.Run("an endless input is read at most one byte past the limit", func(t *testing.T) {
		e := &endless{}
		if _, err := Read(e); !errors.Is(err, ErrTooLarge) {
			t.Errorf("error %v", err)
		}
		if e.read > MaxBytes+1+4096 { // io.ReadAll asks in blocks, but the limit reader cuts them
			t.Errorf("read %d bytes of an endless input", e.read)
		}
	})
	t.Run("the limit error says the rule", func(t *testing.T) {
		if ErrTooLarge.Error() != "a draft is a .yaml file (or - for stdin) of at most 64 KiB" {
			t.Errorf("text %q", ErrTooLarge)
		}
	})
}

func primary(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestPutCreatesTheFolderAndItsIgnoreFile(t *testing.T) {
	root := primary(t)
	data := []byte("id: scout\nstatus: draft") // no trailing newline, kept
	if err := Put(root, "scout", data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".jitso", "drafts", "scout.yaml"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("draft file %q, %v", got, err)
	}
	ignore, err := os.ReadFile(filepath.Join(root, ".jitso", "drafts", ".gitignore"))
	if err != nil || string(ignore) != "*\n" {
		t.Fatalf(".gitignore %q, %v", ignore, err)
	}
	if got := names(t, Dir(root)); strings.Join(got, ",") != ".gitignore,scout.yaml" {
		t.Errorf("entries %q: a temporary file was left", got)
	}
	info, _ := os.Stat(File(root, "scout"))
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
}

func TestPutReplacesAndKeepsAnExistingIgnoreFile(t *testing.T) {
	root := primary(t)
	if err := Put(root, "scout", []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(Dir(root), IgnoreFile), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Put(root, "scout", []byte("second")); err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(root, "scout"); string(got) != "second" {
		t.Errorf("draft %q", got)
	}
	if ignore, _ := os.ReadFile(filepath.Join(Dir(root), IgnoreFile)); string(ignore) != "mine\n" {
		t.Errorf(".gitignore %q", ignore)
	}
}

func TestPutRefusesWhatCouldLeaveTheFolder(t *testing.T) {
	t.Run("a bad id", func(t *testing.T) {
		root := primary(t)
		for _, id := range []string{"", "../x", "a/b", "Up", "x.yaml"} {
			if err := Put(root, id, []byte("x")); err == nil {
				t.Errorf("Put(%q) succeeded", id)
			}
		}
		if _, err := os.Lstat(filepath.Join(root, ".jitso")); !errors.Is(err, os.ErrNotExist) {
			t.Error("a refused id created the folder")
		}
	})
	t.Run("a folder that is a symlink", func(t *testing.T) {
		root := primary(t)
		outside := primary(t)
		if err := os.MkdirAll(filepath.Join(root, ".jitso"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, Dir(root)); err != nil {
			t.Skipf("no symlinks: %v", err)
		}
		if err := Put(root, "scout", []byte("x")); err == nil {
			t.Error("Put wrote through a symlinked folder")
		}
		if got := names(t, outside); len(got) != 0 {
			t.Errorf("a file reached the folder behind the symlink: %q", got)
		}
	})
	t.Run("a symlink at the file name is replaced, not followed", func(t *testing.T) {
		root := primary(t)
		outside := filepath.Join(primary(t), "target")
		if err := os.WriteFile(outside, []byte("untouched"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(Dir(root), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, File(root, "scout")); err != nil {
			t.Skipf("no symlinks: %v", err)
		}
		if err := Put(root, "scout", []byte("draft")); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(outside); string(got) != "untouched" {
			t.Errorf("the symlink target was written: %q", got)
		}
		if got, _ := Get(root, "scout"); string(got) != "draft" {
			t.Errorf("draft %q", got)
		}
	})
}

func TestGetListRemove(t *testing.T) {
	root := primary(t)
	if ids, err := List(root); err != nil || len(ids) != 0 {
		t.Fatalf("List of a missing folder = %q, %v", ids, err)
	}
	if _, err := Get(root, "scout"); !errors.Is(err, ErrNoDraft) {
		t.Errorf("Get of a missing draft: %v", err)
	}
	for _, id := range []string{"b-two", "a-one", "c-three"} {
		if err := Put(root, id, []byte(id)); err != nil {
			t.Fatal(err)
		}
	}
	// Files that are not drafts: another extension, a name that is no id, a folder.
	for _, name := range []string{"notes.txt", "Bad_Name.yaml", "x.yml"} {
		if err := os.WriteFile(filepath.Join(Dir(root), name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(Dir(root), "sub.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	ids, err := List(root)
	if err != nil || strings.Join(ids, ",") != "a-one,b-two,c-three" {
		t.Fatalf("List = %q, %v", ids, err)
	}
	if got, err := Get(root, "b-two"); err != nil || string(got) != "b-two" {
		t.Errorf("Get = %q, %v", got, err)
	}
	if _, err := Get(root, "../x"); !errors.Is(err, ErrNoDraft) {
		t.Errorf("Get of a path: %v", err)
	}

	if err := Remove(root, "b-two"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"b-two", "nope", "../b", "", "Up"} {
		if err := Remove(root, id); !errors.Is(err, ErrNoDraft) {
			t.Errorf("Remove(%q) = %v, want ErrNoDraft", id, err)
		}
	}
	if ids, _ := List(root); strings.Join(ids, ",") != "a-one,c-three" {
		t.Errorf("after Remove: %q", ids)
	}
}

func TestRemoveAll(t *testing.T) {
	root := primary(t)
	if n, err := RemoveAll(root); err != nil || n != 0 {
		t.Fatalf("RemoveAll of a missing folder = %d, %v", n, err)
	}
	for _, id := range []string{"a-one", "b-two"} {
		if err := Put(root, id, []byte(id)); err != nil {
			t.Fatal(err)
		}
	}
	// Every *.yaml goes, whatever its name; anything else and the folder stay.
	for name, content := range map[string]string{"Odd_Name.yaml": "x", "notes.txt": "keep", "keep.yml": "keep"} {
		if err := os.WriteFile(filepath.Join(Dir(root), name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(Dir(root), "sub.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	n, err := RemoveAll(root)
	if err != nil || n != 3 {
		t.Fatalf("RemoveAll = %d, %v", n, err)
	}
	if got := strings.Join(names(t, Dir(root)), ","); got != ".gitignore,keep.yml,notes.txt,sub.yaml" {
		t.Errorf("entries after RemoveAll: %s", got)
	}
	if n, err := RemoveAll(root); err != nil || n != 0 {
		t.Errorf("second RemoveAll = %d, %v", n, err)
	}
}
