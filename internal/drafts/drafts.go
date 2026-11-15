// Package drafts is the drafts folder of a project: <primary>/.jitso/drafts/,
// where `jitso build draft` keeps a build that is not saved yet (defect N-15).
// A draft is the file <id>.yaml of that folder. The folder holds its own
// .gitignore, so a draft is never committed by accident.
//
// The pure checks (what an input may be, what an id may be) are separate from
// the functions that touch the file system, which take the primary checkout
// and write only inside the folder. What a draft means (the build rules, the
// layers) is the catalog's; this package only keeps files.
package drafts

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
)

// MaxBytes is the largest input a draft may be: 64 KiB.
const MaxBytes = 64 << 10

// ExtensionYAML is the extension of a draft file.
const ExtensionYAML = ".yaml"

// IgnoreFile is the file that makes the folder ignore itself, and IgnoreContent
// its bytes: one line, `*`.
const (
	IgnoreFile    = ".gitignore"
	IgnoreContent = "*\n"
)

// InputRule is what a draft's input must be, as the command says it when the
// input is not.
const InputRule = "a draft is a .yaml file (or - for stdin) of at most 64 KiB"

// ErrBadInput is returned by CheckInput for an argument that is neither a
// .yaml or .yml path nor `-`, and ErrTooLarge by Read for an input of more
// than MaxBytes. Both say InputRule.
var (
	ErrBadInput = errors.New(InputRule)
	ErrTooLarge = errors.New(InputRule)
)

// ErrNoDraft is returned when the folder has no draft of the id.
var ErrNoDraft = errors.New("no such draft")

// idPattern is the launcher's id pattern: a kebab-case word of 1 to 64
// characters.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)

// ValidID reports whether id may name a draft file: kebab-case, at most 64
// characters. It is what keeps an id from being read as a path.
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Stdin is the input word that means standard input.
const Stdin = "-"

// CheckInput is the pure check of the argument that names a draft's source: a
// path ending in .yaml or .yml, or `-` for standard input. It reports whether
// the source is stdin. Any other argument, the empty one included, is an
// error.
func CheckInput(arg string) (stdin bool, err error) {
	if arg == Stdin {
		return true, nil
	}
	if strings.HasSuffix(arg, ".yaml") || strings.HasSuffix(arg, ".yml") {
		return false, nil
	}
	return false, ErrBadInput
}

// Read reads a draft's bytes from r, at most MaxBytes: it never reads more than
// MaxBytes+1 bytes, so a device or an endless input cannot fill memory. More
// than MaxBytes is ErrTooLarge.
func Read(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, ErrTooLarge
	}
	return data, nil
}

// Dir is the drafts folder of the project whose primary checkout is primary.
func Dir(primary string) string {
	return filepath.Join(primary, filepath.FromSlash(catalog.DraftsDir))
}

// File is the path of the draft of id.
func File(primary, id string) string {
	return filepath.Join(Dir(primary), id+ExtensionYAML)
}

// Put writes the draft of id with exactly data, replacing an earlier draft of
// that id, and creates the folder and its .gitignore (`*`) when they are
// missing. An existing .gitignore is left as it is. The file is written next to
// its place and renamed into it, so a reader never sees half a draft, and a
// symlink at the final name is replaced, never followed. It refuses an id that
// is not ValidID and a folder that is a symlink.
func Put(primary, id string, data []byte) error {
	if !ValidID(id) {
		return fmt.Errorf("%q is not a draft id", id)
	}
	dir := Dir(primary)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(dir); err != nil {
		return err
	} else if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", catalog.DraftsDir)
	}
	ignore := filepath.Join(dir, IgnoreFile)
	if _, err := os.Lstat(ignore); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(ignore, []byte(IgnoreContent), 0o644); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(dir, ".draft-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, File(primary, id)); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Get is the bytes of the draft of id, ErrNoDraft when there is none.
func Get(primary, id string) ([]byte, error) {
	if !ValidID(id) {
		return nil, ErrNoDraft
	}
	data, err := os.ReadFile(File(primary, id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNoDraft
	}
	return data, err
}

// List are the ids of the drafts, by file name: the *.yaml files of the folder
// whose name is a valid id. A missing folder has none.
func List(primary string) ([]string, error) {
	entries, err := os.ReadDir(Dir(primary))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ExtensionYAML)
		if ok && !e.IsDir() && ValidID(id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Remove deletes the draft of id, ErrNoDraft when there is none. It refuses an
// id that is not ValidID with ErrNoDraft too: such an id names no draft, and it
// is never turned into a path.
func Remove(primary, id string) error {
	if !ValidID(id) {
		return ErrNoDraft
	}
	if err := os.Remove(File(primary, id)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNoDraft
		}
		return err
	}
	return nil
}

// RemoveAll deletes every *.yaml file of the folder, whatever its name, and
// returns how many it deleted. The .gitignore and any other file stay, and so
// does the folder. A missing folder has none to delete.
func RemoveAll(primary string) (int, error) {
	dir := Dir(primary)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ExtensionYAML) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
