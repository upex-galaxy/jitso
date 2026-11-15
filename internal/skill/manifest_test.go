package skill

import (
	"strings"
	"testing"
)

func TestHashBytes(t *testing.T) {
	// SHA-256 of "old skill\n", measured with shasum.
	if got, want := HashBytes([]byte("old skill\n")), "f6f3108de5373de3c872bb097f49e4abdd0c0c471bf03fc0083775c7f853dcd4"; got != want {
		t.Errorf("HashBytes = %s, want %s", got, want)
	}
}

func TestContentHashIsOverPathsInOrder(t *testing.T) {
	a, b := HashBytes([]byte("a")), HashBytes([]byte("b"))
	want := HashBytes([]byte("SKILL.md\x00" + a + "\nreferences/x.md\x00" + b + "\n"))
	// "SKILL.md" sorts before "references/x.md" (upper case first).
	if got := ContentHash(map[string]string{"references/x.md": b, "SKILL.md": a}); got != want {
		t.Errorf("ContentHash = %s, want %s", got, want)
	}
	if ContentHash(map[string]string{"SKILL.md": b}) == ContentHash(map[string]string{"SKILL.md": a}) {
		t.Error("a changed file hash did not change the content hash")
	}
	if ContentHash(map[string]string{"a": a}) == ContentHash(map[string]string{"b": a}) {
		t.Error("a renamed file did not change the content hash")
	}
}

func TestNewManifestAndMarshal(t *testing.T) {
	m := NewManifest("v0.1.0", []File{{"SKILL.md", []byte("a")}, {"references/x.md", []byte("b")}})
	if m.V != 1 || m.Skill != "jitso" || m.JitsoVersion != "v0.1.0" {
		t.Errorf("manifest %+v", m)
	}
	if m.Files["SKILL.md"] != HashBytes([]byte("a")) || m.ContentSHA256 != ContentHash(m.Files) {
		t.Errorf("hashes %+v", m)
	}
	out, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "v": 1,
  "skill": "jitso",
  "jitso_version": "v0.1.0",
  "content_sha256": "` + m.ContentSHA256 + `",
  "files": {
    "SKILL.md": "` + HashBytes([]byte("a")) + `",
    "references/x.md": "` + HashBytes([]byte("b")) + `"
  }
}
`
	if string(out) != want {
		t.Errorf("Marshal =\n%s\nwant\n%s", out, want)
	}
	back, err := ParseManifest(out)
	if err != nil || !back.Matches(m) || back.JitsoVersion != "v0.1.0" {
		t.Errorf("round trip: %+v, %v", back, err)
	}
}

func TestMatches(t *testing.T) {
	base := NewManifest("v1", []File{{"SKILL.md", []byte("a")}})
	other := NewManifest("v2", []File{{"SKILL.md", []byte("a")}})
	if !base.Matches(other) {
		t.Error("the version alone must not make two manifests differ")
	}
	if base.Matches(NewManifest("v1", []File{{"SKILL.md", []byte("b")}})) {
		t.Error("a changed file matched")
	}
	if base.Matches(NewManifest("v1", []File{{"SKILL.md", []byte("a")}, {"x.md", []byte("a")}})) {
		t.Error("an added file matched")
	}
	forged := base
	forged.ContentSHA256 = strings.Repeat("0", 64)
	if forged.Matches(base) {
		t.Error("a forged content hash matched")
	}
}

func TestParseManifestRefusals(t *testing.T) {
	h := strings.Repeat("a", 64)
	tests := []struct {
		name, text, want string
	}{
		{"not json", `{`, "not valid JSON"},
		{"another format", `{"v":2,"skill":"jitso","files":{}}`, "unknown format version 2"},
		{"another skill", `{"v":1,"skill":"x","files":{}}`, "another skill"},
		{"no files", `{"v":1,"skill":"jitso"}`, "lists no files"},
		{"a path out of the folder", `{"v":1,"skill":"jitso","files":{"../x":"` + h + `"}}`, `"../x"`},
		{"an absolute path", `{"v":1,"skill":"jitso","files":{"/etc/x":"` + h + `"}}`, `"/etc/x"`},
		{"an unclean path", `{"v":1,"skill":"jitso","files":{"a/../b":"` + h + `"}}`, `"a/../b"`},
		{"a backslash", `{"v":1,"skill":"jitso","files":{"a\\b":"` + h + `"}}`, `"a\\b"`},
		{"the manifest itself", `{"v":1,"skill":"jitso","files":{".jitso-skill.json":"` + h + `"}}`, ManifestName},
		{"a short hash", `{"v":1,"skill":"jitso","files":{"a":"abc"}}`, "bad hash"},
		{"an upper case hash", `{"v":1,"skill":"jitso","files":{"a":"` + strings.ToUpper(h) + `"}}`, "bad hash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tt.text))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v, want one that holds %q", err, tt.want)
			}
		})
	}
	if _, err := ParseManifest([]byte(`{"v":1,"skill":"jitso","jitso_version":"v0","content_sha256":"` + h + `","files":{"a/b.md":"` + h + `"},"extra":true}`)); err != nil {
		t.Errorf("a manifest with an unknown key must still read: %v", err)
	}
}
