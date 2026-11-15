package adopt

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/upex-galaxy/jitso/internal/fail"
)

// Apply writes the changes to disk, in the order given. Backups come first:
// the existing files the changes alter or remove are saved once, before any
// write (NewBackup). Then each change runs and prints one line:
//
//	backed up <path> to <backup path>
//	wrote <path>        an existing file was replaced
//	created <path>      a new file, or a new folder
//	removed <path>
//	linked <path> -> <target>
//	unlinked <path>
//
// A file is written to a temporary file in its own folder and renamed over
// the target, so a reader never sees half a file. An existing file keeps its
// mode; new files get 0644 and new folders 0755. A change that would leave
// its path as it is, and the removal of something already gone, print nothing.
// A removal of a folder happens only when it is empty.
//
// The first failure stops the run with an environment error (exit 3) that
// names the path and, when a backup was made, its id.
func Apply(spec BackupSpec, changes []Change, out io.Writer) error {
	if err := preflight(changes); err != nil {
		return err
	}
	bk, err := NewBackup(spec, changes)
	if err != nil {
		return err
	}
	if bk != nil {
		for _, c := range bk.Copied {
			fmt.Fprintf(out, "backed up %s to %s\n", c.Display, spec.display(c.Copy))
		}
	}
	for i, c := range changes {
		line, err := applyOne(c)
		if err != nil {
			return writeError(c, err, i, bk)
		}
		if line != "" {
			io.WriteString(out, line+"\n")
		}
	}
	return nil
}

// preflight refuses, before anything is saved or written, a file change whose
// path is a symbolic link or a folder. jitso never writes through a link (the
// rename would replace the link with a plain file, and the backup would hold
// no content), so a dotfile that is a link is left for its owner to edit.
func preflight(changes []Change) error {
	for _, c := range changes {
		if c.Kind != KindFile {
			continue
		}
		info, err := os.Lstat(c.Path)
		switch {
		case err != nil:
		case info.Mode()&os.ModeSymlink != 0:
			return fail.Envf("%s is a symbolic link and jitso does not write through links; edit the file it points to by hand; nothing was written", c.Display)
		case info.IsDir():
			return fail.Envf("%s is a folder, not a file; nothing was written", c.Display)
		}
	}
	return nil
}

func writeError(c Change, err error, applied int, bk *Backup) error {
	var verb string
	switch c.Kind {
	case KindDelete:
		verb = "remove"
	case KindSymlink:
		verb = "link"
	case KindMkdir:
		verb = "create"
	default:
		verb = "write"
	}
	var tail string
	switch {
	case applied == 0 && bk == nil:
		tail = "nothing was written"
	case applied == 0:
		tail = "nothing was changed; backup " + bk.ID + " holds the originals"
	case bk == nil:
		tail = fmt.Sprintf("%d earlier change(s) were applied; none needed a backup", applied)
	default:
		tail = fmt.Sprintf("%d earlier change(s) were applied; backup %s holds the originals", applied, bk.ID)
	}
	return fail.Envf("could not %s %s: %v; %s", verb, c.Display, unwrapPath(err), tail)
}

// applyOne performs one change and returns the line to print, "" for none.
func applyOne(c Change) (string, error) {
	switch c.Kind {
	case KindFile:
		existed, err := writeFile(c.Path, c.New)
		if err != nil {
			return "", err
		}
		if existed {
			return "wrote " + c.Display, nil
		}
		return "created " + c.Display, nil
	case KindSymlink:
		return setSymlink(c)
	case KindDelete:
		return remove(c)
	case KindMkdir:
		info, err := os.Lstat(c.Path)
		if err == nil && info.IsDir() {
			return "", nil
		}
		if err := os.MkdirAll(c.Path, 0o755); err != nil {
			return "", err
		}
		return "created " + c.Display, nil
	}
	return "", fmt.Errorf("unknown change kind %d", c.Kind)
}

// writeFile replaces path with data through a temporary file in the same
// folder. existed tells whether a file was there.
func writeFile(path string, data []byte) (existed bool, err error) {
	mode := os.FileMode(0o644)
	switch info, err := os.Lstat(path); {
	case err == nil:
		if info.IsDir() {
			return false, fmt.Errorf("%s is a folder", path)
		}
		existed = true
		if info.Mode().IsRegular() {
			mode = info.Mode().Perm()
		}
	case !errors.Is(err, fs.ErrNotExist):
		return false, err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".jitso-*")
	if err != nil {
		return false, err
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	if werr == nil {
		werr = tmp.Sync()
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Chmod(name, mode)
	}
	if werr == nil {
		werr = os.Rename(name, path)
	}
	if werr != nil {
		os.Remove(name)
		return false, werr
	}
	return existed, nil
}

// setSymlink points the link at c.Target, or removes it when the target is "".
func setSymlink(c Change) (string, error) {
	existed := false
	switch info, err := os.Lstat(c.Path); {
	case err == nil:
		if info.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf("%s exists and is not a link", c.Path)
		}
		if err := os.Remove(c.Path); err != nil {
			return "", err
		}
		existed = true
	case !errors.Is(err, fs.ErrNotExist):
		return "", err
	}
	if c.Target == "" {
		if !existed {
			return "", nil
		}
		return "unlinked " + c.Display, nil
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return "", err
	}
	if err := os.Symlink(c.Target, c.Path); err != nil {
		return "", err
	}
	return "linked " + c.Display + " -> " + c.Target, nil
}

// remove deletes a file or a link, or a folder when it is empty.
func remove(c Change) (string, error) {
	info, err := os.Lstat(c.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if c.Prune && !info.IsDir() {
		return "", nil
	}
	if info.IsDir() {
		entries, err := os.ReadDir(c.Path)
		if err != nil {
			return "", err
		}
		if len(entries) > 0 {
			return "", nil
		}
	}
	if err := os.Remove(c.Path); err != nil {
		return "", err
	}
	return "removed " + c.Display, nil
}

// unwrapPath drops the operation and path an *fs.PathError repeats, keeping
// the reason ("permission denied"), because the message already names the file.
func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		return le.Err
	}
	return err
}
