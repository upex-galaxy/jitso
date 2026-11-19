package brief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func TestPointer(t *testing.T) {
	tests := []struct {
		name, text, target string
		ok                 bool
	}{
		{"plain", "Brief: docs/c.md\n", "docs/c.md", true},
		{"any case", "bRiEf:docs/c.md", "docs/c.md", true},
		{"backticks", "BRIEF: `my docs/c.md`\n", "my docs/c.md", true},
		{"blank lines around", "\n \n  Brief: x.md  \n\n", "x.md", true},
		{"no path", "Brief:", "", true},
		{"two lines", "Brief: x.md\nmore\n", "", false},
		{"other word", "Briefs: x.md", "", false},
		{"empty file", "", "", false},
		{"a heading is not a pointer", "## Goal\n", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, ok := Pointer(tt.text)
			if target != tt.target || ok != tt.ok {
				t.Errorf("got %q %v, want %q %v", target, ok, tt.target, tt.ok)
			}
		})
	}
}

func TestResolveTarget(t *testing.T) {
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	root := filepath.Join(tmp, "p")
	outside := filepath.Join(tmp, "outside.md")
	files := map[string]string{filepath.Join(root, "docs", "c.md"): "x", filepath.Join(root, "notes.txt"): "x", filepath.Join(root, "UP.MD"): "x", outside: "x"}
	for path, data := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{"in.md": "docs/c.md", "out.md": outside, "txt.md": "notes.txt"} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		target, want, reason string
	}{
		{"docs/c.md", filepath.Join(root, "docs", "c.md"), ""},
		{"in.md", filepath.Join(root, "docs", "c.md"), ""},
		{"UP.MD", filepath.Join(root, "UP.MD"), ""},
		{filepath.Join(root, "docs", "c.md"), filepath.Join(root, "docs", "c.md"), ""},
		{"", "", "brief target path is empty or contains a NUL byte"},
		{"a\x00b", "", "brief target path is empty or contains a NUL byte"},
		{"nope.md", "", "brief target does not exist: nope.md"},
		{"docs", "", "brief target is not a regular file: docs"},
		{"out.md", "", "brief target is outside the project: out.md"},
		{outside, "", "brief target is outside the project: " + outside},
		{"notes.txt", "", "brief target must be a .md file: notes.txt"},
		{"txt.md", "", "brief target must be a .md file: txt.md"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got, err := ResolveTarget(tt.target, root)
			if tt.reason == "" {
				if err != nil || got != tt.want {
					t.Errorf("got %q %v", got, err)
				}
				return
			}
			if err == nil || fail.Code(err) != 2 || !strings.Contains(err.Error(), tt.reason) {
				t.Errorf("got %v, want %q", err, tt.reason)
			}
		})
	}
}

func TestCheckTarget(t *testing.T) {
	short := "Name the file that holds the sync entry point here"
	if got := CheckTarget(short, LanguageEnglish); len(got) != 1 || got[0] != (Failure{CheckLength, "10 words, expected at least 40"}) {
		t.Errorf("short: %v", got)
	}
	long := strings.Repeat("Read the file and name the entry point of the sync code. ", 10)
	if got := CheckTarget(long, LanguageEnglish); got != nil {
		t.Errorf("long: %v", got)
	}
	if got := CheckTarget(strings.Repeat("palabra ", 50), LanguageAny); got != nil {
		t.Errorf("any: %v", got)
	}
	if got, want := TargetReason(CheckTarget(strings.Repeat("el la de que ", 20), LanguageEnglish)), "brief target failed: language (the brief must be written in English)"; got != want {
		t.Errorf("reason %q", got)
	}
}
