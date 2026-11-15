package adopt

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func spec(root string) BackupSpec {
	return BackupSpec{
		Dir:     filepath.Join(root, ".jitso", "backups"),
		Root:    root,
		Command: "setup hook",
		Scope:   "project",
		Version: "v0.1.0",
		Now:     func() time.Time { return fixedNow },
		Rand:    randBytes(0xa3, 0xf9),
		Display: relTo(root),
	}
}

func TestNewID(t *testing.T) {
	id, err := NewID(fixedNow, randBytes(0xa3, 0xf9))
	if err != nil || id != "20261115T073455Z-a3f9" {
		t.Fatalf("id = %q, err %v", id, err)
	}
	// a clock in another zone still gives UTC
	zone := time.FixedZone("x", 5*3600)
	id, _ = NewID(fixedNow.In(zone), randBytes(0x00, 0x0a))
	if id != "20261115T073455Z-000a" {
		t.Errorf("id = %q", id)
	}
	re := regexp.MustCompile(`^\d{8}T\d{6}Z-[0-9a-f]{4}$`)
	if !re.MatchString(id) {
		t.Errorf("id %q has the wrong shape", id)
	}
	if _, err := NewID(fixedNow, randBytes(0x01)); err == nil {
		t.Error("a short random source must fail")
	}
}

func TestBackupOnlyWhenAnExistingFileChanges(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "keep.txt"), "x", 0o644)
	tests := []struct {
		name    string
		changes []Change
	}{
		{"nothing", nil},
		{"only new files", []Change{{Path: filepath.Join(root, "new.txt"), Display: "new.txt", Kind: KindFile, New: []byte("n")}}},
		{"a folder", []Change{{Path: filepath.Join(root, "d"), Display: "d", Kind: KindMkdir}}},
		{"a link to create", []Change{{Path: filepath.Join(root, "l"), Display: "l", Kind: KindSymlink, Target: "keep.txt"}}},
		{"a removal of a path that is gone", []Change{{Path: filepath.Join(root, "gone"), Display: "gone", Kind: KindDelete}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bk, err := NewBackup(spec(root), tt.changes)
			if err != nil || bk != nil {
				t.Fatalf("backup %v, err %v; want none", bk, err)
			}
			if exists(filepath.Join(root, ".jitso")) {
				t.Error("no backup, yet .jitso was created")
			}
		})
	}
}

func TestBackupLayoutAndManifest(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".claude", "settings.json"), "{\"a\":1}\n", 0o600)
	write(t, filepath.Join(root, "gone.txt"), "bye", 0o644)
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	linked := runtime.GOOS != "windows"
	if linked {
		if err := os.Symlink("../old", filepath.Join(root, ".claude", "skills", "jitso")); err != nil {
			t.Fatal(err)
		}
	}
	changes := []Change{
		{Path: filepath.Join(root, ".claude", "settings.json"), Display: ".claude/settings.json", Kind: KindFile, New: []byte("{}\n")},
		{Path: filepath.Join(root, "new.txt"), Display: "new.txt", Kind: KindFile, New: []byte("n")},
		{Path: filepath.Join(root, "gone.txt"), Display: "gone.txt", Kind: KindDelete},
		{Path: filepath.Join(root, ".claude", "skills", "jitso"), Display: ".claude/skills/jitso", Kind: KindSymlink, Target: "../new", OldTarget: "../old"},
		{Path: filepath.Join(root, "newdir"), Display: "newdir", Kind: KindMkdir},
	}
	bk, err := NewBackup(spec(root), changes)
	if err != nil || bk == nil {
		t.Fatalf("backup %v, err %v", bk, err)
	}
	if bk.ID != "20261115T073455Z-a3f9" {
		t.Errorf("id = %q", bk.ID)
	}
	backups := filepath.Join(root, ".jitso", "backups")
	if got := read(t, filepath.Join(backups, ".gitignore")); got != "*\n" {
		t.Errorf(".gitignore = %q", got)
	}
	if got := read(t, filepath.Join(bk.Dir, ".claude", "settings.json")); got != "{\"a\":1}\n" {
		t.Errorf("saved settings = %q", got)
	}
	if info, _ := os.Stat(filepath.Join(bk.Dir, ".claude", "settings.json")); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("saved mode = %v", info.Mode().Perm())
	}
	if got := read(t, filepath.Join(bk.Dir, "gone.txt")); got != "bye" {
		t.Errorf("saved gone.txt = %q", got)
	}
	if len(bk.Copied) != 2 || bk.Copied[0].Display != ".claude/settings.json" || bk.Copied[1].Display != "gone.txt" {
		t.Errorf("copied = %+v", bk.Copied)
	}
	want := `{"v":1,"command":"setup hook","scope":"project","ts":"2026-11-15T07:34:55.000Z","jitso_version":"v0.1.0","files":[` +
		`{"path":".claude/settings.json","existed":true},` +
		`{"path":"new.txt","existed":false},` +
		`{"path":"gone.txt","existed":true}`
	if linked {
		want += `,{"path":".claude/skills/jitso","existed":true,"symlink":"../old"}`
	} else {
		want += `,{"path":".claude/skills/jitso","existed":false}`
	}
	want += "]}\n"
	if got := read(t, filepath.Join(bk.Dir, ManifestName)); got != want {
		t.Errorf("manifest\n got %s\nwant %s", got, want)
	}
	if exists(filepath.Join(bk.Dir, "newdir")) {
		t.Error("a folder must not be in the backup")
	}
}

