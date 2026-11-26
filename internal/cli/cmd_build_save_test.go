package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/drafts"
	"github.com/upex-galaxy/jitso/internal/fail"
)

func TestParseSaveArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		err  string // the rejection text, "" when accepted
	}{
		{"an id", []string{"scout"}, "scout", ""},
		{"an id and --yes", []string{"scout", "--yes"}, "scout", ""},
		{"--yes first", []string{"--yes", "scout"}, "scout", ""},
		{"--yes twice is once", []string{"scout", "--yes", "--yes"}, "scout", ""},
		{"no id", nil, "", "JITSO REJECTED: build save needs a draft id"},
		{"only --yes", []string{"--yes"}, "", "JITSO REJECTED: build save needs a draft id"},
		{"--confirm is gone", []string{"scout", "--confirm", "abc"}, "", `JITSO REJECTED: unknown argument "--confirm"`},
		{"--confirm without a value is gone", []string{"scout", "--confirm"}, "", `JITSO REJECTED: unknown argument "--confirm"`},
		{"a second id", []string{"a", "b"}, "", `JITSO REJECTED: unknown argument "b"`},
		{"another flag", []string{"scout", "--force"}, "", `JITSO REJECTED: unknown argument "--force"`},
		{"--dry-run is not taken", []string{"scout", "--dry-run"}, "", `JITSO REJECTED: unknown argument "--dry-run"`},
		{"the equals spelling is another flag", []string{"scout", "--confirm=abc"}, "", `JITSO REJECTED: unknown argument "--confirm=abc"`},
		{"a path", []string{"../scout"}, "", "JITSO REJECTED: build id must be kebab-case"},
		{"an upper case id", []string{"Scout"}, "", "JITSO REJECTED: build id must be kebab-case"},
		{"an id with the extension", []string{"scout.yaml"}, "", "JITSO REJECTED: build id must be kebab-case"},
		{"an id of 65 characters", []string{"a" + strings.Repeat("b", 64)}, "", "JITSO REJECTED: build id must be kebab-case"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSaveArgs(tt.args)
			if tt.err == "" {
				if err != nil || got != tt.want {
					t.Errorf("parseSaveArgs = %q, %v; want %q", got, err, tt.want)
				}
				return
			}
			if err == nil || fail.Code(err) != fail.CodeRejected || fail.Text(err) != tt.err {
				t.Errorf("error %v, want %q", err, tt.err)
			}
		})
	}
}

// runWith runs a command with an injected terminal answer and standard input.
func runWith(t *testing.T, dir, stdin string, terminal bool, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	home := filepath.Join(dir, "no-home")
	code := Run(args, System{
		Stdin:           strings.NewReader(stdin),
		Stdout:          &out,
		Stderr:          &errOut,
		LookupEnv:       func(k string) (string, bool) { return map[string]string{"HOME": home}[k], k == "HOME" },
		Getwd:           func() (string, error) { return dir, nil },
		StdinIsTerminal: func() bool { return terminal },
	})
	return code, out.String(), errOut.String()
}

func draftIn(t *testing.T, root string) {
	t.Helper()
	if code, _, errOut := runIn(t, root, draftText, "build", "draft", "-"); code != 0 {
		t.Fatalf("draft: exit %d, %q", code, errOut)
	}
}

func fileAt(root, rel string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	return string(data), err == nil
}

var approvedText = strings.Replace(draftText, "status: draft", "status: approved", 1)

