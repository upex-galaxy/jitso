package rules

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedCorePins(t *testing.T) {
	if !strings.HasPrefix(embeddedCore, "# JITSO hard rules\n") {
		t.Errorf("core.md does not start with the heading %q", "# JITSO hard rules")
	}
	if !strings.HasPrefix(embeddedStyle, "# JITSO reply style\n") {
		t.Errorf("style.md does not start with the heading %q", "# JITSO reply style")
	}
	for name, text := range map[string]string{"core.md": embeddedCore, "style.md": embeddedStyle} {
		for _, bad := range []string{"\u2014", "\u2013", "\t", "\r"} {
			if strings.Contains(text, bad) {
				t.Errorf("%s contains %q", name, bad)
			}
		}
		if !strings.HasSuffix(text, "\n") {
			t.Errorf("%s does not end with a newline", name)
		}
	}
}

func TestEmbeddedCoreHasTenRules(t *testing.T) {
	n := 0
	for _, l := range strings.Split(embeddedCore, "\n") {
		if len(l) > 3 && l[0] >= '0' && l[0] <= '9' {
			n++
		}
	}
	if n != 10 {
		t.Errorf("core.md has %d numbered rules, want 10", n)
	}
	// Rule 9 names the files a session must not edit.
	for _, want := range []string{"jitso/builds/", "jitso/jitso.yaml", "jitso/hard-rules/", ".jitso/", "jitso build new", "jitso build draft", "jitso build save"} {
		if !strings.Contains(embeddedCore, want) {
			t.Errorf("core.md does not mention %q", want)
		}
	}
}

func TestLoad(t *testing.T) {
	file := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	tests := []struct {
		name    string
		fsys    fs.FS
		want    string
		wantErr string
	}{
		{"no folder: the embedded pair", fstest.MapFS{"other": file("x")}, Embedded(), ""},
		{"both files: joined by one newline", fstest.MapFS{"jitso/hard-rules/core.md": file("CORE\n"), "jitso/hard-rules/style.md": file("STYLE\n")}, "CORE\n\nSTYLE\n", ""},
		{"no trailing newline", fstest.MapFS{"jitso/hard-rules/core.md": file("CORE"), "jitso/hard-rules/style.md": file("STYLE")}, "CORE\nSTYLE", ""},
		{"empty files", fstest.MapFS{"jitso/hard-rules/core.md": file(""), "jitso/hard-rules/style.md": file("")}, "\n", ""},
		{"style missing", fstest.MapFS{"jitso/hard-rules/core.md": file("CORE")}, "", "style.md"},
		{"core missing", fstest.MapFS{"jitso/hard-rules/style.md": file("STYLE")}, "", "core.md"},
		{"folder empty", fstest.MapFS{"jitso/hard-rules/other.md": file("x")}, "", "core.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Load(tt.fsys)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want one naming %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmbeddedIsCoreNewlineStyle(t *testing.T) {
	if got, want := Embedded(), embeddedCore+"\n"+embeddedStyle; got != want {
		t.Errorf("Embedded() is not core, one newline, style")
	}
}
