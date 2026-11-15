package skill

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var shipped = []File{
	{"SKILL.md", []byte("---\nname: jitso\ndescription: x\n---\nbody\n")},
	{"references/a.md", []byte("a\n")},
}

func put(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// installed writes a copy of files and a manifest made by version into dir.
func installed(t *testing.T, dir, version string, files []File) {
	t.Helper()
	for _, f := range files {
		put(t, dir, f.Path, string(f.Data))
	}
	text, err := NewManifest(version, files).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	put(t, dir, ManifestName, string(text))
}

func TestInspect(t *testing.T) {
	want := NewManifest("v2", shipped)
	older := []File{{"SKILL.md", []byte("old\n")}}
	tests := []struct {
		name   string
		setup  func(t *testing.T, dir string)
		state  State
		edited []string
		why    string
	}{
		{"absent", func(t *testing.T, dir string) { os.RemoveAll(dir) }, Absent, nil, ""},
		{"current", func(t *testing.T, dir string) { installed(t, dir, "v1", shipped) }, Current, nil, ""},
		{"current with an empty extra folder", func(t *testing.T, dir string) {
			installed(t, dir, "v1", shipped)
			os.MkdirAll(filepath.Join(dir, "empty"), 0o755)
		}, Current, nil, ""},
		{"outdated: other content, every hash right", func(t *testing.T, dir string) { installed(t, dir, "v1", older) }, Outdated, nil, ""},
		{"outdated: a file added in the new version", func(t *testing.T, dir string) { installed(t, dir, "v1", shipped[:1]) }, Outdated, nil, ""},
		{"modified: a listed file edited", func(t *testing.T, dir string) {
			installed(t, dir, "v1", shipped)
			put(t, dir, "SKILL.md", "mine\n")
		}, Modified, []string{"SKILL.md"}, ""},
		{"modified: a listed file missing", func(t *testing.T, dir string) {
			installed(t, dir, "v1", shipped)
			os.Remove(filepath.Join(dir, "references", "a.md"))
		}, Modified, []string{"references/a.md"}, ""},
		{"modified: an extra file", func(t *testing.T, dir string) {
			installed(t, dir, "v1", shipped)
			put(t, dir, "notes.md", "n\n")
		}, Modified, []string{"notes.md"}, ""},
		{"modified: edited, missing and extra come in that order", func(t *testing.T, dir string) {
			installed(t, dir, "v1", shipped)
			put(t, dir, "SKILL.md", "mine\n")
			os.Remove(filepath.Join(dir, "references", "a.md"))
			put(t, dir, "z.md", "z\n")
			put(t, dir, "b/c.md", "c\n")
		}, Modified, []string{"SKILL.md", "references/a.md", "b/c.md", "z.md"}, ""},
		{"modified beats outdated", func(t *testing.T, dir string) {
			installed(t, dir, "v1", older)
			put(t, dir, "SKILL.md", "mine\n")
		}, Modified, []string{"SKILL.md"}, ""},
		{"unmanaged: no manifest", func(t *testing.T, dir string) { put(t, dir, "SKILL.md", "x\n") }, Unmanaged, nil, "no " + ManifestName},
		{"unmanaged: an empty folder", func(t *testing.T, dir string) { os.MkdirAll(dir, 0o755) }, Unmanaged, nil, "no " + ManifestName},
		{"unmanaged: a manifest that is not JSON", func(t *testing.T, dir string) { put(t, dir, ManifestName, "not json") }, Unmanaged, nil, "is not valid"},
		{"unmanaged: a manifest of another skill", func(t *testing.T, dir string) {
			put(t, dir, ManifestName, `{"v":1,"skill":"other","files":{}}`)
		}, Unmanaged, nil, "another skill"},
		{"unmanaged: a manifest naming a path outside", func(t *testing.T, dir string) {
			put(t, dir, ManifestName, `{"v":1,"skill":"jitso","files":{"../../x":"`+strings.Repeat("a", 64)+`"}}`)
		}, Unmanaged, nil, `"../../x"`},
		{"a link in place of the folder", func(t *testing.T, dir string) {
			os.RemoveAll(dir)
			if err := os.Symlink("elsewhere", dir); err != nil {
				t.Skip("no symbolic links here:", err)
			}
		}, NotFolder, nil, "a link to elsewhere"},
		{"a file in place of the folder", func(t *testing.T, dir string) {
			os.RemoveAll(dir)
			put(t, filepath.Dir(dir), filepath.Base(dir), "x")
		}, NotFolder, nil, "a file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "jitso")
			tt.setup(t, dir)
			f, err := Inspect(dir, want)
			if err != nil {
				t.Fatal(err)
			}
			if f.State != tt.state {
				t.Fatalf("state %v, want %v (%+v)", f.State, tt.state, f)
			}
			if !reflect.DeepEqual(f.Edited, tt.edited) {
				t.Errorf("edited %q, want %q", f.Edited, tt.edited)
			}
			if tt.why != "" && !strings.Contains(f.Why, tt.why) {
				t.Errorf("why %q, want one that holds %q", f.Why, tt.why)
			}
		})
	}
}

func TestInspectListsEntries(t *testing.T) {
	dir := t.TempDir()
	installed(t, dir, "v1", shipped)
	os.MkdirAll(filepath.Join(dir, "deep", "er"), 0o755)
	f, err := Inspect(dir, NewManifest("v1", shipped))
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{".jitso-skill.json", "SKILL.md", "references/a.md"}
	if !reflect.DeepEqual(f.Files, wantFiles) {
		t.Errorf("files %q, want %q", f.Files, wantFiles)
	}
	wantDirs := []string{"deep", "deep/er", "references"}
	if !reflect.DeepEqual(f.Dirs, wantDirs) {
		t.Errorf("dirs %q, want %q", f.Dirs, wantDirs)
	}
	if !f.Has("SKILL.md") || f.Has("nope") || f.Has("references") {
		t.Error("Has is wrong")
	}
}

func TestInspectKeepsALinkInsideAsAnEntry(t *testing.T) {
	dir := t.TempDir()
	installed(t, dir, "v1", shipped)
	if err := os.Symlink("SKILL.md", filepath.Join(dir, "alias.md")); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	f, err := Inspect(dir, NewManifest("v1", shipped))
	if err != nil {
		t.Fatal(err)
	}
	if f.State != Modified || !reflect.DeepEqual(f.Edited, []string{"alias.md"}) {
		t.Errorf("state %v edited %q", f.State, f.Edited)
	}
}

func TestStateString(t *testing.T) {
	for s, want := range map[State]string{Absent: "absent", Current: "current", Outdated: "outdated", Modified: "modified", Unmanaged: "unmanaged", NotFolder: "not a folder"} {
		if s.String() != want {
			t.Errorf("%d prints %q, want %q", int(s), s.String(), want)
		}
	}
}