// A save has no owner gate: with or without a terminal, with or without --yes,
// it shows the file and writes it, and a no on standard input changes nothing.
// The terminal is injected here because the contract never gets one.
func TestBuildSaveAsksNothing(t *testing.T) {
	tests := []struct {
		name     string
		terminal bool
		args     []string
	}{
		{"no terminal", false, []string{"build", "save", "scout"}},
		{"no terminal with --yes", false, []string{"build", "save", "scout", "--yes"}},
		{"a terminal", true, []string{"build", "save", "scout"}},
		{"a terminal with --yes", true, []string{"build", "save", "scout", "--yes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := gitProject(t)
			draftIn(t, root)
			code, out, errOut := runWith(t, root, "n\n", tt.terminal, tt.args...)
			wantOut := drafts.Show([]byte(approvedText)) + "saved scout: jitso/builds/scout.yaml (status approved); the draft is removed\n"
			if code != 0 || out != wantOut || errOut != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
			if got, _ := fileAt(root, "jitso/builds/scout.yaml"); got != approvedText {
				t.Errorf("build file %q", got)
			}
			if _, ok := fileAt(root, ".jitso/drafts/scout.yaml"); ok {
				t.Error("the draft is still there")
			}
			if code, out, _ := runIn(t, root, "", "build", "list"); code != 0 || !strings.HasPrefix(out, "- scout: ") {
				t.Errorf("list after save: exit %d, %q", code, out)
			}
			// Saving again: no draft any more.
			if code, _, errOut := runWith(t, root, "", tt.terminal, tt.args...); code != 2 || errOut != "JITSO REJECTED: no draft \"scout\"\n" {
				t.Errorf("second save: exit %d, %q", code, errOut)
			}
		})
	}
}

func TestBuildSaveRechecksTheDraft(t *testing.T) {
	root := gitProject(t)
	// The draft was valid when it was written; the allowlists changed since.
	draftIn(t, root)
	cfg := filepath.Join(root, "jitso/jitso.yaml")
	if err := os.WriteFile(cfg, []byte(strings.Replace(draftConfig, "claude-haiku-5-5", "claude-other", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runWith(t, root, "", false, "build", "save", "scout", "--yes")
	if code != 2 || out != "" || errOut != "JITSO REJECTED: draft \"scout\" is invalid: model \"claude-haiku-5-5\" is not in the allowlist\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if _, ok := fileAt(root, "jitso/builds/scout.yaml"); ok {
		t.Error("an invalid draft was saved")
	}
}

func TestBuildSaveNeverOverwrites(t *testing.T) {
	root := gitProject(t)
	draftIn(t, root)
	if err := os.MkdirAll(filepath.Join(root, "jitso/builds"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A build file of this name that holds another id.
	other := strings.Replace(approvedText, "id: scout", "id: other", 1)
	if err := os.WriteFile(filepath.Join(root, "jitso/builds/scout.yaml"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runWith(t, root, "", false, "build", "save", "scout", "--yes")
	if code != 2 || out != "" || errOut != "JITSO REJECTED: jitso/builds/scout.yaml already exists: save never overwrites a build\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got, _ := fileAt(root, "jitso/builds/scout.yaml"); got != other {
		t.Errorf("the build was overwritten: %q", got)
	}
	if _, ok := fileAt(root, ".jitso/drafts/scout.yaml"); !ok {
		t.Error("the draft was removed")
	}
}

func TestBuildSaveRefusals(t *testing.T) {
	root := gitProject(t)
	tests := []struct {
		name, file, text, want string
	}{
		{"an id that is not the file name", "wrong.yaml", draftText,
			`JITSO REJECTED: draft "wrong" is invalid: the id "scout" must be the file name "wrong"` + "\n"},
		{"a status that cannot be changed in place", "tagged.yaml", strings.Replace(strings.Replace(draftText, "id: scout", "id: tagged", 1), "status: draft", "status: !!str draft", 1),
			`JITSO REJECTED: draft "tagged" cannot be saved: write its status as the plain word draft, or in quotes` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(root, ".jitso/drafts", tt.file)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tt.text), 0o644); err != nil {
				t.Fatal(err)
			}
			id := strings.TrimSuffix(tt.file, ".yaml")
			code, out, errOut := runWith(t, root, "", false, "build", "save", id, "--yes")
			if code != 2 || out != "" || errOut != tt.want {
				t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
			if _, ok := fileAt(root, "jitso/builds/"+tt.file); ok {
				t.Error("the build was written")
			}
		})
	}
}

func TestBuildSaveNoDraft(t *testing.T) {
	root := gitProject(t)
	code, out, errOut := runWith(t, root, "", false, "build", "save", "nope")
	if code != 2 || out != "" || errOut != "JITSO REJECTED: no draft \"nope\"\n" {
		t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}
