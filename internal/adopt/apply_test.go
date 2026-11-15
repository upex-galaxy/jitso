package adopt

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func unixOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("file modes and links are not portable to Windows")
	}
}

func TestApplyWritesWithBackupsFirst(t *testing.T) {
	unixOnly(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "edit.txt"), "old\n", 0o600)
	write(t, filepath.Join(root, "drop.txt"), "drop\n", 0o644)
	write(t, filepath.Join(root, "same.txt"), "same\n", 0o644)
	changes := []Change{
		{Path: filepath.Join(root, "new", "deep", "a.txt"), Display: "new/deep/a.txt", Kind: KindFile, New: []byte("a\n")},
		{Path: filepath.Join(root, "edit.txt"), Display: "edit.txt", Kind: KindFile, Old: []byte("old\n"), New: []byte("new\n")},
		{Path: filepath.Join(root, "drop.txt"), Display: "drop.txt", Kind: KindDelete},
		{Path: filepath.Join(root, "dir"), Display: "dir", Kind: KindMkdir},
		{Path: filepath.Join(root, "link"), Display: "link", Kind: KindSymlink, Target: "edit.txt"},
	}
	var out bytes.Buffer
	if err := Apply(spec(root), changes, &out); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"backed up edit.txt to .jitso/backups/20261115T073455Z-a3f9/edit.txt",
		"backed up drop.txt to .jitso/backups/20261115T073455Z-a3f9/drop.txt",
		"created new/deep/a.txt",
		"wrote edit.txt",
		"removed drop.txt",
		"created dir",
		"linked link -> edit.txt",
	}, "\n") + "\n"
	if out.String() != want {
		t.Errorf("output\n%s\nwant\n%s", out.String(), want)
	}
	if read(t, filepath.Join(root, "edit.txt")) != "new\n" || read(t, filepath.Join(root, "new", "deep", "a.txt")) != "a\n" {
		t.Error("files not written")
	}
	if exists(filepath.Join(root, "drop.txt")) {
		t.Error("drop.txt still there")
	}
	if target, err := os.Readlink(filepath.Join(root, "link")); err != nil || target != "edit.txt" {
		t.Errorf("link -> %q, %v", target, err)
	}
	if info, _ := os.Stat(filepath.Join(root, "dir")); info == nil || !info.IsDir() {
		t.Error("dir not created")
	}
	// the backup holds what was there, before the write
	bdir := filepath.Join(root, ".jitso", "backups", "20261115T073455Z-a3f9")
	if read(t, filepath.Join(bdir, "edit.txt")) != "old\n" || read(t, filepath.Join(bdir, "drop.txt")) != "drop\n" {
		t.Error("backup content is wrong")
	}
	// no leftover temporary files
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".jitso-") {
			t.Errorf("temporary file left: %s", e.Name())
		}
	}
}

func TestApplyModes(t *testing.T) {
	unixOnly(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "private.txt"), "1", 0o600)
	write(t, filepath.Join(root, "exec.sh"), "1", 0o755)
	write(t, filepath.Join(root, "readonly.txt"), "1", 0o444)
	changes := []Change{
		{Path: filepath.Join(root, "private.txt"), Display: "private.txt", Kind: KindFile, New: []byte("2")},
		{Path: filepath.Join(root, "exec.sh"), Display: "exec.sh", Kind: KindFile, New: []byte("2")},
		{Path: filepath.Join(root, "readonly.txt"), Display: "readonly.txt", Kind: KindFile, New: []byte("2")},
		{Path: filepath.Join(root, "fresh", "new.txt"), Display: "fresh/new.txt", Kind: KindFile, New: []byte("2")},
	}
	if err := Apply(spec(root), changes, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{
		"private.txt": 0o600, "exec.sh": 0o755, "readonly.txt": 0o444, "fresh/new.txt": 0o644,
	} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s mode = %v (%v), want %v", name, info.Mode().Perm(), err, want)
		}
		if read(t, filepath.Join(root, filepath.FromSlash(name))) != "2" {
			t.Errorf("%s not rewritten", name)
		}
	}
	if info, _ := os.Stat(filepath.Join(root, "fresh")); info.Mode().Perm() != 0o755 {
		t.Errorf("new folder mode = %v", info.Mode().Perm())
	}
}

