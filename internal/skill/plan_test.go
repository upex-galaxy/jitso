package skill

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/upex-galaxy/jitso/internal/adopt"
	"github.com/upex-galaxy/jitso/internal/fail"
)

// scope is a project or a user scope in a temp folder.
type scope struct {
	t    *testing.T
	root string
	user bool
}

func newScope(t *testing.T, user bool) scope {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return scope{t: t, root: root, user: user}
}

func (s scope) display(p string) string {
	rel, err := filepath.Rel(s.root, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	if s.user {
		return "~/" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(rel)
}

func (s scope) options(version string) Options {
	name := "project"
	if s.user {
		name = "user"
	}
	return Options{Scope: name, Root: s.root, Display: s.display, Version: version, Files: shipped}
}

func (s scope) path(rel string) string { return filepath.Join(s.root, filepath.FromSlash(rel)) }

func (s scope) read(rel string) string {
	s.t.Helper()
	b, err := os.ReadFile(s.path(rel))
	if err != nil {
		s.t.Fatal(err)
	}
	return string(b)
}

func (s scope) exists(rel string) bool {
	_, err := os.Lstat(s.path(rel))
	return err == nil
}

func (s scope) put(rel, content string) {
	s.t.Helper()
	put(s.t, s.root, rel, content)
}

// apply runs a plan through adopt.Run with --yes and returns what it printed.
func (s scope) apply(res Result, command string) (string, error) {
	s.t.Helper()
	var out bytes.Buffer
	err := adopt.Run(adopt.Options{
		Command:    command,
		Scope:      "project",
		Version:    "vtest",
		Root:       s.root,
		BackupsDir: s.path(".jitso/backups"),
		Yes:        true,
		Display:    s.display,
	}, res.Plan, adopt.Ports{
		Stdout: &out,
		Now:    func() time.Time { return time.Date(2026, 11, 15, 7, 34, 55, 0, time.UTC) },
		Rand:   bytes.NewReader([]byte{0xa3, 0xf9, 0xa3, 0xf9, 0xa3, 0xf9, 0xa3, 0xf9}),
	})
	return out.String(), err
}

func (s scope) build(o Options) Result {
	s.t.Helper()
	res, err := Build(o)
	if err != nil {
		s.t.Fatal(err)
	}
	return res
}

func (s scope) install(version string) string {
	s.t.Helper()
	out, err := s.apply(s.build(s.options(version)), "install skill")
	if err != nil {
		s.t.Fatalf("install: %v\n%s", err, out)
	}
	return out
}

func TestInstallFromAbsent(t *testing.T) {
	for _, user := range []bool{false, true} {
		name := map[bool]string{false: "project", true: "user"}[user]
		t.Run(name, func(t *testing.T) {
			s := newScope(t, user)
			res := s.build(s.options("v1"))
			if res.Refusal != "" || res.Plan.Empty() {
				t.Fatalf("refusal %q, empty %v", res.Refusal, res.Plan.Empty())
			}
			prefix := ""
			if user {
				prefix = "~/"
			}
			var preview bytes.Buffer
			adopt.Preview(&preview, res.Plan)
			want := "new file " + prefix + ".agents/skills/jitso/SKILL.md (5 lines)\n" +
				"new file " + prefix + ".agents/skills/jitso/references/a.md (1 lines)\n" +
				"new file " + prefix + ".agents/skills/jitso/.jitso-skill.json (10 lines)\n" +
				"link " + prefix + ".claude/skills/jitso -> ../../.agents/skills/jitso\n"
			if preview.String() != want {
				t.Errorf("preview\n%s\nwant\n%s", preview.String(), want)
			}
			out := s.install("v1")
			if !strings.Contains(out, "linked "+prefix+".claude/skills/jitso -> ../../.agents/skills/jitso\n") || !strings.HasSuffix(out, "jitso install skill: done\n") {
				t.Errorf("output %q", out)
			}
			if s.read(".agents/skills/jitso/SKILL.md") != string(shipped[0].Data) || s.read(".claude/skills/jitso/references/a.md") != "a\n" {
				t.Error("files or link content wrong")
			}
			f, err := Inspect(s.path(".agents/skills/jitso"), NewManifest("v1", shipped))
			if err != nil || f.State != Current || f.Manifest.JitsoVersion != "v1" {
				t.Errorf("after install: %+v, %v", f, err)
			}
			if s.exists(".jitso/backups") {
				t.Error("a fresh install made a backup")
			}
		})
	}
}

// The link is a symbolic link with the relative target, not a copy.
func TestLinkIsRelativeSymlink(t *testing.T) {
	s := newScope(t, false)
	if ok, why := canSymlink(nil); !ok {
		t.Skip("no symbolic links here:", why)
	}
	s.install("v1")
	info, err := os.Lstat(s.path(".claude/skills/jitso"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("lstat %v, %v: want a symbolic link", info, err)
	}
	if target, _ := os.Readlink(s.path(".claude/skills/jitso")); filepath.ToSlash(target) != LinkTarget {
		t.Errorf("link target %q, want %q", target, LinkTarget)
	}
	// The real files are real files.
	if info, _ := os.Lstat(s.path(".agents/skills/jitso/SKILL.md")); info == nil || !info.Mode().IsRegular() {
		t.Error("SKILL.md under .agents is not a regular file")
	}
}

func TestSecondInstallIsNothingToDo(t *testing.T) {
	s := newScope(t, false)
	s.install("v1")
	res := s.build(s.options("v2"))
	if !res.Plan.Empty() || res.Refusal != "" || res.Plan.Why != "the jitso skill v1 is installed and up to date" {
		t.Errorf("plan empty %v why %q refusal %q", res.Plan.Empty(), res.Plan.Why, res.Refusal)
	}
}

func TestMissingLinkIsRepaired(t *testing.T) {
	s := newScope(t, false)
	s.install("v1")
	if err := os.Remove(s.path(".claude/skills/jitso")); err != nil {
		t.Skip("cannot remove the link:", err)
	}
	res := s.build(s.options("v1"))
	if res.Plan.Empty() || len(res.Plan.Effective()) != 1 || res.Plan.Effective()[0].Kind != adopt.KindSymlink {
		t.Fatalf("plan %+v", res.Plan.Effective())
	}
	s.apply(res, "install skill")
	if s.read(".claude/skills/jitso/SKILL.md") != string(shipped[0].Data) {
		t.Error("the link was not recreated")
	}
}

func TestLinkedSkillsFolderPlansNoLink(t *testing.T) {
	for _, targetExists := range []bool{true, false} {
		s := newScope(t, false)
		if err := os.MkdirAll(s.path(".claude"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../.agents/skills", s.path(".claude/skills")); err != nil {
			t.Skip("cannot make a link:", err)
		}
		if targetExists {
			s.put(".agents/skills/other.txt", "x")
		}
		out := s.install("v1")
		if strings.Contains(out, ".claude/skills/jitso") {
			t.Errorf("targetExists=%v: a link entry was planned:\n%s", targetExists, out)
		}
		if s.read(".agents/skills/jitso/SKILL.md") != string(shipped[0].Data) {
			t.Errorf("targetExists=%v: the real copy is missing", targetExists)
		}
		res := s.build(s.options("v1"))
		if !res.Plan.Empty() {
			t.Errorf("targetExists=%v: second plan is not empty: %+v", targetExists, res.Plan.Effective())
		}
		o := s.options("v1")
		o.Remove = true
		s.apply(s.build(o), "install skill")
		if s.exists(".agents/skills/jitso") {
			t.Errorf("targetExists=%v: the real copy was not removed", targetExists)
		}
		if _, err := os.Lstat(s.path(".claude/skills")); err != nil {
			t.Errorf("targetExists=%v: the parent link was removed: %v", targetExists, err)
		}
	}
}

func TestUpgrade(t *testing.T) {
	s := newScope(t, false)
	old := []File{{"SKILL.md", []byte("old skill\n")}, {"gone.md", []byte("gone\n")}, {"sub/gone2.md", []byte("g2\n")}}
	o := s.options("v0.0.0-old")
	o.Files = old
	if _, err := s.apply(s.build(o), "install skill"); err != nil {
		t.Fatal(err)
	}

	res := s.build(s.options("v1"))
	if res.Refusal != "" {
		t.Fatalf("an outdated copy was refused: %s", res.Refusal)
	}
	if !reflect.DeepEqual(res.Plan.Header, []string{"upgrade the jitso skill from v0.0.0-old to v1"}) {
		t.Errorf("header %q", res.Plan.Header)
	}
	out, err := s.apply(res, "install skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"backed up .agents/skills/jitso/SKILL.md to ",
		"backed up .agents/skills/jitso/gone.md to ",
		"backed up .agents/skills/jitso/.jitso-skill.json to ",
		"wrote .agents/skills/jitso/SKILL.md\n",
		"created .agents/skills/jitso/references/a.md\n",
		"removed .agents/skills/jitso/gone.md\n",
		"removed .agents/skills/jitso/sub/gone2.md\n",
		"removed .agents/skills/jitso/sub\n",
		"wrote .agents/skills/jitso/.jitso-skill.json\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if s.exists(".agents/skills/jitso/gone.md") || s.exists(".agents/skills/jitso/sub") {
		t.Error("files the new version no longer ships are still there")
	}
	f, _ := Inspect(s.path(".agents/skills/jitso"), NewManifest("v1", shipped))
	if f.State != Current || f.Manifest.JitsoVersion != "v1" {
		t.Errorf("after the upgrade: %v %+v", f.State, f.Manifest)
	}
}

func TestHandEditNeedsForce(t *testing.T) {
	s := newScope(t, false)
	s.install("v1")
	s.put(".agents/skills/jitso/SKILL.md", "mine\n")
	s.put(".agents/skills/jitso/notes.md", "n\n")

	res := s.build(s.options("v1"))
	want := "the jitso skill in .agents/skills/jitso was edited by hand (SKILL.md, notes.md); nothing written; run again with --force to replace it (a backup is kept)"
	if res.Refusal != want {
		t.Fatalf("refusal %q, want %q", res.Refusal, want)
	}
	if res.Plan.Empty() {
		t.Error("the refused plan must still show what --force would do")
	}
	if s.read(".agents/skills/jitso/SKILL.md") != "mine\n" {
		t.Error("Build wrote something")
	}

	o := s.options("v1")
	o.Force = true
	res = s.build(o)
	if res.Refusal != "" {
		t.Fatalf("--force was refused: %s", res.Refusal)
	}
	out, err := s.apply(res, "install skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "backed up .agents/skills/jitso/SKILL.md to .jitso/backups/") && !strings.Contains(out, "backed up .agents/skills/jitso/SKILL.md to "+s.root) {
		t.Errorf("no backup line:\n%s", out)
	}
	if s.exists(".agents/skills/jitso/notes.md") {
		t.Error("an extra file survived --force")
	}
	matches, _ := filepath.Glob(s.path(".jitso/backups/*/.agents/skills/jitso/SKILL.md"))
	if len(matches) != 1 {
		t.Fatalf("backups %v", matches)
	}
	if b, _ := os.ReadFile(matches[0]); string(b) != "mine\n" {
		t.Errorf("the backup holds %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(filepath.Dir(matches[0]), "notes.md")); string(b) != "n\n" {
		t.Errorf("the extra file is not in the backup: %q", b)
	}
}

func TestUnmanagedFolder(t *testing.T) {
	s := newScope(t, false)
	s.put(".agents/skills/jitso/SKILL.md", "x\n")
	res := s.build(s.options("v1"))
	want := "the jitso skill in .agents/skills/jitso is not a copy jitso installed (no .jitso-skill.json); nothing written; run again with --force to replace it (a backup is kept)"
	if res.Refusal != want {
		t.Errorf("refusal %q", res.Refusal)
	}
	o := s.options("v1")
	o.Force = true
	if _, err := s.apply(s.build(o), "install skill"); err != nil {
		t.Fatal(err)
	}
	if s.read(".agents/skills/jitso/SKILL.md") != string(shipped[0].Data) {
		t.Error("--force did not replace the file")
	}
	if !s.exists(".jitso/backups") {
		t.Error("--force replaced a file with no backup")
	}
}

func TestInvalidManifestIsUnmanaged(t *testing.T) {
	s := newScope(t, false)
	s.put(".agents/skills/jitso/SKILL.md", "x\n")
	s.put(".agents/skills/jitso/"+ManifestName, "{ not json")
	res := s.build(s.options("v1"))
	if !strings.Contains(res.Refusal, "is not a copy jitso installed (.jitso-skill.json is not valid: ") {
		t.Errorf("refusal %q", res.Refusal)
	}
}

func TestNotAFolderIsAnEnvironmentError(t *testing.T) {
	s := newScope(t, false)
	s.put(".agents/skills/jitso", "a file where the folder goes")
	_, err := Build(s.options("v1"))
	if fail.Code(err) != fail.CodeEnv || !strings.Contains(err.Error(), ".agents/skills/jitso is a file, not a folder") {
		t.Errorf("error %v", err)
	}
	s2 := newScope(t, false)
	s2.install("v1")
	os.Remove(s2.path(".claude/skills/jitso"))
	s2.put(".claude/skills/jitso", "a file")
	_, err = Build(s2.options("v1"))
	if fail.Code(err) != fail.CodeEnv || !strings.Contains(err.Error(), ".claude/skills/jitso is a file") {
		t.Errorf("error %v", err)
	}
}

func TestForeignLink(t *testing.T) {
	s := newScope(t, false)
	if err := os.MkdirAll(s.path(".claude/skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../elsewhere", s.path(".claude/skills/jitso")); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	res := s.build(s.options("v1"))
	want := "the jitso skill in .claude/skills/jitso is not a copy jitso installed (it is a link to ../elsewhere); nothing written; run again with --force to replace it (a backup is kept)"
	if res.Refusal != want {
		t.Fatalf("refusal %q", res.Refusal)
	}
	o := s.options("v1")
	o.Force = true
	out, err := s.apply(s.build(o), "install skill")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "linked .claude/skills/jitso -> ../../.agents/skills/jitso") || s.read(".claude/skills/jitso/SKILL.md") != string(shipped[0].Data) {
		t.Errorf("the link was not replaced:\n%s", out)
	}
}

// A link that leads to the real folder by any spelling is ours.
func TestLinkSpellings(t *testing.T) {
	s := newScope(t, false)
	s.install("v1")
	for _, target := range []string{LinkTarget, s.path(".agents/skills/jitso"), "../skills/../../.agents/skills/jitso"} {
		os.Remove(s.path(".claude/skills/jitso"))
		if err := os.Symlink(target, s.path(".claude/skills/jitso")); err != nil {
			t.Skip("no symbolic links here:", err)
		}
		if res := s.build(s.options("v1")); !res.Plan.Empty() || res.Refusal != "" {
			t.Errorf("link %q: plan empty %v, refusal %q", target, res.Plan.Empty(), res.Refusal)
		}
	}
}

// Where a symbolic link cannot be made, a second managed copy takes its place.
func TestSymlinkFallbackWritesASecondCopy(t *testing.T) {
	s := newScope(t, false)
	o := s.options("v1")
	o.Symlink = func(target, link string) error {
		return &os.LinkError{Op: "symlink", Old: target, New: link, Err: errors.New("a required privilege is not held")}
	}
	res := s.build(o)
	wantHeader := "warning: this system cannot create symbolic links (a required privilege is not held); .claude/skills/jitso gets a second copy of the skill"
	if !reflect.DeepEqual(res.Plan.Header, []string{wantHeader}) {
		t.Fatalf("header %q", res.Plan.Header)
	}
	var preview bytes.Buffer
	adopt.Preview(&preview, res.Plan)
	if !strings.HasPrefix(preview.String(), wantHeader+"\n") || !strings.Contains(preview.String(), "new file .claude/skills/jitso/SKILL.md") || strings.Contains(preview.String(), "link .claude") {
		t.Errorf("preview:\n%s", preview.String())
	}
	if _, err := s.apply(res, "install skill"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(s.path(".claude/skills/jitso"))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the second copy is %v, %v", info, err)
	}
	for _, rel := range []string{".agents/skills/jitso", ".claude/skills/jitso"} {
		f, err := Inspect(s.path(rel), NewManifest("v1", shipped))
		if err != nil || f.State != Current {
			t.Errorf("%s: %v, %v", rel, f.State, err)
		}
	}

	// The second copy is handled like the real folder: a second install does nothing,
	// a hand edit in it is refused, and --remove takes both away.
	if res := s.build(o); !res.Plan.Empty() || res.Refusal != "" {
		t.Errorf("second install: empty %v, refusal %q", res.Plan.Empty(), res.Refusal)
	}
	s.put(".claude/skills/jitso/SKILL.md", "mine\n")
	if res := s.build(o); !strings.Contains(res.Refusal, "the jitso skill in .claude/skills/jitso was edited by hand (SKILL.md)") {
		t.Errorf("edit of the copy: refusal %q", res.Refusal)
	}
	o.Remove = true
	if res := s.build(o); !strings.Contains(res.Refusal, "the jitso skill in .claude/skills/jitso was edited by hand") || !strings.Contains(res.Refusal, "--force to remove it") {
		t.Errorf("remove with an edited copy: refusal %q", res.Refusal)
	}
	o.Force = true
	if _, err := s.apply(s.build(o), "install skill"); err != nil {
		t.Fatal(err)
	}
	if s.exists(".claude") || s.exists(".agents") {
		t.Error("the removal left folders behind")
	}
}

func TestRemoveRoundTrip(t *testing.T) {
	for _, user := range []bool{false, true} {
		name := map[bool]string{false: "project", true: "user"}[user]
		t.Run(name, func(t *testing.T) {
			s := newScope(t, user)
			s.put(".agents/instructions/keep.md", "keep\n")
			s.put(".claude/settings.json", "{}\n")
			s.install("v1")
			o := s.options("v1")
			o.Remove = true
			res := s.build(o)
			if res.Refusal != "" || res.Plan.Empty() {
				t.Fatalf("refusal %q empty %v", res.Refusal, res.Plan.Empty())
			}
			out, err := s.apply(res, "install skill")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "unlinked ") || !strings.Contains(out, "removed ") {
				t.Errorf("output %q", out)
			}
			for _, rel := range []string{".agents/skills", ".claude/skills"} {
				if s.exists(rel) {
					t.Errorf("%s is still there", rel)
				}
			}
			// What was there before stays, and so do the folders that hold it.
			if s.read(".agents/instructions/keep.md") != "keep\n" || s.read(".claude/settings.json") != "{}\n" {
				t.Error("a file that is not the skill's changed")
			}
			// The second removal has nothing to do.
			res = s.build(o)
			wantWhy := "the jitso skill is not installed at " + o.Scope + " scope"
			if !res.Plan.Empty() || res.Plan.Why != wantWhy {
				t.Errorf("second remove: empty %v why %q", res.Plan.Empty(), res.Plan.Why)
			}
		})
	}
}

func TestRemovePrunesTheFoldersItEmptied(t *testing.T) {
	s := newScope(t, false)
	s.install("v1")
	o := s.options("v1")
	o.Remove = true
	if _, err := s.apply(s.build(o), "install skill"); err != nil {
		t.Fatal(err)
	}
	if s.exists(".agents") || s.exists(".claude") {
		t.Error("an emptied .agents or .claude was left behind")
	}
	if !s.exists(".") {
		t.Error("the scope root is gone")
	}
}

func TestRemoveLeavesWhatIsNotOurs(t *testing.T) {
	// A foreign link and a folder jitso did not install at .claude/skills/jitso.
	t.Run("foreign link", func(t *testing.T) {
		s := newScope(t, false)
		s.install("v1")
		os.Remove(s.path(".claude/skills/jitso"))
		if err := os.Symlink("../elsewhere", s.path(".claude/skills/jitso")); err != nil {
			t.Skip("no symbolic links here:", err)
		}
		o := s.options("v1")
		o.Remove = true
		res := s.build(o)
		if res.Refusal != "" {
			t.Fatalf("refusal %q", res.Refusal)
		}
		if _, err := s.apply(res, "install skill"); err != nil {
			t.Fatal(err)
		}
		if target, err := os.Readlink(s.path(".claude/skills/jitso")); err != nil || filepath.ToSlash(target) != "../elsewhere" {
			t.Errorf("the foreign link is %q, %v", target, err)
		}
		if s.exists(".agents") {
			t.Error("the real copy was not removed")
		}
	})
	t.Run("foreign folder", func(t *testing.T) {
		s := newScope(t, false)
		s.install("v1")
		os.Remove(s.path(".claude/skills/jitso"))
		s.put(".claude/skills/jitso/SKILL.md", "theirs\n")
		o := s.options("v1")
		o.Remove = true
		res := s.build(o)
		if res.Refusal != "" {
			t.Fatalf("refusal %q", res.Refusal)
		}
		if _, err := s.apply(res, "install skill"); err != nil {
			t.Fatal(err)
		}
		if s.read(".claude/skills/jitso/SKILL.md") != "theirs\n" {
			t.Error("a folder jitso did not install was removed")
		}
	})
	t.Run("nothing installed", func(t *testing.T) {
		s := newScope(t, false)
		s.put(".claude/skills/other/SKILL.md", "o\n")
		o := s.options("v1")
		o.Remove = true
		if res := s.build(o); !res.Plan.Empty() || res.Plan.Why == "" {
			t.Errorf("plan %+v", res.Plan)
		}
	})
}

func TestRemoveRefusesHandEditsAndUnmanaged(t *testing.T) {
	s := newScope(t, false)
	s.install("v1")
	s.put(".agents/skills/jitso/SKILL.md", "mine\n")
	o := s.options("v1")
	o.Remove = true
	res := s.build(o)
	want := "the jitso skill in .agents/skills/jitso was edited by hand (SKILL.md); nothing written; run again with --force to remove it (a backup is kept)"
	if res.Refusal != want {
		t.Errorf("refusal %q", res.Refusal)
	}
	o.Force = true
	if _, err := s.apply(s.build(o), "install skill"); err != nil {
		t.Fatal(err)
	}
	if s.exists(".agents") || s.exists(".claude") {
		t.Error("--remove --force left the skill")
	}
	matches, _ := filepath.Glob(s.path(".jitso/backups/*/.agents/skills/jitso/SKILL.md"))
	if len(matches) != 1 {
		t.Errorf("backup %v", matches)
	}

	// An unmanaged folder at the real path is refused the same way.
	u := newScope(t, false)
	u.put(".agents/skills/jitso/SKILL.md", "x\n")
	o = u.options("v1")
	o.Remove = true
	if res := u.build(o); !strings.Contains(res.Refusal, "is not a copy jitso installed (no .jitso-skill.json); nothing written; run again with --force to remove it") {
		t.Errorf("refusal %q", res.Refusal)
	}
}

func TestPathsPerScope(t *testing.T) {
	p, u := newScope(t, false), newScope(t, true)
	for _, s := range []scope{p, u} {
		res := s.build(s.options("v1"))
		var paths []string
		for _, c := range res.Plan.Changes {
			paths = append(paths, c.Path)
			if !strings.HasPrefix(c.Path, s.root+string(filepath.Separator)) {
				t.Errorf("%s is outside the scope root %s", c.Path, s.root)
			}
		}
		want := []string{
			s.path(".agents/skills/jitso/SKILL.md"), s.path(".agents/skills/jitso/references/a.md"),
			s.path(".agents/skills/jitso/.jitso-skill.json"), s.path(".claude/skills/jitso"),
		}
		if !reflect.DeepEqual(paths, want) {
			t.Errorf("paths %q, want %q", paths, want)
		}
		if err := res.Plan.Validate(s.root); err != nil {
			t.Error(err)
		}
	}
	// Display differs by scope only in the prefix.
	if got := p.display(p.path(".claude/skills/jitso")); got != ".claude/skills/jitso" {
		t.Errorf("project display %q", got)
	}
	if got := u.display(u.path(".claude/skills/jitso")); got != "~/.claude/skills/jitso" {
		t.Errorf("user display %q", got)
	}
}

func TestEmbeddedSkillInstalls(t *testing.T) {
	s := newScope(t, false)
	o := s.options("v1")
	o.Files = nil // the real embedded tree
	if _, err := s.apply(s.build(o), "install skill"); err != nil {
		t.Fatal(err)
	}
	f, err := Inspect(s.path(".agents/skills/jitso"), NewManifest("v1", Files()))
	if err != nil || f.State != Current {
		t.Errorf("%v, %v", f.State, err)
	}
}

// A plain folder that is not jitso's, at the link path, cannot become a link
// in the same plan (the plan changes one path once), so --force turns it into
// the second managed copy. The refusal without --force leaves everything alone.
func TestForceTurnsAForeignFolderAtTheLinkPathIntoACopy(t *testing.T) {
	s := newScope(t, false)
	s.put(".claude/skills/jitso/SKILL.md", "theirs\n")
	res := s.build(s.options("v1"))
	if !strings.Contains(res.Refusal, "the jitso skill in .claude/skills/jitso is not a copy jitso installed (no .jitso-skill.json)") {
		t.Fatalf("refusal %q", res.Refusal)
	}
	o := s.options("v1")
	o.Force = true
	if _, err := s.apply(s.build(o), "install skill"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(s.path(".claude/skills/jitso"))
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("the path is %v, %v: want a folder (a second managed copy)", info, err)
	}
	for _, rel := range []string{".agents/skills/jitso", ".claude/skills/jitso"} {
		if f, err := Inspect(s.path(rel), NewManifest("v1", shipped)); err != nil || f.State != Current {
			t.Errorf("%s: %v, %v", rel, f.State, err)
		}
	}
	matches, _ := filepath.Glob(s.path(".jitso/backups/*/.claude/skills/jitso/SKILL.md"))
	if len(matches) != 1 {
		t.Fatalf("backups %v", matches)
	}
	if b, _ := os.ReadFile(matches[0]); string(b) != "theirs\n" {
		t.Errorf("the backup holds %q", b)
	}
}
