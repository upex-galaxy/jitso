package drafts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/catalog"
)

// The status words of a build. A draft is saved by changing the first into the
// second and nothing else.
const (
	statusDraft    = "draft"
	statusApproved = "approved"
)

// TokenLength is the number of hex characters of a confirmation token.
const TokenLength = 12

// ErrStatusForm is returned by SaveContent for a draft whose status cannot be
// changed without touching any other byte: it is not written as the plain word
// `draft` or as `draft` in single or double quotes, or it is not a top-level
// scalar.
var ErrStatusForm = errors.New("the status is not written as the plain word draft, or in quotes")

// ErrExists is returned by Save when the target file already exists.
var ErrExists = errors.New("the build file already exists")

// SaveContent is what saving a draft writes: the draft bytes with the value of
// the top-level `status` changed from draft to approved and every other byte
// kept (a quoted value keeps its quotes). The position of the value comes from
// the YAML node, so the word draft elsewhere in the file is never touched. A
// status that is not that scalar is ErrStatusForm.
func SaveContent(draft []byte) ([]byte, error) {
	start, end, ok := catalog.StatusSpan(draft)
	if !ok || string(draft[start:end]) != statusDraft {
		return nil, ErrStatusForm
	}
	out := make([]byte, 0, len(draft)+len(statusApproved)-len(statusDraft))
	out = append(out, draft[:start]...)
	out = append(out, statusApproved...)
	return append(out, draft[end:]...), nil
}

// Token is the confirmation token of content: the first TokenLength hex
// characters of its SHA-256. It binds an approval to the exact bytes that were
// read; it is not a secret.
func Token(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])[:TokenLength]
}

// Show is how content is shown to the person who approves it: every line
// prefixed with `+ `, each ended by a newline. A control character other than
// a tab, and a Unicode line or paragraph separator, is shown as \xNN (the code
// point in hex), so content cannot hide text with a terminal escape sequence or
// a carriage return; ordinary files are shown as they are.
func Show(content []byte) string {
	text := string(content)
	text = strings.TrimSuffix(text, "\n")
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		b.WriteString("+ ")
		for _, r := range line {
			if r == '\t' || (r >= 0x20 && r != 0x7f && !(r >= 0x80 && r < 0xa0) && r != 0x2028 && r != 0x2029) {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(&b, `\x%02x`, r)
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// BuildFile is the path of the saved build of id: <primary>/jitso/builds/<id>.yaml.
func BuildFile(primary, id string) string {
	return filepath.Join(primary, filepath.FromSlash(catalog.BuildsDir), id+ExtensionYAML)
}

// Save writes content to jitso/builds/<id>.yaml of the project whose primary
// checkout is primary, then removes the draft of id. It never overwrites: a
// target that exists, a dangling symlink included, is ErrExists, and the file
// is created exclusively so a race cannot replace it either. It refuses an id
// that is not ValidID. When the draft cannot be removed after the build was
// written, the error says so and the build stays.
func Save(primary, id string, content []byte) error {
	if !ValidID(id) {
		return fmt.Errorf("%q is not a build id", id)
	}
	target := BuildFile(primary, id)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrExists
		}
		return err
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		os.Remove(target)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(target)
		return err
	}
	if err := Remove(primary, id); err != nil && !errors.Is(err, ErrNoDraft) {
		return fmt.Errorf("saved %s but could not remove the draft: %w", catalog.BuildsDir+"/"+id+ExtensionYAML, err)
	}
	return nil
}