func TestApplySymlinks(t *testing.T) {
	unixOnly(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "real", "f"), "x", 0o644)
	link := filepath.Join(root, ".claude", "skills", "jitso")
	create := Change{Path: link, Display: ".claude/skills/jitso", Kind: KindSymlink, Target: "../../real"}
	var out bytes.Buffer
	if err := Apply(spec(root), []Change{create}, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "linked .claude/skills/jitso -> ../../real\n" {
		t.Errorf("out = %q", out.String())
	}
	if read(t, filepath.Join(link, "f")) != "x" {
		t.Error("the link does not resolve")
	}
	if exists(filepath.Join(root, ".jitso")) {
		t.Error("a link alone must not make a backup")
	}

	out.Reset()
	repoint := Change{Path: link, Display: ".claude/skills/jitso", Kind: KindSymlink, Target: "../../other", OldTarget: "../../real"}
	if err := Apply(spec(root), []Change{repoint}, &out); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != "../../other" || out.String() != "linked .claude/skills/jitso -> ../../other\n" {
		t.Errorf("link -> %q, out %q", got, out.String())
	}

	out.Reset()
	remove := Change{Path: link, Display: ".claude/skills/jitso", Kind: KindSymlink, OldTarget: "../../other"}
	if err := Apply(spec(root), []Change{remove}, &out); err != nil {
		t.Fatal(err)
	}
	if exists(link) || out.String() != "unlinked .claude/skills/jitso\n" {
		t.Errorf("link still there or out %q", out.String())
	}
	if !exists(filepath.Join(root, "real", "f")) {
		t.Error("removing the link removed its target")
	}

	// a path that is not a link is never replaced
	write(t, filepath.Join(root, "plain"), "x", 0o644)
	err := Apply(spec(root), []Change{{Path: filepath.Join(root, "plain"), Display: "plain", Kind: KindSymlink, Target: "t"}}, &bytes.Buffer{})
	if err == nil || fail.Code(err) != 3 || !strings.Contains(err.Error(), "is not a link") {
		t.Errorf("err = %v", err)
	}
}

func TestApplyRemoveFolders(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "b", "f"), "x", 0o644)
	write(t, filepath.Join(root, "keep", "f"), "x", 0o644)
	var out bytes.Buffer
	changes := []Change{
		{Path: filepath.Join(root, "a", "b", "f"), Display: "a/b/f", Kind: KindDelete},
		{Path: filepath.Join(root, "a", "b"), Display: "a/b", Kind: KindDelete},
		{Path: filepath.Join(root, "a"), Display: "a", Kind: KindDelete},
		{Path: filepath.Join(root, "keep"), Display: "keep", Kind: KindDelete}, // not empty: left alone
		{Path: filepath.Join(root, "never-there"), Display: "never-there", Kind: KindDelete},
	}
	if err := Apply(spec(root), changes, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "removed a/b/f\nremoved a/b\nremoved a\n") || strings.Contains(out.String(), "keep") || strings.Contains(out.String(), "never") {
		t.Errorf("out = %q", out.String())
	}
	if exists(filepath.Join(root, "a")) || !exists(filepath.Join(root, "keep", "f")) {
		t.Error("wrong folders removed")
	}
}

func TestApplyWriteFailure(t *testing.T) {
	unixOnly(t)
	if os.Geteuid() == 0 {
		t.Skip("root can write into a read-only folder")
	}
	newFile := func(root, rel string) Change {
		return Change{Path: filepath.Join(root, filepath.FromSlash(rel)), Display: rel, Kind: KindFile, New: []byte("n")}
	}
	t.Run("after some writes, naming the file and the backup", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "a.txt"), "old", 0o644)
		ro := filepath.Join(root, "ro")
		if err := os.Mkdir(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		edit := newFile(root, "a.txt")
		var out bytes.Buffer
		err := Apply(spec(root), []Change{edit, newFile(root, "ro/b.txt"), newFile(root, "c.txt")}, &out)
		if fail.Code(err) != 3 {
			t.Fatalf("exit %d, err %v", fail.Code(err), err)
		}
		text := fail.Text(err)
		if !strings.HasPrefix(text, "jitso: could not write ro/b.txt: permission denied; ") ||
			!strings.Contains(text, "1 earlier change(s) were applied; backup 20261115T073455Z-a3f9 holds the originals") {
			t.Errorf("text = %q", text)
		}
		if read(t, filepath.Join(root, "a.txt")) != "n" || exists(filepath.Join(root, "c.txt")) {
			t.Error("the first change should be written and the last not")
		}
		if exists(filepath.Join(ro, "b.txt")) {
			t.Error("b.txt exists")
		}
		if err := Restore(filepath.Join(root, ".jitso", "backups", "20261115T073455Z-a3f9"), root); err != nil || read(t, filepath.Join(root, "a.txt")) != "old" {
			t.Errorf("restore: %v", err)
		}
	})
	t.Run("the first write, no backup", func(t *testing.T) {
		root := t.TempDir()
		ro := filepath.Join(root, "ro")
		if err := os.Mkdir(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		err := Apply(spec(root), []Change{newFile(root, "ro/b.txt")}, &bytes.Buffer{})
		if got := fail.Text(err); got != "jitso: could not write ro/b.txt: permission denied; nothing was written" {
			t.Errorf("text = %q", got)
		}
	})
	t.Run("the first write, backup already saved", func(t *testing.T) {
		root := t.TempDir()
		write(t, filepath.Join(root, "a.txt"), "old", 0o644)
		ro := filepath.Join(root, "ro")
		if err := os.Mkdir(ro, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(ro, 0o755) })
		err := Apply(spec(root), []Change{newFile(root, "ro/b.txt"), newFile(root, "a.txt")}, &bytes.Buffer{})
		if got := fail.Text(err); got != "jitso: could not write ro/b.txt: permission denied; nothing was changed; backup 20261115T073455Z-a3f9 holds the originals" {
			t.Errorf("text = %q", got)
		}
	})
	t.Run("a folder where a file should go", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		err := Apply(spec(root), []Change{newFile(root, "dir")}, &bytes.Buffer{})
		if got := fail.Text(err); got != "jitso: dir is a folder, not a file; nothing was written" {
			t.Errorf("text = %q", got)
		}
	})
}

