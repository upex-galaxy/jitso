package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
)

func TestDraftSource(t *testing.T) {
	const rule = "JITSO REJECTED: a draft is a .yaml file (or - for stdin) of at most 64 KiB"
	tests := []struct {
		name string
		args []string
		want string // the source, or the rejection text
		ok   bool
	}{
		{"a yaml file", []string{"b.yaml"}, "b.yaml", true},
		{"a yml file", []string{"dir/b.yml"}, "dir/b.yml", true},
		{"stdin", []string{"-"}, "-", true},
		{"no argument", nil, rule, false},
		{"another extension", []string{"b.txt"}, rule, false},
		{"an empty word", []string{""}, rule, false},
		{"a second word", []string{"a.yaml", "b.yaml"}, `JITSO REJECTED: unknown argument "b.yaml"`, false},
		{"a flag", []string{"--all"}, `JITSO REJECTED: unknown argument "--all"`, false},
		{"a flag after the file", []string{"a.yaml", "-"}, `JITSO REJECTED: unknown argument "-"`, false},
		{"a flag that looks like a file", []string{"-x.yaml"}, `JITSO REJECTED: unknown argument "-x.yaml"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := draftSource(tt.args)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Errorf("draftSource = %q, %v; want %q", got, err, tt.want)
				}
				return
			}
			if err == nil || fail.Code(err) != fail.CodeRejected || fail.Text(err) != tt.want {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDiscardTarget(t *testing.T) {
	tests := []struct {
		name string
		args []string
		id   string
		all  bool
		want string // the rejection text, "" when accepted
	}{
		{"an id", []string{"scout"}, "scout", false, ""},
		{"all", []string{"--all"}, "", true, ""},
		{"no argument", nil, "", false, "JITSO REJECTED: build discard needs a draft id or --all"},
		{"a path", []string{"../jitso/builds/x"}, "", false, "JITSO REJECTED: build id must be kebab-case"},
		{"an upper case id", []string{"Scout"}, "", false, "JITSO REJECTED: build id must be kebab-case"},
		{"an id with the extension", []string{"scout.yaml"}, "", false, "JITSO REJECTED: build id must be kebab-case"},
		{"two ids", []string{"a", "b"}, "", false, `JITSO REJECTED: unknown argument "b"`},
		{"an id then all", []string{"a", "--all"}, "", false, `JITSO REJECTED: unknown argument "--all"`},
		{"all then an id", []string{"--all", "a"}, "", false, `JITSO REJECTED: unknown argument "a"`},
		{"another flag", []string{"-f"}, "", false, `JITSO REJECTED: unknown argument "-f"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, all, err := discardTarget(tt.args)
			if tt.want == "" {
				if err != nil || id != tt.id || all != tt.all {
					t.Errorf("discardTarget = %q, %v, %v", id, all, err)
				}
				return
			}
			if err == nil || fail.Code(err) != fail.CodeRejected || fail.Text(err) != tt.want {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}

const draftConfig = "version: 1\nmodels: [claude-haiku-5-5]\nefforts: [low]\nmcp: []\nskills: []\ntools: [Read]\n"

const draftText = `id: scout
version: 1
status: draft
description: Scan.
harness: claude
model: claude-haiku-5-5
effort: low
route_scope: [none]
tools: [Read]
budget: { tokens: 1000, usd_list_cap: 0.5 }
`

// gitProject is a git checkout holding the allowlists and one instruction, the
// minimum a catalog command needs. Its real path is returned.
func gitProject(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v: %s", err, out)
	}
	for rel, text := range map[string]string{
		"jitso/jitso.yaml":                      draftConfig,
		".agents/instructions/agent-project.md": "---\nid: project\n---\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func runIn(t *testing.T, dir, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	home := filepath.Join(dir, "no-home")
	code := Run(args, System{
		Stdin:     strings.NewReader(stdin),
		Stdout:    &out,
		Stderr:    &errOut,
		LookupEnv: func(k string) (string, bool) { return map[string]string{"HOME": home}[k], k == "HOME" },
		Getwd:     func() (string, error) { return dir, nil },
	})
	return code, out.String(), errOut.String()
}

// One round through the commands on a real checkout: draft, list, discard.
func TestBuildDraftListDiscard(t *testing.T) {
	root := gitProject(t)

	code, out, errOut := runIn(t, root, draftText, "build", "draft", "-")
	want := "drafted scout: .jitso/drafts/scout.yaml (launch it with --build scout; only the owner can save it)\n"
	if code != 0 || out != want || errOut != "" {
		t.Fatalf("draft: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got, _ := os.ReadFile(filepath.Join(root, ".jitso", "drafts", "scout.yaml")); string(got) != draftText {
		t.Errorf("draft file %q", got)
	}

	// A relative path is read from the working directory.
	if err := os.WriteFile(filepath.Join(root, "again.yml"), []byte(strings.Replace(draftText, "Scan.", "Scan again.", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runIn(t, root, "", "build", "draft", "again.yml"); code != 0 {
		t.Fatalf("draft from a file: exit %d, %q", code, errOut)
	}

	code, out, _ = runIn(t, root, "", "build", "list", "--drafts")
	if code != 0 || out != "- scout: claude-haiku-5-5 low. Scan again. requires: nothing (draft .jitso/drafts/scout.yaml)\n" {
		t.Errorf("list --drafts: exit %d, %q", code, out)
	}
	if code, out, _ = runIn(t, root, "", "build", "list"); code != 0 || out != "\n" {
		t.Errorf("list: exit %d, %q (a draft is never listed)", code, out)
	}
	if code, _, errOut = runIn(t, root, "", "build", "list", "extra"); code != 2 || errOut != "JITSO REJECTED: unknown argument \"extra\"\n" {
		t.Errorf("list extra: exit %d, %q", code, errOut)
	}

	if code, out, _ = runIn(t, root, "", "build", "discard", "scout"); code != 0 || out != "discarded scout\n" {
		t.Errorf("discard: exit %d, %q", code, out)
	}
	if code, _, errOut = runIn(t, root, "", "build", "discard", "scout"); code != 2 || errOut != "JITSO REJECTED: no draft \"scout\"\n" {
		t.Errorf("discard again: exit %d, %q", code, errOut)
	}
}

func TestBuildDraftRejections(t *testing.T) {
	root := gitProject(t)
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"a draft that says approved", strings.Replace(draftText, "status: draft", "status: approved", 1), []string{"-"},
			"JITSO REJECTED: draft stdin is invalid: status must be draft in a draft\n"},
		{"an id of 65 characters", strings.Replace(draftText, "id: scout", "id: a"+strings.Repeat("b", 64), 1), []string{"-"},
			"JITSO REJECTED: draft stdin is invalid: id must be at most 64 characters\n"},
		{"a missing file", "", []string{"nope.yaml"}, "JITSO REJECTED: cannot read nope.yaml: no such file\n"},
		{"a folder", "", []string{"."}, "JITSO REJECTED: a draft is a .yaml file (or - for stdin) of at most 64 KiB\n"},
		{"too large", strings.Repeat("#", 64<<10+1), []string{"-"}, "JITSO REJECTED: a draft is a .yaml file (or - for stdin) of at most 64 KiB\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runIn(t, root, tt.stdin, append([]string{"build", "draft"}, tt.args...)...)
			if code != 2 || out != "" || errOut != tt.want {
				t.Errorf("exit %d, stdout %q, stderr %q; want stderr %q", code, out, errOut, tt.want)
			}
			if _, err := os.Lstat(filepath.Join(root, ".jitso")); err == nil {
				t.Error("a rejected draft created .jitso")
			}
		})
	}
}
