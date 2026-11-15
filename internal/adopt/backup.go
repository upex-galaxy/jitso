package adopt

import (
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/jsonx"
)

// ManifestName is the file in a backup folder that lists what the command touched.
const ManifestName = "manifest.json"

// idLayout is the UTC time in the backup id: 20261115T073455Z.
const idLayout = "20060102T150405Z"

// maxIDTries bounds the search for a free backup id.
const maxIDTries = 32

// Manifest is manifest.json: every path the command touched, relative to the
// scope root, with whether it existed before. A symlink is recorded with its
// target and not copied.
type Manifest struct {
	V            int            `json:"v"`
	Command      string         `json:"command"`
	Scope        string         `json:"scope"`
	TS           string         `json:"ts"`
	JitsoVersion string         `json:"jitso_version"`
	Files        []ManifestFile `json:"files"`
}

// ManifestFile is one entry of a Manifest.
type ManifestFile struct {
	Path    string `json:"path"`
	Existed bool   `json:"existed"`
	Symlink string `json:"symlink,omitempty"`
}

// BackupSpec is what a backup needs besides the changes.
type BackupSpec struct {
	// Dir is the folder that holds the backups: <primary>/.jitso/backups for
	// project scope, ~/.jitso/backups for user scope.
	Dir string
	// Root is the scope root (the checkout, or HOME): the tree inside a backup
	// mirrors the paths relative to it.
	Root string
	// Command is the command name for the manifest, for example "setup hook".
	Command string
	// Scope is "project" or "user".
	Scope string
	// Version is the jitso version for the manifest.
	Version string
	// Now is the clock and Rand the randomness source of the id.
	Now  func() time.Time
	Rand io.Reader
	// Display renders an absolute path for the output lines; nil prints the path as is.
	Display func(string) string
}

// Copied is one file saved in a backup.
type Copied struct {
	// Display is the path of the original as the plan shows it.
	Display string
	// Copy is the absolute path of the saved copy.
	Copy string
}

// Backup is a backup that was written.
type Backup struct {
	// ID is <UTC YYYYMMDDThhmmssZ>-<4 hex>.
	ID string
	// Dir is the absolute path of the folder of this backup.
	Dir string
	// Copied lists the files saved, in plan order.
	Copied []Copied
}

// NewID is a backup id: the UTC time as YYYYMMDDThhmmssZ, a dash and 4 random
// hex digits (two bytes read from rand).
func NewID(now time.Time, rand io.Reader) (string, error) {
	var b [2]byte
	if _, err := io.ReadFull(rand, b[:]); err != nil {
		return "", fail.Envf("could not read random bytes for the backup id: %v", err)
	}
	return now.UTC().Format(idLayout) + "-" + hex.EncodeToString(b[:]), nil
}

// NewBackup saves the current bytes of every existing file the changes alter
// or remove, before anything is written. It writes nothing and returns nil
// when no existing file is touched (a backup exists only when something that
// exists is changed or removed). A symlink or a path that does not exist is
// recorded in the manifest but never copied. The copy is read from the disk
// now, not from Change.Old, so it holds what is really there.
func NewBackup(spec BackupSpec, changes []Change) (*Backup, error) {
	type item struct {
		change Change
		rel    string
		info   os.FileInfo // nil when the path does not exist
	}
	var items []item
	needed := false
	for _, c := range changes {
		if c.Kind == KindMkdir {
			continue
		}
		rel, err := relInside(spec.Root, c.Path)
		if err != nil {
			return nil, err
		}
		info, err := os.Lstat(c.Path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			info = nil
		case err != nil:
			return nil, fail.Envf("could not look at %s: %v", c.Display, err)
		case info.IsDir():
			// A folder is neither copied nor listed (an empty folder is pruned,
			// and restoring must never remove one).
			continue
		case info.Mode().IsRegular():
			needed = true
		}
		items = append(items, item{c, rel, info})
	}
	if !needed {
		return nil, nil
	}

	if err := os.MkdirAll(spec.Dir, 0o755); err != nil {
		return nil, fail.Envf("could not create the backup folder %s: %v", spec.display(spec.Dir), err)
	}
	if err := writeIfMissing(filepath.Join(spec.Dir, ".gitignore"), []byte("*\n")); err != nil {
		return nil, fail.Envf("could not write the backup folder's .gitignore: %v", err)
	}
	id, dir, err := spec.claim()
	if err != nil {
		return nil, err
	}
	bk := &Backup{ID: id, Dir: dir}
	man := Manifest{
		V:            1,
		Command:      spec.Command,
		Scope:        spec.Scope,
		TS:           jsonx.Time(spec.Now()),
		JitsoVersion: spec.Version,
		Files:        []ManifestFile{},
	}
	for _, it := range items {
		entry := ManifestFile{Path: it.rel, Existed: it.info != nil}
		if it.info != nil && it.info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(it.change.Path)
			if err != nil {
				return nil, fail.Envf("could not read the link %s: %v (backup %s)", it.change.Display, err, id)
			}
			entry.Symlink = target
		} else if it.info != nil && it.info.Mode().IsRegular() {
			data, err := os.ReadFile(it.change.Path)
			if err != nil {
				return nil, fail.Envf("could not read %s for the backup: %v (backup %s)", it.change.Display, err, id)
			}
			copyPath := filepath.Join(dir, filepath.FromSlash(it.rel))
			if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
				return nil, fail.Envf("could not create the folder for the copy of %s: %v (backup %s)", it.change.Display, err, id)
			}
			if err := os.WriteFile(copyPath, data, it.info.Mode().Perm()); err != nil {
				return nil, fail.Envf("could not save the copy of %s: %v (backup %s)", it.change.Display, err, id)
			}
			bk.Copied = append(bk.Copied, Copied{Display: it.change.Display, Copy: copyPath})
		}
		man.Files = append(man.Files, entry)
	}
	text, err := jsonx.Marshal(man)
	if err != nil {
		return nil, fail.Envf("could not encode the backup manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), append(text, '\n'), 0o644); err != nil {
		return nil, fail.Envf("could not write the backup manifest: %v (backup %s)", err, id)
	}
	return bk, nil
}

// claim creates the folder of a new backup. An id that already has a folder is
// never reused: it draws another one.
func (s BackupSpec) claim() (id, dir string, err error) {
	for range maxIDTries {
		id, err = NewID(s.Now(), s.Rand)
		if err != nil {
			return "", "", err
		}
		dir = filepath.Join(s.Dir, id)
		switch err := os.Mkdir(dir, 0o755); {
		case err == nil:
			return id, dir, nil
		case errors.Is(err, fs.ErrExist):
		default:
			return "", "", fail.Envf("could not create the backup folder %s: %v", s.display(dir), err)
		}
	}
	return "", "", fail.Envf("could not find a free backup id in %s", s.display(s.Dir))
}

func (s BackupSpec) display(path string) string {
	if s.Display == nil {
		return path
	}
	return s.Display(path)
}

// writeIfMissing writes data to path unless something is already there.
func writeIfMissing(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr
}