func TestApplyLeavesNoTempFileOnFailure(t *testing.T) {
	unixOnly(t)
	root := t.TempDir()
	// renaming a file over a non-empty folder fails after the temp file is written
	write(t, filepath.Join(root, "d", "inner"), "x", 0o644)
	_, err := writeFile(filepath.Join(root, "d"), []byte("y"))
	if err == nil {
		t.Fatal("expected an error")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || entries[0].Name() != "d" {
		t.Errorf("entries = %v", entries)
	}
}

func TestApplyRefusesToWriteThroughALink(t *testing.T) {
	unixOnly(t)
	root := t.TempDir()
	write(t, filepath.Join(root, "real.md"), "real\n", 0o644)
	write(t, filepath.Join(root, "other.txt"), "old\n", 0o644)
	if err := os.Symlink("real.md", filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	changes := []Change{
		{Path: filepath.Join(root, "other.txt"), Display: "other.txt", Kind: KindFile, New: []byte("new\n")},
		{Path: filepath.Join(root, "CLAUDE.md"), Display: "CLAUDE.md", Kind: KindFile, New: []byte("x\n")},
	}
	var out bytes.Buffer
	err := Apply(spec(root), changes, &out)
	want := "jitso: CLAUDE.md is a symbolic link and jitso does not write through links; edit the file it points to by hand; nothing was written"
	if fail.Code(err) != 3 || fail.Text(err) != want {
		t.Fatalf("code %d text %q", fail.Code(err), fail.Text(err))
	}
	if out.Len() != 0 || read(t, filepath.Join(root, "other.txt")) != "old\n" || read(t, filepath.Join(root, "real.md")) != "real\n" || exists(filepath.Join(root, ".jitso")) {
		t.Errorf("something was written or saved: %q", out.String())
	}
	if target, _ := os.Readlink(filepath.Join(root, "CLAUDE.md")); target != "real.md" {
		t.Errorf("the link was changed: %q", target)
	}
}

func TestApplyPrune(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a", "b", "f"), "x", 0o644)
	write(t, filepath.Join(root, "a", "keep"), "x", 0o644)
	write(t, filepath.Join(root, "plain"), "x", 0o644)
	var out bytes.Buffer
	changes := []Change{
		{Path: filepath.Join(root, "a", "b", "f"), Display: "a/b/f", Kind: KindDelete},
		{Path: filepath.Join(root, "a", "b"), Display: "a/b", Kind: KindDelete, Prune: true},
		{Path: filepath.Join(root, "a"), Display: "a", Kind: KindDelete, Prune: true},
		{Path: filepath.Join(root, "plain"), Display: "plain", Kind: KindDelete, Prune: true},
	}
	if err := Apply(spec(root), changes, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "removed a/b/f\nremoved a/b\n") {
		t.Errorf("out = %q", out.String())
	}
	if exists(filepath.Join(root, "a", "b")) || !exists(filepath.Join(root, "a", "keep")) || !exists(filepath.Join(root, "plain")) {
		t.Error("prune removed the wrong things")
	}
}
