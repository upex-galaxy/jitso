package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/upex-galaxy/jitso/internal/fail"
	"github.com/upex-galaxy/jitso/internal/templates"
)

func TestParseNewArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		id, tier string
		err      string // the rejection text, "" when accepted
	}{
		{"an id", []string{"scout"}, "scout", "worker", ""},
		{"a tier", []string{"scout", "--tier", "conductor"}, "scout", "conductor", ""},
		{"a tier first", []string{"--tier", "subagent", "scout"}, "scout", "subagent", ""},
		{"a later tier wins", []string{"scout", "--tier", "subagent", "--tier", "worker"}, "scout", "worker", ""},
		{"no id", nil, "", "", "JITSO REJECTED: build new needs a build id"},
		{"only a tier", []string{"--tier", "worker"}, "", "", "JITSO REJECTED: build new needs a build id"},
		{"a tier without a value", []string{"scout", "--tier"}, "", "", "JITSO REJECTED: --tier needs a value"},
		{"an unknown tier", []string{"scout", "--tier", "boss"}, "", "", "JITSO REJECTED: --tier must be worker, conductor or subagent"},
		{"an upper case tier", []string{"scout", "--tier", "Worker"}, "", "", "JITSO REJECTED: --tier must be worker, conductor or subagent"},
		{"an empty tier", []string{"scout", "--tier", ""}, "", "", "JITSO REJECTED: --tier must be worker, conductor or subagent"},
		{"a bad id before a bad tier", []string{"Bad", "--tier", "boss"}, "", "", "JITSO REJECTED: build id must be kebab-case"},
		{"a path", []string{"../x"}, "", "", "JITSO REJECTED: build id must be kebab-case"},
		{"an id with the extension", []string{"x.yaml"}, "", "", "JITSO REJECTED: build id must be kebab-case"},
		{"a digit first", []string{"1x"}, "", "", "JITSO REJECTED: build id must be kebab-case"},
		{"65 characters", []string{"a" + strings.Repeat("b", 64)}, "", "", "JITSO REJECTED: build id must be kebab-case"},
		{"64 characters", []string{"a" + strings.Repeat("b", 63)}, "a" + strings.Repeat("b", 63), "worker", ""},
		{"a second id", []string{"a", "b"}, "", "", `JITSO REJECTED: unknown argument "b"`},
		{"another flag", []string{"a", "--force"}, "", "", `JITSO REJECTED: unknown argument "--force"`},
		{"the equals spelling is another flag", []string{"a", "--tier=worker"}, "", "", `JITSO REJECTED: unknown argument "--tier=worker"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, tier, err := parseNewArgs(tt.args)
			if tt.err == "" {
				if err != nil || id != tt.id || tier != tt.tier {
					t.Errorf("parseNewArgs = %q, %q, %v; want %q, %q", id, tier, err, tt.id, tt.tier)
				}
				return
			}
			if err == nil || fail.Code(err) != fail.CodeRejected || fail.Text(err) != tt.err {
				t.Errorf("error %v, want %q", err, tt.err)
			}
		})
	}
}

const newLine = "wrote .jitso/drafts/%s.yaml (tier %s, status draft): fill every empty value, check it with jitso build validate %s, and ask the owner to run jitso build save %s\n"

func newLineOf(id, tier string) string { return fmt.Sprintf(newLine, id, tier, id, id) }

func TestBuildNewWritesTheTemplate(t *testing.T) {
	for _, tier := range templates.Tiers {
		t.Run(tier, func(t *testing.T) {
			root := gitProject(t)
			code, out, errOut := runIn(t, root, "", "build", "new", "my-build", "--tier", tier)
			if code != 0 || out != newLineOf("my-build", tier) || errOut != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
			want, _ := templates.Build(tier, "my-build")
			if got, _ := fileAt(root, ".jitso/drafts/my-build.yaml"); got != want {
				t.Errorf("draft %q, want the %s template", got, tier)
			}
			if got, _ := fileAt(root, ".jitso/drafts/.gitignore"); got != "*\n" {
				t.Errorf(".gitignore %q", got)
			}
			if _, err := os.Lstat(filepath.Join(root, "jitso/builds")); err == nil {
				t.Error("build new wrote jitso/builds")
			}
			if got, _ := fileAt(root, "jitso/jitso.yaml"); got != draftConfig {
				t.Errorf("the allowlists changed: %q", got)
			}
			// An invalid draft is skipped, never fatal: the catalog still loads.
			if code, _, errOut := runIn(t, root, "", "build", "list"); code != 0 || !strings.Contains(errOut, "jitso: warning: skipped build draft .jitso/drafts/my-build.yaml: ") {
				t.Errorf("list: exit %d, stderr %q", code, errOut)
			}
			code, out, _ = runIn(t, root, "", "build", "validate", "my-build")
			if code != 2 || !strings.HasPrefix(out, "invalid my-build draft .jitso/drafts/my-build.yaml\n") {
				t.Errorf("validate: exit %d, %q", code, out)
			}
		})
	}
}

func TestBuildNewDefaultsToWorker(t *testing.T) {
	root := gitProject(t)
	code, out, _ := runIn(t, root, "", "build", "new", "plain")
	if code != 0 || out != newLineOf("plain", "worker") {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
	if got, _ := fileAt(root, ".jitso/drafts/plain.yaml"); !regexp.MustCompile(`(?m)^tier: worker$`).MatchString(got) {
		t.Errorf("draft %q", got)
	}
}

func TestBuildNewNeverOverwrites(t *testing.T) {
	root := gitProject(t)
	if code, _, errOut := runIn(t, root, "", "build", "new", "mine"); code != 0 {
		t.Fatalf("first: exit %d, %q", code, errOut)
	}
	if err := os.WriteFile(filepath.Join(root, ".jitso/drafts/mine.yaml"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runIn(t, root, "", "build", "new", "mine", "--tier", "subagent")
	if code != 2 || out != "" || errOut != "JITSO REJECTED: draft \"mine\" already exists: edit .jitso/drafts/mine.yaml or discard it\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got, _ := fileAt(root, ".jitso/drafts/mine.yaml"); got != "edited\n" {
		t.Errorf("the draft was overwritten: %q", got)
	}
}

func TestBuildNewRefusesASavedID(t *testing.T) {
	root := gitProject(t)
	draftIn(t, root)
	code, _, _ := runWith(t, root, "", false, "build", "save", "scout", "--yes")
	if code != 0 {
		t.Fatalf("save: exit %d", code)
	}
	code, out, errOut := runIn(t, root, "", "build", "new", "scout")
	if code != 2 || out != "" || errOut != "JITSO REJECTED: build \"scout\" already exists (repo jitso/builds/scout.yaml): a draft needs an id no build has\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if _, err := os.Lstat(filepath.Join(root, ".jitso/drafts/scout.yaml")); err == nil {
		t.Error("a refused call wrote a draft")
	}
}

func TestBuildNewInAFreshProject(t *testing.T) {
	root := gitProject(t)
	// A fresh project: no allowlists and no builds (an instruction section is
	// there, as in every project that has agent instructions).
	if err := os.Remove(filepath.Join(root, "jitso/jitso.yaml")); err != nil {
		t.Fatal(err)
	}

	code, out, errOut := runIn(t, root, "", "build", "new", "first")
	want := "wrote jitso/jitso.yaml (every allowlist is open: add a list there to restrict builds)\n" + newLineOf("first", "worker")
	if code != 0 || out != want || errOut != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if got, _ := fileAt(root, "jitso/jitso.yaml"); got != templates.Config() {
		t.Errorf("skeleton %q", got)
	}
	if _, err := os.Lstat(filepath.Join(root, "jitso/builds")); err == nil {
		t.Error("jitso/builds was created")
	}

	// The skeleton is there now: only the draft is written, and it is not touched.
	if err := os.WriteFile(filepath.Join(root, "jitso/jitso.yaml"), []byte(draftConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ = runIn(t, root, "", "build", "new", "second")
	if code != 0 || out != newLineOf("second", "worker") {
		t.Fatalf("second: exit %d, stdout %q", code, out)
	}
	if got, _ := fileAt(root, "jitso/jitso.yaml"); got != draftConfig {
		t.Errorf("the allowlists were rewritten: %q", got)
	}
}

// A fresh project may have an empty jitso/builds folder: still fresh.
func TestBuildNewWithAnEmptyBuildsFolder(t *testing.T) {
	root := gitProject(t)
	if err := os.Remove(filepath.Join(root, "jitso/jitso.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "jitso/builds"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "jitso/builds/notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runIn(t, root, "", "build", "new", "first")
	if code != 0 || !strings.HasPrefix(out, "wrote jitso/jitso.yaml") {
		t.Fatalf("exit %d, stdout %q", code, out)
	}
}

// The whole-catalog rule holds without jitso/jitso.yaml too: an invalid saved
// build stops the command before it writes anything.
func TestBuildNewWithAnInvalidBuildAndNoFileWritesNothing(t *testing.T) {
	root := gitProject(t)
	if err := os.Remove(filepath.Join(root, "jitso/jitso.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "jitso/builds"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "jitso/builds/a.yaml"), []byte("id: a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runIn(t, root, "", "build", "new", "first")
	if code != 3 || out != "" || !strings.HasPrefix(errOut, "jitso: build jitso/builds/a.yaml is invalid: ") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if _, err := os.Lstat(filepath.Join(root, "jitso/jitso.yaml")); err == nil {
		t.Error("the skeleton was written")
	}
	if _, err := os.Lstat(filepath.Join(root, ".jitso")); err == nil {
		t.Error("a draft folder was created")
	}
}

func TestBuildNewRejectsBeforeTouchingTheProject(t *testing.T) {
	root := gitProject(t)
	if err := os.Remove(filepath.Join(root, "jitso/jitso.yaml")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"Bad"}, {"x", "--tier", "boss"}, {}, {"x", "y"}} {
		code, out, _ := runIn(t, root, "", append([]string{"build", "new"}, args...)...)
		if code != 2 || out != "" {
			t.Errorf("%v: exit %d, stdout %q", args, code, out)
		}
		if _, err := os.Lstat(filepath.Join(root, "jitso/jitso.yaml")); err == nil {
			t.Errorf("%v: the skeleton was written", args)
		}
	}
}
