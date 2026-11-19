package skill

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The embed tree is text only. go:embed silently skips a dotfile and cannot
// hold a link, so the source folder is read from disk to catch what it would
// drop, and the manifest name must never be shipped.
func TestSourceTreeHasNoDotfileOrLink(t *testing.T) {
	err := filepath.WalkDir(Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == Name {
			return nil
		}
		base := d.Name()
		if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") {
			t.Errorf("%s: the embed would skip a name that starts with %q", p, base[:1])
		}
		if base == ManifestName {
			t.Errorf("%s: the manifest is written by the installer, never embedded", p)
		}
		if d.Type()&fs.ModeSymlink != 0 {
			t.Errorf("%s is a symbolic link", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedFiles(t *testing.T) {
	files := Files()
	if len(files) == 0 {
		t.Fatal("nothing is embedded")
	}
	disk := map[string]string{}
	err := filepath.WalkDir(Name, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(Name, p)
		disk[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(disk) != len(files) {
		t.Errorf("the embed holds %d files, the folder %d", len(files), len(disk))
	}
	prev := ""
	for _, f := range files {
		if f.Path <= prev {
			t.Errorf("%q is not after %q: Files must be sorted by path", f.Path, prev)
		}
		prev = f.Path
		if !validRel(f.Path) {
			t.Errorf("%q is not a clean relative path", f.Path)
		}
		if disk[f.Path] != string(f.Data) {
			t.Errorf("%s differs from the source file", f.Path)
		}
		if strings.ContainsAny(string(f.Data), string([]rune{0x2013, 0x2014})) {
			t.Errorf("%s holds an em or en dash", f.Path)
		}
	}
}

// SKILL.md carries only name and description in its frontmatter, and the name
// is the folder's.
func TestSkillFrontmatter(t *testing.T) {
	var skill string
	for _, f := range Files() {
		if f.Path == "SKILL.md" {
			skill = string(f.Data)
		}
	}
	if !strings.HasPrefix(skill, "---\nname: jitso\ndescription: ") {
		t.Fatalf("frontmatter starts %q", skill[:min(len(skill), 60)])
	}
	end := strings.Index(skill[4:], "\n---\n")
	if end < 0 {
		t.Fatal("the frontmatter is not closed")
	}
	front := strings.Split(skill[4:4+end], "\n")
	if len(front) != 4 || front[2] != "metadata:" || front[3] != "  kind: workflow" {
		t.Fatalf("frontmatter is %q, want name, description and metadata.kind workflow only", front)
	}
	desc := strings.TrimPrefix(front[1], "description: ")
	if strings.TrimSpace(desc) == "" || strings.Contains(desc, ": ") {
		t.Errorf("description %q must be one plain line (a colon and space would need quoting)", desc)
	}
}
