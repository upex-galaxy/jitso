package doctor

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/upex-galaxy/jitso/internal/skill"
)

// skillFound is what a scope's skill folder turned out to be, for the check of
// the other scope to compare.
type skillFound struct {
	// Installed is true when the folder holds a copy jitso installed.
	Installed bool
	// Content is the manifest's content hash.
	Content string
}

// checkSkill inspects the jitso skill of one scope: the real folder under
// .agents/skills and the link or copy Claude Code reads under .claude/skills.
// root is the checkout for "project" and HOME for "user". other is the
// project's result when checking the user scope (nil otherwise).
func checkSkill(p Ports, scope, root string, other *skillFound) (Line, skillFound) {
	check := "skill-" + scope
	shown := func(rel string) string {
		if scope == "user" {
			return "~/" + rel
		}
		return rel
	}
	real, link := shown(skill.RealRel), shown(skill.LinkRel)
	install := "run jitso install skill --scope " + scope
	if root == "" {
		return Line{Warn, check, "HOME is not set, so there is no user skill"}, skillFound{}
	}

	want := skill.NewManifest(p.Version, skill.Files())
	dir := filepath.Join(root, filepath.FromSlash(skill.RealRel))
	f, err := skill.Inspect(dir, want)
	if err != nil {
		return Line{Warn, check, "cannot read " + real + ": " + message(err)}, skillFound{}
	}
	found := skillFound{}
	if f.State == skill.Current || f.State == skill.Outdated || f.State == skill.Modified {
		found = skillFound{Installed: true, Content: f.Manifest.ContentSHA256}
	}

	switch f.State {
	case skill.Absent:
		if scope == "user" {
			return Line{OK, check, "not installed (optional)"}, found
		}
		return Line{OK, check, "not installed (optional: a launch carries the jitso skill for a conductor build or a build that names jitso; " + install + " to keep a copy in the checkout)"}, found
	case skill.NotFolder:
		return Line{Warn, check, real + " is " + f.Why + ", not a folder jitso installed"}, found
	case skill.Unmanaged:
		return Line{Warn, check, fmt.Sprintf("%s is not a copy jitso installed (%s); %s --force to replace it", real, f.Why, install)}, found
	case skill.Modified:
		return Line{Warn, check, fmt.Sprintf("edited by hand in %s (%s); %s --force to replace it", real, strings.Join(f.Edited, ", "), install)}, found
	case skill.Outdated:
		return Line{Warn, check, fmt.Sprintf("out of date in %s: %s installed, %s shipped; %s", real, f.Manifest.JitsoVersion, p.Version, install)}, found
	}

	if why := linkProblem(p, root, want); why != "" {
		return Line{Warn, check, strings.ReplaceAll(why, "<link>", link) + " (" + install + ")"}, found
	}
	if other != nil && other.Installed && other.Content != found.Content {
		return Line{Warn, check, "installed at both scopes with different content (a launched session reads the project copy or the one its launch carries; your own session reads the user copy)"}, found
	}
	return Line{OK, check, fmt.Sprintf("jitso skill %s in %s and %s", f.Manifest.JitsoVersion, real, link)}, found
}

// linkProblem says why the entry Claude Code reads is not the skill, "" when
// it is: a relative link to the real folder, or a second managed copy (the
// fallback where links cannot be made). "<link>" stands for the shown path.
func linkProblem(p Ports, root string, want skill.Manifest) string {
	linkPath := filepath.Join(root, filepath.FromSlash(skill.LinkRel))
	realPath := filepath.Join(root, filepath.FromSlash(skill.RealRel))
	info, err := p.Lstat(linkPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "the link <link> is missing"
	case err != nil:
		return "cannot read <link>: " + message(err)
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := p.Readlink(linkPath)
		target = filepath.ToSlash(target)
		if err != nil {
			return "cannot read the link <link>: " + message(err)
		}
		abs := target
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(filepath.Dir(linkPath), filepath.FromSlash(target))
		}
		if filepath.Clean(abs) == filepath.Clean(realPath) || sameFolder(abs, realPath) {
			return ""
		}
		return "<link> points to " + target + ", not to " + skill.RealRel
	case info.IsDir():
		if sameFolder(linkPath, realPath) {
			return ""
		}
		f, err := skill.Inspect(linkPath, want)
		if err != nil {
			return "cannot read <link>: " + message(err)
		}
		if f.State == skill.Current {
			return ""
		}
		return "<link> is a copy that is " + f.State.String()
	}
	return "<link> is a file, not a link or a folder"
}

// sameFolder reports whether two paths resolve to the same existing folder.
func sameFolder(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