func TestBackupNeverReusesAnID(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "f"), "1", 0o644)
	s := spec(root)
	taken := filepath.Join(s.Dir, "20261115T073455Z-a3f9")
	if err := os.MkdirAll(taken, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(taken, "marker"), "mine", 0o644)
	s.Rand = randBytes(0xa3, 0xf9, 0xa3, 0xf9, 0x11, 0x22)
	bk, err := NewBackup(s, []Change{{Path: filepath.Join(root, "f"), Display: "f", Kind: KindFile, New: []byte("2")}})
	if err != nil {
		t.Fatal(err)
	}
	if bk.ID != "20261115T073455Z-1122" {
		t.Errorf("id = %q, want the third draw", bk.ID)
	}
	if got := read(t, filepath.Join(taken, "marker")); got != "mine" {
		t.Error("the taken backup was touched")
	}
}

func TestBackupGivesUpWhenEveryIDIsTaken(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "f"), "1", 0o644)
	s := spec(root)
	if err := os.MkdirAll(filepath.Join(s.Dir, "20261115T073455Z-a3f9"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.Rand = repeatReader{0xa3, 0xf9}
	_, err := NewBackup(s, []Change{{Path: filepath.Join(root, "f"), Display: "f", Kind: KindFile, New: []byte("2")}})
	if err == nil || !strings.Contains(err.Error(), "free backup id") {
		t.Fatalf("err = %v", err)
	}
}

type repeatReader []byte

func (r repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r[i%len(r)]
	}
	return len(p), nil
}

func TestBackupKeepsAnExistingGitignore(t *testing.T) {
	root := t.TempDir()
	s := spec(root)
	write(t, filepath.Join(s.Dir, ".gitignore"), "custom\n", 0o644)
	write(t, filepath.Join(root, "f"), "1", 0o644)
	if _, err := NewBackup(s, []Change{{Path: filepath.Join(root, "f"), Display: "f", Kind: KindFile, New: []byte("2")}}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(s.Dir, ".gitignore")); got != "custom\n" {
		t.Errorf(".gitignore = %q", got)
	}
}

func TestBackupRefusesAPathOutsideTheRoot(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	write(t, filepath.Join(other, "f"), "1", 0o644)
	_, err := NewBackup(spec(root), []Change{{Path: filepath.Join(other, "f"), Display: "f", Kind: KindFile}})
	if err == nil || !strings.Contains(err.Error(), "not inside") {
		t.Fatalf("err = %v", err)
	}
}

func TestRestore(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "one.txt"), "one", 0o640)
	write(t, filepath.Join(root, "two.txt"), "two", 0o644)
	changes := []Change{
		{Path: filepath.Join(root, "a", "one.txt"), Display: "a/one.txt", Kind: KindFile, New: []byte("ONE")},
		{Path: filepath.Join(root, "two.txt"), Display: "two.txt", Kind: KindDelete},
		{Path: filepath.Join(root, "three.txt"), Display: "three.txt", Kind: KindFile, New: []byte("3")},
	}
	bk, err := NewBackup(spec(root), changes)
	if err != nil {
		t.Fatal(err)
	}
	// do what the plan says, then restore
	write(t, filepath.Join(root, "a", "one.txt"), "ONE", 0o640)
	os.Remove(filepath.Join(root, "two.txt"))
	write(t, filepath.Join(root, "three.txt"), "3", 0o644)
	if err := Restore(bk.Dir, root); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(root, "a", "one.txt")) != "one" || read(t, filepath.Join(root, "two.txt")) != "two" || exists(filepath.Join(root, "three.txt")) {
		t.Error("the tree was not restored")
	}
}
